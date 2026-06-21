package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"g0litellama/internal/litert"
)

const DefaultMainRunnerID = "main-litert"

const (
	runnerStartupTimeout      = 3 * time.Minute
	runnerStartupPollInterval = 250 * time.Millisecond
)

const DefaultLlamaExecutableName = "llama-server"

type Runtime string

const (
	RuntimeLiteRT   Runtime = "litert"
	RuntimeLlamaCPP Runtime = "llamacpp"
)

type Role string

const (
	RoleMain      Role = "main"
	RoleEmbedding Role = "embedding"
	RoleReranking Role = "reranking"
)

type Backend string

const (
	BackendCPU      Backend = "cpu"
	BackendGPU      Backend = "gpu"
	BackendNPU      Backend = "npu"
	BackendMetal    Backend = "metal"
	BackendVulkan   Backend = "vulkan"
	BackendCUDA     Backend = "cuda"
	BackendOpenVINO Backend = "openvino"
	BackendSYCL     Backend = "sycl"
)

type State string

const (
	StateCreated     State = "created"
	StateExternal    State = "external"
	StateStarting    State = "starting"
	StateRunning     State = "running"
	StateStopped     State = "stopped"
	StateExited      State = "exited"
	StateUnavailable State = "unavailable"
)

type RuntimeMode string

const (
	RuntimeModeRelease RuntimeMode = "release"
	RuntimeModeDebug   RuntimeMode = "debug"
)

type Config struct {
	DefaultLiteRT        LiteRTConfig
	DisableDefaultLiteRT bool
	Logs                 LogSink
	StdoutTee            io.Writer
	StderrTee            io.Writer
	ImportModel          bool
	OnStatusChange       func(Snapshot)
}

type LiteRTConfig struct {
	Launch           bool
	Executable       string
	Host             string
	Port             int
	Upstream         string
	ModelFile        string
	ModelID          string
	HuggingFaceToken string
	Verbose          bool
}

type LiteRTPatch struct {
	Launch           *bool
	Executable       string
	Host             string
	Port             int
	Upstream         string
	ModelPath        string
	ModelID          string
	HuggingFaceToken *string
	ImportModel      *bool
	Verbose          *bool
}

type DefaultLiteRTConfig struct {
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
}

type RunnerSpec struct {
	ID               string
	Runtime          Runtime
	Role             Role
	Backend          Backend
	Executable       string
	ModelPath        string
	ModelID          string
	Host             string
	Port             int
	Launch           bool
	Upstream         string
	Command          []string
	CommandLine      string
	HuggingFaceToken string
	Verbose          bool
}

type RunnerPatch struct {
	Runtime          Runtime
	Role             Role
	Backend          Backend
	Executable       string
	ModelPath        string
	ModelID          string
	Host             string
	Port             int
	Launch           *bool
	Upstream         string
	Command          []string
	CommandLine      *string
	HuggingFaceToken *string
	Verbose          *bool
}

type RunnerSnapshot struct {
	ID           string            `json:"id"`
	Runtime      Runtime           `json:"runtime"`
	Role         Role              `json:"role"`
	Backend      Backend           `json:"backend"`
	Executable   string            `json:"executable,omitempty"`
	Version      string            `json:"version,omitempty"`
	ModelPath    string            `json:"modelPath,omitempty"`
	ModelID      string            `json:"modelId,omitempty"`
	Host         string            `json:"host,omitempty"`
	Port         int               `json:"port,omitempty"`
	Launch       bool              `json:"launch"`
	Verbose      bool              `json:"verbose"`
	State        State             `json:"state"`
	PID          int               `json:"pid,omitempty"`
	Upstream     string            `json:"upstream,omitempty"`
	Command      []string          `json:"command,omitempty"`
	Capabilities map[string]string `json:"capabilities,omitempty"`
	LastError    string            `json:"lastError,omitempty"`
	LogSequence  uint64            `json:"logSequence,omitempty"`
	Detail       string            `json:"detail,omitempty"`
}

