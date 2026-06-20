package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"time"

	"litert-sidecar/internal/proxy"
	"litert-sidecar/internal/server"
	"litert-sidecar/internal/supervisor"
)

const (
	realRuntimeEnv = "SIDECAR_E2E_REAL"
	timeoutEnv     = "SIDECAR_E2E_TIMEOUT_SECONDS"
)

type RuntimeBackendResult struct {
	RunnerID     string
	Endpoint     string
	ResponseText string
}

func SuiteOptionsFromEnvironment() SuiteOptions {
	repoRoot := findRepoRoot()
	return SuiteOptions{
		RepoRoot:          repoRoot,
		BackendConfigPath: os.Getenv("RUNTIME_BACKEND_CONFIG"),
		LiteRTExecutable:  os.Getenv("LITERT_LM_BIN"),
		LlamaExecutable:   os.Getenv("LLAMA_SERVER_BIN"),
		LiteRTRuntimeRoot: os.Getenv("LITERT_RUNTIME_ROOT"),
		LlamaRuntimeRoot:  os.Getenv("LLAMA_RUNTIME_ROOT"),
		ForceReal:         RealRuntimeE2EForced(),
	}
}

func RealRuntimeE2EForced() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(realRuntimeEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func RuntimeBackendE2ETimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv(timeoutEnv))
	if value == "" {
		return 4 * time.Minute
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 4 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func RunRuntimeBackendCombo(
	ctx context.Context,
	combo RuntimeBackendCombo,
) (RuntimeBackendResult, error) {
	if combo.SkipReason != "" {
		return RuntimeBackendResult{}, fmt.Errorf("combo prerequisites missing: %s", combo.SkipReason)
	}

	port, err := pickFreePort()
	if err != nil {
		return RuntimeBackendResult{}, err
	}

	runtimeSupervisor := supervisor.New(supervisor.Config{
		DisableDefaultLiteRT: true,
		Logs:                 server.NewLogBroadcaster(128),
		ImportModel:          true,
	})

	runnerID := comboRunnerID(combo)
	createdID, err := runtimeSupervisor.CreateRunner(supervisor.RunnerSpec{
		ID:         runnerID,
		Runtime:    supervisor.Runtime(combo.Runtime),
		Role:       supervisor.Role(combo.Role),
		Backend:    supervisor.Backend(combo.RunnerBackend),
		Executable: combo.Executable,
		ModelPath:  combo.ModelPath,
		ModelID:    combo.ModelID,
		Host:       "127.0.0.1",
		Port:       port,
		Launch:     true,
	})
	if err != nil {
		return RuntimeBackendResult{}, fmt.Errorf("create runner: %w", err)
	}
	runnerID = createdID

	if err := withOptionalIsolatedHome(combo, func() error {
		return runtimeSupervisor.StartRunner(ctx, runnerID)
	}); err != nil {
		return RuntimeBackendResult{}, fmt.Errorf("start runner: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = runtimeSupervisor.CloseRunner(closeCtx, runnerID)
	}()

	upstream, ok := runtimeSupervisor.UpstreamForPath("/v1/chat/completions")
	if !ok || strings.TrimSpace(upstream) == "" {
		return RuntimeBackendResult{}, fmt.Errorf("runner did not register chat route")
	}
	upstreamProxy, err := proxy.New(upstream)
	if err != nil {
		return RuntimeBackendResult{}, fmt.Errorf("create proxy: %w", err)
	}
	upstreamProxy.SetTargetResolver(func(r *http.Request) (string, bool) {
		return runtimeSupervisor.UpstreamForPath(r.URL.Path)
	})

	handler := server.New(server.Options{
		Proxy: upstreamProxy,
		Logs:  server.NewLogBroadcaster(128),
	}).Handler()
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	endpoint := httpServer.URL + "/v1/chat/completions"
	responseText, err := postChatCompletion(ctx, endpoint, combo.ModelID)
	if err != nil {
		return RuntimeBackendResult{}, err
	}

	return RuntimeBackendResult{
		RunnerID:     runnerID,
		Endpoint:     endpoint,
		ResponseText: responseText,
	}, nil
}

func postChatCompletion(ctx context.Context, endpoint string, modelID string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "user", "content": "Say OK."},
		},
		"stream": false,
	})
	if err != nil {
		return "", fmt.Errorf("encode chat payload: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("create chat request: %w", err)
	}
	request.Header.Set("content-type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("post chat completion: %w", err)
	}
	defer response.Body.Close()

	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return "", fmt.Errorf("read chat response: %w", readErr)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("chat completion status %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", fmt.Errorf("decode chat response: %w: %s", err, strings.TrimSpace(string(responseBody)))
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("chat response had no choices: %s", strings.TrimSpace(string(responseBody)))
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("chat response had empty assistant content: %s", strings.TrimSpace(string(responseBody)))
	}
	return content, nil
}

func comboRunnerID(combo RuntimeBackendCombo) string {
	role := combo.Role
	if role == "" {
		role = "main"
	}
	return strings.NewReplacer("/", "-", "_", "-").Replace(
		"e2e-" + combo.Runtime + "-" + combo.ConfigBackend + "-" + role,
	)
}

func withOptionalIsolatedHome(combo RuntimeBackendCombo, run func() error) error {
	if combo.Runtime != "litert" {
		return run()
	}
	if strings.TrimSpace(os.Getenv("SIDECAR_E2E_LITERT_HOME")) != "" {
		return run()
	}

	home, err := os.MkdirTemp("", "litert-sidecar-e2e-home.")
	if err != nil {
		return fmt.Errorf("create isolated LiteRT home: %w", err)
	}
	defer os.RemoveAll(home)

	previous, hadPrevious := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		return fmt.Errorf("set isolated HOME: %w", err)
	}
	defer func() {
		if hadPrevious {
			_ = os.Setenv("HOME", previous)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()
	return run()
}

func pickFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("pick free port: %w", err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("free port listener address was %T", listener.Addr())
	}
	return address.Port, nil
}
