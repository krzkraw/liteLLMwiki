package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"litert-sidecar/internal/proxy"
)

func TestStatusReturnsBackendEvidence(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, "http://127.0.0.1:9381")
	req := httptest.NewRequest(http.MethodGet, "/sidecar/v1/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}

	if body.State != "available" {
		t.Fatalf("body.state = %q", body.State)
	}
	assertBackendState(t, body.Backends, "cpu", "unknown")
	assertBackendState(t, body.Backends, "gpu", "unknown")
	assertBackendState(t, body.Backends, "npu", "unknown")
	assertBackendState(t, body.Backends, "cuda", "not-a-litert-backend")
}

func TestStatusReportsUnavailableRuntime(t *testing.T) {
	t.Parallel()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	handler := New(Options{
		Proxy: upstreamProxy,
		RuntimeReporter: func() RuntimeStatus {
			return RuntimeStatus{
				State:  "unavailable",
				Detail: "litert-lm executable was not found",
			}
		},
	}).Handler()

	req := httptest.NewRequest(http.MethodGet, "/sidecar/v1/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.State != "unavailable" {
		t.Fatalf("body.state = %q, want unavailable", body.State)
	}
	if body.Runtime == nil || body.Runtime.State != "unavailable" {
		t.Fatalf("runtime = %#v", body.Runtime)
	}
}

func TestStatusReportsExternalRuntimeAsAvailable(t *testing.T) {
	t.Parallel()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	handler := New(Options{
		Proxy: upstreamProxy,
		RuntimeReporter: func() RuntimeStatus {
			return RuntimeStatus{
				State:  "external",
				Detail: "external runtime",
			}
		},
	}).Handler()

	req := httptest.NewRequest(http.MethodGet, "/sidecar/v1/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.State != "available" {
		t.Fatalf("body.state = %q, want available", body.State)
	}
	if body.Runtime == nil || body.Runtime.State != "external" {
		t.Fatalf("runtime = %#v", body.Runtime)
	}
	assertBackendState(t, body.Backends, "cpu", "available")
	assertBackendState(t, body.Backends, "gpu", "available")
	assertBackendState(t, body.Backends, "npu", "available")
}

