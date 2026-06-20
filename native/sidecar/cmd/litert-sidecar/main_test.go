package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"litert-sidecar/internal/proxy"
	"litert-sidecar/internal/server"
	"litert-sidecar/internal/supervisor"
)

func TestModelsEndpointHandlesBaseAndV1Upstreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		upstream string
		want     string
	}{
		{
			name:     "base upstream",
			upstream: "http://127.0.0.1:9381",
			want:     "http://127.0.0.1:9381/v1/models",
		},
		{
			name:     "v1 upstream",
			upstream: "http://127.0.0.1:9381/v1",
			want:     "http://127.0.0.1:9381/v1/models",
		},
		{
			name:     "v1 upstream with trailing slash",
			upstream: "http://127.0.0.1:9381/v1/",
			want:     "http://127.0.0.1:9381/v1/models",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := modelsEndpoint(tt.upstream)
			if err != nil {
				t.Fatalf("modelsEndpoint() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("modelsEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSidecarModeDefaultsToTUI(t *testing.T) {
	t.Parallel()

	if got := sidecarMode(false); got != sidecarModeTUI {
		t.Fatalf("default mode = %q, want TUI", got)
	}
	if got := sidecarMode(true); got != sidecarModeHeadless {
		t.Fatalf("headless mode = %q, want headless", got)
	}
}

func TestTerminalLogTeesAreDisabledInTUIMode(t *testing.T) {
	t.Parallel()

	stdout, stderr := terminalLogTees(sidecarModeTUI)
	if stdout != nil || stderr != nil {
		t.Fatalf("TUI log tees = %v/%v, want nil writers so child logs stay inside the TUI", stdout, stderr)
	}
}

func TestTerminalLogTeesRemainEnabledInHeadlessMode(t *testing.T) {
	t.Parallel()

	stdout, stderr := terminalLogTees(sidecarModeHeadless)
	if stdout != os.Stdout || stderr != os.Stderr {
		t.Fatalf("headless log tees = %v/%v, want stdout/stderr", stdout, stderr)
	}
}

func TestDefaultRunnerIsLazyInTUIMode(t *testing.T) {
	t.Parallel()

	if shouldStartDefaultRunner(sidecarModeTUI) {
		t.Fatalf("TUI mode should not start a default runner before the launch wizard")
	}
	if !shouldStartDefaultRunner(sidecarModeHeadless) {
		t.Fatalf("headless mode should preserve the legacy default runner for automation")
	}
}

func TestSupervisorRuntimeControllerMapsLegacyExternalConfig(t *testing.T) {
	t.Parallel()

	launch := false
	runtimeSupervisor := supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch: true,
			Host:   "127.0.0.1",
			Port:   9381,
		},
	})
	controller := supervisorRuntimeController{
		supervisor: runtimeSupervisor,
	}

	if err := controller.Start(context.Background(), server.RuntimeModeRelease, server.RuntimeControlConfig{
		LaunchRuntime: &launch,
		Upstream:      "http://127.0.0.1:9999",
		RuntimeHost:   "127.0.0.1",
		RuntimePort:   9481,
	}); err != nil {
		t.Fatalf("start runtime: %v", err)
	}

	status := controller.Status()
	if status.State != "external" {
		t.Fatalf("state = %q, want external", status.State)
	}
	if status.Upstream != "http://127.0.0.1:9999" {
		t.Fatalf("upstream = %q, want explicit upstream", status.Upstream)
	}
}

func TestSupervisorRunnerControllerCreatesAndControlsRunner(t *testing.T) {
	t.Parallel()

	runtimeSupervisor := supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch:   false,
			ModelID:  "gemma4-e2b",
			Upstream: "http://127.0.0.1:9381",
		},
	})
	controller := supervisorRunnerController{supervisor: runtimeSupervisor}

	runner, err := controller.CreateRunner(context.Background(), server.RunnerSpec{
		ID:       "embedding-llamacpp",
		Runtime:  "llamacpp",
		Role:     "embedding",
		Backend:  "cpu",
		ModelID:  "qwen3-embedding",
		Host:     "127.0.0.1",
		Port:     9492,
		Launch:   false,
		Upstream: "http://127.0.0.1:9492",
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}
	if runner.ID != "embedding-llamacpp" || runner.State != "external" {
		t.Fatalf("runner = %#v, want external embedding runner", runner)
	}

	if _, err := controller.StartRunner(context.Background(), runner.ID); err != nil {
		t.Fatalf("start runner: %v", err)
	}
	if got, ok := runtimeSupervisor.UpstreamForPath("/v1/embeddings"); !ok || got != "http://127.0.0.1:9492" {
		t.Fatalf("embedding upstream = %q/%v", got, ok)
	}
	if _, err := controller.RestartRunner(context.Background(), runner.ID); err != nil {
		t.Fatalf("restart runner: %v", err)
	}
	if _, err := controller.StopRunner(context.Background(), runner.ID); err != nil {
		t.Fatalf("stop runner: %v", err)
	}

	snapshot := controller.Snapshot()
	if snapshot.Routes["embedding"] != runner.ID {
		t.Fatalf("embedding route = %q", snapshot.Routes["embedding"])
	}

	closed, err := controller.CloseRunner(context.Background(), runner.ID)
	if err != nil {
		t.Fatalf("close runner: %v", err)
	}
	if closed.ID != runner.ID {
		t.Fatalf("closed runner id = %q, want %q", closed.ID, runner.ID)
	}
	snapshot = controller.Snapshot()
	if got, ok := snapshot.Routes["embedding"]; ok {
		t.Fatalf("embedding route = %q/%v, want removed after close", got, ok)
	}
	for _, snapshotRunner := range snapshot.Runners {
		if snapshotRunner.ID == runner.ID {
			t.Fatalf("runner %q should be removed after close", runner.ID)
		}
	}
}

