package litert

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestBuildServeCommand(t *testing.T) {
	t.Parallel()

	cmd := BuildServeCommand("litert-lm", "127.0.0.1", 9381, true)
	got := cmd.Args
	want := []string{
		"litert-lm",
		"serve",
		"--host",
		"127.0.0.1",
		"--port",
		"9381",
		"--verbose",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestBuildImportCommand(t *testing.T) {
	t.Parallel()

	cmd := BuildImportCommand("litert-lm", "models/litert/main/gemma-4-E2B-it.litertlm", "gemma4-e2b")
	got := cmd.Args
	want := []string{
		"litert-lm",
		"import",
		"models/litert/main/gemma-4-E2B-it.litertlm",
		"gemma4-e2b",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestBuildImportCommandInjectsHuggingFaceTokenEnvironment(t *testing.T) {
	t.Parallel()

	cmd := BuildImportCommandWithHuggingFaceToken(
		"litert-lm",
		"models/litert/main/gemma-4-E2B-it.litertlm",
		"gemma4-e2b",
		"hf_secret",
	)

	assertCommandDoesNotLeakSecret(t, cmd, "hf_secret")
	assertCommandEnvContains(t, cmd, "HF_TOKEN", "hf_secret")
	assertCommandEnvContains(t, cmd, "HUGGING_FACE_HUB_TOKEN", "hf_secret")
}

func TestDefaultModelSearchPathsPreferLiteRTDirectory(t *testing.T) {
	t.Parallel()

	paths := defaultModelSearchPaths()
	want := filepath.Join("models", "litert", "main", "gemma-4-E2B-it.litertlm")

	if len(paths) == 0 {
		t.Fatal("default model search paths are empty")
	}
	if paths[0] != want {
		t.Fatalf("first default model path = %q, want %q", paths[0], want)
	}
}

func TestEnsureModelImportedRedactsHuggingFaceTokenFromImportOutput(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is unix-specific")
	}

	exe := writeFailingTokenEchoHelper(t)
	modelFile := filepath.Join(t.TempDir(), "model.litertlm")
	if err := os.WriteFile(modelFile, []byte("model"), 0o600); err != nil {
		t.Fatalf("write model file: %v", err)
	}

	err := EnsureModelImportedWithHuggingFaceToken(
		context.Background(),
		exe,
		modelFile,
		"gemma4-e2b",
		"hf_secret",
	)

	if err == nil {
		t.Fatal("expected import error")
	}
	if strings.Contains(err.Error(), "hf_secret") {
		t.Fatalf("import error leaked token: %s", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("import error = %q, want redaction marker", err.Error())
	}
}

func TestBuildRunCommandIncludesMultimodalAttachments(t *testing.T) {
	t.Parallel()

	cmd := BuildRunCommand("litert-lm", RunRequest{
		ModelID:         "gemma4-e2b",
		Backend:         "gpu",
		VisionBackend:   "gpu",
		AudioBackend:    "cpu",
		Prompt:          "Describe this image.",
		AttachmentPaths: []string{"/tmp/sample.png", "/tmp/audio.wav"},
	})
	got := cmd.Args
	want := []string{
		"litert-lm",
		"run",
		"gemma4-e2b",
		"--backend=gpu",
		"--vision-backend=gpu",
		"--audio-backend=cpu",
		"--attachment=/tmp/sample.png",
		"--attachment=/tmp/audio.wav",
		"--prompt=Describe this image.",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestBuildRunCommandIncludesAdvancedRunOptions(t *testing.T) {
	t.Parallel()

	topP := 0.75
	temperature := 0.2
	cmd := BuildRunCommand("litert-lm", RunRequest{
		ModelID:                         "gemma4-e2b",
		Backend:                         "cpu",
		Prompt:                          "Hello",
		MaxNumTokens:                    4096,
		TopK:                            8,
		TopP:                            &topP,
		Temperature:                     &temperature,
		Seed:                            123,
		Preset:                          "creative.json",
		NoTemplate:                      true,
		FilterChannelContentFromKVCache: true,
		EnableSpeculativeDecoding:       "false",
		Cache:                           "memory",
		Verbose:                         true,
		FromHuggingFaceRepo:             "google/gemma",
		HuggingFaceToken:                "hf_secret",
	})
	got := cmd.Args
	want := []string{
		"litert-lm",
		"run",
		"gemma4-e2b",
		"--backend=cpu",
		"--max-num-tokens=4096",
		"--top-k=8",
		"--top-p=0.75",
		"--temperature=0.2",
		"--seed=123",
		"--preset=creative.json",
		"--no-template",
		"--filter-channel-content-from-kv-cache",
		"--enable-speculative-decoding=false",
		"--cache=memory",
		"--verbose",
		"--from-huggingface-repo=google/gemma",
		"--prompt=Hello",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	assertCommandDoesNotLeakSecret(t, cmd, "hf_secret")
	assertCommandEnvContains(t, cmd, "HF_TOKEN", "hf_secret")
	assertCommandEnvContains(t, cmd, "HUGGING_FACE_HUB_TOKEN", "hf_secret")
}

func TestRunOnceRedactsHuggingFaceTokenFromFailedOutput(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is unix-specific")
	}

	exe := writeFailingTokenEchoHelper(t)

	_, err := RunOnce(context.Background(), exe, RunRequest{
		ModelID:          "gemma4-e2b",
		Prompt:           "hello",
		HuggingFaceToken: "hf_secret",
	})

	if err == nil {
		t.Fatal("expected run error")
	}
	if strings.Contains(err.Error(), "hf_secret") {
		t.Fatalf("run error leaked token: %s", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("run error = %q, want redaction marker", err.Error())
	}
}

func TestBuildRunCommandPreservesZeroFloatOptions(t *testing.T) {
	t.Parallel()

	topP := 0.0
	temperature := 0.0
	cmd := BuildRunCommand("litert-lm", RunRequest{
		ModelID:     "gemma4-e2b",
		TopP:        &topP,
		Temperature: &temperature,
		Prompt:      "Hello",
	})
	got := cmd.Args
	want := []string{
		"litert-lm",
		"run",
		"gemma4-e2b",
		"--top-p=0",
		"--temperature=0",
		"--prompt=Hello",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestModelIDInList(t *testing.T) {
	t.Parallel()

	output := `ID              SIZE            MODIFIED
gemma3-1b       557.3 MB        2026-03-05 17:00:53
gemma4-e2b      2.4 GB          2026-04-02 10:44:33
`

	if !ModelIDInList(output, "gemma4-e2b") {
		t.Fatal("gemma4-e2b was not detected")
	}
	if ModelIDInList(output, "gemma4") {
		t.Fatal("partial model ID matched unexpectedly")
	}
}

func TestBuildUpstreamURL(t *testing.T) {
	t.Parallel()

	got := BuildUpstreamURL("127.0.0.1", 9381)
	want := "http://127.0.0.1:9381"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestPlatformExecutableNameForWindows(t *testing.T) {
	t.Parallel()

	if got := platformExecutableNameFor("windows"); got != "litert-lm.exe" {
		t.Fatalf("windows executable name = %q", got)
	}
	if got := platformExecutableNameFor("darwin"); got != "litert-lm" {
		t.Fatalf("darwin executable name = %q", got)
	}
	if got := platformExecutableNameFor("linux"); got != "litert-lm" {
		t.Fatalf("linux executable name = %q", got)
	}
}

func TestIsExecutableForWindowsAllowsRegularFiles(t *testing.T) {
	t.Parallel()

	if !isExecutableFor("windows", 0o644) {
		t.Fatal("windows should accept a regular readable file as runnable")
	}
	if isExecutableFor("darwin", 0o644) {
		t.Fatal("darwin should require executable bits")
	}
	if !isExecutableFor("darwin", 0o755) {
		t.Fatal("darwin should accept executable bits")
	}
}

func TestManagerNoLaunchReportsExternalRuntime(t *testing.T) {
	t.Parallel()

	manager := NewManager(Config{
		Launch:   false,
		ModelID:  "gemma4-e2b",
		Upstream: "http://127.0.0.1:9999",
	})

	status := manager.Status()
	if status.State != "external" {
		t.Fatalf("state = %q, want external", status.State)
	}
	if status.ModelID != "gemma4-e2b" {
		t.Fatalf("model id = %q", status.ModelID)
	}
	if status.Upstream != "http://127.0.0.1:9999" {
		t.Fatalf("upstream = %q, want configured external upstream", status.Upstream)
	}
}

func TestManagerStartModeWithConfigUsesExternalUpstreamInStatus(t *testing.T) {
	t.Parallel()

	launch := false
	manager := NewManager(Config{
		Launch: false,
		Host:   "127.0.0.1",
		Port:   9381,
	})

	if err := manager.StartModeWithConfig(context.Background(), RuntimeModeRelease, ConfigPatch{
		Launch:   &launch,
		Upstream: "http://127.0.0.1:9999",
	}); err != nil {
		t.Fatalf("start external runtime: %v", err)
	}
	status := manager.Status()
	if status.State != "external" {
		t.Fatalf("state = %q, want external", status.State)
	}
	if status.Upstream != "http://127.0.0.1:9999" {
		t.Fatalf("upstream = %q, want browser-configured external upstream", status.Upstream)
	}
}

func TestManagerStartModeControlsVerboseFlag(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeRuntimeHelper(t)
	var helperOutput bytes.Buffer
	manager := NewManager(Config{
		Launch:      true,
		Executable:  exe,
		ImportModel: false,
		Verbose:     true,
		Stdout:      &helperOutput,
		Stderr:      &helperOutput,
	})
	ctx := context.Background()

	if err := manager.StartMode(ctx, RuntimeModeDebug); err != nil {
		t.Fatalf("start debug mode: %v", err)
	}
	args := readRuntimeHelperArgs(t, argsFile, &helperOutput, 1)
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("stop debug runtime: %v", err)
	}

	if !strings.Contains(args, "--verbose") {
		t.Fatalf("debug start args = %q, want --verbose", args)
	}
	status := manager.Status()
	if status.Mode != string(RuntimeModeDebug) {
		t.Fatalf("mode = %q, want %q", status.Mode, RuntimeModeDebug)
	}
}

func TestManagerStartModeHonorsRuntimeVerboseInReleaseMode(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeRuntimeHelper(t)
	var helperOutput bytes.Buffer
	manager := NewManager(Config{
		Launch:      true,
		Executable:  exe,
		ImportModel: false,
		Verbose:     true,
		Stdout:      &helperOutput,
		Stderr:      &helperOutput,
	})
	ctx := context.Background()

	if err := manager.StartMode(ctx, RuntimeModeRelease); err != nil {
		t.Fatalf("start release mode: %v", err)
	}
	args := readRuntimeHelperArgs(t, argsFile, &helperOutput, 1)
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("stop release runtime: %v", err)
	}

	if !strings.Contains(args, "--verbose") {
		t.Fatalf("release start args = %q, want --verbose from runtimeVerbose option", args)
	}
}

func TestManagerStartModeWithConfigAppliesBrowserRuntimeConfig(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeRuntimeHelper(t)
	var helperOutput bytes.Buffer
	launch := true
	importModel := false
	verbose := false
	manager := NewManager(Config{
		Launch:      false,
		Executable:  "missing-litert-lm",
		Host:        "127.0.0.1",
		Port:        9381,
		ModelID:     "old-model",
		ImportModel: true,
		Stdout:      &helperOutput,
		Stderr:      &helperOutput,
	})
	ctx := context.Background()

	if err := manager.StartModeWithConfig(ctx, RuntimeModeRelease, ConfigPatch{
		Launch:      &launch,
		Executable:  exe,
		Host:        "127.0.0.1",
		Port:        9481,
		ModelID:     "gemma4-e2b",
		ImportModel: &importModel,
		Verbose:     &verbose,
	}); err != nil {
		t.Fatalf("start with browser runtime config: %v", err)
	}
	args := readRuntimeHelperArgs(t, argsFile, &helperOutput, 1)
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("stop configured runtime: %v", err)
	}

	if !strings.Contains(args, "serve --host 127.0.0.1 --port 9481") {
		t.Fatalf("start args = %q, want browser-selected host and port", args)
	}
	if strings.Contains(args, "--verbose") {
		t.Fatalf("start args = %q, release mode should not include verbose", args)
	}
	status := manager.Status()
	if status.ModelID != "gemma4-e2b" {
		t.Fatalf("model id = %q, want gemma4-e2b", status.ModelID)
	}
	if status.Upstream != "http://127.0.0.1:9481" {
		t.Fatalf("upstream = %q, want browser-selected port", status.Upstream)
	}
}

func TestManagerConfigSnapshotReflectsBrowserRuntimeConfig(t *testing.T) {
	t.Parallel()

	launch := true
	importModel := false
	verbose := true
	huggingFaceToken := "hf_secret"
	manager := NewManager(Config{
		Launch:      false,
		Executable:  "startup-litert-lm",
		Host:        "127.0.0.1",
		Port:        9381,
		ModelFile:   "startup-model.litertlm",
		ModelID:     "startup-model",
		ImportModel: true,
	})

	if err := manager.ApplyConfigPatch(ConfigPatch{
		Launch:           &launch,
		Executable:       "/opt/litert-lm",
		Host:             "127.0.0.1",
		Port:             9481,
		ModelFile:        "runtime-model.litertlm",
		ModelID:          "runtime-model",
		ImportModel:      &importModel,
		Verbose:          &verbose,
		HuggingFaceToken: &huggingFaceToken,
	}); err != nil {
		t.Fatalf("apply config patch: %v", err)
	}

	snapshot := manager.ConfigSnapshot()
	if !snapshot.Launch {
		t.Fatal("launch = false, want true")
	}
	if snapshot.Executable != "/opt/litert-lm" {
		t.Fatalf("executable = %q", snapshot.Executable)
	}
	if snapshot.Port != 9481 {
		t.Fatalf("port = %d, want 9481", snapshot.Port)
	}
	if snapshot.ModelFile != "runtime-model.litertlm" {
		t.Fatalf("model file = %q", snapshot.ModelFile)
	}
	if snapshot.ModelID != "runtime-model" {
		t.Fatalf("model id = %q", snapshot.ModelID)
	}
	if snapshot.ImportModel {
		t.Fatal("import model = true, want false")
	}
	if !snapshot.Verbose {
		t.Fatal("verbose = false, want true")
	}
	if snapshot.HuggingFaceToken != "hf_secret" {
		t.Fatalf("hugging face token = %q, want browser token", snapshot.HuggingFaceToken)
	}
	huggingFaceToken = ""
	if err := manager.ApplyConfigPatch(ConfigPatch{
		HuggingFaceToken: &huggingFaceToken,
	}); err != nil {
		t.Fatalf("clear token config patch: %v", err)
	}
	if snapshot := manager.ConfigSnapshot(); snapshot.HuggingFaceToken != "" {
		t.Fatalf("cleared hugging face token = %q, want empty", snapshot.HuggingFaceToken)
	}
}

func TestManagerRestartCanStartAfterStopWithNewMode(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeRuntimeHelper(t)
	var helperOutput bytes.Buffer
	manager := NewManager(Config{
		Launch:      true,
		Executable:  exe,
		ImportModel: false,
		Stdout:      &helperOutput,
		Stderr:      &helperOutput,
	})
	ctx := context.Background()

	if err := manager.StartMode(ctx, RuntimeModeDebug); err != nil {
		t.Fatalf("start debug mode: %v", err)
	}
	readRuntimeHelperArgs(t, argsFile, &helperOutput, 1)
	restartCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := manager.Restart(restartCtx, RuntimeModeRelease); err != nil {
		t.Fatalf("restart release mode: %v", err)
	}
	argsText := readRuntimeHelperArgs(t, argsFile, &helperOutput, 2)
	stopCtx, stopCancel := context.WithTimeout(ctx, 2*time.Second)
	defer stopCancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("stop release runtime: %v", err)
	}

	args := strings.Split(strings.TrimSpace(argsText), "\n")
	if len(args) != 2 {
		t.Fatalf("helper invocation count = %d, want 2; args = %#v", len(args), args)
	}
	if !strings.Contains(args[0], "--verbose") {
		t.Fatalf("first args = %q, want debug --verbose", args[0])
	}
	if strings.Contains(args[1], "--verbose") {
		t.Fatalf("second args = %q, release should not include --verbose", args[1])
	}
	status := manager.Status()
	if status.Mode != string(RuntimeModeRelease) {
		t.Fatalf("mode = %q, want %q", status.Mode, RuntimeModeRelease)
	}
	if status.State != "stopped" {
		t.Fatalf("state = %q, want stopped", status.State)
	}
}

func TestManagerSerializesConcurrentStart(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeRuntimeHelper(t)
	var helperOutput bytes.Buffer
	manager := NewManager(Config{
		Launch:      true,
		Executable:  exe,
		ImportModel: false,
		Stdout:      &helperOutput,
		Stderr:      &helperOutput,
	})
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- manager.StartMode(ctx, RuntimeModeRelease)
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent start: %v", err)
		}
	}
	argsText := readRuntimeHelperArgs(t, argsFile, &helperOutput, 1)
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("stop runtime: %v", err)
	}

	serveCount := 0
	for _, line := range strings.Split(strings.TrimSpace(argsText), "\n") {
		if strings.HasPrefix(line, "serve ") {
			serveCount++
		}
	}
	if serveCount != 1 {
		t.Fatalf("serve invocation count = %d, want 1; args = %q", serveCount, argsText)
	}
}

func TestManagerPublishesStatusWhenRuntimeExits(t *testing.T) {
	t.Parallel()

	exe, argsFile := writeRuntimeHelperMode(t, "exit")
	statuses := make(chan RuntimeStatus, 8)
	var helperOutput bytes.Buffer
	manager := NewManager(Config{
		Launch:      true,
		Executable:  exe,
		ImportModel: false,
		Stdout:      &helperOutput,
		Stderr:      &helperOutput,
		OnStatusChange: func(status RuntimeStatus) {
			statuses <- status
		},
	})
	ctx := context.Background()

	if err := manager.StartMode(ctx, RuntimeModeRelease); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	readRuntimeHelperArgs(t, argsFile, &helperOutput, 1)
	status := waitForManagerStatus(t, statuses, "exited")

	if status.Detail != "LiteRT-LM server process exited cleanly." {
		t.Fatalf("exit detail = %q", status.Detail)
	}
}

func TestManagerWaitIgnoresStaleProcessStatus(t *testing.T) {
	t.Parallel()

	manager := NewManager(Config{Launch: true})
	oldCmd := &exec.Cmd{}
	newCmd := &exec.Cmd{}
	done := make(chan error, 1)
	manager.mu.Lock()
	manager.cmd = newCmd
	manager.done = make(chan error, 1)
	manager.status = RuntimeStatus{
		State:  "running",
		Detail: "new runtime is running",
	}
	manager.mu.Unlock()

	manager.wait(context.Background(), oldCmd, done)

	status := manager.Status()
	if status.State != "running" {
		t.Fatalf("state = %q, want stale wait to preserve running status", status.State)
	}
	if status.Detail != "new runtime is running" {
		t.Fatalf("detail = %q, want current runtime detail", status.Detail)
	}
}

func writeRuntimeHelper(t *testing.T) (string, string) {
	t.Helper()

	return writeRuntimeHelperMode(t, "")
}

func writeRuntimeHelperMode(t *testing.T, mode string) (string, string) {
	t.Helper()

	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	exe := filepath.Join(dir, "litert-lm-test")
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is unix-specific")
	}
	modeEnv := ""
	if mode != "" {
		modeEnv = " LITERT_RUNTIME_HELPER_MODE=" + shellQuote(mode)
	}
	script := "#!/bin/sh\n" +
		"LITERT_RUNTIME_HELPER=1 ARGS_FILE=" + shellQuote(argsFile) + modeEnv + " exec " +
		shellQuote(os.Args[0]) + " -test.run=TestRuntimeHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper executable: %v", err)
	}

	return exe, argsFile
}

