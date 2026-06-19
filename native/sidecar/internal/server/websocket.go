package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	webSocketGUID          = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	maxWebSocketFrameBytes = 80 << 20
)

type wsClientMessage struct {
	Type       string                `json:"type"`
	ID         string                `json:"id,omitempty"`
	Mode       RuntimeMode           `json:"mode,omitempty"`
	Config     *RuntimeControlConfig `json:"config,omitempty"`
	Method     string                `json:"method,omitempty"`
	Path       string                `json:"path,omitempty"`
	Headers    map[string]string     `json:"headers,omitempty"`
	BodyBase64 string                `json:"bodyBase64,omitempty"`
}

type wsWriter struct {
	mu   sync.Mutex
	conn net.Conn
}

type wsAPIRequests struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !s.isAllowedOrigin(origin) {
		http.Error(w, "origin is not allowed", http.StatusForbidden)
		return
	}
	if !isWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}

	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" || r.Header.Get("Sec-WebSocket-Version") != "13" {
		http.Error(w, "invalid websocket handshake", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket hijacking is not supported", http.StatusInternalServerError)
		return
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	if _, err := fmt.Fprintf(
		rw,
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n",
		webSocketAccept(key),
	); err != nil {
		return
	}
	if err := rw.Flush(); err != nil {
		return
	}

	writer := &wsWriter{conn: conn}
	s.serveWebSocket(r.Context(), rw.Reader, writer)
}

func (s *Server) serveWebSocket(ctx context.Context, reader *bufio.Reader, writer *wsWriter) {
	var stopLogs func()
	stopStatus := s.startStatusSubscription(ctx, writer)
	apiRequests := newWSAPIRequests()
	defer func() {
		apiRequests.cancelAll()
		if stopLogs != nil {
			stopLogs()
		}
		if stopStatus != nil {
			stopStatus()
		}
	}()

	for {
		payload, err := readClientTextFrame(reader)
		if err != nil {
			return
		}

		var message wsClientMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			_ = writer.sendJSON(map[string]string{
				"type":    "error",
				"message": "decode websocket message",
			})
			continue
		}

		switch message.Type {
		case "status.get":
			_ = writer.sendStatus(s.statusResponse(ctx))
		case "runtime.start":
			if err := s.controlRuntime(ctx, message.Mode, message.Config, "start"); err != nil {
				_ = writer.sendError(err.Error())
				continue
			}
			_ = writer.sendStatus(s.statusResponse(ctx))
		case "runtime.stop":
			if s.runtimeController == nil {
				_ = writer.sendError("runtime controller is not configured")
				continue
			}
			if err := s.runtimeController.Stop(ctx); err != nil {
				_ = writer.sendError(err.Error())
				continue
			}
			_ = writer.sendStatus(s.statusResponse(ctx))
		case "runtime.restart":
			if err := s.controlRuntime(ctx, message.Mode, message.Config, "restart"); err != nil {
				_ = writer.sendError(err.Error())
				continue
			}
			_ = writer.sendStatus(s.statusResponse(ctx))
		case "logs.subscribe":
			if stopLogs != nil {
				stopLogs()
			}
			stopLogs = s.startLogSubscription(writer)
		case "api.request":
			s.startWebSocketAPIRequest(ctx, apiRequests, writer, message)
		case "api.cancel":
			apiRequests.cancel(message.ID)
		default:
			_ = writer.sendError("unknown message type")
		}
	}
}

func newWSAPIRequests() *wsAPIRequests {
	return &wsAPIRequests{cancels: make(map[string]context.CancelFunc)}
}

func (r *wsAPIRequests) add(id string, cancel context.CancelFunc) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.cancels[id]; exists {
		return false
	}
	r.cancels[id] = cancel
	return true
}

func (r *wsAPIRequests) finish(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.cancels, id)
}

