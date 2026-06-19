package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"litert-sidecar/internal/catalog"
	"litert-sidecar/internal/server"
)

func TestModelRendersRequiredTabs(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	view := model.View()

	for _, label := range []string{
		"Dashboard",
		"main-litert",
		"embed-qwen",
		"Models",
		"Logs",
		"Settings",
	} {
		if !strings.Contains(view, label) {
			t.Fatalf("view missing tab %q:\n%s", label, view)
		}
	}
	if !strings.Contains(view, "LiteRT sidecar") {
		t.Fatalf("dashboard view missing sidecar title:\n%s", view)
	}
}

func TestDashboardRendersSpecsAndRunnableBackends(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	view := model.View()

	for _, expected := range []string{
		"Specs",
		"Executable",
		"litert-lm 0.13.1",
		"Model",
		"gemma4-e2b",
		"Routes",
		"main -> main-litert",
		"Runnable backends",
		"cpu",
		"gpu",
		"embedding",
		"llamacpp",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("dashboard missing %q:\n%s", expected, view)
		}
	}
}

func TestModelSwitchesTabsWithKeys(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := next.(Model)

	if updated.activeTabID() != "runner:main-litert" {
		t.Fatalf("active tab = %q, want runner:main-litert", updated.activeTabID())
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	updated = next.(Model)
	if updated.activeTabID() != "models" {
		t.Fatalf("active tab = %q, want models", updated.activeTabID())
	}
}

func TestRunnerTabShowsFullSettingsDetailsAndControls(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := next.(Model)
	view := updated.View()

	for _, expected := range []string{
		"Runner main-litert",
		"Controls",
		"Start",
		"Stop",
		"Restart",
		"Edit settings",
		"b Backend",
		"Settings",
		"Runtime",
		"Role",
		"Backend",
		"Executable",
		"Model path",
		"Model ID",
		"Host",
		"Port",
		"Launch",
		"Upstream",
		"Details",
		"PID",
		"Command",
		"Capabilities",
		"Last error",
		"Log sequence",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("runner tab missing %q:\n%s", expected, view)
		}
	}
}

func TestRunnerTabUpdatesBackendThroughSharedRunnerController(t *testing.T) {
	t.Parallel()

	runners := testRunnerController()
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  runners,
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	updated := next.(Model)

	nextModel, cmd := updated.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("b"),
	})
	if cmd == nil {
		t.Fatalf("backend edit returned no command")
	}

	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if got := runners.lastCall(); got != "update:embed-qwen:backend=cpu" {
		t.Fatalf("last call = %q, want backend update", got)
	}
	view := updated.View()
	if !strings.Contains(view, "updated embed-qwen backend cpu") {
		t.Fatalf("view missing update notice:\n%s", view)
	}
	if !strings.Contains(view, "Backend:       cpu") {
		t.Fatalf("view missing updated backend:\n%s", view)
	}
}

func TestRunnerControlsUseSharedRunnerController(t *testing.T) {
	t.Parallel()

	runners := testRunnerController()
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  runners,
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := next.(Model)

	for _, tc := range []struct {
		key       string
		wantCall  string
		wantToast string
	}{
		{key: "s", wantCall: "start:main-litert", wantToast: "started main-litert"},
		{key: "x", wantCall: "stop:main-litert", wantToast: "stopped main-litert"},
		{key: "r", wantCall: "restart:main-litert", wantToast: "restarted main-litert"},
	} {
		nextModel, cmd := updated.Update(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune(tc.key),
		})
		if cmd == nil {
			t.Fatalf("key %q returned no command", tc.key)
		}

		message := cmd()
		afterAction, _ := nextModel.(Model).Update(message)
		updated = afterAction.(Model)

		if got := runners.lastCall(); got != tc.wantCall {
			t.Fatalf("last call = %q, want %q", got, tc.wantCall)
		}
		if !strings.Contains(updated.View(), tc.wantToast) {
			t.Fatalf("view missing action result %q:\n%s", tc.wantToast, updated.View())
		}
	}
}

func TestModelLogsViewShowsRecentEntries(t *testing.T) {
	t.Parallel()

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runner:main-litert", "stdout", "runtime ready")
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              logs,
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	updated := next.(Model)
	view := updated.View()

	if !strings.Contains(view, "runtime ready") {
		t.Fatalf("logs view missing log entry:\n%s", view)
	}
}

func TestModelModelsViewShowsCatalogEntries(t *testing.T) {
	t.Parallel()

	modelCatalog := catalog.NewDefault(t.TempDir())
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           modelCatalog,
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	updated := next.(Model)
	view := updated.View()

	if !strings.Contains(view, "gemma4-gguf") {
		t.Fatalf("models view missing catalog entry:\n%s", view)
	}
	if !strings.Contains(view, "Create runners") {
		t.Fatalf("models view missing create controls:\n%s", view)
	}
}

