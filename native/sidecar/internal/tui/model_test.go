package tui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"litert-sidecar/internal/catalog"
	"litert-sidecar/internal/server"
)

func TestRunEnablesAlternateScreenAndMouseRenderer(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("read model.go: %v", err)
	}
	for _, expected := range []string{
		"tea.WithAltScreen()",
		"tea.WithMouseCellMotion()",
	} {
		if !strings.Contains(string(source), expected) {
			t.Fatalf("Run must configure %s", expected)
		}
	}
}

func TestModelStartsWithDashboardAndLaunchWizardOnly(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 120
	model.height = 32

	view := model.View()
	for _, expected := range []string{
		"LiteRT sidecar",
		"LiteRT: idle",
		"llama.cpp: idle",
		"1 Dashboard",
		"2 Launch Wizard",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("initial TUI missing %q:\n%s", expected, view)
		}
	}
	for _, removed := range []string{
		"Runtime:",
		"Chat",
		"Logs",
		"Settings",
		"main-litert",
	} {
		if strings.Contains(view, removed) {
			t.Fatalf("initial TUI should not render deprecated %q:\n%s", removed, view)
		}
	}
	if strings.Contains(view, "3 Models") {
		t.Fatalf("initial TUI should not render the deprecated Models tab:\n%s", view)
	}
}

func TestDashboardRendersOnlyStatusWidget(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController([]server.RunnerSnapshot{
			testRunner("LR-M-1", "litert", "main", "running"),
			testRunner("LM-E-1", "llamacpp", "embedding", "created"),
		}),
		Logs:    server.NewLogBroadcaster(8),
		Catalog: testCatalogWithPresentModels(t),
	})
	model.width = 140
	model.height = 32

	view := model.View()
	for _, expected := range []string{
		"Status",
		"Runners by runtime",
		"LiteRT      1 alive",
		"llama.cpp   0 alive",
		"Runners by role",
		"Main        1 alive",
		"Embedding   0 alive",
		"Reranking   0 alive",
		"Models ---- Main 4 -- Embedding 3 -- Reranking 1",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("dashboard status missing %q:\n%s", expected, view)
		}
	}
	for _, removed := range []string{
		"System health",
		"Signal board",
		"Runtime topology",
		"Backend matrix",
		"Route map",
		"Recent activity",
		"Hotkeys",
	} {
		if strings.Contains(view, removed) {
			t.Fatalf("dashboard should not render deprecated %q:\n%s", removed, view)
		}
	}
}

func TestDashboardMouseClickOpensModelRoleDropdown(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 140
	model.height = 32

	next, cmd := model.Update(leftClick(dashboardModelMainX, dashboardModelRowY))
	if cmd != nil {
		t.Fatalf("dashboard model click returned unexpected command")
	}
	updated := next.(Model)
	view := updated.View()
	for _, expected := range []string{
		"Main models",
		"gemma4-gguf",
		"qwen35-2b-gguf",
		"gemma4-litert",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("dashboard dropdown missing %q:\n%s", expected, view)
		}
	}
}

func TestBottomBarListsGlobalMenuAndCurrentTabActions(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 120
	model.height = 24

	view := model.View()
	bottom := lastNonEmptyLine(view)
	for _, expected := range []string{
		"F1 Menu",
		"Tab Next",
		"Dashboard: click model roles",
	} {
		if !strings.Contains(bottom, expected) {
			t.Fatalf("bottom action bar missing %q in %q:\n%s", expected, bottom, view)
		}
	}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	updated := next.(Model)
	wizardBottom := lastNonEmptyLine(updated.View())
	if !strings.Contains(wizardBottom, "Wizard: click toggles | Enter Start") {
		t.Fatalf("wizard bottom action bar = %q:\n%s", wizardBottom, updated.View())
	}
}

