package litert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultExecutableName = "litert-lm"
	DefaultModelFileName  = "gemma-4-E2B-it.litertlm"
	DefaultModelDirectory = "litert/main"
	DefaultModelID        = "gemma4-e2b"
	DefaultRuntimeHost    = "127.0.0.1"
	DefaultRuntimePort    = 9381
)

type Config struct {
	Launch           bool
	Executable       string
	Host             string
	Port             int
	Upstream         string
	ModelFile        string
	ModelID          string
	HuggingFaceToken string
	ImportModel      bool
	Verbose          bool
	Stdout           io.Writer
	Stderr           io.Writer
	OnStatusChange   func(RuntimeStatus)
}

type ConfigPatch struct {
	Launch           *bool
	Executable       string
	Host             string
	Port             int
	Upstream         string
	ModelFile        string
	ModelID          string
	HuggingFaceToken *string
	ImportModel      *bool
	Verbose          *bool
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

type RunRequest struct {
	ModelID                         string
	Backend                         string
	VisionBackend                   string
	AudioBackend                    string
	Prompt                          string
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

type Manager struct {
	config Config

	opMu    sync.Mutex
	mu      sync.RWMutex
	cmd     *exec.Cmd
	done    chan error
	status  RuntimeStatus
	stopped bool
}

func NewManager(config Config) *Manager {
	if config.Host == "" {
		config.Host = DefaultRuntimeHost
	}
	if config.Port == 0 {
		config.Port = DefaultRuntimePort
	}
	if config.ModelID == "" {
		config.ModelID = DefaultModelID
	}

	state := "created"
	detail := "LiteRT-LM runtime has not been started yet."
	mode := string(RuntimeModeRelease)
	if config.Verbose {
		mode = string(RuntimeModeDebug)
	}
	if !config.Launch {
		state = "external"
		detail = "Sidecar is proxying an externally managed LiteRT-LM server."
	}

	return &Manager{
		config: config,
		status: RuntimeStatus{
			State:     state,
			ModelID:   config.ModelID,
			ModelFile: config.ModelFile,
			Upstream:  configuredUpstream(config),
			Mode:      mode,
			Detail:    detail,
		},
	}
}

func (m *Manager) Start(ctx context.Context) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	config := m.ConfigSnapshot()
	mode := RuntimeModeRelease
	if config.Verbose {
		mode = RuntimeModeDebug
	}
	return m.start(ctx, mode, config.Verbose)
}

func (m *Manager) StartMode(ctx context.Context, mode RuntimeMode) error {
	return m.StartModeWithConfig(ctx, mode, ConfigPatch{})
}

func (m *Manager) StartModeWithConfig(ctx context.Context, mode RuntimeMode, patch ConfigPatch) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if !isRuntimeMode(mode) {
		return fmt.Errorf("runtime mode must be %q or %q", RuntimeModeRelease, RuntimeModeDebug)
	}

	if err := m.ApplyConfigPatch(patch); err != nil {
		return err
	}
	config := m.ConfigSnapshot()
	return m.start(ctx, mode, mode == RuntimeModeDebug || config.Verbose)
}

func (m *Manager) Restart(ctx context.Context, mode RuntimeMode) error {
	return m.RestartWithConfig(ctx, mode, ConfigPatch{})
}

func (m *Manager) RestartWithConfig(ctx context.Context, mode RuntimeMode, patch ConfigPatch) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if !isRuntimeMode(mode) {
		return fmt.Errorf("runtime mode must be %q or %q", RuntimeModeRelease, RuntimeModeDebug)
	}
	if err := m.stop(ctx); err != nil {
		return err
	}

	if err := m.ApplyConfigPatch(patch); err != nil {
		return err
	}
	config := m.ConfigSnapshot()
	return m.start(ctx, mode, mode == RuntimeModeDebug || config.Verbose)
}

