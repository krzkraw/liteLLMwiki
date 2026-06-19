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

	"litert-sidecar/internal/catalog"
	"litert-sidecar/internal/proxy"
	"litert-sidecar/internal/supervisor"
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

func TestModelsEndpointListsCatalog(t *testing.T) {
	t.Parallel()

	modelCatalog := catalog.NewDefault(t.TempDir())
	handler := New(Options{
		ModelCatalog: modelCatalog,
	}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/sidecar/v1/models", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Models []catalog.Entry `json:"models"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	if len(body.Models) != 9 {
		t.Fatalf("models = %d, want 9", len(body.Models))
	}
	if body.Models[0].ID == "" || body.Models[0].TargetPath == "" {
		t.Fatalf("first model is incomplete: %#v", body.Models[0])
	}
}

func TestModelDownloadEndpointDownloadsCatalogEntry(t *testing.T) {
	t.Setenv("HF_TOKEN", "hf_secret")

	var sawAuth string
	modelHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "model-data")
	}))
	t.Cleanup(modelHost.Close)

	modelCatalog := catalog.NewDefault(t.TempDir(), catalog.WithBaseURL(modelHost.URL))
	handler := New(Options{
		ModelCatalog: modelCatalog,
	}).Handler()
	req := httptest.NewRequest(
		http.MethodPost,
		"/sidecar/v1/models/download",
		strings.NewReader(`{"id":"gemma4-gguf"}`),
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if sawAuth != "Bearer hf_secret" {
		t.Fatalf("authorization = %q, want bearer token", sawAuth)
	}
	var body struct {
		Model catalog.Entry `json:"model"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode model: %v", err)
	}
	if body.Model.ID != "gemma4-gguf" || body.Model.State != catalog.StatePresent {
		t.Fatalf("model = %#v, want downloaded gemma4-gguf", body.Model)
	}
}

func TestRunnerEndpointsCreateListAndControlRunner(t *testing.T) {
	t.Parallel()

	runtimeSupervisor := supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch:   false,
			ModelID:  "gemma4-e2b",
			Upstream: "http://127.0.0.1:9381",
		},
	})
	handler := New(Options{
		RunnerController: testRunnerController{supervisor: runtimeSupervisor},
	}).Handler()

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/sidecar/v1/runners",
		strings.NewReader(`{
			"id": "embedding-llamacpp",
			"runtime": "llamacpp",
			"role": "embedding",
			"backend": "cpu",
			"executable": "/opt/llama-server",
			"modelPath": "models/llamacpp/Qwen3-Embedding-0.6B-q8_0.gguf",
			"modelId": "qwen3-embedding",
			"host": "127.0.0.1",
			"port": 9492,
			"launch": false,
			"verbose": true,
			"upstream": "http://127.0.0.1:9492"
		}`),
	)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf(
			"create status = %d, want %d: %s",
			createRec.Code,
			http.StatusCreated,
			createRec.Body.String(),
		)
	}
	var createBody struct {
		Runner RunnerSnapshot `json:"runner"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createBody); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createBody.Runner.ID != "embedding-llamacpp" {
		t.Fatalf("runner id = %q", createBody.Runner.ID)
	}
	if createBody.Runner.State != "external" {
		t.Fatalf("runner state = %q, want external", createBody.Runner.State)
	}
	if createBody.Runner.Launch {
		t.Fatalf("runner launch = true, want false")
	}
	if !createBody.Runner.Verbose {
		t.Fatalf("runner verbose = false, want true")
	}

	patchReq := httptest.NewRequest(
		http.MethodPatch,
		"/sidecar/v1/runners/embedding-llamacpp",
		strings.NewReader(`{
			"backend": "gpu",
			"port": 9592,
			"verbose": false,
			"modelId": "qwen3-embedding-gpu"
		}`),
	)
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)

	if patchRec.Code != http.StatusOK {
		t.Fatalf(
			"patch status = %d, want %d: %s",
			patchRec.Code,
			http.StatusOK,
			patchRec.Body.String(),
		)
	}
	var patchBody struct {
		Runner RunnerSnapshot `json:"runner"`
	}
	if err := json.NewDecoder(patchRec.Body).Decode(&patchBody); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if patchBody.Runner.Backend != "gpu" {
		t.Fatalf("patched backend = %q, want gpu", patchBody.Runner.Backend)
	}
	if patchBody.Runner.Port != 9592 {
		t.Fatalf("patched port = %d, want 9592", patchBody.Runner.Port)
	}
	if patchBody.Runner.ModelID != "qwen3-embedding-gpu" {
		t.Fatalf("patched model id = %q", patchBody.Runner.ModelID)
	}
	if patchBody.Runner.Launch {
		t.Fatalf("patched launch = true, want false")
	}
	if patchBody.Runner.Verbose {
		t.Fatalf("patched verbose = true, want false")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/sidecar/v1/runners", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	var listBody RunnerSnapshotResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode runner list: %v", err)
	}
	if len(listBody.Runners) != 2 {
		t.Fatalf("runner count = %d, want 2", len(listBody.Runners))
	}
	if listBody.Routes["embedding"] != "embedding-llamacpp" {
		t.Fatalf("embedding route = %q", listBody.Routes["embedding"])
	}

	for _, action := range []string{"start", "restart", "stop"} {
		req := httptest.NewRequest(
			http.MethodPost,
			"/sidecar/v1/runners/embedding-llamacpp/"+action,
			nil,
		)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf(
				"%s status = %d, want %d: %s",
				action,
				rec.Code,
				http.StatusOK,
				rec.Body.String(),
			)
		}
	}
}