type Snapshot struct {
	Runners []RunnerSnapshot `json:"runners"`
	Routes  map[Role]string  `json:"routes"`
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

type LogSink interface {
	Writer(source string, stream string, tee io.Writer) io.Writer
	LatestSeq() uint64
}

type Supervisor struct {
	opMu sync.Mutex
	mu   sync.RWMutex

	runners        map[string]*runnerRecord
	routes         map[Role]string
	logs           LogSink
	stdoutTee      io.Writer
	stderrTee      io.Writer
	importModel    bool
	onStatusChange func(Snapshot)
	nextID         int
}

type runnerRecord struct {
	snapshot         RunnerSnapshot
	launch           bool
	executable       string
	commandOverride  []string
	huggingFaceToken string
	verbose          bool
	mode             RuntimeMode
	cmd              *exec.Cmd
	done             chan error
	stopped          bool
}

func New(config Config) *Supervisor {
	supervisor := &Supervisor{
		runners:        map[string]*runnerRecord{},
		routes:         map[Role]string{},
		logs:           config.Logs,
		stdoutTee:      config.StdoutTee,
		stderrTee:      config.StderrTee,
		importModel:    config.ImportModel,
		onStatusChange: config.OnStatusChange,
	}
	if !config.DisableDefaultLiteRT {
		supervisor.addDefaultLiteRTRunner(config.DefaultLiteRT)
	}
	return supervisor
}

func (s *Supervisor) addDefaultLiteRTRunner(config LiteRTConfig) {
	host := config.Host
	if host == "" {
		host = litert.DefaultRuntimeHost
	}
	port := config.Port
	if port == 0 {
		port = litert.DefaultRuntimePort
	}
	modelID := config.ModelID
	if modelID == "" {
		modelID = litert.DefaultModelID
	}

	state := StateCreated
	detail := "LiteRT-LM runtime has not been started yet."
	if !config.Launch {
		state = StateExternal
		detail = "G0LiteLLaMa is proxying an externally managed LiteRT-LM server."
	}

	upstream := configuredUpstream(config.Launch, config.Upstream, host, port)
	record := &runnerRecord{
		snapshot: RunnerSnapshot{
			ID:         DefaultMainRunnerID,
			Runtime:    RuntimeLiteRT,
			Role:       RoleMain,
			Backend:    BackendCPU,
			Executable: config.Executable,
			ModelPath:  config.ModelFile,
			ModelID:    modelID,
			Host:       host,
			Port:       port,
			Launch:     config.Launch,
			Verbose:    config.Verbose,
			State:      state,
			Upstream:   upstream,
			Detail:     detail,
		},
		launch:           config.Launch,
		executable:       config.Executable,
		huggingFaceToken: strings.TrimSpace(config.HuggingFaceToken),
		verbose:          config.Verbose,
		mode:             initialRuntimeMode(config.Verbose),
	}
	record.snapshot.Command = runnerCommandPreview(record)
	s.runners[DefaultMainRunnerID] = record
	s.routes[RoleMain] = DefaultMainRunnerID
}

func (s *Supervisor) CreateRunner(spec RunnerSpec) (string, error) {
	s.mu.Lock()
	normalized, err := normalizeRunnerSpec(spec, s.nextRunnerIDLocked(spec))
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	if _, exists := s.runners[normalized.ID]; exists {
		s.mu.Unlock()
		return "", fmt.Errorf("runner %q already exists", normalized.ID)
	}

	record := &runnerRecord{
		snapshot: RunnerSnapshot{
			ID:         normalized.ID,
			Runtime:    normalized.Runtime,
			Role:       normalized.Role,
			Backend:    normalized.Backend,
			Executable: normalized.Executable,
			ModelPath:  normalized.ModelPath,
			ModelID:    normalized.ModelID,
			Host:       normalized.Host,
			Port:       normalized.Port,
			Launch:     normalized.Launch,
			Verbose:    normalized.Verbose,
			State:      initialState(normalized.Launch),
			Upstream:   configuredUpstream(normalized.Launch, normalized.Upstream, normalized.Host, normalized.Port),
			Detail:     initialDetail(normalized.Launch),
		},
		launch:           normalized.Launch,
		executable:       normalized.Executable,
		commandOverride:  append([]string(nil), normalized.Command...),
		huggingFaceToken: strings.TrimSpace(normalized.HuggingFaceToken),
		verbose:          normalized.Verbose,
		mode:             initialRuntimeMode(normalized.Verbose),
	}
	record.snapshot.Command = runnerCommandPreview(record)

	s.runners[record.snapshot.ID] = record
	s.routes[record.snapshot.Role] = record.snapshot.ID
	s.mu.Unlock()
	s.publishStatusChange()
	return record.snapshot.ID, nil
}

func (s *Supervisor) Runner(id string) (RunnerSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.runners[id]
	if !ok {
		return RunnerSnapshot{}, false
	}

	return record.snapshot, true
}

func (s *Supervisor) UpdateRunner(id string, patch RunnerPatch) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.mu.Lock()
	record := s.runners[id]
	if record == nil {
		s.mu.Unlock()
		return fmt.Errorf("runner %q not found", id)
	}
	if record.cmd != nil || record.snapshot.State == StateRunning || record.snapshot.State == StateStarting {
		s.mu.Unlock()
		return fmt.Errorf("runner %q cannot be updated while %s", id, record.snapshot.State)
	}

	spec := recordSpec(record)
	applyRunnerPatch(&spec, patch)
	normalized, err := normalizeRunnerSpec(spec, spec.ID)
	if err != nil {
		s.mu.Unlock()
		return err
	}

	oldRole := record.snapshot.Role
	record.launch = normalized.Launch
	record.executable = normalized.Executable
	record.commandOverride = append([]string(nil), normalized.Command...)
	if patch.HuggingFaceToken != nil {
		record.huggingFaceToken = strings.TrimSpace(*patch.HuggingFaceToken)
	}
	if patch.Verbose != nil {
		record.verbose = *patch.Verbose
	}
	record.mode = initialRuntimeMode(record.verbose)
	record.snapshot.Runtime = normalized.Runtime
	record.snapshot.Role = normalized.Role
	record.snapshot.Backend = normalized.Backend
	record.snapshot.Executable = normalized.Executable
	record.snapshot.ModelPath = normalized.ModelPath
	record.snapshot.ModelID = normalized.ModelID
	record.snapshot.Host = normalized.Host
	record.snapshot.Port = normalized.Port
	record.snapshot.Launch = normalized.Launch
	record.snapshot.Verbose = record.verbose
	record.snapshot.PID = 0
	record.snapshot.Upstream = configuredUpstream(
		normalized.Launch,
		normalized.Upstream,
		normalized.Host,
		normalized.Port,
	)
	record.snapshot.State = initialState(normalized.Launch)
	record.snapshot.Detail = initialDetail(normalized.Launch)
	record.snapshot.LastError = ""
	record.snapshot.Command = runnerCommandPreview(record)
	if oldRole != normalized.Role {
		delete(s.routes, oldRole)
	}
	s.routes[normalized.Role] = record.snapshot.ID
	s.mu.Unlock()
	s.publishStatusChange()
	return nil
}