func TestGlobalActionsOpenAsBottomLeftMenu(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 120
	model.height = 24

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyF1})
	if cmd != nil {
		t.Fatalf("F1 returned unexpected command")
	}
	updated := next.(Model)
	for _, expected := range []string{
		"Global menu",
		"Tab/Click tabs",
		"Esc Quit",
	} {
		if !strings.Contains(updated.View(), expected) {
			t.Fatalf("global menu missing %q:\n%s", expected, updated.View())
		}
	}

	next, cmd = updated.Update(leftClick(2, updated.height-1))
	if cmd != nil {
		t.Fatalf("F1 mouse click returned unexpected command")
	}
	if strings.Contains(next.(Model).View(), "Global menu") {
		t.Fatalf("global menu did not close after F1 mouse click:\n%s", next.(Model).View())
	}
}

func TestLaunchWizardClickStartCreatesAndStartsNumberedRunner(t *testing.T) {
	t.Parallel()

	controller := newFakeRunnerController(nil)
	model := NewModel(ModelOptions{
		RunnerController: controller,
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
		LlamaRuntimeRoot: testLlamaRuntimeRoot(t, "llama-win-cuda-13.3-x64"),
	})
	model.width = 140
	model.height = 36
	model.setActiveTab("wizard")

	next, _ := model.Update(leftClick(wizardRuntimeLlamaX, wizardRuntimeRowY))
	model = next.(Model)
	if !strings.Contains(model.View(), "runtime ---- litert [llama.cpp]") {
		t.Fatalf("wizard did not select llama.cpp by mouse:\n%s", model.View())
	}

	next, _ = model.Update(leftClick(wizardRoleRerankingX, wizardRoleRowY))
	model = next.(Model)
	view := model.View()
	for _, expected := range []string{
		"llama type ---- cpu gpu openvino [cuda13] cuda12 sycl",
		"model role ---- main embedding [reranking]",
		"Qwen3-Reranker-0.6B-Q4_K_M.gguf",
		"[ START ]",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("wizard missing %q:\n%s", expected, view)
		}
	}

	nextModel, cmd := model.Update(leftClick(wizardStartX, wizardStartRowY))
	if cmd == nil {
		t.Fatalf("wizard start click returned no command")
	}
	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	updated := afterAction.(Model)

	if got := strings.Join(controller.calls, ","); got != "create:LM-R-1:llamacpp:reranking:qwen3-reranker-q4km,start:LM-R-1" {
		t.Fatalf("runner calls = %q", got)
	}
	if updated.activeTabID() != "runner:LM-R-1" {
		t.Fatalf("active tab = %q, want new runner tab", updated.activeTabID())
	}
	if !strings.Contains(updated.View(), "3 ● LM-R-1") {
		t.Fatalf("new runner tab missing:\n%s", updated.View())
	}
}

func TestRunnerTabsAreInsertedAfterLaunchWizardAndNumberedByRole(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController([]server.RunnerSnapshot{
			testRunner("LR-M-1", "litert", "main", "running"),
			testRunner("LM-E-1", "llamacpp", "embedding", "running"),
			testRunner("LM-M-2", "llamacpp", "main", "created"),
		}),
		Logs:    server.NewLogBroadcaster(8),
		Catalog: testCatalogWithPresentModels(t),
	})
	view := model.View()
	for _, expected := range []string{
		"1 Dashboard",
		"2 Launch Wizard",
		"3 ● LR-M-1",
		"4 ● LM-E-1",
		"5 ◐ LM-M-2",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("tab order missing %q:\n%s", expected, view)
		}
	}
	if got := model.nextRunnerID("llamacpp", "main"); got != "LM-M-3" {
		t.Fatalf("next main runner id = %q, want role-numbered id", got)
	}
	if got := model.nextRunnerID("llamacpp", "reranking"); got != "LM-R-1" {
		t.Fatalf("next reranking runner id = %q, want role-numbered id", got)
	}
}

func lastNonEmptyLine(view string) string {
	lines := strings.Split(view, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line != "" {
			return line
		}
	}
	return ""
}