func (m *Manager) ApplyConfigPatch(patch ConfigPatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if patch.Launch != nil {
		m.config.Launch = *patch.Launch
	}
	if patch.Executable != "" {
		m.config.Executable = patch.Executable
	}
	if patch.Host != "" {
		m.config.Host = patch.Host
	}
	if patch.Port > 0 {
		m.config.Port = patch.Port
	} else if patch.Port < 0 {
		return fmt.Errorf("runtime port must be positive")
	}
	if patch.Upstream != "" {
		m.config.Upstream = patch.Upstream
	}
	if patch.ModelFile != "" {
		m.config.ModelFile = patch.ModelFile
	}
	if patch.ModelID != "" {
		m.config.ModelID = patch.ModelID
	}
	if patch.HuggingFaceToken != nil {
		m.config.HuggingFaceToken = strings.TrimSpace(*patch.HuggingFaceToken)
	}
	if patch.ImportModel != nil {
		m.config.ImportModel = *patch.ImportModel
	}
	if patch.Verbose != nil {
		m.config.Verbose = *patch.Verbose
	}

	m.refreshConfiguredStatusLocked()
	return nil
}

func (m *Manager) ConfigSnapshot() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.config
}

func (m *Manager) refreshConfiguredStatusLocked() {
	if m.status.State == "running" || m.status.State == "starting" {
		return
	}

	m.status.ModelID = m.config.ModelID
	m.status.ModelFile = m.config.ModelFile
	m.status.Upstream = configuredUpstream(m.config)
}

func (m *Manager) start(ctx context.Context, mode RuntimeMode, verbose bool) error {
	config := m.ConfigSnapshot()
	if !config.Launch {
		m.setStatus(RuntimeStatus{
			State:     "external",
			ModelID:   config.ModelID,
			ModelFile: config.ModelFile,
			Upstream:  configuredUpstream(config),
			Mode:      string(mode),
			Detail:    "Sidecar is proxying an externally managed LiteRT-LM server.",
		})
		return nil
	}

	if m.isRunning() {
		return nil
	}

	exe, err := resolveConfiguredExecutable(config.Executable)
	if err != nil {
		m.setStatus(RuntimeStatus{
			State:     "unavailable",
			ModelID:   config.ModelID,
			ModelFile: config.ModelFile,
			Upstream:  configuredUpstream(config),
			Mode:      string(mode),
			Detail:    err.Error(),
		})
		return err
	}

	modelFile := config.ModelFile
	if modelFile == "" {
		modelFile = FindDefaultModelFile()
	}

	status := RuntimeStatus{
		State:      "starting",
		Executable: exe,
		Version:    DetectVersion(exe),
		ModelID:    config.ModelID,
		ModelFile:  modelFile,
		Upstream:   configuredUpstream(config),
		Mode:       string(mode),
		Detail:     "Starting LiteRT-LM OpenAI-compatible server.",
	}
	m.setStatus(status)

	if config.ImportModel {
		if err := EnsureModelImportedWithHuggingFaceToken(ctx, exe, modelFile, config.ModelID, config.HuggingFaceToken); err != nil {
			status.State = "unavailable"
			status.Detail = err.Error()
			m.setStatus(status)
			return err
		}
	}

	cmd := BuildServeCommandContext(ctx, exe, config.Host, config.Port, verbose)
	cmd.Stdout = config.Stdout
	cmd.Stderr = config.Stderr

	if err := cmd.Start(); err != nil {
		status.State = "unavailable"
		status.Detail = fmt.Sprintf("start litert-lm serve: %v", err)
		m.setStatus(status)
		return err
	}

	m.mu.Lock()
	done := make(chan error, 1)
	m.cmd = cmd
	m.done = done
	m.stopped = false
	status.State = "running"
	status.Detail = "LiteRT-LM server process is running."
	m.status = status
	onStatusChange := m.config.OnStatusChange
	m.mu.Unlock()

	notifyStatusChange(onStatusChange, status)
	go m.wait(ctx, cmd, done)
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	return m.stop(ctx)
}

func (m *Manager) stop(ctx context.Context) error {
	m.mu.Lock()
	cmd := m.cmd
	done := m.done
	if cmd == nil || cmd.Process == nil || m.stopped {
		m.mu.Unlock()
		return nil
	}
	m.stopped = true
	m.mu.Unlock()

	if runtime.GOOS != "windows" {
		_ = cmd.Process.Signal(os.Interrupt)
	} else {
		_ = cmd.Process.Kill()
	}

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (m *Manager) Status() RuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.status
}

func (m *Manager) isRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.cmd != nil && !m.stopped && m.status.State == "running"
}

func resolveConfiguredExecutable(executable string) (string, error) {
	if executable != "" {
		return FindExecutable(executable)
	}

	return FindExecutable()
}