func (s *Supervisor) RouteRunner(role Role, id string) (RunnerSnapshot, error) {
	if !isRole(role) {
		return RunnerSnapshot{}, fmt.Errorf("invalid runner role %q", role)
	}

	s.mu.Lock()
	record := s.runners[id]
	if record == nil {
		s.mu.Unlock()
		return RunnerSnapshot{}, fmt.Errorf("runner %q not found", id)
	}
	if record.snapshot.Role != role {
		s.mu.Unlock()
		return RunnerSnapshot{}, fmt.Errorf("runner %q role %q does not match route %q", id, record.snapshot.Role, role)
	}
	s.routes[role] = id
	runner := record.snapshot
	s.mu.Unlock()
	s.publishStatusChange()
	return runner, nil
}

func (s *Supervisor) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshotLocked()
}

func (s *Supervisor) LegacyStatus() RuntimeStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record := s.runners[DefaultMainRunnerID]
	if record == nil {
		return RuntimeStatus{
			State:  string(StateUnavailable),
			Detail: "default LiteRT runner is not configured",
		}
	}

	snapshot := record.snapshot
	status := RuntimeStatus{
		State:       string(snapshot.State),
		Executable:  snapshot.Executable,
		Version:     snapshot.Version,
		ModelID:     snapshot.ModelID,
		ModelFile:   snapshot.ModelPath,
		Upstream:    snapshot.Upstream,
		Mode:        string(record.mode),
		LogSequence: s.latestLogSeq(),
		Detail:      snapshot.Detail,
	}
	return status
}

func (s *Supervisor) UpstreamForPath(path string) (string, bool) {
	role := roleForPath(path)

	s.mu.RLock()
	defer s.mu.RUnlock()

	runnerID := s.routes[role]
	record := s.runners[runnerID]
	if record == nil {
		return "", false
	}
	if !isRouteable(record.snapshot.State) || record.snapshot.Upstream == "" {
		return "", false
	}

	return record.snapshot.Upstream, true
}

func (s *Supervisor) StartRunner(ctx context.Context, id string) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	record, err := s.recordForUpdate(id)
	if err != nil {
		return err
	}
	if s.runnerState(record) == StateRunning {
		return nil
	}
	if !record.launch {
		s.updateRecord(record, func(snapshot *RunnerSnapshot) {
			snapshot.State = StateExternal
			snapshot.Upstream = configuredUpstream(false, snapshot.Upstream, snapshot.Host, snapshot.Port)
			snapshot.Detail = "G0LiteLLaMa is proxying an externally managed runner."
			snapshot.LastError = ""
		})
		return nil
	}

	return s.startRecord(ctx, record)
}

func (s *Supervisor) StopRunner(ctx context.Context, id string) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	record, err := s.recordForUpdate(id)
	if err != nil {
		return err
	}
	return s.stopRecord(ctx, record)
}

func (s *Supervisor) CloseRunner(ctx context.Context, id string) (RunnerSnapshot, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	record, err := s.recordForUpdate(id)
	if err != nil {
		return RunnerSnapshot{}, err
	}
	closed := s.runnerSnapshot(record)
	if err := s.stopRecord(ctx, record); err != nil {
		return RunnerSnapshot{}, err
	}
	if closed.State == StateRunning || closed.State == StateStarting {
		closed.State = StateStopped
		closed.Detail = "Runner process was stopped and closed."
		closed.LastError = ""
	}

	s.mu.Lock()
	current := s.runners[id]
	if current == nil {
		s.mu.Unlock()
		return RunnerSnapshot{}, fmt.Errorf("runner %q not found", id)
	}
	if routedID := s.routes[current.snapshot.Role]; routedID == id {
		delete(s.routes, current.snapshot.Role)
	}
	delete(s.runners, id)
	s.mu.Unlock()
	s.publishStatusChange()
	return closed, nil
}

func (s *Supervisor) RestartRunner(ctx context.Context, id string) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	record, err := s.recordForUpdate(id)
	if err != nil {
		return err
	}
	if err := s.stopRecord(ctx, record); err != nil {
		return err
	}
	if !record.launch {
		s.updateRecord(record, func(snapshot *RunnerSnapshot) {
			snapshot.State = StateExternal
			snapshot.Detail = "G0LiteLLaMa is proxying an externally managed runner."
			snapshot.LastError = ""
		})
		return nil
	}
	return s.startRecord(ctx, record)
}