func (r *wsAPIRequests) cancel(id string) {
	r.mu.Lock()
	cancel := r.cancels[id]
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (r *wsAPIRequests) cancelAll() {
	r.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(r.cancels))
	for _, cancel := range r.cancels {
		cancels = append(cancels, cancel)
	}
	r.cancels = make(map[string]context.CancelFunc)
	r.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Server) startWebSocketAPIRequest(
	ctx context.Context,
	apiRequests *wsAPIRequests,
	writer *wsWriter,
	message wsClientMessage,
) {
	id := strings.TrimSpace(message.ID)
	if id == "" {
		_ = writer.sendAPIError("", "api request id is required")
		return
	}

	requestCtx, cancel := context.WithCancel(ctx)
	if !apiRequests.add(id, cancel) {
		cancel()
		_ = writer.sendAPIError(id, "api request id is already active")
		return
	}

	go func() {
		defer apiRequests.finish(id)
		defer cancel()
		s.handleWebSocketAPIRequest(requestCtx, writer, message)
	}()
}

func (s *Server) handleWebSocketAPIRequest(
	ctx context.Context,
	writer *wsWriter,
	message wsClientMessage,
) {
	method := strings.ToUpper(strings.TrimSpace(message.Method))
	if method == "" {
		method = http.MethodGet
	}
	requestPath, err := parseWebSocketAPIPath(message.Path)
	if err != nil {
		_ = writer.sendAPIError(message.ID, err.Error())
		return
	}
	body, err := decodeWebSocketAPIBody(message.BodyBase64)
	if err != nil {
		_ = writer.sendAPIError(message.ID, err.Error())
		return
	}

	switch {
	case requestPath.Path == "/sidecar/v1/status":
		if method != http.MethodGet {
			_ = writer.sendAPITextResponse(
				message.ID,
				http.StatusMethodNotAllowed,
				"method not allowed\n",
			)
			return
		}
		_ = writer.sendAPIJSONResponse(
			message.ID,
			http.StatusOK,
			s.statusResponse(ctx),
		)
	case requestPath.Path == "/sidecar/v1/multimodal":
		if method != http.MethodPost {
			_ = writer.sendAPITextResponse(
				message.ID,
				http.StatusMethodNotAllowed,
				"method not allowed\n",
			)
			return
		}
		var request MultimodalGenerateRequest
		if err := json.Unmarshal(body, &request); err != nil {
			_ = writer.sendAPITextResponse(
				message.ID,
				http.StatusBadRequest,
				"decode multimodal request\n",
			)
			return
		}
		response, err := s.runMultimodalGenerate(ctx, request)
		if err != nil {
			if ctx.Err() != nil {
				_ = writer.sendAPIError(message.ID, ctx.Err().Error())
				return
			}
			status, responseMessage := multimodalErrorResponse(err)
			_ = writer.sendAPITextResponse(message.ID, status, responseMessage+"\n")
			return
		}
		_ = writer.sendAPIJSONResponse(message.ID, http.StatusOK, response)
	case requestPath.Path == "/v1" || strings.HasPrefix(requestPath.Path, "/v1/"):
		s.proxyWebSocketAPIRequest(ctx, writer, message, method, requestPath, body)
	default:
		_ = writer.sendAPITextResponse(message.ID, http.StatusNotFound, "not found\n")
	}
}

func parseWebSocketAPIPath(rawPath string) (*url.URL, error) {
	if strings.TrimSpace(rawPath) == "" {
		return nil, errors.New("api request path is required")
	}
	parsed, err := url.ParseRequestURI(rawPath)
	if err != nil {
		return nil, fmt.Errorf("parse api request path: %w", err)
	}
	if !strings.HasPrefix(parsed.Path, "/") {
		return nil, errors.New("api request path must be absolute")
	}
	return parsed, nil
}

func decodeWebSocketAPIBody(bodyBase64 string) ([]byte, error) {
	if bodyBase64 == "" {
		return nil, nil
	}
	body, err := base64.StdEncoding.DecodeString(bodyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode api request body: %w", err)
	}
	return body, nil
}