func leftClick(x int, y int) tea.MouseMsg {
	return tea.MouseMsg{
		X:      x,
		Y:      y,
		Type:   tea.MouseLeft,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
}

func testRunner(id string, runtime string, role string, state string) server.RunnerSnapshot {
	return server.RunnerSnapshot{
		ID:         id,
		Runtime:    runtime,
		Role:       role,
		Backend:    "cpu",
		ModelID:    strings.ToLower(id),
		ModelPath:  "/models/" + runtime + "/" + role + "/" + id,
		Host:       "127.0.0.1",
		Port:       9381,
		Launch:     true,
		State:      state,
		Upstream:   "http://127.0.0.1:9381",
		Executable: runtime + "-server",
	}
}

func testCatalogWithPresentModels(t *testing.T, ids ...string) *catalog.Catalog {
	t.Helper()

	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	all := len(wanted) == 0

	root := t.TempDir()
	modelCatalog := catalog.NewDefault(root)
	for _, entry := range modelCatalog.Entries() {
		if !all && !wanted[entry.ID] {
			continue
		}
		if entry.Role == "browser" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(entry.TargetPath), 0o755); err != nil {
			t.Fatalf("create model fixture directory: %v", err)
		}
		if err := os.WriteFile(entry.TargetPath, []byte("x"), 0o644); err != nil {
			t.Fatalf("write model fixture: %v", err)
		}
	}
	return modelCatalog
}

func testLlamaRuntimeRoot(t *testing.T, names ...string) string {
	t.Helper()

	root := t.TempDir()
	for _, name := range names {
		path := filepath.Join(root, name, "bin", "llama-server")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create llama runtime fixture directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write llama runtime fixture: %v", err)
		}
	}
	return root
}

type fakeRunnerController struct {
	runners []server.RunnerSnapshot
	calls   []string
}

func newFakeRunnerController(runners []server.RunnerSnapshot) *fakeRunnerController {
	return &fakeRunnerController{
		runners: append([]server.RunnerSnapshot{}, runners...),
	}
}

func (c *fakeRunnerController) Snapshot() server.RunnerSnapshotResponse {
	runners := append([]server.RunnerSnapshot{}, c.runners...)
	routes := map[string]string{}
	for _, runner := range runners {
		if runner.State == "running" {
			routes[runner.Role] = runner.ID
		}
	}
	return server.RunnerSnapshotResponse{
		Runners: runners,
		Routes:  routes,
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
		ID:         spec.ID,
		Runtime:    spec.Runtime,
		Role:       spec.Role,
		Backend:    spec.Backend,
		Executable: spec.Executable,
		ModelPath:  spec.ModelPath,
		ModelID:    spec.ModelID,
		Host:       spec.Host,
		Port:       spec.Port,
		Launch:     spec.Launch,
		State:      "created",
		Upstream:   spec.Upstream,
	}
	c.runners = append(c.runners, runner)
	return runner, nil
}

func (c *fakeRunnerController) UpdateRunner(
	context.Context,
	string,
	server.RunnerPatch,
) (server.RunnerSnapshot, error) {
	return server.RunnerSnapshot{}, nil
}

func (c *fakeRunnerController) StartRunner(
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "start:"+id)
	for index := range c.runners {
		if c.runners[index].ID == id {
			c.runners[index].State = "running"
			return c.runners[index], nil
		}
	}
	return server.RunnerSnapshot{}, nil
}

func (c *fakeRunnerController) StopRunner(
	context.Context,
	string,
) (server.RunnerSnapshot, error) {
	return server.RunnerSnapshot{}, nil
}

func (c *fakeRunnerController) RestartRunner(
	context.Context,
	string,
) (server.RunnerSnapshot, error) {
	return server.RunnerSnapshot{}, nil
}

func (c *fakeRunnerController) lastCreatedIDNumber(role string) int {
	maxID := 0
	roleNeedle := "-" + roleLetter(role) + "-"
	for _, runner := range c.runners {
		if !strings.Contains(runner.ID, roleNeedle) {
			continue
		}
		parts := strings.Split(runner.ID, "-")
		if len(parts) == 0 {
			continue
		}
		number, err := strconv.Atoi(parts[len(parts)-1])
		if err == nil && number > maxID {
			maxID = number
		}
	}
	return maxID
}
