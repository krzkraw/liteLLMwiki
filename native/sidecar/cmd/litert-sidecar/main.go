package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"litert-sidecar/internal/catalog"
	"litert-sidecar/internal/litert"
	"litert-sidecar/internal/platform"
	"litert-sidecar/internal/proxy"
	"litert-sidecar/internal/server"
	"litert-sidecar/internal/supervisor"
	"litert-sidecar/internal/tui"
)

type launchMode string

const (
	sidecarModeTUI      launchMode = "tui"
	sidecarModeHeadless launchMode = "headless"
)

func main() {
	addr := flag.String("addr", platform.DefaultListenAddr, "sidecar listen address")
	upstream := flag.String("upstream", platform.DefaultUpstreamURL, "litert-lm server URL")
	launchRuntime := flag.Bool("launch-runtime", true, "start and manage a litert-lm serve child process")
	runtimeExe := flag.String("runtime-exe", "", "path to litert-lm executable; defaults to PATH or bundled executable")
	runtimeHost := flag.String("runtime-host", litert.DefaultRuntimeHost, "litert-lm serve host")
	runtimePort := flag.Int("runtime-port", litert.DefaultRuntimePort, "litert-lm serve port")
	modelFile := flag.String("model-file", litert.FindDefaultModelFile(), "local .litertlm file to import before serving")
	modelID := flag.String("model-id", litert.DefaultModelID, "LiteRT-LM registry model ID used by the web UI")
	importModel := flag.Bool("import-model", true, "import -model-file into the LiteRT-LM registry when missing")
	runtimeVerbose := flag.Bool("runtime-verbose", false, "pass --verbose to litert-lm serve")
	headless := flag.Bool("headless", false, "run without the interactive terminal UI")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logs := server.NewLogBroadcaster(512)
	statusEvents := server.NewStatusBroadcaster()
	modelCatalog := catalog.NewDefault(catalog.FindModelRoot())
	mode := sidecarMode(*headless)
	stdoutTee, stderrTee := terminalLogTees(mode)
	startDefaultRunner := shouldStartDefaultRunner(mode)
	runtimeSupervisor := supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch:     *launchRuntime,
			Executable: *runtimeExe,
			Host:       *runtimeHost,
			Port:       *runtimePort,
			Upstream:   *upstream,
			ModelFile:  *modelFile,
			ModelID:    *modelID,
			Verbose:    *runtimeVerbose,
		},
		DisableDefaultLiteRT: !startDefaultRunner,
		Logs:                 logs,
		StdoutTee:            stdoutTee,
		StderrTee:            stderrTee,
		ImportModel:          *importModel,
		OnStatusChange: func(supervisor.Snapshot) {
			statusEvents.Publish()
		},
	})
	if startDefaultRunner {
		if err := runtimeSupervisor.StartRunner(ctx, supervisor.DefaultMainRunnerID); err != nil {
			log.Printf("litert-lm runtime is not ready: %v", err)
		}
	}

	initialUpstream := *upstream
	if routedUpstream, ok := runtimeSupervisor.UpstreamForPath("/v1/chat/completions"); ok {
		initialUpstream = routedUpstream
	} else if status := runtimeSupervisor.LegacyStatus(); status.Upstream != "" {
		initialUpstream = status.Upstream
	}
	upstreamProxy, err := proxy.New(initialUpstream)
	if err != nil {
		log.Fatalf("create upstream proxy: %v", err)
	}
	upstreamProxy.SetTargetResolver(proxyTargetResolver(runtimeSupervisor))

	runtimeController := supervisorRuntimeController{
		supervisor: runtimeSupervisor,
	}
	runnerController := supervisorRunnerController{
		supervisor: runtimeSupervisor,
	}
	handler := server.New(server.Options{
		Proxy:             upstreamProxy,
		RuntimeController: runtimeController,
		RunnerController:  runnerController,
		Logs:              logs,
		StatusEvents:      statusEvents,
		ModelCatalog:      modelCatalog,
		BackendReporter: func(ctx context.Context) ([]server.BackendStatus, error) {
			status := runtimeSupervisor.LegacyStatus()
			return reportBackends(ctx, status.Upstream, status.ModelID)
		},
		MultimodalRunner: func(
			ctx context.Context,
			request server.MultimodalRunRequest,
		) (server.MultimodalRunResponse, error) {
			config := runtimeSupervisor.DefaultLiteRTConfig()
			return runMultimodal(
				ctx,
				config.Executable,
				config.ModelFile,
				config.ModelID,
				config.HuggingFaceToken,
				config.ImportModel,
				request,
			)
		},
	}).Handler()
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("litert sidecar listening on http://%s", *addr)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- httpServer.ListenAndServe()
	}()

	if mode == sidecarModeTUI {
		tuiErr := make(chan error, 1)
		go func() {
			tuiErr <- tui.Run(ctx, runtimeController, runnerController, logs, modelCatalog)
		}()
		select {
		case <-ctx.Done():
		case err := <-serverErr:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("serve sidecar: %v", err)
			}
		case err := <-tuiErr:
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("run sidecar TUI: %v", err)
			}
		}
	} else {
		select {
		case <-ctx.Done():
		case err := <-serverErr:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("serve sidecar: %v", err)
			}
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown sidecar: %v", err)
	}
	if startDefaultRunner {
		if err := runtimeSupervisor.StopRunner(shutdownCtx, supervisor.DefaultMainRunnerID); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("stop litert-lm runtime: %v", err)
		}
	}
}

