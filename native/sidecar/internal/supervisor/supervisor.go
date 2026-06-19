package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"litert-sidecar/internal/litert"
)

const DefaultMainRunnerID = "main-litert"

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

type Config struct {
	DefaultLiteRT  LiteRTConfig
	Logs           LogSink
	StdoutTee      io.Writer
	StderrTee      io.Writer
	ImportModel    bool
	OnStatusChange func(Snapshot)
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
	HuggingFaceToken string
	Verbose          bool
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
	huggingFaceToken string
	verbose          bool
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
	supervisor.addDefaultLiteRTRunner(config.DefaultLiteRT)
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
		detail = "Sidecar is proxying an externally managed LiteRT-LM server."
	}

	upstream := configuredUpstream(config.Launch, config.Upstream, host, port)
	s.runners[DefaultMainRunnerID] = &runnerRecord{
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
			State:      state,
			Upstream:   upstream,
			Detail:     detail,
		},
		launch:           config.Launch,
		executable:       config.Executable,
		huggingFaceToken: strings.TrimSpace(config.HuggingFaceToken),
		verbose:          config.Verbose,
	}
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
			State:      initialState(normalized.Launch),
			Upstream:   configuredUpstream(normalized.Launch, normalized.Upstream, normalized.Host, normalized.Port),
			Detail:     initialDetail(normalized.Launch),
		},
		launch:           normalized.Launch,
		executable:       normalized.Executable,
		huggingFaceToken: strings.TrimSpace(normalized.HuggingFaceToken),
		verbose:          normalized.Verbose,
	}

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
		Mode:        runtimeMode(record.verbose),
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
			snapshot.Detail = "Sidecar is proxying an externally managed runner."
			snapshot.LastError = ""
		})
		return nil
	}

	switch record.snapshot.Runtime {
	case RuntimeLiteRT:
		return s.startLiteRTRunner(ctx, record)
	default:
		err := fmt.Errorf("runtime %q cannot be started yet", record.snapshot.Runtime)
		s.markUnavailable(record, err)
		return err
	}
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
			snapshot.Detail = "Sidecar is proxying an externally managed runner."
			snapshot.LastError = ""
		})
		return nil
	}
	if record.snapshot.Runtime != RuntimeLiteRT {
		err := fmt.Errorf("runtime %q cannot be started yet", record.snapshot.Runtime)
		s.markUnavailable(record, err)
		return err
	}
	return s.startLiteRTRunner(ctx, record)
}

func (s *Supervisor) startLiteRTRunner(ctx context.Context, record *runnerRecord) error {
	exe, err := findLiteRTExecutable(record.executable)
	if err != nil {
		s.markUnavailable(record, err)
		return err
	}

	snapshot := s.runnerSnapshot(record)
	modelFile := snapshot.ModelPath
	if modelFile == "" {
		modelFile = litert.FindDefaultModelFile()
	}
	version := litert.DetectVersion(exe)

	s.updateRecord(record, func(next *RunnerSnapshot) {
		next.State = StateStarting
		next.Executable = exe
		next.Version = version
		next.ModelPath = modelFile
		next.Upstream = litert.BuildUpstreamURL(snapshot.Host, snapshot.Port)
		next.Command = nil
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

	cmd := litert.BuildServeCommandContext(ctx, exe, snapshot.Host, snapshot.Port, record.verbose)
	cmd.Stdout = s.writer(record.snapshot.ID, "stdout", s.stdoutTee)
	cmd.Stderr = s.writer(record.snapshot.ID, "stderr", s.stderrTee)
	if err := cmd.Start(); err != nil {
		err := fmt.Errorf("start litert-lm serve: %w", err)
		s.markUnavailable(record, err)
		return err
	}

	done := make(chan error, 1)
	s.mu.Lock()
	record.cmd = cmd
	record.done = done
	record.stopped = false
	record.snapshot.State = StateRunning
	record.snapshot.PID = cmd.Process.Pid
	record.snapshot.Command = append([]string(nil), cmd.Args...)
	record.snapshot.Upstream = litert.BuildUpstreamURL(snapshot.Host, snapshot.Port)
	record.snapshot.Detail = "LiteRT-LM server process is running."
	record.snapshot.LastError = ""
	s.mu.Unlock()
	s.publishStatusChange()

	go s.wait(ctx, record.snapshot.ID, cmd, done)
	return nil
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
		return "Sidecar is proxying an externally managed runner."
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
	if verbose {
		return string(litert.RuntimeModeDebug)
	}
	return string(litert.RuntimeModeRelease)
}

func findLiteRTExecutable(configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return litert.FindExecutable(configured)
	}
	return litert.FindExecutable()
}