func TestWebSocketAPIRequestControlsRunner(t *testing.T) {
	t.Parallel()

	runtimeSupervisor := supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch:   false,
			ModelID:  "gemma4-e2b",
			Upstream: "http://127.0.0.1:9381",
		},
	})
	handler := New(Options{
		RunnerController: testRunnerController{supervisor: runtimeSupervisor},
	}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	createBody := `{
		"id": "rerank-llamacpp",
		"runtime": "llamacpp",
		"role": "reranking",
		"backend": "cpu",
		"modelPath": "models/llamacpp/Qwen3-Reranker-0.6B-Q4_K_M.gguf",
		"modelId": "qwen3-reranker-q4km",
		"host": "127.0.0.1",
		"port": 9493,
		"launch": false,
		"upstream": "http://127.0.0.1:9493"
	}`
	client.WriteJSON(t, map[string]any{
		"type":       "api.request",
		"id":         "create-runner",
		"method":     "POST",
		"path":       "/sidecar/v1/runners",
		"headers":    map[string]string{"content-type": "application/json"},
		"bodyBase64": base64.StdEncoding.EncodeToString([]byte(createBody)),
	})
	createResponse := readAPIResponse(t, client, "create-runner")
	if createResponse.status != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createResponse.status, http.StatusCreated)
	}

	client.WriteJSON(t, map[string]any{
		"type":   "api.request",
		"id":     "start-runner",
		"method": "POST",
		"path":   "/sidecar/v1/runners/rerank-llamacpp/start",
	})
	startResponse := readAPIResponse(t, client, "start-runner")
	if startResponse.status != http.StatusOK {
		t.Fatalf("start status = %d, want %d", startResponse.status, http.StatusOK)
	}
	if got, ok := runtimeSupervisor.UpstreamForPath("/v1/rerank"); !ok || got != "http://127.0.0.1:9493" {
		t.Fatalf("rerank upstream = %q/%v, want created runner", got, ok)
	}

	patchBody := `{"backend":"gpu","port":9593}`
	client.WriteJSON(t, map[string]any{
		"type":       "api.request",
		"id":         "patch-runner",
		"method":     "PATCH",
		"path":       "/sidecar/v1/runners/rerank-llamacpp",
		"headers":    map[string]string{"content-type": "application/json"},
		"bodyBase64": base64.StdEncoding.EncodeToString([]byte(patchBody)),
	})
	patchResponse := readAPIResponse(t, client, "patch-runner")
	if patchResponse.status != http.StatusOK {
		t.Fatalf("patch status = %d, want %d", patchResponse.status, http.StatusOK)
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
			"modelFile":        "models/litert/gemma-4-E2B-it.litertlm",
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
	if got.ModelFile != "models/litert/gemma-4-E2B-it.litertlm" {
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

func TestWebSocketAPIRequestUsesProxyTargetResolver(t *testing.T) {
	t.Parallel()

	embedding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("upstream path = %q, want /v1/embeddings", r.URL.Path)
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"embedding":[0.1]}]}`)
	}))
	t.Cleanup(embedding.Close)

	main := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "wrong upstream", http.StatusBadGateway)
	}))
	t.Cleanup(main.Close)

	upstreamProxy, err := proxy.New(main.URL)
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	upstreamProxy.SetTargetResolver(func(r *http.Request) (string, bool) {
		if r.URL.Path == "/v1/embeddings" {
			return embedding.URL, true
		}
		return "", false
	})
	handler := New(Options{Proxy: upstreamProxy}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	client.WriteJSON(t, map[string]any{
		"type":       "api.request",
		"id":         "request-1",
		"method":     "POST",
		"path":       "/v1/embeddings",
		"headers":    map[string]string{"content-type": "application/json"},
		"bodyBase64": base64.StdEncoding.EncodeToString([]byte(`{"input":"hello"}`)),
	})

	var start struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Status int    `json:"status"`
	}
	client.ReadJSON(t, &start)
	if start.Type != "api.response.start" || start.ID != "request-1" {
		t.Fatalf("start envelope = %#v", start)
	}
	if start.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", start.Status)
	}
}

func TestWebSocketAPIRequestListsModelCatalog(t *testing.T) {
	t.Parallel()

	modelCatalog := catalog.NewDefault(t.TempDir())
	handler := New(Options{ModelCatalog: modelCatalog}).Handler()
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTestWebSocket(t, httpServer.URL, "/sidecar/v1/ws")
	defer client.Close()

	client.WriteJSON(t, map[string]any{
		"type":   "api.request",
		"id":     "request-1",
		"method": "GET",
		"path":   "/sidecar/v1/models",
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
	body, err := base64.StdEncoding.DecodeString(chunk.DataBase64)
	if err != nil {
		t.Fatalf("decode chunk: %v", err)
	}
	var response struct {
		Models []catalog.Entry `json:"models"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if len(response.Models) != 9 {
		t.Fatalf("models = %d, want 9", len(response.Models))
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

type testRunnerController struct {
	supervisor *supervisor.Supervisor
}

func (c testRunnerController) Snapshot() RunnerSnapshotResponse {
	return testRunnerSnapshotResponse(c.supervisor.Snapshot())
}

func (c testRunnerController) CreateRunner(
	ctx context.Context,
	spec RunnerSpec,
) (RunnerSnapshot, error) {
	id, err := c.supervisor.CreateRunner(testSupervisorRunnerSpec(spec))
	if err != nil {
		return RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c testRunnerController) UpdateRunner(
	ctx context.Context,
	id string,
	patch RunnerPatch,
) (RunnerSnapshot, error) {
	if _, err := c.runner(id); err != nil {
		return RunnerSnapshot{}, err
	}
	if err := c.supervisor.UpdateRunner(id, testSupervisorRunnerPatch(patch)); err != nil {
		return RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c testRunnerController) StartRunner(
	ctx context.Context,
	id string,
) (RunnerSnapshot, error) {
	if _, err := c.runner(id); err != nil {
		return RunnerSnapshot{}, err
	}
	if err := c.supervisor.StartRunner(ctx, id); err != nil {
		return RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c testRunnerController) StopRunner(
	ctx context.Context,
	id string,
) (RunnerSnapshot, error) {
	if _, err := c.runner(id); err != nil {
		return RunnerSnapshot{}, err
	}
	if err := c.supervisor.StopRunner(ctx, id); err != nil {
		return RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c testRunnerController) RestartRunner(
	ctx context.Context,
	id string,
) (RunnerSnapshot, error) {
	if _, err := c.runner(id); err != nil {
		return RunnerSnapshot{}, err
	}
	if err := c.supervisor.RestartRunner(ctx, id); err != nil {
		return RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c testRunnerController) runner(id string) (RunnerSnapshot, error) {
	runner, ok := c.supervisor.Runner(id)
	if !ok {
		return RunnerSnapshot{}, fmt.Errorf("%w: %s", ErrRunnerNotFound, id)
	}
	return testRunnerSnapshot(runner), nil
}

func testSupervisorRunnerSpec(spec RunnerSpec) supervisor.RunnerSpec {
	return supervisor.RunnerSpec{
		ID:               spec.ID,
		Runtime:          supervisor.Runtime(spec.Runtime),
		Role:             supervisor.Role(spec.Role),
		Backend:          supervisor.Backend(spec.Backend),
		Executable:       spec.Executable,
		ModelPath:        spec.ModelPath,
		ModelID:          spec.ModelID,
		Host:             spec.Host,
		Port:             spec.Port,
		Launch:           spec.Launch,
		Upstream:         spec.Upstream,
		HuggingFaceToken: spec.HuggingFaceToken,
		Verbose:          spec.Verbose,
	}
}

func testSupervisorRunnerPatch(patch RunnerPatch) supervisor.RunnerPatch {
	return supervisor.RunnerPatch{
		Runtime:          supervisor.Runtime(patch.Runtime),
		Role:             supervisor.Role(patch.Role),
		Backend:          supervisor.Backend(patch.Backend),
		Executable:       patch.Executable,
		ModelPath:        patch.ModelPath,
		ModelID:          patch.ModelID,
		Host:             patch.Host,
		Port:             patch.Port,
		Launch:           patch.Launch,
		Upstream:         patch.Upstream,
		HuggingFaceToken: patch.HuggingFaceToken,
		Verbose:          patch.Verbose,
	}
}

func testRunnerSnapshotResponse(snapshot supervisor.Snapshot) RunnerSnapshotResponse {
	runners := make([]RunnerSnapshot, 0, len(snapshot.Runners))
	for _, runner := range snapshot.Runners {
		runners = append(runners, testRunnerSnapshot(runner))
	}
	routes := make(map[string]string, len(snapshot.Routes))
	for role, id := range snapshot.Routes {
		routes[string(role)] = id
	}
	return RunnerSnapshotResponse{
		Runners: runners,
		Routes:  routes,
	}
}

func testRunnerSnapshot(snapshot supervisor.RunnerSnapshot) RunnerSnapshot {
	return RunnerSnapshot{
		ID:           snapshot.ID,
		Runtime:      string(snapshot.Runtime),
		Role:         string(snapshot.Role),
		Backend:      string(snapshot.Backend),
		Executable:   snapshot.Executable,
		Version:      snapshot.Version,
		ModelPath:    snapshot.ModelPath,
		ModelID:      snapshot.ModelID,
		Host:         snapshot.Host,
		Port:         snapshot.Port,
		Launch:       snapshot.Launch,
		Verbose:      snapshot.Verbose,
		State:        string(snapshot.State),
		PID:          snapshot.PID,
		Upstream:     snapshot.Upstream,
		Command:      snapshot.Command,
		Capabilities: snapshot.Capabilities,
		LastError:    snapshot.LastError,
		LogSequence:  snapshot.LogSequence,
		Detail:       snapshot.Detail,
	}
}

type apiResponse struct {
	status int
	body   []byte
}

func readAPIResponse(t *testing.T, client *testWebSocket, id string) apiResponse {
	t.Helper()

	var start struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Status int    `json:"status"`
	}
	client.ReadJSON(t, &start)
	if start.Type != "api.response.start" || start.ID != id {
		t.Fatalf("api start envelope = %#v, want id %q", start, id)
	}

	var body []byte
	for {
		var envelope struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			DataBase64 string `json:"dataBase64"`
		}
		client.ReadJSON(t, &envelope)
		if envelope.ID != id {
			t.Fatalf("api envelope id = %q, want %q", envelope.ID, id)
		}
		switch envelope.Type {
		case "api.response.chunk":
			chunk, err := base64.StdEncoding.DecodeString(envelope.DataBase64)
			if err != nil {
				t.Fatalf("decode api chunk: %v", err)
			}
			body = append(body, chunk...)
		case "api.response.end":
			return apiResponse{status: start.Status, body: body}
		default:
			t.Fatalf("api envelope type = %q, want chunk/end", envelope.Type)
		}
	}
}