func sidecarMode(headless bool) launchMode {
	if headless {
		return sidecarModeHeadless
	}
	return sidecarModeTUI
}

func terminalLogTees(mode launchMode) (io.Writer, io.Writer) {
	if mode == sidecarModeHeadless {
		return os.Stdout, os.Stderr
	}
	return nil, nil
}

func shouldStartDefaultRunner(mode launchMode) bool {
	return mode == sidecarModeHeadless
}

type supervisorRuntimeController struct {
	supervisor *supervisor.Supervisor
}

func (c supervisorRuntimeController) Start(
	ctx context.Context,
	mode server.RuntimeMode,
	config server.RuntimeControlConfig,
) error {
	return c.supervisor.StartDefaultLiteRT(
		context.WithoutCancel(ctx),
		toSupervisorRuntimeMode(mode),
		toSupervisorLiteRTPatch(config),
	)
}

func (c supervisorRuntimeController) Stop(ctx context.Context) error {
	return c.supervisor.StopRunner(ctx, supervisor.DefaultMainRunnerID)
}

func (c supervisorRuntimeController) Restart(
	ctx context.Context,
	mode server.RuntimeMode,
	config server.RuntimeControlConfig,
) error {
	return c.supervisor.RestartDefaultLiteRT(
		context.WithoutCancel(ctx),
		toSupervisorRuntimeMode(mode),
		toSupervisorLiteRTPatch(config),
	)
}

func (c supervisorRuntimeController) Status() server.RuntimeStatus {
	return toServerRuntimeStatus(c.supervisor.LegacyStatus())
}

type supervisorRunnerController struct {
	supervisor *supervisor.Supervisor
}

func (c supervisorRunnerController) Snapshot() server.RunnerSnapshotResponse {
	return toServerRunnerSnapshotResponse(c.supervisor.Snapshot())
}

