package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"g0litellama/internal/catalog"
	"g0litellama/internal/platform"
	"g0litellama/internal/proxy"
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

var ErrRunnerNotFound = errors.New("runner not found")

type RunnerSpec struct {
	ID               string   `json:"id,omitempty"`
	Runtime          string   `json:"runtime,omitempty"`
	Role             string   `json:"role,omitempty"`
	Backend          string   `json:"backend,omitempty"`
	Executable       string   `json:"executable,omitempty"`
	ModelPath        string   `json:"modelPath,omitempty"`
	ModelID          string   `json:"modelId,omitempty"`
	Host             string   `json:"host,omitempty"`
	Port             int      `json:"port,omitempty"`
	Launch           bool     `json:"launch"`
	Upstream         string   `json:"upstream,omitempty"`
	Command          []string `json:"command,omitempty"`
	CommandLine      string   `json:"commandLine,omitempty"`
	HuggingFaceToken string   `json:"huggingfaceToken,omitempty"`
	Verbose          bool     `json:"verbose,omitempty"`
}

type RunnerPatch struct {
	Runtime          string   `json:"runtime,omitempty"`
	Role             string   `json:"role,omitempty"`
	Backend          string   `json:"backend,omitempty"`
	Executable       string   `json:"executable,omitempty"`
	ModelPath        string   `json:"modelPath,omitempty"`
	ModelID          string   `json:"modelId,omitempty"`
	Host             string   `json:"host,omitempty"`
	Port             int      `json:"port,omitempty"`
	Launch           *bool    `json:"launch,omitempty"`
	Upstream         string   `json:"upstream,omitempty"`
	Command          []string `json:"command,omitempty"`
	CommandLine      *string  `json:"commandLine,omitempty"`
	HuggingFaceToken *string  `json:"huggingfaceToken,omitempty"`
	Verbose          *bool    `json:"verbose,omitempty"`
}

type RunnerSnapshot struct {
	ID           string            `json:"id"`
	Runtime      string            `json:"runtime"`
	Role         string            `json:"role"`
	Backend      string            `json:"backend"`
	Executable   string            `json:"executable,omitempty"`
	Version      string            `json:"version,omitempty"`
	ModelPath    string            `json:"modelPath,omitempty"`
	ModelID      string            `json:"modelId,omitempty"`
	Host         string            `json:"host,omitempty"`
	Port         int               `json:"port,omitempty"`
	Launch       bool              `json:"launch"`
	Verbose      bool              `json:"verbose"`
	State        string            `json:"state"`
	PID          int               `json:"pid,omitempty"`
	Upstream     string            `json:"upstream,omitempty"`
	Command      []string          `json:"command,omitempty"`
	Capabilities map[string]string `json:"capabilities,omitempty"`
	LastError    string            `json:"lastError,omitempty"`
	LogSequence  uint64            `json:"logSequence,omitempty"`
	Detail       string            `json:"detail,omitempty"`
}

type RunnerSnapshotResponse struct {
	Runners []RunnerSnapshot  `json:"runners"`
	Routes  map[string]string `json:"routes"`
}

type RunnerController interface {
	Snapshot() RunnerSnapshotResponse
	CreateRunner(context.Context, RunnerSpec) (RunnerSnapshot, error)
	UpdateRunner(context.Context, string, RunnerPatch) (RunnerSnapshot, error)
	RouteRunner(context.Context, string, string) (RunnerSnapshot, error)
	StartRunner(context.Context, string) (RunnerSnapshot, error)
	StopRunner(context.Context, string) (RunnerSnapshot, error)
	RestartRunner(context.Context, string) (RunnerSnapshot, error)
	CloseRunner(context.Context, string) (RunnerSnapshot, error)
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
	RunnerController  RunnerController
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
	runnerController  RunnerController
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
		runnerController:  options.RunnerController,
		logs:              logs,
		statusEvents:      statusEvents,
		backendReporter:   options.BackendReporter,
		multimodalRunner:  options.MultimodalRunner,
		modelCatalog:      options.ModelCatalog,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/g0litellama/v1/status", s.handleStatus)
	mux.HandleFunc("/g0litellama/v1/ws", s.handleWebSocket)
	mux.HandleFunc("/g0litellama/v1/multimodal", s.handleMultimodal)
	mux.HandleFunc("/g0litellama/v1/models/download", s.handleModelDownload)
	mux.HandleFunc("/g0litellama/v1/models", s.handleModels)
	mux.HandleFunc("/g0litellama/v1/runners/", s.handleRunnerAction)
	mux.HandleFunc("/g0litellama/v1/runners", s.handleRunners)
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

func (s *Server) handleRunners(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read runner request", http.StatusBadRequest)
		return
	}

	writeRawAPIResponse(
		w,
		s.runnerAPIResponse(r.Context(), r.Method, r.URL.Path, body),
	)
}

