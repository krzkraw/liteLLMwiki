package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"litert-sidecar/internal/server"
)

func TestSupervisorCreatesDefaultLiteRTRunner(t *testing.T) {
	t.Parallel()

	supervisor := New(Config{
		DefaultLiteRT: LiteRTConfig{
			Launch:    false,
			ModelID:   "gemma4-e2b",
			ModelFile: "models/litert/main/gemma-4-E2B-it.litertlm",
			Upstream:  "http://127.0.0.1:9999",
		},
	})

	snapshot := supervisor.Snapshot()
	if len(snapshot.Runners) != 1 {
		t.Fatalf("runner count = %d, want 1", len(snapshot.Runners))
	}

	runner := snapshot.Runners[0]
	if runner.ID != DefaultMainRunnerID {
		t.Fatalf("runner id = %q, want %q", runner.ID, DefaultMainRunnerID)
	}
	if runner.Runtime != RuntimeLiteRT || runner.Role != RoleMain {
		t.Fatalf("runner runtime/role = %q/%q, want litert/main", runner.Runtime, runner.Role)
	}
	if runner.State != StateExternal {
		t.Fatalf("runner state = %q, want external", runner.State)
	}
	if runner.Upstream != "http://127.0.0.1:9999" {
		t.Fatalf("upstream = %q, want configured external upstream", runner.Upstream)
	}

	status := supervisor.LegacyStatus()
	if status.State != string(StateExternal) {
		t.Fatalf("legacy state = %q, want external", status.State)
	}
	if status.ModelID != "gemma4-e2b" {
		t.Fatalf("legacy model id = %q", status.ModelID)
	}
	if status.Upstream != "http://127.0.0.1:9999" {
		t.Fatalf("legacy upstream = %q, want configured external upstream", status.Upstream)
	}
}

func TestSupervisorCanStartWithoutDefaultLiteRTRunner(t *testing.T) {
	t.Parallel()

	supervisor := New(Config{
		DisableDefaultLiteRT: true,
	})

	snapshot := supervisor.Snapshot()
	if len(snapshot.Runners) != 0 {
		t.Fatalf("runners = %#v, want no default runner", snapshot.Runners)
	}
	if _, ok := snapshot.Routes[RoleMain]; ok {
		t.Fatalf("main route = %q, want no default route", snapshot.Routes[RoleMain])
	}
}

func TestSupervisorRoutesByRunnerRole(t *testing.T) {
	t.Parallel()

	supervisor := New(Config{
		DefaultLiteRT: LiteRTConfig{
			Launch:   false,
			ModelID:  "gemma4-e2b",
			Upstream: "http://127.0.0.1:9381",
		},
	})
	embeddingID, err := supervisor.CreateRunner(RunnerSpec{
		Runtime:  RuntimeLlamaCPP,
		Role:     RoleEmbedding,
		Backend:  BackendCPU,
		ModelID:  "qwen3-embedding",
		Host:     "127.0.0.1",
		Port:     9391,
		Launch:   false,
		Upstream: "http://127.0.0.1:9391",
	})
	if err != nil {
		t.Fatalf("create embedding runner: %v", err)
	}

	if got, ok := supervisor.UpstreamForPath("/v1/chat/completions"); !ok || got != "http://127.0.0.1:9381" {
		t.Fatalf("chat upstream = %q/%v, want default main", got, ok)
	}
	if got, ok := supervisor.UpstreamForPath("/v1/embeddings"); !ok || got != "http://127.0.0.1:9391" {
		t.Fatalf("embedding upstream = %q/%v, want embedding runner", got, ok)
	}

	runner, ok := supervisor.Runner(embeddingID)
	if !ok {
		t.Fatalf("runner %q not found", embeddingID)
	}
	if runner.Role != RoleEmbedding || runner.State != StateExternal {
		t.Fatalf("runner = %#v, want external embedding runner", runner)
	}
}