func (s *Supervisor) StartDefaultLiteRT(
	ctx context.Context,
	mode RuntimeMode,
	patch LiteRTPatch,
) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	record, err := s.recordForUpdate(DefaultMainRunnerID)
	if err != nil {
		return err
	}
	if err := s.applyDefaultLiteRTPatch(record, mode, patch); err != nil {
		return err
	}
	if !record.launch {
		s.updateRecord(record, func(snapshot *RunnerSnapshot) {
			snapshot.State = StateExternal
			snapshot.Detail = "G0LiteLLaMa is proxying an externally managed runner."
			snapshot.LastError = ""
		})
		return nil
	}
	return s.startRecord(ctx, record)
}

func (s *Supervisor) RestartDefaultLiteRT(
	ctx context.Context,
	mode RuntimeMode,
	patch LiteRTPatch,
) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	record, err := s.recordForUpdate(DefaultMainRunnerID)
	if err != nil {
		return err
	}
	if err := s.stopRecord(ctx, record); err != nil {
		return err
	}
	if err := s.applyDefaultLiteRTPatch(record, mode, patch); err != nil {
		return err
	}
	if !record.launch {
		s.updateRecord(record, func(snapshot *RunnerSnapshot) {
			snapshot.State = StateExternal
			snapshot.Detail = "G0LiteLLaMa is proxying an externally managed runner."
			snapshot.LastError = ""
		})
		return nil
	}
	return s.startRecord(ctx, record)
}

func (s *Supervisor) DefaultLiteRTConfig() DefaultLiteRTConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record := s.runners[DefaultMainRunnerID]
	if record == nil {
		return DefaultLiteRTConfig{}
	}

	return DefaultLiteRTConfig{
		Launch:           record.launch,
		Executable:       record.executable,
		Host:             record.snapshot.Host,
		Port:             record.snapshot.Port,
		Upstream:         record.snapshot.Upstream,
		ModelFile:        record.snapshot.ModelPath,
		ModelID:          record.snapshot.ModelID,
		HuggingFaceToken: record.huggingFaceToken,
		ImportModel:      s.importModel,
		Verbose:          record.verbose,
	}
}

func (s *Supervisor) startRecord(ctx context.Context, record *runnerRecord) error {
	switch record.snapshot.Runtime {
	case RuntimeLiteRT:
		return s.startLiteRTRunner(ctx, record)
	case RuntimeLlamaCPP:
		return s.startLlamaRunner(ctx, record)
	default:
		err := fmt.Errorf("runtime %q cannot be started yet", record.snapshot.Runtime)
		s.markUnavailable(record, err)
		return err
	}
}

func (s *Supervisor) startLlamaRunner(ctx context.Context, record *runnerRecord) error {
	snapshot := s.runnerSnapshot(record)
	command := append([]string(nil), record.commandOverride...)
	if len(command) == 0 {
		exe, err := findExecutable(record.executable, DefaultLlamaExecutableName)
		if err != nil {
			s.markUnavailable(record, err)
			return err
		}
		if strings.TrimSpace(snapshot.ModelPath) == "" {
			err := errors.New("llama.cpp model path is required")
			s.markUnavailable(record, err)
			return err
		}
		if stat, err := os.Stat(snapshot.ModelPath); err != nil || stat.IsDir() {
			if err == nil {
				err = fmt.Errorf("%s is a directory", snapshot.ModelPath)
			}
			err = fmt.Errorf("llama.cpp model file %q is not usable: %w", snapshot.ModelPath, err)
			s.markUnavailable(record, err)
			return err
		}
		command = buildDefaultRunnerCommand(record, exe)
	}

	exe := command[0]
	version := detectExecutableVersion(exe)
	s.updateRecord(record, func(next *RunnerSnapshot) {
		next.State = StateStarting
		next.Executable = exe
		next.Version = version
		next.Upstream = litert.BuildUpstreamURL(snapshot.Host, snapshot.Port)
		next.Command = append([]string(nil), command...)
		next.Detail = "Starting llama.cpp OpenAI-compatible server."
		next.LastError = ""
	})

	processCtx := context.WithoutCancel(ctx)
	cmd, err := commandContext(processCtx, command)
	if err != nil {
		s.markUnavailable(record, err)
		return err
	}
	cmd.Stdout = s.writer(record.snapshot.ID, "stdout", s.stdoutTee)
	cmd.Stderr = s.writer(record.snapshot.ID, "stderr", s.stderrTee)
	if err := cmd.Start(); err != nil {
		err := fmt.Errorf("start llama-server: %w", err)
		s.markUnavailable(record, err)
		return err
	}

	done := make(chan error, 1)
	upstream := litert.BuildUpstreamURL(snapshot.Host, snapshot.Port)
	s.mu.Lock()
	record.cmd = cmd
	record.done = done
	record.stopped = false
	record.snapshot.State = StateStarting
	record.snapshot.PID = cmd.Process.Pid
	record.snapshot.Command = append([]string(nil), cmd.Args...)
	record.snapshot.Upstream = upstream
	record.snapshot.Detail = "Waiting for llama.cpp OpenAI-compatible server."
	record.snapshot.LastError = ""
	s.mu.Unlock()
	s.publishStatusChange()

	go s.wait(processCtx, record.snapshot.ID, cmd, done)
	if err := s.waitForRunnerReady(ctx, upstream, done); err != nil {
		s.handleStartupFailure(record, done, err)
		return err
	}
	s.updateRecord(record, func(next *RunnerSnapshot) {
		next.State = StateRunning
		next.Detail = "llama.cpp server process is serving."
		next.LastError = ""
	})
	return nil
}

