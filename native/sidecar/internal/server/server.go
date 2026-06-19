package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"litert-sidecar/internal/catalog"
	"litert-sidecar/internal/platform"
	"litert-sidecar/internal/proxy"
)

const (
	maxMultimodalRequestBytes    = 40 << 20
	maxMultimodalAttachmentBytes = 32 << 20
)

type BackendStatus struct {
	Backend string `json:"backend"`
	State   string `json:"state"`
	Detail  string `json:"detail,omitempty"`
}

type StatusResponse struct {
	State        string          `json:"state"`
	Backends     []BackendStatus `json:"backends"`
	Detail       string          `json:"detail,omitempty"`
	Runtime      *RuntimeStatus  `json:"runtime,omitempty"`
	Capabilities Capabilities    `json:"capabilities"`
}

type RuntimeStatus struct {
	State       string `json:"state"`
	Executable  string `json:"executable,omitempty"`
	Version     string `json:"version,omitempty"`
	ModelID     string `json:"modelId,omitempty"`
	ModelFile   string `json:"modelFile,omitempty"`
	Upstream    string `json:"upstream,omitempty"`
	Mode        string `json:"mode,omitempty"`
	LogSequence uint64 `json:"logSequence,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

type RuntimeMode string

const (
	RuntimeModeRelease RuntimeMode = "release"
	RuntimeModeDebug   RuntimeMode = "debug"
)

type RuntimeController interface {
	Start(context.Context, RuntimeMode, RuntimeControlConfig) error
	Stop(context.Context) error
	Restart(context.Context, RuntimeMode, RuntimeControlConfig) error
	Status() RuntimeStatus
}

type RuntimeControlConfig struct {
	Upstream         string  `json:"upstream,omitempty"`
	RuntimeExe       string  `json:"runtimeExe,omitempty"`
	RuntimeHost      string  `json:"runtimeHost,omitempty"`
	RuntimePort      int     `json:"runtimePort,omitempty"`
	ModelFile        string  `json:"modelFile,omitempty"`
	ModelID          string  `json:"modelId,omitempty"`
	HuggingFaceToken *string `json:"huggingfaceToken,omitempty"`
	ImportModel      *bool   `json:"importModel,omitempty"`
	LaunchRuntime    *bool   `json:"launchRuntime,omitempty"`
	RuntimeVerbose   *bool   `json:"runtimeVerbose,omitempty"`
}

type Capabilities struct {
	Multimodal CapabilityStatus `json:"multimodal"`
}

type CapabilityStatus struct {
	State         string   `json:"state"`
	Endpoint      string   `json:"endpoint,omitempty"`
	Detail        string   `json:"detail,omitempty"`
	ImageBackends []string `json:"imageBackends,omitempty"`
	AudioBackends []string `json:"audioBackends,omitempty"`
}

type MultimodalAttachmentRequest struct {
	Name       string `json:"name"`
	MimeType   string `json:"mimeType,omitempty"`
	DataBase64 string `json:"dataBase64"`
}

type MultimodalGenerateRequest struct {
	Prompt                          string                        `json:"prompt"`
	ModelID                         string                        `json:"modelId,omitempty"`
	Backend                         string                        `json:"backend,omitempty"`
	VisionBackend                   string                        `json:"visionBackend,omitempty"`
	AudioBackend                    string                        `json:"audioBackend,omitempty"`
	MaxNumTokens                    int                           `json:"maxNumTokens,omitempty"`
	TopK                            int                           `json:"topK,omitempty"`
	TopP                            *float64                      `json:"topP,omitempty"`
	Temperature                     *float64                      `json:"temperature,omitempty"`
	Seed                            int64                         `json:"seed,omitempty"`
	Preset                          string                        `json:"preset,omitempty"`
	NoTemplate                      bool                          `json:"noTemplate,omitempty"`
	FilterChannelContentFromKVCache bool                          `json:"filterChannelContentFromKvCache,omitempty"`
	EnableSpeculativeDecoding       string                        `json:"enableSpeculativeDecoding,omitempty"`
	Cache                           string                        `json:"cache,omitempty"`
	Verbose                         bool                          `json:"verbose,omitempty"`
	FromHuggingFaceRepo             string                        `json:"fromHuggingFaceRepo,omitempty"`
	HuggingFaceToken                string                        `json:"huggingfaceToken,omitempty"`
	Attachments                     []MultimodalAttachmentRequest `json:"attachments"`
}

type MultimodalRunRequest struct {
	Prompt                          string
	ModelID                         string
	Backend                         string
	VisionBackend                   string
	AudioBackend                    string
	MaxNumTokens                    int
	TopK                            int
	TopP                            *float64
	Temperature                     *float64
	Seed                            int64
	Preset                          string
	NoTemplate                      bool
	FilterChannelContentFromKVCache bool
	EnableSpeculativeDecoding       string
	Cache                           string
	Verbose                         bool
	FromHuggingFaceRepo             string
	HuggingFaceToken                string
	AttachmentPaths                 []string
}

type MultimodalRunResponse struct {
	Text   string `json:"text"`
	Detail string `json:"detail,omitempty"`
}

type MultimodalRunner func(
	context.Context,
	MultimodalRunRequest,
) (MultimodalRunResponse, error)

type multimodalHTTPError struct {
	status  int
	message string
}

func (e multimodalHTTPError) Error() string {
	return e.message
}

type BackendReporter func(context.Context) ([]BackendStatus, error)

type Options struct {
	Proxy             *proxy.Proxy
	AllowedOrigins    []string
	RuntimeReporter   func() RuntimeStatus
	RuntimeController RuntimeController
	Logs              *LogBroadcaster
	StatusEvents      *StatusBroadcaster
	BackendReporter   BackendReporter
	MultimodalRunner  MultimodalRunner
	ModelCatalog      *catalog.Catalog
}

type Server struct {
	proxy             *proxy.Proxy
	allowedOrigins    map[string]struct{}
	runtimeReporter   func() RuntimeStatus
	runtimeController RuntimeController
	logs              *LogBroadcaster
	statusEvents      *StatusBroadcaster
	backendReporter   BackendReporter
	multimodalRunner  MultimodalRunner
	modelCatalog      *catalog.Catalog
}

func New(options Options) *Server {
	allowedOrigins := options.AllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{
			"http://127.0.0.1:5173",
			"http://localhost:5173",
		}
	}

	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		originSet[origin] = struct{}{}
	}

	logs := options.Logs
	if logs == nil {
		logs = NewLogBroadcaster(256)
	}
	statusEvents := options.StatusEvents
	if statusEvents == nil {
		statusEvents = NewStatusBroadcaster()
	}

	return &Server{
		proxy:             options.Proxy,
		allowedOrigins:    originSet,
		runtimeReporter:   options.RuntimeReporter,
		runtimeController: options.RuntimeController,
		logs:              logs,
		statusEvents:      statusEvents,
		backendReporter:   options.BackendReporter,
		multimodalRunner:  options.MultimodalRunner,
		modelCatalog:      options.ModelCatalog,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sidecar/v1/status", s.handleStatus)
	mux.HandleFunc("/sidecar/v1/ws", s.handleWebSocket)
	mux.HandleFunc("/sidecar/v1/multimodal", s.handleMultimodal)
	mux.HandleFunc("/sidecar/v1/models/download", s.handleModelDownload)
	mux.HandleFunc("/sidecar/v1/models", s.handleModels)
	mux.HandleFunc("/v1/", s.handleProxy)
	mux.HandleFunc("/v1", s.handleProxy)

	return s.withCORS(mux)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.modelCatalog == nil {
		http.Error(w, "model catalog is not configured", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("content-type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Models []catalog.Entry `json:"models"`
	}{
		Models: s.modelCatalog.Entries(),
	}); err != nil {
		http.Error(w, "encode model catalog", http.StatusInternalServerError)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	response := s.statusResponse(r.Context())
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode status response", http.StatusInternalServerError)
	}
}

func (s *Server) statusResponse(ctx context.Context) StatusResponse {
	state := "available"
	detail := "LiteRT sidecar ready. Backend probing is pending."
	var runtimeStatus *RuntimeStatus
	runtimeState := ""

	if nextStatus, ok := s.runtimeStatus(); ok {
		runtimeStatus = &nextStatus
		runtimeState = nextStatus.State
		if nextStatus.Detail != "" {
			detail = nextStatus.Detail
		}
		if isUnavailableRuntimeState(nextStatus.State) {
			state = "unavailable"
		}
	}

	if s.proxy != nil {
		if lastErr := s.proxy.LastError(); lastErr != "" {
			detail = strings.TrimSpace(detail + " last upstream error: " + lastErr)
		}
	}

	return StatusResponse{
		State:        state,
		Backends:     s.backendStatuses(ctx, runtimeState),
		Detail:       detail,
		Runtime:      runtimeStatus,
		Capabilities: s.capabilities(runtimeState),
	}
}

func (s *Server) runtimeStatus() (RuntimeStatus, bool) {
	if s.runtimeController != nil {
		status := s.runtimeController.Status()
		if s.logs != nil && status.LogSequence == 0 {
			status.LogSequence = s.logs.LatestSeq()
		}
		return status, true
	}
	if s.runtimeReporter != nil {
		status := s.runtimeReporter()
		if s.logs != nil && status.LogSequence == 0 {
			status.LogSequence = s.logs.LatestSeq()
		}
		return status, true
	}

	return RuntimeStatus{}, false
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	if s.proxy == nil {
		http.Error(w, "upstream proxy is not configured", http.StatusBadGateway)
		return
	}

	s.proxy.ServeHTTP(w, r)
}

type modelDownloadRequest struct {
	ID string `json:"id"`
}

func (s *Server) handleModelDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.modelCatalog == nil {
		http.Error(w, "model catalog is not configured", http.StatusServiceUnavailable)
		return
	}

	var request modelDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "decode model download request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(request.ID) == "" {
		http.Error(w, "model id is required", http.StatusBadRequest)
		return
	}

	entry, err := s.modelCatalog.Download(r.Context(), request.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("content-type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Model catalog.Entry `json:"model"`
	}{
		Model: entry,
	}); err != nil {
		http.Error(w, "encode model download response", http.StatusInternalServerError)
	}
}

func (s *Server) handleMultimodal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request MultimodalGenerateRequest
	body := http.MaxBytesReader(w, r.Body, maxMultimodalRequestBytes)
	if err := json.NewDecoder(body).Decode(&request); err != nil {
		http.Error(w, "decode multimodal request", http.StatusBadRequest)
		return
	}

	response, err := s.runMultimodalGenerate(r.Context(), request)
	if err != nil {
		status, message := multimodalErrorResponse(err)
		http.Error(w, message, status)
		return
	}

	w.Header().Set("content-type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode multimodal response", http.StatusInternalServerError)
	}
}

func (s *Server) runMultimodalGenerate(
	ctx context.Context,
	request MultimodalGenerateRequest,
) (MultimodalRunResponse, error) {
	if s.multimodalRunner == nil {
		return MultimodalRunResponse{}, multimodalHTTPError{
			status:  http.StatusServiceUnavailable,
			message: "multimodal runner is not configured",
		}
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return MultimodalRunResponse{}, multimodalHTTPError{
			status:  http.StatusBadRequest,
			message: "prompt is required",
		}
	}

	attachmentDir, err := os.MkdirTemp("", "litert-sidecar-attachments-*")
	if err != nil {
		return MultimodalRunResponse{}, multimodalHTTPError{
			status:  http.StatusInternalServerError,
			message: "create attachment workspace",
		}
	}
	defer os.RemoveAll(attachmentDir)

	attachmentPaths, err := writeMultimodalAttachments(
		attachmentDir,
		request.Attachments,
	)
	if err != nil {
		return MultimodalRunResponse{}, multimodalHTTPError{
			status:  http.StatusBadRequest,
			message: err.Error(),
		}
	}

	response, err := s.multimodalRunner(ctx, MultimodalRunRequest{
		Prompt:                          request.Prompt,
		ModelID:                         request.ModelID,
		Backend:                         request.Backend,
		VisionBackend:                   request.VisionBackend,
		AudioBackend:                    request.AudioBackend,
		MaxNumTokens:                    request.MaxNumTokens,
		TopK:                            request.TopK,
		TopP:                            request.TopP,
		Temperature:                     request.Temperature,
		Seed:                            request.Seed,
		Preset:                          request.Preset,
		NoTemplate:                      request.NoTemplate,
		FilterChannelContentFromKVCache: request.FilterChannelContentFromKVCache,
		EnableSpeculativeDecoding:       request.EnableSpeculativeDecoding,
		Cache:                           request.Cache,
		Verbose:                         request.Verbose,
		FromHuggingFaceRepo:             request.FromHuggingFaceRepo,
		HuggingFaceToken:                request.HuggingFaceToken,
		AttachmentPaths:                 attachmentPaths,
	})
	if err != nil {
		return MultimodalRunResponse{}, multimodalHTTPError{
			status:  http.StatusBadGateway,
			message: "run multimodal prompt",
		}
	}

	return response, nil
}

func multimodalErrorResponse(err error) (int, string) {
	var responseError multimodalHTTPError
	if errors.As(err, &responseError) {
		return responseError.status, responseError.message
	}

	return http.StatusInternalServerError, "run multimodal prompt"
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); s.isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "content-type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}

	_, ok := s.allowedOrigins[strings.TrimRight(origin, "/")]
	return ok
}

func (s *Server) capabilities(runtimeState string) Capabilities {
	state := "unavailable"
	detail := "Multimodal runner is not configured."
	if s.multimodalRunner != nil {
		state = "available"
		detail = "Use LiteRT-LM CLI attachments for native image and audio prompts."
	}
	if isUnavailableRuntimeState(runtimeState) {
		state = "unavailable"
		detail = "LiteRT-LM runtime is not available."
	}

	return Capabilities{
		Multimodal: CapabilityStatus{
			State:         state,
			Endpoint:      "/sidecar/v1/multimodal",
			Detail:        detail,
			ImageBackends: []string{"cpu", "gpu"},
			AudioBackends: []string{"cpu", "gpu"},
		},
	}
}

func (s *Server) backendStatuses(ctx context.Context, runtimeState string) []BackendStatus {
	if s.backendReporter != nil && !isUnavailableRuntimeState(runtimeState) {
		statuses, err := s.backendReporter(ctx)
		if err == nil && len(statuses) > 0 {
			return statuses
		}
	}

	evidence := platform.BackendEvidenceForRuntime(runtimeState)
	statuses := make([]BackendStatus, 0, len(evidence))

	for _, item := range evidence {
		statuses = append(statuses, BackendStatus{
			Backend: item.Backend,
			State:   item.State,
			Detail:  item.Detail,
		})
	}

	return statuses
}

func isUnavailableRuntimeState(state string) bool {
	return state == "unavailable" || state == "exited"
}

func writeMultimodalAttachments(
	dir string,
	attachments []MultimodalAttachmentRequest,
) ([]string, error) {
	paths := make([]string, 0, len(attachments))
	for index, attachment := range attachments {
		decoded, err := decodeAttachmentData(attachment.DataBase64)
		if err != nil {
			return nil, fmt.Errorf("decode attachment %d: %w", index+1, err)
		}
		if len(decoded) > maxMultimodalAttachmentBytes {
			return nil, fmt.Errorf("attachment %d is too large", index+1)
		}

		path := filepath.Join(
			dir,
			fmt.Sprintf("%02d-%s", index+1, safeAttachmentName(attachment.Name)),
		)
		if err := os.WriteFile(path, decoded, 0o600); err != nil {
			return nil, fmt.Errorf("write attachment %d: %w", index+1, err)
		}

		paths = append(paths, path)
	}

	return paths, nil
}

func decodeAttachmentData(value string) ([]byte, error) {
	payload := value
	if comma := strings.Index(payload, ","); comma >= 0 {
		prefix := payload[:comma]
		if strings.HasPrefix(prefix, "data:") {
			payload = payload[comma+1:]
		}
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, err
	}

	return decoded, nil
}

func safeAttachmentName(name string) string {
	normalized := strings.ReplaceAll(name, "\\", "/")
	base := filepath.Base(normalized)
	if base == "." || base == "/" || base == "" {
		return "attachment.bin"
	}

	return base
}