func TestSupervisorRunnerControllerMapsCommandOverrides(t *testing.T) {
	t.Parallel()

	runtimeSupervisor := supervisor.New(supervisor.Config{
		DisableDefaultLiteRT: true,
	})
	controller := supervisorRunnerController{supervisor: runtimeSupervisor}

	runner, err := controller.CreateRunner(context.Background(), server.RunnerSpec{
		ID:       "main-llamacpp-command",
		Runtime:  "llamacpp",
		Role:     "main",
		Backend:  "cpu",
		ModelID:  "gemma4-gguf",
		Host:     "127.0.0.1",
		Port:     9491,
		Launch:   false,
		Upstream: "http://127.0.0.1:9491",
		Command:  []string{"custom-runner", "--host", "127.0.0.1", "--port", "9491"},
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}
	if got := strings.Join(runner.Command, "\x00"); got != "custom-runner\x00--host\x00127.0.0.1\x00--port\x009491" {
		t.Fatalf("created command = %#v", runner.Command)
	}

	commandLine := "edited-runner --host 127.0.0.1 --port 9491 --alias edited"
	updated, err := controller.UpdateRunner(
		context.Background(),
		runner.ID,
		server.RunnerPatch{
			CommandLine: &commandLine,
		},
	)
	if err != nil {
		t.Fatalf("update runner command: %v", err)
	}
	if got := strings.Join(updated.Command, " "); got != "edited-runner --host 127.0.0.1 --port 9491 --alias edited" {
		t.Fatalf("updated command = %q", got)
	}
}

func TestSupervisorRunnerControllerStartOutlivesRequestContext(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell helper uses POSIX signals")
	}

	exe := writeLongRunningLlamaHelper(t)
	modelPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	runtimeSupervisor := supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch: false,
		},
	})
	controller := supervisorRunnerController{supervisor: runtimeSupervisor}

	runner, err := controller.CreateRunner(context.Background(), server.RunnerSpec{
		ID:         "main-llamacpp-context",
		Runtime:    "llamacpp",
		Role:       "main",
		Backend:    "cpu",
		Executable: exe,
		ModelPath:  modelPath,
		ModelID:    "context-model",
		Host:       "127.0.0.1",
		Port:       9497,
		Launch:     true,
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}

	startCtx, cancelStart := context.WithCancel(context.Background())
	if _, err := controller.StartRunner(startCtx, runner.ID); err != nil {
		t.Fatalf("start runner: %v", err)
	}
	cancelStart()
	time.Sleep(200 * time.Millisecond)

	afterCancel, ok := runtimeSupervisor.Runner(runner.ID)
	if !ok {
		t.Fatalf("runner %q not found", runner.ID)
	}
	if afterCancel.State != supervisor.StateRunning {
		t.Fatalf("state after request cancel = %q, want running", afterCancel.State)
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStop()
	if _, err := controller.StopRunner(stopCtx, runner.ID); err != nil {
		t.Fatalf("stop runner: %v", err)
	}
}

func TestProxyTargetResolverUsesSupervisorRoutes(t *testing.T) {
	t.Parallel()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	runtimeSupervisor := supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch: false,
			Host:   "127.0.0.1",
			Port:   9481,
		},
	})
	upstreamProxy.SetTargetResolver(proxyTargetResolver(runtimeSupervisor))

	if got := upstreamProxy.TargetForPath("/v1/chat/completions"); got != "http://127.0.0.1:9481" {
		t.Fatalf("proxy target = %q, want initial runtime port", got)
	}
}