func TestStatusUsesBackendReporterWhenAvailable(t *testing.T) {
	t.Parallel()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	handler := New(Options{
		Proxy: upstreamProxy,
		RuntimeReporter: func() RuntimeStatus {
			return RuntimeStatus{State: "running"}
		},
		BackendReporter: func(context.Context) ([]BackendStatus, error) {
			return []BackendStatus{
				{Backend: "cpu", State: "available"},
				{Backend: "gpu", State: "available"},
				{Backend: "npu", State: "unavailable"},
				{Backend: "cuda", State: "not-a-litert-backend"},
			}, nil
		},
	}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/sidecar/v1/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	assertBackendState(t, body.Backends, "cpu", "available")
	assertBackendState(t, body.Backends, "gpu", "available")
	assertBackendState(t, body.Backends, "npu", "unavailable")
	assertBackendState(t, body.Backends, "cuda", "not-a-litert-backend")
}

func TestStatusFallsBackWhenBackendReporterFails(t *testing.T) {
	t.Parallel()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	handler := New(Options{
		Proxy: upstreamProxy,
		RuntimeReporter: func() RuntimeStatus {
			return RuntimeStatus{State: "running"}
		},
		BackendReporter: func(context.Context) ([]BackendStatus, error) {
			return nil, errors.New("models endpoint unavailable")
		},
	}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/sidecar/v1/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	assertBackendState(t, body.Backends, "cpu", "available")
	assertBackendState(t, body.Backends, "gpu", "available")
	assertBackendState(t, body.Backends, "npu", "available")
	assertBackendState(t, body.Backends, "cuda", "not-a-litert-backend")
}

func TestStatusReportsMultimodalCapabilityWhenRunnerConfigured(t *testing.T) {
	t.Parallel()

	handler := newMultimodalTestHandler(t, func(
		context.Context,
		MultimodalRunRequest,
	) (MultimodalRunResponse, error) {
		return MultimodalRunResponse{}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/sidecar/v1/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.Capabilities.Multimodal.State != "available" {
		t.Fatalf(
			"multimodal state = %q, want available",
			body.Capabilities.Multimodal.State,
		)
	}
	if body.Capabilities.Multimodal.Endpoint != "/sidecar/v1/multimodal" {
		t.Fatalf("multimodal endpoint = %q", body.Capabilities.Multimodal.Endpoint)
	}
}

func TestStatusReportsMultimodalUnavailableWhenRuntimeUnavailable(t *testing.T) {
	t.Parallel()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	handler := New(Options{
		Proxy: upstreamProxy,
		RuntimeReporter: func() RuntimeStatus {
			return RuntimeStatus{State: "unavailable"}
		},
		MultimodalRunner: func(
			context.Context,
			MultimodalRunRequest,
		) (MultimodalRunResponse, error) {
			return MultimodalRunResponse{}, nil
		},
	}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/sidecar/v1/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.Capabilities.Multimodal.State != "unavailable" {
		t.Fatalf(
			"multimodal state = %q, want unavailable",
			body.Capabilities.Multimodal.State,
		)
	}
}

func TestCorsHeadersAllowLocalWebUI(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, "http://127.0.0.1:9381")
	req := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Fatalf("allow methods = %q", got)
	}
}

func TestUpstreamFailureReportsLastError(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, "http://127.0.0.1:1")
	proxyReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	proxyRec := httptest.NewRecorder()

	handler.ServeHTTP(proxyRec, proxyReq)

	if proxyRec.Code != http.StatusBadGateway {
		t.Fatalf("proxy status = %d, want %d", proxyRec.Code, http.StatusBadGateway)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/sidecar/v1/status", nil)
	statusRec := httptest.NewRecorder()

	handler.ServeHTTP(statusRec, statusReq)

	var body StatusResponse
	if err := json.NewDecoder(statusRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !strings.Contains(body.Detail, "last upstream error") {
		t.Fatalf("detail = %q", body.Detail)
	}
}

func TestWebSocketStatusGetReturnsStatusEnvelope(t *testing.T) {
	t.Parallel()

	controller := &fakeRuntimeController{
		status: RuntimeStatus{
			State:       "running",
			Mode:        "debug",
			ModelID:     "gemma4-e2b",
			LogSequence: 7,
		},
	}
	handler := New(Options{
		RuntimeController: controller,
		Logs:              NewLogBroadcaster(8),
	}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	client.WriteJSON(t, map[string]string{"type": "status.get"})
	var envelope struct {
		Type   string         `json:"type"`
		Status StatusResponse `json:"status"`
	}
	client.ReadJSON(t, &envelope)

	if envelope.Type != "status" {
		t.Fatalf("message type = %q, want status", envelope.Type)
	}
	if envelope.Status.Runtime == nil {
		t.Fatal("status.runtime is nil")
	}
	if envelope.Status.Runtime.Mode != "debug" {
		t.Fatalf("runtime mode = %q, want debug", envelope.Status.Runtime.Mode)
	}
	if envelope.Status.Runtime.LogSequence != 7 {
		t.Fatalf("log sequence = %d, want 7", envelope.Status.Runtime.LogSequence)
	}
}

func TestWebSocketRuntimeControlInvokesController(t *testing.T) {
	t.Parallel()

	controller := &fakeRuntimeController{
		status: RuntimeStatus{State: "stopped", Mode: "release"},
	}
	handler := New(Options{
		RuntimeController: controller,
		Logs:              NewLogBroadcaster(8),
	}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	client.WriteJSON(t, map[string]string{"type": "runtime.start", "mode": "debug"})
	readStatusMessage(t, client)
	client.WriteJSON(t, map[string]string{"type": "runtime.restart", "mode": "release"})
	readStatusMessage(t, client)
	client.WriteJSON(t, map[string]string{"type": "runtime.stop"})
	readStatusMessage(t, client)

	controller.mu.Lock()
	defer controller.mu.Unlock()
	got := strings.Join(controller.calls, ",")
	want := "start:debug,restart:release,stop"
	if got != want {
		t.Fatalf("controller calls = %q, want %q", got, want)
	}
}

func TestWebSocketRuntimeControlForwardsConfig(t *testing.T) {
	t.Parallel()

	controller := &fakeRuntimeController{
		status: RuntimeStatus{State: "stopped", Mode: "release"},
	}
	handler := New(Options{
		RuntimeController: controller,
		Logs:              NewLogBroadcaster(8),
	}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	client.WriteJSON(t, map[string]any{
		"type": "runtime.start",
		"mode": "debug",
		"config": map[string]any{
			"runtimeExe":       "/opt/litert-lm",
			"runtimeHost":      "127.0.0.1",
			"runtimePort":      9481,
			"modelFile":        "demo/models/gemma-4-E2B-it.litertlm",
			"modelId":          "gemma4-e2b",
			"huggingfaceToken": "hf_secret",
			"importModel":      false,
			"launchRuntime":    true,
			"runtimeVerbose":   true,
		},
	})
	readStatusMessage(t, client)

	controller.mu.Lock()
	defer controller.mu.Unlock()
	if len(controller.configs) != 1 {
		t.Fatalf("captured configs = %d, want 1", len(controller.configs))
	}
	got := controller.configs[0]
	if got.RuntimeExe != "/opt/litert-lm" {
		t.Fatalf("runtime exe = %q", got.RuntimeExe)
	}
	if got.RuntimeHost != "127.0.0.1" || got.RuntimePort != 9481 {
		t.Fatalf("runtime address = %s:%d", got.RuntimeHost, got.RuntimePort)
	}
	if got.ModelFile != "demo/models/gemma-4-E2B-it.litertlm" {
		t.Fatalf("model file = %q", got.ModelFile)
	}
	if got.ModelID != "gemma4-e2b" {
		t.Fatalf("model id = %q", got.ModelID)
	}
	if got.HuggingFaceToken == nil || *got.HuggingFaceToken != "hf_secret" {
		t.Fatalf("hugging face token = %#v", got.HuggingFaceToken)
	}
	if got.ImportModel == nil || *got.ImportModel {
		t.Fatalf("import model = %#v, want false", got.ImportModel)
	}
	if got.LaunchRuntime == nil || !*got.LaunchRuntime {
		t.Fatalf("launch runtime = %#v, want true", got.LaunchRuntime)
	}
	if got.RuntimeVerbose == nil || !*got.RuntimeVerbose {
		t.Fatalf("runtime verbose = %#v, want true", got.RuntimeVerbose)
	}
}

func TestWebSocketLogsSubscribeReplaysAndStreamsLogEntries(t *testing.T) {
	t.Parallel()

	logs := NewLogBroadcaster(8)
	first := logs.Publish("runtime", "stdout", "ready")
	handler := New(Options{
		RuntimeController: &fakeRuntimeController{
			status: RuntimeStatus{State: "running"},
		},
		Logs: logs,
	}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	client.WriteJSON(t, map[string]string{"type": "logs.subscribe"})
	replayed := readLogMessage(t, client)
	if replayed.Seq != first.Seq || replayed.Line != "ready" {
		t.Fatalf("replayed log = %#v, want seq %d line ready", replayed, first.Seq)
	}

	next := logs.Publish("runtime", "stderr", "warn")
	streamed := readLogMessage(t, client)
	if streamed.Seq != next.Seq || streamed.Stream != "stderr" || streamed.Line != "warn" {
		t.Fatalf("streamed log = %#v, want %#v", streamed, next)
	}
}

func TestWebSocketStreamsRuntimeStatusChanges(t *testing.T) {
	t.Parallel()

	controller := &fakeRuntimeController{
		status: RuntimeStatus{State: "running", Mode: "release"},
	}
	statusEvents := NewStatusBroadcaster()
	handler := New(Options{
		RuntimeController: controller,
		Logs:              NewLogBroadcaster(8),
		StatusEvents:      statusEvents,
	}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	controller.setStatus(RuntimeStatus{
		State:  "exited",
		Mode:   "release",
		Detail: "runtime exited",
	})
	statusEvents.Publish()

	status := readStatusMessage(t, client)
	if status.Runtime == nil {
		t.Fatal("status.runtime is nil")
	}
	if status.Runtime.State != "exited" {
		t.Fatalf("runtime state = %q, want exited", status.Runtime.State)
	}
	if status.Runtime.Detail != "runtime exited" {
		t.Fatalf("runtime detail = %q", status.Runtime.Detail)
	}
}

func TestWebSocketInvalidRuntimeModeReturnsError(t *testing.T) {
	t.Parallel()

	handler := New(Options{
		RuntimeController: &fakeRuntimeController{},
		Logs:              NewLogBroadcaster(8),
	}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	client.WriteJSON(t, map[string]string{"type": "runtime.start", "mode": "loud"})
	var envelope struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	client.ReadJSON(t, &envelope)

	if envelope.Type != "error" {
		t.Fatalf("message type = %q, want error", envelope.Type)
	}
	if !strings.Contains(envelope.Message, "mode") {
		t.Fatalf("error message = %q, want mode detail", envelope.Message)
	}
}

func TestWebSocketAPIRequestStreamsOpenAIProxyResponse(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("upstream path = %q, want /v1/chat/completions", r.URL.Path)
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("upstream method = %q, want POST", r.Method)
			http.Error(w, "bad method", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream request body: %v", err)
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		if string(body) != `{"stream":true}` {
			t.Errorf("upstream body = %q", body)
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		_, _ = io.WriteString(w, "data: one\n\n")
		_, _ = io.WriteString(w, "data: two\n\n")
	}))
	t.Cleanup(upstream.Close)
	upstreamProxy, err := proxy.New(upstream.URL + "/v1")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	handler := New(Options{Proxy: upstreamProxy}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	client.WriteJSON(t, map[string]any{
		"type":       "api.request",
		"id":         "request-1",
		"method":     "POST",
		"path":       "/v1/chat/completions",
		"headers":    map[string]string{"content-type": "application/json"},
		"bodyBase64": base64.StdEncoding.EncodeToString([]byte(`{"stream":true}`)),
	})

	var start struct {
		Type    string            `json:"type"`
		ID      string            `json:"id"`
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
	}
	client.ReadJSON(t, &start)
	if start.Type != "api.response.start" || start.ID != "request-1" {
		t.Fatalf("start envelope = %#v", start)
	}
	if start.Status != http.StatusOK {
		t.Fatalf("start status = %d, want %d", start.Status, http.StatusOK)
	}

	var got string
	for {
		var envelope struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			DataBase64 string `json:"dataBase64"`
		}
		client.ReadJSON(t, &envelope)
		if envelope.ID != "request-1" {
			t.Fatalf("api envelope id = %q, want request-1", envelope.ID)
		}
		if envelope.Type == "api.response.end" {
			break
		}
		if envelope.Type != "api.response.chunk" {
			t.Fatalf("api envelope type = %q, want chunk/end", envelope.Type)
		}
		chunk, err := base64.StdEncoding.DecodeString(envelope.DataBase64)
		if err != nil {
			t.Fatalf("decode chunk: %v", err)
		}
		got += string(chunk)
	}
	if got != "data: one\n\ndata: two\n\n" {
		t.Fatalf("streamed body = %q", got)
	}
}

func TestWebSocketAPIRequestRunsMultimodal(t *testing.T) {
	t.Parallel()

	var captured MultimodalRunRequest
	handler := New(Options{
		MultimodalRunner: func(
			_ context.Context,
			request MultimodalRunRequest,
		) (MultimodalRunResponse, error) {
			captured = request
			if len(request.AttachmentPaths) != 1 {
				t.Errorf("attachment paths = %d, want 1", len(request.AttachmentPaths))
			}
			return MultimodalRunResponse{Text: "image summary"}, nil
		},
	}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	requestBody := `{"prompt":"Describe","modelId":"gemma4-e2b","backend":"gpu","huggingfaceToken":"hf_secret","attachments":[{"name":"sample.txt","dataBase64":"aGVsbG8="}]}`
	client.WriteJSON(t, map[string]any{
		"type":       "api.request",
		"id":         "request-1",
		"method":     "POST",
		"path":       "/sidecar/v1/multimodal",
		"headers":    map[string]string{"content-type": "application/json"},
		"bodyBase64": base64.StdEncoding.EncodeToString([]byte(requestBody)),
	})

	var start struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Status int    `json:"status"`
	}
	client.ReadJSON(t, &start)
	if start.Type != "api.response.start" || start.Status != http.StatusOK {
		t.Fatalf("start envelope = %#v", start)
	}
	var chunk struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		DataBase64 string `json:"dataBase64"`
	}
	client.ReadJSON(t, &chunk)
	if chunk.Type != "api.response.chunk" || chunk.ID != "request-1" {
		t.Fatalf("chunk envelope = %#v", chunk)
	}
	body, err := base64.StdEncoding.DecodeString(chunk.DataBase64)
	if err != nil {
		t.Fatalf("decode chunk: %v", err)
	}
	var response MultimodalRunResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Text != "image summary" {
		t.Fatalf("response text = %q", response.Text)
	}
	var end struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	client.ReadJSON(t, &end)
	if end.Type != "api.response.end" || end.ID != "request-1" {
		t.Fatalf("end envelope = %#v", end)
	}
	if captured.Prompt != "Describe" || captured.ModelID != "gemma4-e2b" || captured.Backend != "gpu" {
		t.Fatalf("captured request = %#v", captured)
	}
	if captured.HuggingFaceToken != "hf_secret" {
		t.Fatalf("hugging face token = %q", captured.HuggingFaceToken)
	}
}

func TestWebSocketAPICancelCancelsActiveRequest(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	cancelled := make(chan struct{})
	handler := New(Options{
		MultimodalRunner: func(
			ctx context.Context,
			_ MultimodalRunRequest,
		) (MultimodalRunResponse, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return MultimodalRunResponse{}, ctx.Err()
		},
	}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	requestBody := `{"prompt":"Describe","attachments":[]}`
	client.WriteJSON(t, map[string]any{
		"type":       "api.request",
		"id":         "request-1",
		"method":     "POST",
		"path":       "/sidecar/v1/multimodal",
		"bodyBase64": base64.StdEncoding.EncodeToString([]byte(requestBody)),
	})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("multimodal runner did not start")
	}

	client.WriteJSON(t, map[string]string{"type": "api.cancel", "id": "request-1"})

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("multimodal runner context was not cancelled")
	}
	var envelope struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	client.ReadJSON(t, &envelope)
	if envelope.Type != "api.error" || envelope.ID != "request-1" {
		t.Fatalf("cancel error envelope = %#v", envelope)
	}
}

func newTestHandler(t *testing.T, upstream string) http.Handler {
	t.Helper()

	upstreamProxy, err := proxy.New(upstream)
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	return New(Options{Proxy: upstreamProxy}).Handler()
}

func assertBackendState(
	t *testing.T,
	backends []BackendStatus,
	backend string,
	state string,
) {
	t.Helper()

	for _, item := range backends {
		if item.Backend == backend {
			if item.State != state {
				t.Fatalf("%s state = %q, want %q", backend, item.State, state)
			}
			return
		}
	}

	t.Fatalf("backend %q not found in %#v", backend, backends)
}

type fakeRuntimeController struct {
	mu      sync.Mutex
	status  RuntimeStatus
	calls   []string
	configs []RuntimeControlConfig
	err     error
}

func (f *fakeRuntimeController) Start(ctx context.Context, mode RuntimeMode, config RuntimeControlConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "start:"+string(mode))
	f.configs = append(f.configs, config)
	f.status.State = "running"
	f.status.Mode = string(mode)
	return f.err
}

func (f *fakeRuntimeController) Stop(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "stop")
	f.status.State = "stopped"
	return f.err
}

func (f *fakeRuntimeController) Restart(ctx context.Context, mode RuntimeMode, config RuntimeControlConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "restart:"+string(mode))
	f.configs = append(f.configs, config)
	f.status.State = "running"
	f.status.Mode = string(mode)
	return f.err
}

func (f *fakeRuntimeController) Status() RuntimeStatus {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.status
}

func (f *fakeRuntimeController) setStatus(status RuntimeStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.status = status
}

type testWebSocket struct {
	conn net.Conn
	r    *bufio.Reader
}

func dialTestWebSocket(t *testing.T, rawURL string, path string) *testWebSocket {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	conn, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("random websocket key: %v", err)
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	request := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nOrigin: http://127.0.0.1:5173\r\n\r\n",
		path,
		parsed.Host,
		key,
	)
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d, want %d", response.StatusCode, http.StatusSwitchingProtocols)
	}
	if got := response.Header.Get("Sec-WebSocket-Accept"); got != testWebSocketAccept(key) {
		t.Fatalf("accept = %q, want valid RFC6455 accept", got)
	}

	return &testWebSocket{conn: conn, r: reader}
}

func (c *testWebSocket) Close() error {
	return c.conn.Close()
}

func (c *testWebSocket) WriteJSON(t *testing.T, value any) {
	t.Helper()

	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal websocket payload: %v", err)
	}
	if err := c.writeText(payload); err != nil {
		t.Fatalf("write websocket text: %v", err)
	}
}

func (c *testWebSocket) ReadJSON(t *testing.T, value any) {
	t.Helper()

	payload, err := c.readText(2 * time.Second)
	if err != nil {
		t.Fatalf("read websocket text: %v", err)
	}
	if err := json.Unmarshal(payload, value); err != nil {
		t.Fatalf("decode websocket json %q: %v", payload, err)
	}
}

func (c *testWebSocket) writeText(payload []byte) error {
	header := []byte{0x81}
	switch {
	case len(payload) < 126:
		header = append(header, 0x80|byte(len(payload)))
	case len(payload) <= 0xffff:
		header = append(header, 0x80|126, byte(len(payload)>>8), byte(len(payload)))
	default:
		header = append(header, 0x80|127)
		length := make([]byte, 8)
		binary.BigEndian.PutUint64(length, uint64(len(payload)))
		header = append(header, length...)
	}

	mask := [4]byte{0x11, 0x22, 0x33, 0x44}
	header = append(header, mask[:]...)
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%len(mask)]
	}
	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	_, err := c.conn.Write(masked)
	return err
}