func (s *Supervisor) startLiteRTRunner(ctx context.Context, record *runnerRecord) error {
	snapshot := s.runnerSnapshot(record)
	modelFile := snapshot.ModelPath
	if modelFile == "" {
		modelFile = litert.FindDefaultModelFile()
	}
	command := append([]string(nil), record.commandOverride...)
	if len(command) == 0 {
		exe, err := findLiteRTExecutable(record.executable)
		if err != nil {
			s.markUnavailable(record, err)
			return err
		}
		command = buildDefaultRunnerCommand(record, exe)
	}
	exe := command[0]
	version := litert.DetectVersion(exe)

	s.updateRecord(record, func(next *RunnerSnapshot) {
		next.State = StateStarting
		next.Executable = exe
		next.Version = version
		next.ModelPath = modelFile
		next.Upstream = litert.BuildUpstreamURL(snapshot.Host, snapshot.Port)
		next.Command = append([]string(nil), command...)
		next.Detail = "Starting LiteRT-LM OpenAI-compatible server."
		next.LastError = ""
	})

	if s.importModel {
		err := litert.EnsureModelImportedWithHuggingFaceToken(
			ctx,
			exe,
			modelFile,
			snapshot.ModelID,
			record.huggingFaceToken,
		)
		if err != nil {
			s.markUnavailable(record, err)
			return err
		}
	}

	serveVerbose := record.mode == RuntimeModeDebug || record.verbose
	processCtx := context.WithoutCancel(ctx)
	if len(record.commandOverride) == 0 {
		command = litert.BuildServeCommandContext(
			processCtx,
			exe,
			snapshot.Host,
			snapshot.Port,
			serveVerbose,
		).Args
	}
	cmd, err := commandContext(processCtx, command)
	if err != nil {
		s.markUnavailable(record, err)
		return err
	}
	cmd.Stdout = s.writer(record.snapshot.ID, "stdout", s.stdoutTee)
	cmd.Stderr = s.writer(record.snapshot.ID, "stderr", s.stderrTee)
	if err := cmd.Start(); err != nil {
		err := fmt.Errorf("start litert-lm serve: %w", err)
		s.markUnavailable(record, err)
		return err
	}

	done := make(chan error, 1)
	upstream := litert.BuildUpstreamURL(snapshot.Host, snapshot.Port)
	s.mu.Lock()
	record.cmd = cmd
	record.done = done
	record.stopped = false
	record.snapshot.State = StateStarting
	record.snapshot.PID = cmd.Process.Pid
	record.snapshot.Command = append([]string(nil), cmd.Args...)
	record.snapshot.Upstream = upstream
	record.snapshot.Detail = "Waiting for LiteRT-LM OpenAI-compatible server."
	record.snapshot.LastError = ""
	s.mu.Unlock()
	s.publishStatusChange()

	go s.wait(processCtx, record.snapshot.ID, cmd, done)
	if err := s.waitForRunnerReady(ctx, upstream, done); err != nil {
		s.handleStartupFailure(record, done, err)
		return err
	}
	s.updateRecord(record, func(next *RunnerSnapshot) {
		next.State = StateRunning
		next.Detail = "LiteRT-LM server process is serving."
		next.LastError = ""
	})
	return nil
}

func (s *Supervisor) waitForRunnerReady(ctx context.Context, upstream string, done chan error) error {
	if upstream == "" {
		return errors.New("runner upstream is empty")
	}

	startupCtx, cancel := context.WithTimeout(ctx, runnerStartupTimeout)
	defer cancel()

	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(runnerStartupPollInterval)
	defer ticker.Stop()

	var lastProbeErr error
	for {
		if err, exited := preserveDoneError(done); exited {
			if err != nil {
				return fmt.Errorf("runner process exited before serving: %w", err)
			}
			return errors.New("runner process exited before serving")
		}

		if err := probeOpenAIModels(startupCtx, client, upstream); err == nil {
			return nil
		} else {
			lastProbeErr = err
		}

		select {
		case <-startupCtx.Done():
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if lastProbeErr != nil {
				return fmt.Errorf("runner did not serve %s before startup timeout: %w", upstream, lastProbeErr)
			}
			return fmt.Errorf("runner did not serve %s before startup timeout", upstream)
		case <-ticker.C:
		}
	}
}

func probeOpenAIModels(ctx context.Context, client *http.Client, upstream string) error {
	endpoint := strings.TrimRight(upstream, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s returned %s", endpoint, resp.Status)
	}
	return nil
}

func preserveDoneError(done chan error) (error, bool) {
	select {
	case err := <-done:
		done <- err
		return err, true
	default:
		return nil, false
	}
}