func writeLongRunningLlamaHelper(t *testing.T) string {
	t.Helper()
	if os.Getenv("MAIN_LLAMA_HELPER") == "1" {
		t.Fatal("helper should not run in parent test process")
	}

	path := filepath.Join(t.TempDir(), "llama-server")
	script := "#!/bin/sh\n" +
		"MAIN_LLAMA_HELPER=1 exec " +
		shellQuote(os.Args[0]) + " -test.run=TestLongRunningLlamaHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	return path
}

func TestLongRunningLlamaHelperProcess(t *testing.T) {
	if os.Getenv("MAIN_LLAMA_HELPER") != "1" {
		return
	}

	args := helperArgs()
	if len(args) > 0 && args[0] == "--version" {
		fmt.Fprintln(os.Stdout, "llama helper version")
		return
	}

	host, port := helperHostPort(args)
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		fmt.Fprintln(os.Stderr, "--port is required")
		os.Exit(2)
	}

	server := &http.Server{
		Addr: net.JoinHostPort(host, port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/models" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"object":"list","data":[]}`)
		}),
	}
	listenErr := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err == http.ErrServerClosed {
			err = nil
		}
		listenErr <- err
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
	case err := <-listenErr:
		if err != nil {
			fmt.Fprintf(os.Stderr, "serve helper: %v\n", err)
			os.Exit(2)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func helperArgs() []string {
	for index, arg := range os.Args {
		if arg == "--" {
			return os.Args[index+1:]
		}
	}
	return nil
}

func helperHostPort(args []string) (string, string) {
	var host string
	var port string
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--host":
			if index+1 < len(args) {
				host = args[index+1]
				index++
			}
		case "--port":
			if index+1 < len(args) {
				port = args[index+1]
				index++
			}
		}
	}
	return host, port
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func TestToLiteRTConfigPatchForwardsHuggingFaceToken(t *testing.T) {
	t.Parallel()

	huggingFaceToken := "hf_secret"
	got := toSupervisorLiteRTPatch(server.RuntimeControlConfig{
		HuggingFaceToken: &huggingFaceToken,
	})
	if got.HuggingFaceToken == nil {
		t.Fatal("hugging face token patch is nil")
	}
	if *got.HuggingFaceToken != "hf_secret" {
		t.Fatalf("hugging face token = %q", *got.HuggingFaceToken)
	}
}

func TestBackendReporterUsesCurrentManagerStatus(t *testing.T) {
	t.Parallel()

	modelsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"runtime-model,gpu"}]}`))
	}))
	defer modelsServer.Close()

	status := supervisor.RuntimeStatus{
		State:    "running",
		ModelID:  "runtime-model",
		Upstream: modelsServer.URL,
	}
	backends, err := reportBackends(context.Background(), status.Upstream, status.ModelID)
	if err != nil {
		t.Fatalf("report backends: %v", err)
	}

	foundGPU := false
	for _, backend := range backends {
		if backend.Backend == "gpu" && backend.State == "available" {
			foundGPU = true
		}
	}
	if !foundGPU {
		t.Fatalf("backend evidence = %#v, want gpu available from current model id", backends)
	}
}