func configuredUpstream(config Config) string {
	upstream := strings.TrimSpace(config.Upstream)
	if !config.Launch && upstream != "" {
		return upstream
	}

	return BuildUpstreamURL(config.Host, config.Port)
}

func (m *Manager) wait(ctx context.Context, cmd *exec.Cmd, done chan<- error) {
	err := cmd.Wait()
	defer func() {
		done <- err
	}()

	m.mu.Lock()
	if m.cmd == cmd {
		m.cmd = nil
		m.done = nil
	} else {
		m.mu.Unlock()
		return
	}

	if m.stopped || errors.Is(ctx.Err(), context.Canceled) {
		m.status.State = "stopped"
		m.status.Detail = "LiteRT-LM server process was stopped."
	} else if err != nil {
		m.status.State = "exited"
		m.status.Detail = fmt.Sprintf("LiteRT-LM server process exited: %v", err)
	} else {
		m.status.State = "exited"
		m.status.Detail = "LiteRT-LM server process exited cleanly."
	}

	status := m.status
	onStatusChange := m.config.OnStatusChange
	m.mu.Unlock()
	notifyStatusChange(onStatusChange, status)
}

func (m *Manager) setStatus(status RuntimeStatus) {
	m.mu.Lock()
	m.status = status
	onStatusChange := m.config.OnStatusChange
	m.mu.Unlock()
	notifyStatusChange(onStatusChange, status)
}

func notifyStatusChange(onStatusChange func(RuntimeStatus), status RuntimeStatus) {
	if onStatusChange != nil {
		onStatusChange(status)
	}
}

func FindExecutable(candidates ...string) (string, error) {
	for _, candidate := range append(candidates, executableSearchPaths()...) {
		if candidate == "" {
			continue
		}

		if path, err := resolveExecutablePath(candidate); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("litert-lm executable was not found; pass -runtime-exe or put %q on PATH", platformExecutableName())
}

func FindDefaultModelFile() string {
	for _, candidate := range defaultModelSearchPaths() {
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return candidate
		}
	}

	return ""
}

func EnsureModelImported(ctx context.Context, exe string, modelFile string, modelID string) error {
	return EnsureModelImportedWithHuggingFaceToken(ctx, exe, modelFile, modelID, "")
}

func EnsureModelImportedWithHuggingFaceToken(
	ctx context.Context,
	exe string,
	modelFile string,
	modelID string,
	huggingFaceToken string,
) error {
	if modelID == "" {
		modelID = DefaultModelID
	}

	imported, listOutput, listErr := ModelInRegistry(ctx, exe, modelID)
	if imported {
		return nil
	}

	if modelFile == "" {
		if listErr != nil {
			return fmt.Errorf("check LiteRT-LM registry for %q: %w", modelID, listErr)
		}
		return fmt.Errorf("LiteRT-LM model %q is not in the registry and no model file was found; run litert-lm import or pass -model-file", modelID)
	}

	if stat, err := os.Stat(modelFile); err != nil || stat.IsDir() {
		if err == nil {
			err = fmt.Errorf("%s is a directory", modelFile)
		}
		return fmt.Errorf("model file %q is not usable: %w", modelFile, err)
	}

	cmd := BuildImportCommandContextWithHuggingFaceToken(ctx, exe, modelFile, modelID, huggingFaceToken)
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := redactSensitiveText(strings.TrimSpace(string(output)), huggingFaceToken)
		if listOutput != "" {
			detail = strings.TrimSpace(
				detail + "\n" + redactSensitiveText(listOutput, huggingFaceToken),
			)
		}
		if detail != "" {
			return fmt.Errorf("import LiteRT-LM model %q: %w: %s", modelID, err, detail)
		}
		return fmt.Errorf("import LiteRT-LM model %q: %w", modelID, err)
	}

	return nil
}

func ModelInRegistry(ctx context.Context, exe string, modelID string) (bool, string, error) {
	cmd := exec.CommandContext(ctx, exe, "list")
	output, err := cmd.CombinedOutput()
	outputText := string(output)
	if err != nil {
		return false, outputText, err
	}

	return ModelIDInList(outputText, modelID), outputText, nil
}

func ModelIDInList(output string, modelID string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == modelID {
			return true
		}
	}

	return false
}

func BuildServeCommand(exe string, host string, port int, verbose bool) *exec.Cmd {
	return BuildServeCommandContext(context.Background(), exe, host, port, verbose)
}