func TestSupervisorUpdatesStoppedRunnerSettings(t *testing.T) {
	t.Parallel()

	supervisor := New(Config{
		DefaultLiteRT: LiteRTConfig{
			Launch:   false,
			ModelID:  "gemma4-e2b",
			Upstream: "http://127.0.0.1:9381",
		},
	})
	runnerID, err := supervisor.CreateRunner(RunnerSpec{
		ID:        "embedding-llamacpp",
		Runtime:   RuntimeLlamaCPP,
		Role:      RoleEmbedding,
		Backend:   BackendGPU,
		ModelPath: "models/llamacpp/embedding/Qwen3-Embedding-0.6B-q8_0.gguf",
		ModelID:   "qwen3-embedding",
		Host:      "127.0.0.1",
		Port:      9492,
		Launch:    false,
		Upstream:  "http://127.0.0.1:9492",
	})
	if err != nil {
		t.Fatalf("create embedding runner: %v", err)
	}

	launch := true
	verbose := true
	err = supervisor.UpdateRunner(runnerID, RunnerPatch{
		Backend:   BackendCPU,
		Port:      9592,
		Launch:    &launch,
		Verbose:   &verbose,
		ModelID:   "qwen3-embedding-cpu",
		ModelPath: "models/llamacpp/embedding-cpu.gguf",
	})
	if err != nil {
		t.Fatalf("update runner: %v", err)
	}

	runner, ok := supervisor.Runner(runnerID)
	if !ok {
		t.Fatalf("runner %q not found", runnerID)
	}
	if runner.Backend != BackendCPU {
		t.Fatalf("backend = %q, want cpu", runner.Backend)
	}
	if runner.Port != 9592 {
		t.Fatalf("port = %d, want 9592", runner.Port)
	}
	if runner.ModelID != "qwen3-embedding-cpu" {
		t.Fatalf("model id = %q", runner.ModelID)
	}
	if runner.ModelPath != "models/llamacpp/embedding-cpu.gguf" {
		t.Fatalf("model path = %q", runner.ModelPath)
	}
	if !runner.Launch {
		t.Fatalf("launch = false, want true")
	}
	if !runner.Verbose {
		t.Fatalf("verbose = false, want true")
	}
	if runner.State != StateCreated {
		t.Fatalf("state = %q, want created after enabling managed launch", runner.State)
	}
	if runner.Upstream != "http://127.0.0.1:9592" {
		t.Fatalf("upstream = %q, want patched managed upstream", runner.Upstream)
	}
}

func TestSupervisorSerializesConcurrentStarts(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeLiteRTHelper(t)
	var childOutput bytes.Buffer
	logs := server.NewLogBroadcaster(16)
	supervisor := New(Config{
		DefaultLiteRT: LiteRTConfig{
			Launch:     true,
			Executable: exe,
			Host:       "127.0.0.1",
			Port:       9481,
			ModelID:    "gemma4-e2b",
		},
		Logs:        logs,
		StdoutTee:   &childOutput,
		StderrTee:   &childOutput,
		ImportModel: false,
	})
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- supervisor.StartRunner(ctx, DefaultMainRunnerID)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("start runner: %v", err)
		}
	}

	args := readHelperArgs(t, argsFile, &childOutput, 1)
	if count := strings.Count(args, "serve --host 127.0.0.1 --port 9481"); count != 1 {
		t.Fatalf("serve invocation count = %d, want 1; args = %q", count, args)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := supervisor.StopRunner(stopCtx, DefaultMainRunnerID); err != nil {
		t.Fatalf("stop runner: %v", err)
	}
	if state := supervisor.LegacyStatus().State; state != string(StateStopped) {
		t.Fatalf("state = %q, want stopped", state)
	}
}