func (s *Supervisor) handleStartupFailure(record *runnerRecord, done chan error, startupErr error) {
	if _, exited := preserveDoneError(done); exited {
		return
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.stopRecord(stopCtx, record); err != nil {
		startupErr = fmt.Errorf("%w; stop runner: %v", startupErr, err)
	}
	s.markUnavailable(record, startupErr)
}

func (s *Supervisor) stopRecord(ctx context.Context, record *runnerRecord) error {
	s.mu.Lock()
	cmd := record.cmd
	done := record.done
	if cmd == nil || cmd.Process == nil || record.stopped {
		if record.snapshot.State == StateRunning || record.snapshot.State == StateStarting {
			record.snapshot.State = StateStopped
			record.snapshot.Detail = "Runner process was stopped."
		}
		s.mu.Unlock()
		s.publishStatusChange()
		return nil
	}
	record.stopped = true
	s.mu.Unlock()

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

func (s *Supervisor) wait(ctx context.Context, id string, cmd *exec.Cmd, done chan<- error) {
	err := cmd.Wait()
	defer func() {
		done <- err
	}()

	s.mu.Lock()
	record := s.runners[id]
	if record == nil || record.cmd != cmd {
		s.mu.Unlock()
		return
	}

	record.cmd = nil
	record.done = nil
	record.snapshot.PID = 0
	if record.stopped || errors.Is(ctx.Err(), context.Canceled) {
		record.snapshot.State = StateStopped
		record.snapshot.Detail = "Runner process was stopped."
	} else if err != nil {
		record.snapshot.State = StateExited
		record.snapshot.Detail = fmt.Sprintf("Runner process exited: %v", err)
		record.snapshot.LastError = err.Error()
	} else {
		record.snapshot.State = StateExited
		record.snapshot.Detail = "Runner process exited cleanly."
	}
	s.mu.Unlock()
	s.publishStatusChange()
}

func (s *Supervisor) recordForUpdate(id string) (*runnerRecord, error) {
	s.mu.RLock()
	record := s.runners[id]
	s.mu.RUnlock()
	if record == nil {
		return nil, fmt.Errorf("runner %q not found", id)
	}
	return record, nil
}

func (s *Supervisor) runnerState(record *runnerRecord) State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return record.snapshot.State
}

func (s *Supervisor) runnerSnapshot(record *runnerRecord) RunnerSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return record.snapshot
}

func (s *Supervisor) updateRecord(record *runnerRecord, update func(*RunnerSnapshot)) {
	s.mu.Lock()
	update(&record.snapshot)
	s.mu.Unlock()
	s.publishStatusChange()
}

func (s *Supervisor) markUnavailable(record *runnerRecord, err error) {
	s.updateRecord(record, func(snapshot *RunnerSnapshot) {
		snapshot.State = StateUnavailable
		snapshot.LastError = err.Error()
		snapshot.Detail = err.Error()
	})
}