func (s *Server) proxyWebSocketAPIRequest(
	ctx context.Context,
	writer *wsWriter,
	message wsClientMessage,
	method string,
	requestPath *url.URL,
	body []byte,
) {
	if s.proxy == nil {
		_ = writer.sendAPITextResponse(
			message.ID,
			http.StatusBadGateway,
			"upstream proxy is not configured\n",
		)
		return
	}

	upstreamURL, err := websocketAPIUpstreamURL(
		s.proxy.TargetForPath(requestPath.Path),
		requestPath,
	)
	if err != nil {
		_ = writer.sendAPIError(message.ID, err.Error())
		return
	}
	request, err := http.NewRequestWithContext(
		ctx,
		method,
		upstreamURL,
		bytes.NewReader(body),
	)
	if err != nil {
		_ = writer.sendAPIError(message.ID, err.Error())
		return
	}
	for key, value := range message.Headers {
		request.Header.Set(key, value)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		_ = writer.sendAPIError(message.ID, err.Error())
		return
	}
	defer response.Body.Close()

	if err := writer.sendAPIResponseStart(
		message.ID,
		response.StatusCode,
		httpHeaderMap(response.Header),
	); err != nil {
		return
	}

	buffer := make([]byte, 32*1024)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if err := writer.sendAPIResponseChunk(message.ID, buffer[:n]); err != nil {
				return
			}
		}
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		_ = writer.sendAPIError(message.ID, readErr.Error())
		return
	}

	_ = writer.sendAPIResponseEnd(message.ID)
}

func websocketAPIUpstreamURL(target string, requestPath *url.URL) (string, error) {
	base, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	upstream := *requestPath
	upstream.Scheme = base.Scheme
	upstream.Host = base.Host
	upstream.User = base.User
	upstream.RawQuery = requestPath.RawQuery
	upstream.Fragment = ""
	return upstream.String(), nil
}

func httpHeaderMap(header http.Header) map[string]string {
	values := make(map[string]string, len(header))
	for key, items := range header {
		values[strings.ToLower(key)] = strings.Join(items, ", ")
	}
	return values
}

func (s *Server) startStatusSubscription(ctx context.Context, writer *wsWriter) func() {
	if s.statusEvents == nil {
		return nil
	}

	ch, unsubscribe := s.statusEvents.Subscribe()
	stop := make(chan struct{})
	var once sync.Once
	go func() {
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return
				}
				if err := writer.sendStatus(s.statusResponse(ctx)); err != nil {
					once.Do(func() {
						close(stop)
						unsubscribe()
					})
					return
				}
			case <-stop:
				return
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(stop)
			unsubscribe()
		})
	}
}

func (s *Server) controlRuntime(
	ctx context.Context,
	mode RuntimeMode,
	config *RuntimeControlConfig,
	action string,
) error {
	if s.runtimeController == nil {
		return errors.New("runtime controller is not configured")
	}
	if !isRuntimeMode(mode) {
		return fmt.Errorf("runtime mode must be %q or %q", RuntimeModeRelease, RuntimeModeDebug)
	}

	controlConfig := RuntimeControlConfig{}
	if config != nil {
		controlConfig = *config
	}

	switch action {
	case "start":
		return s.runtimeController.Start(ctx, mode, controlConfig)
	case "restart":
		return s.runtimeController.Restart(ctx, mode, controlConfig)
	default:
		return fmt.Errorf("unsupported runtime action %q", action)
	}
}

func (s *Server) startLogSubscription(writer *wsWriter) func() {
	snapshot, ch, unsubscribe := s.logs.Subscribe()
	for _, entry := range snapshot {
		if err := writer.sendLog(entry); err != nil {
			unsubscribe()
			return func() {}
		}
	}

	stop := make(chan struct{})
	var once sync.Once
	go func() {
		for {
			select {
			case entry, ok := <-ch:
				if !ok {
					return
				}
				if err := writer.sendLog(entry); err != nil {
					once.Do(func() {
						close(stop)
						unsubscribe()
					})
					return
				}
			case <-stop:
				return
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(stop)
			unsubscribe()
		})
	}
}

func isRuntimeMode(mode RuntimeMode) bool {
	return mode == RuntimeModeRelease || mode == RuntimeModeDebug
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		headerContainsToken(r.Header.Get("Connection"), "upgrade")
}