func (s *Server) handleRunnerAction(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read runner request", http.StatusBadRequest)
		return
	}

	writeRawAPIResponse(
		w,
		s.runnerAPIResponse(r.Context(), r.Method, r.URL.Path, body),
	)
}

type rawAPIResponse struct {
	status      int
	contentType string
	body        []byte
}

type openAIModelsResponse struct {
	Object string              `json:"object"`
	Data   []openAIModelRecord `json:"data"`
}

type openAIModelRecord struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func (s *Server) runnerAPIResponse(
	ctx context.Context,
	method string,
	path string,
	body []byte,
) rawAPIResponse {
	if s.runnerController == nil {
		return textAPIResponse(
			http.StatusServiceUnavailable,
			"runner controller is not configured\n",
		)
	}

	switch {
	case path == "/g0litellama/v1/runners":
		return s.runnerCollectionAPIResponse(ctx, method, body)
	case strings.HasPrefix(path, "/g0litellama/v1/runners/"):
		return s.runnerResourceAPIResponse(ctx, method, path, body)
	default:
		return textAPIResponse(http.StatusNotFound, "not found\n")
	}
}

func (s *Server) runnerCollectionAPIResponse(
	ctx context.Context,
	method string,
	body []byte,
) rawAPIResponse {
	switch method {
	case http.MethodGet:
		return jsonAPIResponse(http.StatusOK, s.runnerController.Snapshot())
	case http.MethodPost:
		var spec RunnerSpec
		if err := json.Unmarshal(body, &spec); err != nil {
			return textAPIResponse(http.StatusBadRequest, "decode runner request\n")
		}
		runner, err := s.runnerController.CreateRunner(ctx, spec)
		if err != nil {
			return textAPIResponse(http.StatusBadRequest, err.Error()+"\n")
		}
		return jsonAPIResponse(
			http.StatusCreated,
			struct {
				Runner RunnerSnapshot `json:"runner"`
			}{
				Runner: runner,
			},
		)
	default:
		return textAPIResponse(http.StatusMethodNotAllowed, "method not allowed\n")
	}
}

func (s *Server) runnerResourceAPIResponse(
	ctx context.Context,
	method string,
	path string,
	body []byte,
) rawAPIResponse {
	if method == http.MethodPatch {
		id, ok := parseRunnerResourcePath(path)
		if !ok {
			return textAPIResponse(http.StatusNotFound, "not found\n")
		}
		var patch RunnerPatch
		if err := json.Unmarshal(body, &patch); err != nil {
			return textAPIResponse(http.StatusBadRequest, "decode runner patch\n")
		}
		runner, err := s.runnerController.UpdateRunner(ctx, id, patch)
		if err != nil {
			if errors.Is(err, ErrRunnerNotFound) {
				return textAPIResponse(http.StatusNotFound, err.Error()+"\n")
			}
			return textAPIResponse(http.StatusBadGateway, err.Error()+"\n")
		}
		return jsonAPIResponse(
			http.StatusOK,
			struct {
				Runner RunnerSnapshot `json:"runner"`
			}{
				Runner: runner,
			},
		)
	}

	if method != http.MethodPost {
		return textAPIResponse(http.StatusMethodNotAllowed, "method not allowed\n")
	}

	id, action, ok := parseRunnerActionPath(path)
	if !ok {
		return textAPIResponse(http.StatusNotFound, "not found\n")
	}

	var (
		runner RunnerSnapshot
		err    error
	)
	switch action {
	case "start":
		runner, err = s.runnerController.StartRunner(ctx, id)
	case "stop":
		runner, err = s.runnerController.StopRunner(ctx, id)
	case "restart":
		runner, err = s.runnerController.RestartRunner(ctx, id)
	case "close":
		runner, err = s.runnerController.CloseRunner(ctx, id)
	case "route":
		var request struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			return textAPIResponse(http.StatusBadRequest, "decode runner route request\n")
		}
		runner, err = s.runnerController.RouteRunner(ctx, request.Role, id)
	default:
		return textAPIResponse(http.StatusNotFound, "not found\n")
	}
	if err != nil {
		if errors.Is(err, ErrRunnerNotFound) {
			return textAPIResponse(http.StatusNotFound, err.Error()+"\n")
		}
		if action == "route" {
			return textAPIResponse(http.StatusBadRequest, err.Error()+"\n")
		}
		return textAPIResponse(http.StatusBadGateway, err.Error()+"\n")
	}

	return jsonAPIResponse(
		http.StatusOK,
		struct {
			Runner RunnerSnapshot `json:"runner"`
		}{
			Runner: runner,
		},
	)
}