func (s *Supervisor) applyDefaultLiteRTPatch(
	record *runnerRecord,
	mode RuntimeMode,
	patch LiteRTPatch,
) error {
	if !isRuntimeMode(mode) {
		return fmt.Errorf("runtime mode must be %q or %q", RuntimeModeRelease, RuntimeModeDebug)
	}
	if patch.Port < 0 {
		return errors.New("runtime port must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record.mode = mode
	if patch.Launch != nil {
		record.launch = *patch.Launch
		record.snapshot.Launch = record.launch
	}
	if patch.Executable != "" {
		record.executable = patch.Executable
		record.snapshot.Executable = patch.Executable
	}
	if patch.Host != "" {
		record.snapshot.Host = patch.Host
	}
	if patch.Port > 0 {
		record.snapshot.Port = patch.Port
	}
	if patch.ModelPath != "" {
		record.snapshot.ModelPath = patch.ModelPath
	}
	if patch.ModelID != "" {
		record.snapshot.ModelID = patch.ModelID
	}
	if patch.HuggingFaceToken != nil {
		record.huggingFaceToken = strings.TrimSpace(*patch.HuggingFaceToken)
	}
	if patch.ImportModel != nil {
		s.importModel = *patch.ImportModel
	}
	if patch.Verbose != nil {
		record.verbose = *patch.Verbose
		record.snapshot.Verbose = record.verbose
	}

	upstream := patch.Upstream
	if upstream == "" {
		upstream = record.snapshot.Upstream
	}
	record.snapshot.Upstream = configuredUpstream(record.launch, upstream, record.snapshot.Host, record.snapshot.Port)
	return nil
}

func (s *Supervisor) writer(id string, stream string, tee io.Writer) io.Writer {
	if s.logs == nil {
		return tee
	}
	return s.logs.Writer("runner:"+id, stream, tee)
}

func (s *Supervisor) publishStatusChange() {
	if s.onStatusChange == nil {
		return
	}
	s.onStatusChange(s.Snapshot())
}

func (s *Supervisor) snapshotLocked() Snapshot {
	runners := make([]RunnerSnapshot, 0, len(s.runners))
	for _, record := range s.runners {
		snapshot := record.snapshot
		if s.logs != nil && snapshot.LogSequence == 0 {
			snapshot.LogSequence = s.logs.LatestSeq()
		}
		runners = append(runners, snapshot)
	}
	sort.Slice(runners, func(i int, j int) bool {
		return runners[i].ID < runners[j].ID
	})

	routes := make(map[Role]string, len(s.routes))
	for role, id := range s.routes {
		routes[role] = id
	}

	return Snapshot{
		Runners: runners,
		Routes:  routes,
	}
}

func recordSpec(record *runnerRecord) RunnerSpec {
	return RunnerSpec{
		ID:               record.snapshot.ID,
		Runtime:          record.snapshot.Runtime,
		Role:             record.snapshot.Role,
		Backend:          record.snapshot.Backend,
		Executable:       record.executable,
		ModelPath:        record.snapshot.ModelPath,
		ModelID:          record.snapshot.ModelID,
		Host:             record.snapshot.Host,
		Port:             record.snapshot.Port,
		Launch:           record.launch,
		Upstream:         record.snapshot.Upstream,
		Command:          append([]string(nil), record.commandOverride...),
		HuggingFaceToken: record.huggingFaceToken,
		Verbose:          record.verbose,
	}
}

func applyRunnerPatch(spec *RunnerSpec, patch RunnerPatch) {
	if patch.Runtime != "" {
		spec.Runtime = patch.Runtime
	}
	if patch.Role != "" {
		spec.Role = patch.Role
	}
	if patch.Backend != "" {
		spec.Backend = patch.Backend
	}
	if patch.Executable != "" {
		spec.Executable = patch.Executable
	}
	if patch.ModelPath != "" {
		spec.ModelPath = patch.ModelPath
	}
	if patch.ModelID != "" {
		spec.ModelID = patch.ModelID
	}
	if patch.Host != "" {
		spec.Host = patch.Host
	}
	if patch.Port > 0 {
		spec.Port = patch.Port
	}
	if patch.Launch != nil {
		spec.Launch = *patch.Launch
	}
	if patch.Upstream != "" {
		spec.Upstream = patch.Upstream
	}
	if patch.CommandLine != nil {
		spec.Command = nil
		spec.CommandLine = *patch.CommandLine
	} else if patch.Command != nil {
		spec.Command = append([]string(nil), patch.Command...)
		spec.CommandLine = ""
	}
	if patch.HuggingFaceToken != nil {
		spec.HuggingFaceToken = strings.TrimSpace(*patch.HuggingFaceToken)
	}
	if patch.Verbose != nil {
		spec.Verbose = *patch.Verbose
	}
}

func (s *Supervisor) latestLogSeq() uint64 {
	if s.logs == nil {
		return 0
	}
	return s.logs.LatestSeq()
}

func (s *Supervisor) nextRunnerIDLocked(spec RunnerSpec) string {
	if strings.TrimSpace(spec.ID) != "" {
		return strings.TrimSpace(spec.ID)
	}
	s.nextID++
	role := spec.Role
	if role == "" {
		role = RoleMain
	}
	runtimeName := spec.Runtime
	if runtimeName == "" {
		runtimeName = RuntimeLiteRT
	}
	return string(role) + "-" + string(runtimeName) + "-" + strconv.Itoa(s.nextID)
}

func normalizeRunnerSpec(spec RunnerSpec, fallbackID string) (RunnerSpec, error) {
	spec.ID = strings.TrimSpace(fallbackID)
	if spec.ID == "" {
		return RunnerSpec{}, errors.New("runner id is required")
	}
	if spec.Runtime == "" {
		spec.Runtime = RuntimeLiteRT
	}
	if !isRuntime(spec.Runtime) {
		return RunnerSpec{}, fmt.Errorf("unsupported runtime %q", spec.Runtime)
	}
	if spec.Role == "" {
		spec.Role = RoleMain
	}
	if !isRole(spec.Role) {
		return RunnerSpec{}, fmt.Errorf("unsupported runner role %q", spec.Role)
	}
	if spec.Backend == "" {
		spec.Backend = BackendCPU
	}
	if spec.Host == "" {
		spec.Host = litert.DefaultRuntimeHost
	}
	if spec.Port == 0 {
		return RunnerSpec{}, errors.New("runner port is required")
	}
	command, err := normalizedCommandOverride(spec.Command, spec.CommandLine)
	if err != nil {
		return RunnerSpec{}, err
	}
	spec.Command = command
	spec.CommandLine = ""
	return spec, nil
}

func initialState(launch bool) State {
	if !launch {
		return StateExternal
	}
	return StateCreated
}

func initialDetail(launch bool) string {
	if !launch {
		return "G0LiteLLaMa is proxying an externally managed runner."
	}
	return "Runner has not been started yet."
}

func configuredUpstream(launch bool, upstream string, host string, port int) string {
	if !launch && strings.TrimSpace(upstream) != "" {
		return strings.TrimSpace(upstream)
	}
	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}).String()
}

func roleForPath(path string) Role {
	switch {
	case path == "/v1/embeddings" || strings.HasPrefix(path, "/v1/embeddings/"):
		return RoleEmbedding
	case path == "/v1/rerank" || strings.HasPrefix(path, "/v1/rerank/"):
		return RoleReranking
	default:
		return RoleMain
	}
}

func isRouteable(state State) bool {
	return state == StateRunning || state == StateExternal
}

func isRuntime(value Runtime) bool {
	return value == RuntimeLiteRT || value == RuntimeLlamaCPP
}

func isRole(value Role) bool {
	return value == RoleMain || value == RoleEmbedding || value == RoleReranking
}

func runtimeMode(verbose bool) string {
	return string(initialRuntimeMode(verbose))
}