func TestModelsTabCreatesRunnersFromCatalogThroughSharedController(t *testing.T) {
	t.Parallel()

	runners := testRunnerController()
	modelCatalog := catalog.NewDefault(t.TempDir())
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  runners,
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           modelCatalog,
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	updated := next.(Model)

	for _, tc := range []struct {
		key       string
		wantCall  string
		wantToast string
		wantTab   string
	}{
		{
			key:       "m",
			wantCall:  "create:main-llamacpp:llamacpp:main:gemma4-gguf",
			wantToast: "created runner main-llamacpp",
			wantTab:   "main-llamacpp",
		},
		{
			key:       "e",
			wantCall:  "create:embedding-llamacpp:llamacpp:embedding:qwen3-embedding",
			wantToast: "created runner embedding-llamacpp",
			wantTab:   "embedding-llamacpp",
		},
		{
			key:       "r",
			wantCall:  "create:rerank-llamacpp:llamacpp:reranking:qwen3-rerank-probe",
			wantToast: "created runner rerank-llamacpp",
			wantTab:   "rerank-llamacpp",
		},
	} {
		nextModel, cmd := updated.Update(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune(tc.key),
		})
		if cmd == nil {
			t.Fatalf("key %q returned no command", tc.key)
		}

		message := cmd()
		afterAction, _ := nextModel.(Model).Update(message)
		updated = afterAction.(Model)

		if got := runners.lastCall(); got != tc.wantCall {
			t.Fatalf("last call = %q, want %q", got, tc.wantCall)
		}
		view := updated.View()
		if !strings.Contains(view, tc.wantToast) {
			t.Fatalf("view missing action result %q:\n%s", tc.wantToast, view)
		}
		if !strings.Contains(view, tc.wantTab) {
			t.Fatalf("view missing created runner tab %q:\n%s", tc.wantTab, view)
		}
	}
}

func TestSettingsViewShowsWebSocketAPIParity(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	updated := next.(Model)
	view := updated.View()

	for _, expected := range []string{
		"WebSocket API parity",
		"status.get",
		"runtime.start",
		"runtime.stop",
		"runtime.restart",
		"api.request GET /sidecar/v1/runners",
		"api.request POST /sidecar/v1/runners/{id}/start",
		"api.request POST /sidecar/v1/runners/{id}/stop",
		"api.request POST /sidecar/v1/runners/{id}/restart",
		"RuntimeController",
		"RunnerController",
		"same methods",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("settings view missing %q:\n%s", expected, view)
		}
	}
}