func parseRunnerResourcePath(path string) (string, bool) {
	trimmed := strings.TrimPrefix(path, "/g0litellama/v1/runners/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", false
	}

	id, err := url.PathUnescape(trimmed)
	if err != nil || strings.TrimSpace(id) == "" {
		return "", false
	}
	return id, true
}

func parseRunnerActionPath(path string) (string, string, bool) {
	trimmed := strings.TrimPrefix(path, "/g0litellama/v1/runners/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	id, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(id) == "" {
		return "", "", false
	}
	return id, parts[1], true
}

func jsonAPIResponse(status int, value any) rawAPIResponse {
	body, err := json.Marshal(value)
	if err != nil {
		return textAPIResponse(http.StatusInternalServerError, "encode response\n")
	}
	return rawAPIResponse{
		status:      status,
		contentType: "application/json",
		body:        append(body, '\n'),
	}
}

func textAPIResponse(status int, body string) rawAPIResponse {
	return rawAPIResponse{
		status:      status,
		contentType: "text/plain; charset=utf-8",
		body:        []byte(body),
	}
}

func (s *Server) openAIModelsAPIResponse(method string) rawAPIResponse {
	if method != http.MethodGet {
		return textAPIResponse(http.StatusMethodNotAllowed, "method not allowed\n")
	}
	if s.runnerController == nil {
		return textAPIResponse(http.StatusBadGateway, "upstream proxy is not configured\n")
	}

	snapshot := s.runnerController.Snapshot()
	runnersByID := map[string]RunnerSnapshot{}
	for _, runner := range snapshot.Runners {
		runnersByID[runner.ID] = runner
	}
	seen := map[string]bool{}
	models := []openAIModelRecord{}
	for _, role := range []string{"main", "embedding", "reranking"} {
		runner := runnersByID[snapshot.Routes[role]]
		modelID := strings.TrimSpace(runner.ModelID)
		if modelID == "" {
			modelID = strings.TrimSpace(runner.ID)
		}
		if modelID == "" || seen[modelID] {
			continue
		}
		seen[modelID] = true
		models = append(models, openAIModelRecord{
			ID:      modelID,
			Object:  "model",
			OwnedBy: "g0litellama",
		})
	}
	return jsonAPIResponse(http.StatusOK, openAIModelsResponse{
		Object: "list",
		Data:   models,
	})
}

func writeRawAPIResponse(w http.ResponseWriter, response rawAPIResponse) {
	w.Header().Set("content-type", response.contentType)
	w.WriteHeader(response.status)
	if len(response.body) == 0 {
		return
	}
	_, _ = w.Write(response.body)
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
	detail := "G0LiteLLaMa ready. Backend probing is pending."
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
	if r.URL.Path == "/v1/models" && s.runnerController != nil {
		writeRawAPIResponse(w, s.openAIModelsAPIResponse(r.Method))
		return
	}
	if s.proxy == nil {
		http.Error(w, "upstream proxy is not configured", http.StatusBadGateway)
		return
	}
	if !s.proxy.HasTargetForPath(r.URL.Path) {
		http.Error(w, "no routed runner for "+r.URL.Path, http.StatusNotImplemented)
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

	attachmentDir, err := os.MkdirTemp("", "g0litellama-attachments-*")
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
			Endpoint:      "/g0litellama/v1/multimodal",
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