func initialRuntimeMode(verbose bool) RuntimeMode {
	if verbose {
		return RuntimeModeDebug
	}
	return RuntimeModeRelease
}

func isRuntimeMode(mode RuntimeMode) bool {
	return mode == RuntimeModeRelease || mode == RuntimeModeDebug
}

func findLiteRTExecutable(configured string) (string, error) {
	return findExecutable(configured, litert.DefaultExecutableName)
}

func findExecutable(configured string, defaultName string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return resolveExecutablePath(configured)
	}
	return resolveExecutablePath(defaultName)
}

func runnerCommandPreview(record *runnerRecord) []string {
	if len(record.commandOverride) > 0 {
		return append([]string(nil), record.commandOverride...)
	}

	switch record.snapshot.Runtime {
	case RuntimeLiteRT:
		return buildDefaultRunnerCommand(record, runnerExecutable(record, litert.DefaultExecutableName))
	case RuntimeLlamaCPP:
		return buildDefaultRunnerCommand(record, runnerExecutable(record, DefaultLlamaExecutableName))
	default:
		return nil
	}
}

func runnerExecutable(record *runnerRecord, defaultName string) string {
	if strings.TrimSpace(record.executable) != "" {
		return strings.TrimSpace(record.executable)
	}
	if strings.TrimSpace(record.snapshot.Executable) != "" {
		return strings.TrimSpace(record.snapshot.Executable)
	}
	return defaultName
}

func buildDefaultRunnerCommand(record *runnerRecord, exe string) []string {
	snapshot := record.snapshot
	switch snapshot.Runtime {
	case RuntimeLiteRT:
		serveVerbose := record.mode == RuntimeModeDebug || record.verbose
		cmd := litert.BuildServeCommandContext(
			context.Background(),
			exe,
			snapshot.Host,
			snapshot.Port,
			serveVerbose,
		)
		return append([]string(nil), cmd.Args...)
	case RuntimeLlamaCPP:
		cmd := buildLlamaServerCommand(context.Background(), exe, snapshot)
		return append([]string(nil), cmd.Args...)
	default:
		return nil
	}
}

func commandContext(ctx context.Context, command []string) (*exec.Cmd, error) {
	if len(command) == 0 {
		return nil, errors.New("runner command is empty")
	}
	return exec.CommandContext(ctx, command[0], command[1:]...), nil
}

func normalizedCommandOverride(command []string, commandLine string) ([]string, error) {
	if strings.TrimSpace(commandLine) != "" {
		parsed, err := splitCommandLine(commandLine)
		if err != nil {
			return nil, err
		}
		return normalizedCommandArgs(parsed), nil
	}
	return normalizedCommandArgs(command), nil
}

func normalizedCommandArgs(command []string) []string {
	normalized := make([]string, 0, len(command))
	for _, arg := range command {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		normalized = append(normalized, arg)
	}
	return normalized
}

func splitCommandLine(line string) ([]string, error) {
	var args []string
	var builder strings.Builder
	var inSingle bool
	var inDouble bool
	var escaped bool
	var hasToken bool

	flush := func() {
		if !hasToken {
			return
		}
		args = append(args, builder.String())
		builder.Reset()
		hasToken = false
	}

	for _, char := range line {
		switch {
		case escaped:
			builder.WriteRune(char)
			hasToken = true
			escaped = false
		case char == '\\' && !inSingle:
			escaped = true
			hasToken = true
		case char == '\'' && !inDouble:
			inSingle = !inSingle
			hasToken = true
		case char == '"' && !inSingle:
			inDouble = !inDouble
			hasToken = true
		case (char == ' ' || char == '\t' || char == '\n' || char == '\r') && !inSingle && !inDouble:
			flush()
		default:
			builder.WriteRune(char)
			hasToken = true
		}
	}
	if escaped {
		return nil, errors.New("runner command has unfinished escape")
	}
	if inSingle || inDouble {
		return nil, errors.New("runner command has unclosed quote")
	}
	flush()
	return args, nil
}

func buildLlamaServerCommand(ctx context.Context, exe string, snapshot RunnerSnapshot) *exec.Cmd {
	modelID := snapshot.ModelID
	if modelID == "" {
		modelID = snapshot.ID
	}

	args := []string{
		"-m",
		snapshot.ModelPath,
		"--alias",
		modelID,
		"--host",
		snapshot.Host,
		"--port",
		strconv.Itoa(snapshot.Port),
	}
	if usesGPUBackend(snapshot.Backend) {
		args = append(args, "--n-gpu-layers", "999")
	}
	switch snapshot.Role {
	case RoleEmbedding:
		args = append(args, "--embedding")
	case RoleReranking:
		args = append(args, "--embedding", "--pooling", "rank", "--reranking")
	}

	return exec.CommandContext(ctx, exe, args...)
}

func usesGPUBackend(backend Backend) bool {
	switch backend {
	case BackendGPU, BackendMetal, BackendVulkan, BackendCUDA, BackendOpenVINO, BackendSYCL, BackendNPU:
		return true
	default:
		return false
	}
}

func detectExecutableVersion(exe string) string {
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
	if runtime.GOOS == "windows" {
		return true
	}

	return mode&0o111 != 0
}