func writeFailingTokenEchoHelper(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	exe := filepath.Join(dir, "litert-lm-token-echo")
	script := `#!/bin/sh
if [ "$1" = "list" ]; then
  exit 0
fi
echo "child output token=$HF_TOKEN" >&2
exit 1
`
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatalf("write token echo helper: %v", err)
	}

	return exe
}

func waitForManagerStatus(t *testing.T, statuses <-chan RuntimeStatus, state string) RuntimeStatus {
	t.Helper()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case status := <-statuses:
			if status.State == state {
				return status
			}
		case <-deadline:
			t.Fatalf("timed out waiting for runtime status %q", state)
		}
	}
}

func readRuntimeHelperArgs(t *testing.T, path string, output *bytes.Buffer, wantLines int) string {
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

func assertCommandDoesNotLeakSecret(t *testing.T, cmd *exec.Cmd, secret string) {
	t.Helper()

	if strings.Contains(strings.Join(cmd.Args, "\x00"), secret) {
		t.Fatalf("command args leak secret %q: %#v", secret, cmd.Args)
	}
}

func assertCommandEnvContains(t *testing.T, cmd *exec.Cmd, key string, value string) {
	t.Helper()

	prefix := key + "="
	for _, item := range cmd.Env {
		if strings.HasPrefix(item, prefix) {
			if got := strings.TrimPrefix(item, prefix); got != value {
				t.Fatalf("%s = %q, want %q", key, got, value)
			}
			return
		}
	}

	t.Fatalf("%s was not set in command environment", key)
}

func TestRuntimeHelperProcess(t *testing.T) {
	if os.Getenv("LITERT_RUNTIME_HELPER") != "1" {
		return
	}

	args := helperArgs()
	if len(args) > 0 && args[0] == "--version" {
		fmt.Fprintln(os.Stdout, "helper-version")
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
		if os.Getenv("LITERT_RUNTIME_HELPER_MODE") == "exit" {
			return
		}
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		<-signals
		return
	}
}

func helperArgs() []string {
	for index, arg := range os.Args {
		if arg == "--" {
			return os.Args[index+1:]
		}
	}
	return nil
}