func headerContainsToken(header string, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func webSocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + webSocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func readClientTextFrame(reader *bufio.Reader) ([]byte, error) {
	first, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	second, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	fin := first&0x80 != 0
	opcode := first & 0x0f
	if opcode == 8 {
		return nil, io.EOF
	}
	if !fin || opcode != 1 {
		return nil, fmt.Errorf("unsupported websocket frame opcode %d", opcode)
	}
	if second&0x80 == 0 {
		return nil, errors.New("client websocket frames must be masked")
	}

	length := uint64(second & 0x7f)
	switch length {
	case 126:
		lengthBytes := make([]byte, 2)
		if _, err := io.ReadFull(reader, lengthBytes); err != nil {
			return nil, err
		}
		length = uint64(binary.BigEndian.Uint16(lengthBytes))
	case 127:
		lengthBytes := make([]byte, 8)
		if _, err := io.ReadFull(reader, lengthBytes); err != nil {
			return nil, err
		}
		length = binary.BigEndian.Uint64(lengthBytes)
	}
	if length > maxWebSocketFrameBytes {
		return nil, fmt.Errorf("websocket frame is too large: %d", length)
	}

	mask := make([]byte, 4)
	if _, err := io.ReadFull(reader, mask); err != nil {
		return nil, err
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	for i := range payload {
		payload[i] ^= mask[i%len(mask)]
	}

	return payload, nil
}

func (w *wsWriter) sendStatus(status StatusResponse) error {
	return w.sendJSON(struct {
		Type   string         `json:"type"`
		Status StatusResponse `json:"status"`
	}{
		Type:   "status",
		Status: status,
	})
}

func (w *wsWriter) sendLog(entry LogEntry) error {
	return w.sendJSON(struct {
		Type  string   `json:"type"`
		Entry LogEntry `json:"entry"`
	}{
		Type:  "log",
		Entry: entry,
	})
}

func (w *wsWriter) sendError(message string) error {
	return w.sendJSON(map[string]string{
		"type":    "error",
		"message": message,
	})
}

func (w *wsWriter) sendAPIError(id string, message string) error {
	return w.sendJSON(map[string]string{
		"type":    "api.error",
		"id":      id,
		"message": message,
	})
}

func (w *wsWriter) sendAPIJSONResponse(id string, status int, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.sendAPIResponse(
		id,
		status,
		map[string]string{"content-type": "application/json"},
		payload,
	)
}

func (w *wsWriter) sendAPITextResponse(id string, status int, body string) error {
	return w.sendAPIResponse(
		id,
		status,
		map[string]string{"content-type": "text/plain; charset=utf-8"},
		[]byte(body),
	)
}

func (w *wsWriter) sendAPIResponse(
	id string,
	status int,
	headers map[string]string,
	body []byte,
) error {
	if err := w.sendAPIResponseStart(id, status, headers); err != nil {
		return err
	}
	if len(body) > 0 {
		if err := w.sendAPIResponseChunk(id, body); err != nil {
			return err
		}
	}
	return w.sendAPIResponseEnd(id)
}

func (w *wsWriter) sendAPIResponseStart(
	id string,
	status int,
	headers map[string]string,
) error {
	return w.sendJSON(struct {
		Type    string            `json:"type"`
		ID      string            `json:"id"`
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers,omitempty"`
	}{
		Type:    "api.response.start",
		ID:      id,
		Status:  status,
		Headers: headers,
	})
}

func (w *wsWriter) sendAPIResponseChunk(id string, body []byte) error {
	return w.sendJSON(struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		DataBase64 string `json:"dataBase64"`
	}{
		Type:       "api.response.chunk",
		ID:         id,
		DataBase64: base64.StdEncoding.EncodeToString(body),
	})
}

func (w *wsWriter) sendAPIResponseEnd(id string) error {
	return w.sendJSON(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{
		Type: "api.response.end",
		ID:   id,
	})
}

func (w *wsWriter) sendJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.writeText(payload)
}

func (w *wsWriter) writeText(payload []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	frame := []byte{0x81}
	switch {
	case len(payload) < 126:
		frame = append(frame, byte(len(payload)))
	case len(payload) <= 0xffff:
		frame = append(frame, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		frame = append(frame, 127)
		length := make([]byte, 8)
		binary.BigEndian.PutUint64(length, uint64(len(payload)))
		frame = append(frame, length...)
	}
	frame = append(frame, payload...)
	_, err := w.conn.Write(frame)
	return err
}