func (c supervisorRunnerController) CreateRunner(
	ctx context.Context,
	spec server.RunnerSpec,
) (server.RunnerSnapshot, error) {
	id, err := c.supervisor.CreateRunner(toSupervisorRunnerSpec(spec))
	if err != nil {
		return server.RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c supervisorRunnerController) UpdateRunner(
	ctx context.Context,
	id string,
	patch server.RunnerPatch,
) (server.RunnerSnapshot, error) {
	if _, err := c.runner(id); err != nil {
		return server.RunnerSnapshot{}, err
	}
	if err := c.supervisor.UpdateRunner(id, toSupervisorRunnerPatch(patch)); err != nil {
		return server.RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c supervisorRunnerController) StartRunner(
	ctx context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	if _, err := c.runner(id); err != nil {
		return server.RunnerSnapshot{}, err
	}
	if err := c.supervisor.StartRunner(context.WithoutCancel(ctx), id); err != nil {
		return server.RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c supervisorRunnerController) StopRunner(
	ctx context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	if _, err := c.runner(id); err != nil {
		return server.RunnerSnapshot{}, err
	}
	if err := c.supervisor.StopRunner(ctx, id); err != nil {
		return server.RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c supervisorRunnerController) RestartRunner(
	ctx context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	if _, err := c.runner(id); err != nil {
		return server.RunnerSnapshot{}, err
	}
	if err := c.supervisor.RestartRunner(context.WithoutCancel(ctx), id); err != nil {
		return server.RunnerSnapshot{}, err
	}
	return c.runner(id)
}

func (c supervisorRunnerController) CloseRunner(
	ctx context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	if _, err := c.runner(id); err != nil {
		return server.RunnerSnapshot{}, err
	}
	runner, err := c.supervisor.CloseRunner(ctx, id)
	if err != nil {
		return server.RunnerSnapshot{}, err
	}
	return toServerRunnerSnapshot(runner), nil
}

func (c supervisorRunnerController) runner(id string) (server.RunnerSnapshot, error) {
	runner, ok := c.supervisor.Runner(id)
	if !ok {
		return server.RunnerSnapshot{}, fmt.Errorf("%w: %s", server.ErrRunnerNotFound, id)
	}
	return toServerRunnerSnapshot(runner), nil
}

func proxyTargetResolver(runtimeSupervisor *supervisor.Supervisor) proxy.TargetResolver {
	return func(r *http.Request) (string, bool) {
		return runtimeSupervisor.UpstreamForPath(r.URL.Path)
	}
}

func toSupervisorRuntimeMode(mode server.RuntimeMode) supervisor.RuntimeMode {
	if mode == server.RuntimeModeDebug {
		return supervisor.RuntimeModeDebug
	}

	return supervisor.RuntimeModeRelease
}

func toSupervisorLiteRTPatch(config server.RuntimeControlConfig) supervisor.LiteRTPatch {
	return supervisor.LiteRTPatch{
		Launch:           config.LaunchRuntime,
		Executable:       config.RuntimeExe,
		Host:             config.RuntimeHost,
		Port:             config.RuntimePort,
		Upstream:         config.Upstream,
		ModelPath:        config.ModelFile,
		ModelID:          config.ModelID,
		HuggingFaceToken: config.HuggingFaceToken,
		ImportModel:      config.ImportModel,
		Verbose:          config.RuntimeVerbose,
	}
}

func toServerRuntimeStatus(status supervisor.RuntimeStatus) server.RuntimeStatus {
	return server.RuntimeStatus{
		State:       status.State,
		Executable:  status.Executable,
		Version:     status.Version,
		ModelID:     status.ModelID,
		ModelFile:   status.ModelFile,
		Upstream:    status.Upstream,
		Mode:        status.Mode,
		LogSequence: status.LogSequence,
		Detail:      status.Detail,
	}
}

func toSupervisorRunnerSpec(spec server.RunnerSpec) supervisor.RunnerSpec {
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

func toSupervisorRunnerPatch(patch server.RunnerPatch) supervisor.RunnerPatch {
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

func toServerRunnerSnapshotResponse(snapshot supervisor.Snapshot) server.RunnerSnapshotResponse {
	runners := make([]server.RunnerSnapshot, 0, len(snapshot.Runners))
	for _, runner := range snapshot.Runners {
		runners = append(runners, toServerRunnerSnapshot(runner))
	}
	routes := make(map[string]string, len(snapshot.Routes))
	for role, id := range snapshot.Routes {
		routes[string(role)] = id
	}
	return server.RunnerSnapshotResponse{
		Runners: runners,
		Routes:  routes,
	}
}

func toServerRunnerSnapshot(snapshot supervisor.RunnerSnapshot) server.RunnerSnapshot {
	return server.RunnerSnapshot{
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

func runMultimodal(
	ctx context.Context,
	runtimeExe string,
	modelFile string,
	modelID string,
	configuredHuggingFaceToken string,
	importModel bool,
	request server.MultimodalRunRequest,
) (server.MultimodalRunResponse, error) {
	exe, err := findRuntimeExecutable(runtimeExe)
	if err != nil {
		return server.MultimodalRunResponse{}, err
	}

	runModelID := modelID
	if request.ModelID != "" {
		runModelID = request.ModelID
	}
	huggingFaceToken := strings.TrimSpace(request.HuggingFaceToken)
	if huggingFaceToken == "" {
		huggingFaceToken = strings.TrimSpace(configuredHuggingFaceToken)
	}
	if importModel {
		if err := litert.EnsureModelImportedWithHuggingFaceToken(ctx, exe, modelFile, runModelID, huggingFaceToken); err != nil {
			return server.MultimodalRunResponse{}, err
		}
	}

	text, err := litert.RunOnce(ctx, exe, litert.RunRequest{
		ModelID:                         runModelID,
		Backend:                         request.Backend,
		VisionBackend:                   request.VisionBackend,
		AudioBackend:                    request.AudioBackend,
		Prompt:                          request.Prompt,
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
		HuggingFaceToken:                huggingFaceToken,
		AttachmentPaths:                 request.AttachmentPaths,
	})
	if err != nil {
		return server.MultimodalRunResponse{}, err
	}

	return server.MultimodalRunResponse{Text: text}, nil
}

func findRuntimeExecutable(runtimeExe string) (string, error) {
	if runtimeExe != "" {
		return litert.FindExecutable(runtimeExe)
	}

	return litert.FindExecutable()
}

type modelsResponse struct {
	Data []modelRecord `json:"data"`
}

type modelRecord struct {
	ID string `json:"id"`
}

func reportBackends(
	ctx context.Context,
	upstream string,
	modelID string,
) ([]server.BackendStatus, error) {
	modelIDs, err := fetchModelIDs(ctx, upstream)
	if err != nil {
		return nil, err
	}

	evidence := platform.BackendEvidenceFromModelIDs(modelID, modelIDs)
	statuses := make([]server.BackendStatus, 0, len(evidence))
	for _, item := range evidence {
		statuses = append(statuses, server.BackendStatus{
			Backend: item.Backend,
			State:   item.State,
			Detail:  item.Detail,
		})
	}

	return statuses, nil
}

func fetchModelIDs(ctx context.Context, upstream string) ([]string, error) {
	modelsURL, err := modelsEndpoint(upstream)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetch models: status %d", response.StatusCode)
	}

	var body modelsResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	modelIDs := make([]string, 0, len(body.Data))
	for _, item := range body.Data {
		if item.ID != "" {
			modelIDs = append(modelIDs, item.ID)
		}
	}

	return modelIDs, nil
}

func modelsEndpoint(upstream string) (string, error) {
	parsed, err := url.Parse(upstream)
	if err != nil {
		return "", fmt.Errorf("parse upstream url: %w", err)
	}

	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1"
	}
	if !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	parsed.Path = path + "/models"
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String(), nil
}