func TestSupervisorStartsLlamaCPPMainRunner(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeLlamaHelper(t)
	modelPath := filepath.Join(t.TempDir(), "gemma.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	var childOutput bytes.Buffer
	logs := server.NewLogBroadcaster(16)
	supervisor := New(Config{
		DefaultLiteRT: LiteRTConfig{
			Launch: false,
			Host:   "127.0.0.1",
			Port:   9381,
		},
		Logs:      logs,
		StdoutTee: &childOutput,
		StderrTee: &childOutput,
	})
	runnerID, err := supervisor.CreateRunner(RunnerSpec{
		ID:         "main-llamacpp",
		Runtime:    RuntimeLlamaCPP,
		Role:       RoleMain,
		Backend:    BackendMetal,
		Executable: exe,
		ModelPath:  modelPath,
		ModelID:    "gemma4-gguf",
		Host:       "127.0.0.1",
		Port:       9491,
		Launch:     true,
	})
	if err != nil {
		t.Fatalf("create llama runner: %v", err)
	}

	if err := supervisor.StartRunner(context.Background(), runnerID); err != nil {
		t.Fatalf("start llama runner: %v", err)
	}
	args := readHelperArgs(t, argsFile, &childOutput, 1)
	if got, ok := supervisor.UpstreamForPath("/v1/chat/completions"); !ok || got != "http://127.0.0.1:9491" {
		t.Fatalf("chat upstream = %q/%v, want llama runner", got, ok)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := supervisor.StopRunner(stopCtx, runnerID); err != nil {
		t.Fatalf("stop llama runner: %v", err)
	}

	for _, want := range []string{
		"-m " + modelPath,
		"--alias gemma4-gguf",
		"--host 127.0.0.1",
		"--port 9491",
		"--n-gpu-layers 999",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("llama args = %q, want %q", args, want)
		}
	}
}

func TestSupervisorStartsLlamaCPPEmbeddingRunner(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeLlamaHelper(t)
	modelPath := filepath.Join(t.TempDir(), "embedding.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	var childOutput bytes.Buffer
	supervisor := New(Config{
		DefaultLiteRT: LiteRTConfig{Launch: false, Host: "127.0.0.1", Port: 9381},
		StdoutTee:     &childOutput,
		StderrTee:     &childOutput,
	})
	runnerID, err := supervisor.CreateRunner(RunnerSpec{
		ID:         "embedding-llamacpp",
		Runtime:    RuntimeLlamaCPP,
		Role:       RoleEmbedding,
		Backend:    BackendCPU,
		Executable: exe,
		ModelPath:  modelPath,
		ModelID:    "qwen3-embedding",
		Host:       "127.0.0.1",
		Port:       9492,
		Launch:     true,
	})
	if err != nil {
		t.Fatalf("create embedding runner: %v", err)
	}

	if err := supervisor.StartRunner(context.Background(), runnerID); err != nil {
		t.Fatalf("start embedding runner: %v", err)
	}
	args := readHelperArgs(t, argsFile, &childOutput, 1)
	if got, ok := supervisor.UpstreamForPath("/v1/embeddings"); !ok || got != "http://127.0.0.1:9492" {
		t.Fatalf("embedding upstream = %q/%v, want embedding runner", got, ok)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := supervisor.StopRunner(stopCtx, runnerID); err != nil {
		t.Fatalf("stop embedding runner: %v", err)
	}

	if !strings.Contains(args, "--embedding") {
		t.Fatalf("embedding args = %q, want --embedding", args)
	}
	if strings.Contains(args, "--reranking") {
		t.Fatalf("embedding args = %q, should not include reranking", args)
	}
}

func TestSupervisorStartsLlamaCPPRerankRunner(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeLlamaHelper(t)
	modelPath := filepath.Join(t.TempDir(), "rerank.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	var childOutput bytes.Buffer
	supervisor := New(Config{
		DefaultLiteRT: LiteRTConfig{Launch: false, Host: "127.0.0.1", Port: 9381},
		StdoutTee:     &childOutput,
		StderrTee:     &childOutput,
	})
	runnerID, err := supervisor.CreateRunner(RunnerSpec{
		ID:         "rerank-llamacpp",
		Runtime:    RuntimeLlamaCPP,
		Role:       RoleReranking,
		Backend:    BackendCPU,
		Executable: exe,
		ModelPath:  modelPath,
		ModelID:    "qwen3-reranker-q4km",
		Host:       "127.0.0.1",
		Port:       9493,
		Launch:     true,
	})
	if err != nil {
		t.Fatalf("create rerank runner: %v", err)
	}

	if err := supervisor.StartRunner(context.Background(), runnerID); err != nil {
		t.Fatalf("start rerank runner: %v", err)
	}
	args := readHelperArgs(t, argsFile, &childOutput, 1)
	if got, ok := supervisor.UpstreamForPath("/v1/rerank"); !ok || got != "http://127.0.0.1:9493" {
		t.Fatalf("rerank upstream = %q/%v, want rerank runner", got, ok)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := supervisor.StopRunner(stopCtx, runnerID); err != nil {
		t.Fatalf("stop rerank runner: %v", err)
	}

	for _, want := range []string{"--embedding", "--pooling rank", "--reranking"} {
		if !strings.Contains(args, want) {
			t.Fatalf("rerank args = %q, want %q", args, want)
		}
	}
}

func TestSupervisorStartsDefaultLiteRTWithControlPatch(t *testing.T) {
	t.Parallel()

	launch := false
	importModel := false
	token := "hf_secret"
	supervisor := New(Config{
		DefaultLiteRT: LiteRTConfig{
			Launch:   true,
			Host:     "127.0.0.1",
			Port:     9381,
			ModelID:  "old-model",
			Upstream: "http://127.0.0.1:9381",
		},
		ImportModel: true,
	})

	err := supervisor.StartDefaultLiteRT(context.Background(), RuntimeModeDebug, LiteRTPatch{
		Launch:           &launch,
		Executable:       "/opt/litert-lm",
		Host:             "127.0.0.1",
		Port:             9481,
		Upstream:         "http://127.0.0.1:9999",
		ModelPath:        "models/litert/main/gemma-4-E2B-it.litertlm",
		ModelID:          "gemma4-e2b",
		HuggingFaceToken: &token,
		ImportModel:      &importModel,
	})
	if err != nil {
		t.Fatalf("start default external litert: %v", err)
	}

	status := supervisor.LegacyStatus()
	if status.State != string(StateExternal) {
		t.Fatalf("state = %q, want external", status.State)
	}
	if status.Mode != string(RuntimeModeDebug) {
		t.Fatalf("mode = %q, want debug", status.Mode)
	}
	if status.Upstream != "http://127.0.0.1:9999" {
		t.Fatalf("upstream = %q, want configured external upstream", status.Upstream)
	}

	config := supervisor.DefaultLiteRTConfig()
	if config.Executable != "/opt/litert-lm" {
		t.Fatalf("executable = %q", config.Executable)
	}
	if config.ModelID != "gemma4-e2b" || config.ModelFile != "models/litert/main/gemma-4-E2B-it.litertlm" {
		t.Fatalf("model config = %#v", config)
	}
	if config.HuggingFaceToken != "hf_secret" {
		t.Fatalf("hugging face token = %q", config.HuggingFaceToken)
	}
	if config.ImportModel {
		t.Fatal("import model = true, want false")
	}
}

func TestSupervisorDefaultLiteRTPatchPreservesExternalUpstream(t *testing.T) {
	t.Parallel()

	supervisor := New(Config{
		DefaultLiteRT: LiteRTConfig{
			Launch:   false,
			Host:     "127.0.0.1",
			Port:     9381,
			Upstream: "http://127.0.0.1:9999",
		},
	})

	err := supervisor.StartDefaultLiteRT(context.Background(), RuntimeModeRelease, LiteRTPatch{})
	if err != nil {
		t.Fatalf("start default external litert: %v", err)
	}

	if got := supervisor.LegacyStatus().Upstream; got != "http://127.0.0.1:9999" {
		t.Fatalf("upstream = %q, want existing external upstream", got)
	}
}

func TestLogBroadcasterRedactsHuggingFaceToken(t *testing.T) {
	t.Setenv("HF_TOKEN", "hf_secret")
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "hub_secret")

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runtime", "stderr", "tokens hf_secret hub_secret")
	snapshot, _, unsubscribe := logs.Subscribe()
	defer unsubscribe()

	if len(snapshot) != 1 {
		t.Fatalf("snapshot count = %d, want 1", len(snapshot))
	}
	line := snapshot[0].Line
	if strings.Contains(line, "hf_secret") || strings.Contains(line, "hub_secret") {
		t.Fatalf("log line leaked token: %q", line)
	}
	if strings.Count(line, "[redacted]") != 2 {
		t.Fatalf("log line = %q, want two redaction markers", line)
	}
}

func writeLiteRTHelper(t *testing.T) (string, string) {
	t.Helper()
	if os.Getenv("SUPERVISOR_LITERT_HELPER") == "1" {
		t.Fatal("helper should not run in parent test process")
	}
	if isWindows() {
		t.Skip("shell helper is unix-specific")
	}

	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	exe := filepath.Join(dir, "litert-lm-test")
	script := "#!/bin/sh\n" +
		"SUPERVISOR_LITERT_HELPER=1 ARGS_FILE=" + shellQuote(argsFile) + " exec " +
		shellQuote(os.Args[0]) + " -test.run=TestSupervisorLiteRTHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	return exe, argsFile
}

func writeLlamaHelper(t *testing.T) (string, string) {
	t.Helper()
	if os.Getenv("SUPERVISOR_LLAMA_HELPER") == "1" {
		t.Fatal("helper should not run in parent test process")
	}
	if isWindows() {
		t.Skip("shell helper is unix-specific")
	}

	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	exe := filepath.Join(dir, "llama-server-test")
	script := "#!/bin/sh\n" +
		"SUPERVISOR_LLAMA_HELPER=1 ARGS_FILE=" + shellQuote(argsFile) + " exec " +
		shellQuote(os.Args[0]) + " -test.run=TestSupervisorLlamaHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	return exe, argsFile
}

func readHelperArgs(t *testing.T, path string, output *bytes.Buffer, wantLines int) string {
	t.Helper()

	var lastErr error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			text := string(data)
			if len(strings.Split(strings.TrimSpace(text), "\n")) >= wantLines {
				return text
			}
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("read helper args: %v; helper output: %q", lastErr, output.String())
	return ""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func isWindows() bool {
	return filepath.Separator == '\\'
}

func TestSupervisorLiteRTHelperProcess(t *testing.T) {
	if os.Getenv("SUPERVISOR_LITERT_HELPER") != "1" {
		return
	}

	args := helperArgs()
	if len(args) > 0 && args[0] == "--version" {
		fmt.Fprintln(os.Stdout, "helper-version")
		return
	}
	if len(args) > 0 && args[0] == "list" {
		return
	}

	argsFile := os.Getenv("ARGS_FILE")
	if argsFile == "" {
		fmt.Fprintln(os.Stderr, "ARGS_FILE is required")
		os.Exit(2)
	}
	file, err := os.OpenFile(argsFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open args: %v\n", err)
		os.Exit(2)
	}
	if _, err := fmt.Fprintln(file, strings.Join(args, " ")); err != nil {
		fmt.Fprintf(os.Stderr, "write args: %v\n", err)
		os.Exit(2)
	}
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close args: %v\n", err)
		os.Exit(2)
	}

	if len(args) > 0 && args[0] == "serve" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		<-signals
	}
}

func TestSupervisorLlamaHelperProcess(t *testing.T) {
	if os.Getenv("SUPERVISOR_LLAMA_HELPER") != "1" {
		return
	}

	args := helperArgs()
	if len(args) > 0 && args[0] == "--version" {
		fmt.Fprintln(os.Stdout, "llama-helper-version")
		return
	}

	argsFile := os.Getenv("ARGS_FILE")
	if argsFile == "" {
		fmt.Fprintln(os.Stderr, "ARGS_FILE is required")
		os.Exit(2)
	}
	file, err := os.OpenFile(argsFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open args: %v\n", err)
		os.Exit(2)
	}
	if _, err := fmt.Fprintln(file, strings.Join(args, " ")); err != nil {
		fmt.Fprintf(os.Stderr, "write args: %v\n", err)
		os.Exit(2)
	}
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close args: %v\n", err)
		os.Exit(2)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
}

func helperArgs() []string {
	for index, arg := range os.Args {
		if arg == "--" {
			return os.Args[index+1:]
		}
	}
	return nil
}