func (c *testWebSocket) readText(timeout time.Duration) ([]byte, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	first, err := c.r.ReadByte()
	if err != nil {
		return nil, err
	}
	second, err := c.r.ReadByte()
	if err != nil {
		return nil, err
	}
	opcode := first & 0x0f
	if opcode != 1 {
		return nil, fmt.Errorf("opcode %d, want text", opcode)
	}
	length := uint64(second & 0x7f)
	switch length {
	case 126:
		lengthBytes := make([]byte, 2)
		if _, err := io.ReadFull(c.r, lengthBytes); err != nil {
			return nil, err
		}
		length = uint64(binary.BigEndian.Uint16(lengthBytes))
	case 127:
		lengthBytes := make([]byte, 8)
		if _, err := io.ReadFull(c.r, lengthBytes); err != nil {
			return nil, err
		}
		length = binary.BigEndian.Uint64(lengthBytes)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func testWebSocketAccept(key string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	sum := sha1.Sum([]byte(key + magic))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func readStatusMessage(t *testing.T, client *testWebSocket) StatusResponse {
	t.Helper()

	var envelope struct {
		Type   string         `json:"type"`
		Status StatusResponse `json:"status"`
	}
	client.ReadJSON(t, &envelope)
	if envelope.Type != "status" {
		t.Fatalf("message type = %q, want status", envelope.Type)
	}
	return envelope.Status
}

func readLogMessage(t *testing.T, client *testWebSocket) LogEntry {
	t.Helper()

	var envelope struct {
		Type  string   `json:"type"`
		Entry LogEntry `json:"entry"`
	}
	client.ReadJSON(t, &envelope)
	if envelope.Type != "log" {
		t.Fatalf("message type = %q, want log", envelope.Type)
	}
	return envelope.Entry
}