func BuildServeCommandContext(ctx context.Context, exe string, host string, port int, verbose bool) *exec.Cmd {
	args := []string{
		"serve",
		"--host",
		host,
		"--port",
		strconv.Itoa(port),
	}
	if verbose {
		args = append(args, "--verbose")
	}

	return exec.CommandContext(ctx, exe, args...)
}

func BuildImportCommand(exe string, modelFile string, modelID string) *exec.Cmd {
	return BuildImportCommandWithHuggingFaceToken(exe, modelFile, modelID, "")
}

func BuildImportCommandWithHuggingFaceToken(
	exe string,
	modelFile string,
	modelID string,
	huggingFaceToken string,
) *exec.Cmd {
	return BuildImportCommandContextWithHuggingFaceToken(
		context.Background(),
		exe,
		modelFile,
		modelID,
		huggingFaceToken,
	)
}

func BuildImportCommandContext(ctx context.Context, exe string, modelFile string, modelID string) *exec.Cmd {
	return BuildImportCommandContextWithHuggingFaceToken(ctx, exe, modelFile, modelID, "")
}

func BuildImportCommandContextWithHuggingFaceToken(
	ctx context.Context,
	exe string,
	modelFile string,
	modelID string,
	huggingFaceToken string,
) *exec.Cmd {
	cmd := exec.CommandContext(
		ctx,
		exe,
		"import",
		modelFile,
		modelID,
	)
	return withHuggingFaceTokenEnv(cmd, huggingFaceToken)
}

func BuildRunCommand(exe string, request RunRequest) *exec.Cmd {
	return BuildRunCommandContext(context.Background(), exe, request)
}

func BuildRunCommandContext(ctx context.Context, exe string, request RunRequest) *exec.Cmd {
	modelID := request.ModelID
	if modelID == "" {
		modelID = DefaultModelID
	}

	args := []string{"run", modelID}
	if isConcreteLiteRTBackend(request.Backend) {
		args = append(args, "--backend="+request.Backend)
	}
	if isConcreteAttachmentBackend(request.VisionBackend) {
		args = append(args, "--vision-backend="+request.VisionBackend)
	}
	if isConcreteAttachmentBackend(request.AudioBackend) {
		args = append(args, "--audio-backend="+request.AudioBackend)
	}
	if request.MaxNumTokens > 0 {
		args = append(args, "--max-num-tokens="+strconv.Itoa(request.MaxNumTokens))
	}
	if request.TopK > 0 {
		args = append(args, "--top-k="+strconv.Itoa(request.TopK))
	}
	if request.TopP != nil {
		args = append(args, "--top-p="+strconv.FormatFloat(*request.TopP, 'f', -1, 64))
	}
	if request.Temperature != nil {
		args = append(args, "--temperature="+strconv.FormatFloat(*request.Temperature, 'f', -1, 64))
	}
	if request.Seed > 0 {
		args = append(args, "--seed="+strconv.FormatInt(request.Seed, 10))
	}
	if request.Preset != "" {
		args = append(args, "--preset="+request.Preset)
	}
	if request.NoTemplate {
		args = append(args, "--no-template")
	}
	if request.FilterChannelContentFromKVCache {
		args = append(args, "--filter-channel-content-from-kv-cache")
	}
	if isSpeculativeDecodingValue(request.EnableSpeculativeDecoding) {
		args = append(args, "--enable-speculative-decoding="+request.EnableSpeculativeDecoding)
	}
	if isCacheValue(request.Cache) {
		args = append(args, "--cache="+request.Cache)
	}
	if request.Verbose {
		args = append(args, "--verbose")
	}
	if request.FromHuggingFaceRepo != "" {
		args = append(args, "--from-huggingface-repo="+request.FromHuggingFaceRepo)
	}
	for _, path := range request.AttachmentPaths {
		if path != "" {
			args = append(args, "--attachment="+path)
		}
	}
	if request.Prompt != "" {
		args = append(args, "--prompt="+request.Prompt)
	}

	return withHuggingFaceTokenEnv(exec.CommandContext(ctx, exe, args...), request.HuggingFaceToken)
}

func withHuggingFaceTokenEnv(cmd *exec.Cmd, huggingFaceToken string) *exec.Cmd {
	huggingFaceToken = strings.TrimSpace(huggingFaceToken)
	if huggingFaceToken == "" {
		return cmd
	}

	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	env = upsertEnv(env, "HF_TOKEN", huggingFaceToken)
	env = upsertEnv(env, "HUGGING_FACE_HUB_TOKEN", huggingFaceToken)
	cmd.Env = env
	return cmd
}