func TestSettingsControlsUseSharedRuntimeController(t *testing.T) {
	t.Parallel()

	runtimeControl := testRuntimeController()
	model := NewModel(ModelOptions{
		RuntimeController: runtimeControl,
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	updated := next.(Model)

	for _, tc := range []struct {
		key       string
		wantCall  string
		wantToast string
	}{
		{key: "s", wantCall: "start:release", wantToast: "started runtime release"},
		{key: "d", wantCall: "start:debug", wantToast: "started runtime debug"},
		{key: "x", wantCall: "stop", wantToast: "stopped runtime"},
		{key: "r", wantCall: "restart:release", wantToast: "restarted runtime release"},
	} {
		nextModel, cmd := updated.Update(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune(tc.key),
		})
		if cmd == nil {
			t.Fatalf("key %q returned no command", tc.key)
		}

		message := cmd()
		afterAction, _ := nextModel.(Model).Update(message)
		updated = afterAction.(Model)

		if got := runtimeControl.lastCall(); got != tc.wantCall {
			t.Fatalf("last call = %q, want %q", got, tc.wantCall)
		}
		if !strings.Contains(updated.View(), tc.wantToast) {
			t.Fatalf("view missing action result %q:\n%s", tc.wantToast, updated.View())
		}
	}
}

type fakeRuntimeController struct {
	status server.RuntimeStatus
	calls  []string
}

func testRuntimeController() *fakeRuntimeController {
	return &fakeRuntimeController{
		status: server.RuntimeStatus{
			State:      "running",
			Executable: "/opt/litert-lm",
			Version:    "litert-lm 0.13.1",
			ModelID:    "gemma4-e2b",
			ModelFile:  "/models/litert/gemma-4-E2B-it.litertlm",
			Upstream:   "http://127.0.0.1:9381",
			Mode:       "release",
			Detail:     "LiteRT-LM server process is running.",
		},
	}
}

func (c *fakeRuntimeController) Start(
	_ context.Context,
	mode server.RuntimeMode,
	_ server.RuntimeControlConfig,
) error {
	c.calls = append(c.calls, "start:"+string(mode))
	return nil
}

func (c *fakeRuntimeController) Stop(context.Context) error {
	c.calls = append(c.calls, "stop")
	return nil
}

func (c *fakeRuntimeController) Restart(
	_ context.Context,
	mode server.RuntimeMode,
	_ server.RuntimeControlConfig,
) error {
	c.calls = append(c.calls, "restart:"+string(mode))
	return nil
}

func (c *fakeRuntimeController) Status() server.RuntimeStatus {
	return c.status
}

func (c *fakeRuntimeController) lastCall() string {
	if len(c.calls) == 0 {
		return ""
	}
	return c.calls[len(c.calls)-1]
}

type fakeRunnerController struct {
	calls   []string
	created []server.RunnerSnapshot
	patches map[string]server.RunnerPatch
}

func testRunnerController() *fakeRunnerController {
	return &fakeRunnerController{}
}

func (c *fakeRunnerController) Snapshot() server.RunnerSnapshotResponse {
	if c.patches == nil {
		c.patches = map[string]server.RunnerPatch{}
	}
	runners := []server.RunnerSnapshot{
		{
			ID:         "main-litert",
			Runtime:    "litert",
			Role:       "main",
			Backend:    "cpu",
			Executable: "/opt/litert-lm",
			Version:    "litert-lm 0.13.1",
			ModelPath:  "/models/litert/gemma-4-E2B-it.litertlm",
			ModelID:    "gemma4-e2b",
			Host:       "127.0.0.1",
			Port:       9381,
			State:      "running",
			PID:        1234,
			Upstream:   "http://127.0.0.1:9381",
			Command: []string{
				"/opt/litert-lm",
				"serve",
				"--host",
				"127.0.0.1",
				"--port",
				"9381",
			},
			Capabilities: map[string]string{
				"chat":       "openai-compatible",
				"multimodal": "litert-run",
			},
			LogSequence: 41,
			Detail:      "LiteRT-LM server process is running.",
		},
		{
			ID:         "embed-qwen",
			Runtime:    "llamacpp",
			Role:       "embedding",
			Backend:    "gpu",
			Executable: "/opt/llama-server",
			ModelPath:  "/models/llamacpp/Qwen3-Embedding-0.6B-Q8_0.gguf",
			ModelID:    "qwen3-embedding",
			Host:       "127.0.0.1",
			Port:       9382,
			State:      "created",
			Upstream:   "http://127.0.0.1:9382",
			Capabilities: map[string]string{
				"embeddings": "openai-compatible",
			},
			LastError: "not started",
			Detail:    "llama.cpp embedding runner is configured.",
		},
	}
	runners = append(runners, c.created...)
	for index := range runners {
		if patch, ok := c.patches[runners[index].ID]; ok {
			applyFakeRunnerPatch(&runners[index], patch)
		}
	}
	return server.RunnerSnapshotResponse{
		Runners: runners,
		Routes: map[string]string{
			"main":      "main-litert",
			"embedding": "embed-qwen",
		},
	}
}

func (c *fakeRunnerController) CreateRunner(
	_ context.Context,
	spec server.RunnerSpec,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, strings.Join([]string{
		"create",
		spec.ID,
		spec.Runtime,
		spec.Role,
		spec.ModelID,
	}, ":"))
	runner := server.RunnerSnapshot{
		ID:        spec.ID,
		Runtime:   spec.Runtime,
		Role:      spec.Role,
		Backend:   spec.Backend,
		ModelPath: spec.ModelPath,
		ModelID:   spec.ModelID,
		Host:      spec.Host,
		Port:      spec.Port,
		State:     "created",
		Upstream:  spec.Upstream,
		Detail:    "Runner has not been started yet.",
	}
	c.created = append(c.created, runner)
	return runner, nil
}

func (c *fakeRunnerController) UpdateRunner(
	_ context.Context,
	id string,
	patch server.RunnerPatch,
) (server.RunnerSnapshot, error) {
	if c.patches == nil {
		c.patches = map[string]server.RunnerPatch{}
	}
	parts := []string{"update", id}
	if patch.Backend != "" {
		parts = append(parts, "backend="+patch.Backend)
	}
	c.calls = append(c.calls, strings.Join(parts, ":"))
	c.patches[id] = patch

	snapshot := c.Snapshot()
	for _, runner := range snapshot.Runners {
		if runner.ID != id {
			continue
		}
		return runner, nil
	}
	return server.RunnerSnapshot{}, nil
}

func applyFakeRunnerPatch(runner *server.RunnerSnapshot, patch server.RunnerPatch) {
	if patch.Backend != "" {
		runner.Backend = patch.Backend
	}
	if patch.Port > 0 {
		runner.Port = patch.Port
	}
	if patch.ModelID != "" {
		runner.ModelID = patch.ModelID
	}
}

func (c *fakeRunnerController) StartRunner(
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "start:"+id)
	return c.Snapshot().Runners[0], nil
}

func (c *fakeRunnerController) StopRunner(
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "stop:"+id)
	return c.Snapshot().Runners[0], nil
}

func (c *fakeRunnerController) RestartRunner(
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "restart:"+id)
	return c.Snapshot().Runners[0], nil
}

func (c *fakeRunnerController) lastCall() string {
	if len(c.calls) == 0 {
		return ""
	}
	return c.calls[len(c.calls)-1]
}