func upsertEnv(env []string, key string, value string) []string {
	prefix := key + "="
	for index, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[index] = prefix + value
			return env
		}
	}

	return append(env, prefix+value)
}

func RunOnce(ctx context.Context, exe string, request RunRequest) (string, error) {
	cmd := BuildRunCommandContext(ctx, exe, request)
	output, err := cmd.CombinedOutput()
	text := redactSensitiveText(strings.TrimSpace(string(output)), request.HuggingFaceToken)
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("run litert-lm prompt: %w: %s", err, text)
		}
		return "", fmt.Errorf("run litert-lm prompt: %w", err)
	}

	return text, nil
}

func redactSensitiveText(text string, secrets ...string) string {
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}

	return text
}

func DetectVersion(exe string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, exe, "--version").CombinedOutput()
	if err != nil {
		return "unknown"
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		return "unknown"
	}

	return version
}

func BuildUpstreamURL(host string, port int) string {
	if host == "" {
		host = DefaultRuntimeHost
	}
	if port == 0 {
		port = DefaultRuntimePort
	}

	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}).String()
}

func executableSearchPaths() []string {
	name := platformExecutableName()
	paths := []string{name}

	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths,
			filepath.Join(cwd, name),
			filepath.Join(cwd, "bin", name),
		)
	}

	if currentExe, err := os.Executable(); err == nil {
		dir := filepath.Dir(currentExe)
		paths = append(paths,
			filepath.Join(dir, name),
			filepath.Join(dir, "bin", name),
			filepath.Join(dir, "..", "bin", name),
		)
	}

	return paths
}

func defaultModelSearchPaths() []string {
	name := DefaultModelFileName
	dir := DefaultModelDirectory
	paths := []string{
		filepath.Join("models", dir, name),
		filepath.Join("..", "models", dir, name),
		filepath.Join("..", "..", "models", dir, name),
		filepath.Join("..", "..", "..", "models", dir, name),
		filepath.Join("models", name),
		filepath.Join("..", "models", name),
		filepath.Join("..", "..", "models", name),
		filepath.Join("..", "..", "..", "models", name),
	}

	if currentExe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(currentExe)
		paths = append(paths,
			filepath.Join(exeDir, "models", dir, name),
			filepath.Join(exeDir, "..", "models", dir, name),
			filepath.Join(exeDir, "..", "..", "models", dir, name),
			filepath.Join(exeDir, "..", "..", "..", "models", dir, name),
			filepath.Join(exeDir, "models", name),
			filepath.Join(exeDir, "..", "models", name),
			filepath.Join(exeDir, "..", "..", "models", name),
			filepath.Join(exeDir, "..", "..", "..", "models", name),
		)
	}

	return paths
}

func platformExecutableName() string {
	return platformExecutableNameFor(runtime.GOOS)
}

func platformExecutableNameFor(goos string) string {
	if goos == "windows" {
		return DefaultExecutableName + ".exe"
	}

	return DefaultExecutableName
}

func resolveExecutablePath(candidate string) (string, error) {
	if strings.ContainsRune(candidate, os.PathSeparator) || filepath.IsAbs(candidate) {
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() && isExecutable(stat.Mode()) {
			return filepath.Abs(candidate)
		}
		return "", os.ErrNotExist
	}

	return exec.LookPath(candidate)
}

func isExecutable(mode os.FileMode) bool {
	return isExecutableFor(runtime.GOOS, mode)
}

func isExecutableFor(goos string, mode os.FileMode) bool {
	if goos == "windows" {
		return true
	}

	return mode&0o111 != 0
}

func isConcreteLiteRTBackend(backend string) bool {
	return backend != "" && backend != "auto" && backend != "cuda"
}

func isConcreteAttachmentBackend(backend string) bool {
	return backend == "cpu" || backend == "gpu"
}

func isSpeculativeDecodingValue(value string) bool {
	return value == "auto" || value == "true" || value == "false"
}

func isCacheValue(value string) bool {
	return value == "disk" || value == "memory" || value == "no"
}

func isRuntimeMode(mode RuntimeMode) bool {
	return mode == RuntimeModeRelease || mode == RuntimeModeDebug
}
