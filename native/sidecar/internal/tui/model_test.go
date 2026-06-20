package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

func TestModelStartsWithDashboardLaunchWizardAndSetupOnly(t *testing.T) {
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
		"3 Setup",
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

	modelRow := lineNumberContaining(model.View(), "Models ----")
	next, cmd := model.Update(leftClick(dashboardModelMainX, modelRow))
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

func TestDashboardModelDropdownUsesResponsiveLayout(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 180
	model.height = 36

	modelRow := lineNumberContaining(model.View(), "Models ----")
	next, _ := model.Update(leftClick(dashboardModelMainX, modelRow))
	wide := next.(Model)
	wideView := wide.View()
	if !lineContainsAll(wideView, "Status", "Main models") {
		t.Fatalf("wide dashboard dropdown should share a masonry row with status:\n%s", wideView)
	}

	wide.width = 120
	narrowView := wide.View()
	if lineContainsAll(narrowView, "Status", "Main models") {
		t.Fatalf("narrow dashboard dropdown should stack full-width under status:\n%s", narrowView)
	}
	for _, expected := range []string{"Status", "Main models"} {
		if !strings.Contains(narrowView, expected) {
			t.Fatalf("narrow dashboard dropdown missing %q:\n%s", expected, narrowView)
		}
	}
}

func TestMouseCanSwitchTabsFromRenderedTabRow(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 120
	model.height = 24

	tabRow := lineNumberContaining(model.View(), "Launch Wizard")
	next, cmd := model.Update(leftClick(18, tabRow))
	if cmd != nil {
		t.Fatalf("tab mouse click returned unexpected command")
	}
	if got := next.(Model).activeTabID(); got != "wizard" {
		t.Fatalf("active tab = %q, want wizard", got)
	}
}

func TestLaunchWizardUsesResponsiveMasonryLayout(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController:  newFakeRunnerController(nil),
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           testCatalogWithPresentModels(t),
		LlamaRuntimeRoot:  testLlamaRuntimeRoot(t, "llama-win-cuda-13.3-x64"),
		BackendConfigPath: filepath.Join(t.TempDir(), "missing-backends.json"),
	})
	model.setActiveTab("wizard")
	model.width = 180
	model.height = 36

	wideView := model.View()
	if !lineContainsAll(wideView, "Launch Wizard", "Local Models") {
		t.Fatalf("wide wizard should render choices and local models in masonry columns:\n%s", wideView)
	}

	model.width = 120
	narrowView := model.View()
	if lineContainsAll(narrowView, "Launch Wizard", "Local Models") {
		t.Fatalf("narrow wizard should stack choices and local models full-width:\n%s", narrowView)
	}
	for _, expected := range []string{"Launch Wizard", "Local Models"} {
		if !strings.Contains(narrowView, expected) {
			t.Fatalf("narrow wizard missing %q:\n%s", expected, narrowView)
		}
	}
}

func TestLaunchWizardMouseUsesRenderedRows(t *testing.T) {
	t.Parallel()

	controller := newFakeRunnerController(nil)
	model := NewModel(ModelOptions{
		RunnerController:  controller,
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           testCatalogWithPresentModels(t),
		LlamaRuntimeRoot:  testLlamaRuntimeRoot(t, "llama-win-cuda-13.3-x64"),
		BackendConfigPath: filepath.Join(t.TempDir(), "missing-backends.json"),
	})
	model.setActiveTab("wizard")
	model.width = 180
	model.height = 36

	runtimeRow := lineNumberContaining(model.View(), "runtime")
	next, _ := model.Update(leftClick(wizardRuntimeLlamaX, runtimeRow))
	model = next.(Model)
	if strings.Contains(model.View(), "runtime ----") {
		t.Fatalf("wizard runtime row should not render decorative dashes:\n%s", model.View())
	}
	if !strings.Contains(compactSpaces(model.View()), "runtime litert [llama.cpp]") {
		t.Fatalf("wizard did not select llama.cpp from rendered runtime row:\n%s", model.View())
	}

	roleRow := lineNumberContaining(model.View(), "model role")
	next, _ = model.Update(leftClick(wizardRoleRerankingX, roleRow))
	model = next.(Model)
	if strings.Contains(model.View(), "model role ----") {
		t.Fatalf("wizard model role row should not render decorative dashes:\n%s", model.View())
	}
	if !strings.Contains(compactSpaces(model.View()), "model role main embedding [reranking]") {
		t.Fatalf("wizard did not select reranking from rendered role row:\n%s", model.View())
	}

	startRow := lineNumberContaining(model.View(), "[ START ]")
	nextModel, cmd := model.Update(leftClick(wizardStartX, startRow))
	if cmd == nil {
		t.Fatalf("wizard start click from rendered row returned no command")
	}
	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	if got := afterAction.(Model).activeTabID(); got != "runner:LM-R-1" {
		t.Fatalf("active tab = %q, want new runner tab", got)
	}
}

func TestRunnerTabUsesResponsiveMasonryLayout(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController([]server.RunnerSnapshot{
			testRunner("LR-M-1", "litert", "main", "running"),
		}),
		Logs:    server.NewLogBroadcaster(8),
		Catalog: testCatalogWithPresentModels(t),
	})
	model.setActiveTab("runner:LR-M-1")
	model.width = 180
	model.height = 36

	wideView := model.View()
	if !lineContainsAll(wideView, "Runner LR-M-1", "Routes / Controls") {
		t.Fatalf("wide runner view should render details and controls in masonry columns:\n%s", wideView)
	}

	model.width = 120
	narrowView := model.View()
	if lineContainsAll(narrowView, "Runner LR-M-1", "Routes / Controls") {
		t.Fatalf("narrow runner view should stack details and controls full-width:\n%s", narrowView)
	}
	for _, expected := range []string{"Runner LR-M-1", "Routes / Controls"} {
		if !strings.Contains(narrowView, expected) {
			t.Fatalf("narrow runner view missing %q:\n%s", expected, narrowView)
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
		"Menu",
		"Tab Next",
		"Dashboard: click model roles",
	} {
		if !strings.Contains(bottom, expected) {
			t.Fatalf("bottom action bar missing %q in %q:\n%s", expected, bottom, view)
		}
	}
	if strings.Contains(bottom, "F1") {
		t.Fatalf("bottom action bar should not depend on F keys on macOS: %q", bottom)
	}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	updated := next.(Model)
	wizardBottom := lastNonEmptyLine(updated.View())
	if !strings.Contains(wizardBottom, "Wizard: click toggles | Enter Start") {
		t.Fatalf("wizard bottom action bar = %q:\n%s", wizardBottom, updated.View())
	}
}

func TestTopAndBottomBarsTakeFullWidth(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
		ManagedScreen:    true,
	})
	model.width = 120
	model.height = 24

	view := model.View()
	lines := strings.Split(view, "\n")
	if got := lipgloss.Width(lines[0]); got != model.width {
		t.Fatalf("top bar width = %d, want %d:\n%s", got, model.width, view)
	}
	bottom := lastRenderedLineWithContent(view)
	if got := lipgloss.Width(bottom); got != model.width {
		t.Fatalf("bottom bar width = %d, want %d in %q:\n%s", got, model.width, bottom, view)
	}
}

func TestBottomBarMouseActionsDoNotRequireFKeys(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 120
	model.height = 24

	next, cmd := model.Update(leftClick(2, model.height-1))
	if cmd != nil {
		t.Fatalf("menu mouse click returned unexpected command")
	}
	updated := next.(Model)
	if !strings.Contains(updated.View(), "Global menu") {
		t.Fatalf("menu click did not open global menu:\n%s", updated.View())
	}

	next, cmd = model.Update(leftClick(12, model.height-1))
	if cmd != nil {
		t.Fatalf("tab-next mouse click returned unexpected command")
	}
	if got := next.(Model).activeTabID(); got != "wizard" {
		t.Fatalf("tab-next bottom click active tab = %q, want wizard", got)
	}
}

func TestLaunchWizardRendersThemedOptionBarsAndModelHighlight(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
		LlamaRuntimeRoot: testLlamaRuntimeRoot(t, "llama-win-cuda-13.3-x64"),
	})
	model.setActiveTab("wizard")
	model.width = 120
	model.height = 36

	choiceLines := model.wizardChoiceLines()
	expectedWidth := model.width - 4
	for _, index := range []int{0, 2, 4, 6} {
		if got := lipgloss.Width(choiceLines[index]); got != expectedWidth {
			t.Fatalf("wizard option row %d width = %d, want %d: %q", index, got, expectedWidth, choiceLines[index])
		}
	}

	modelLines := model.wizardLocalModelLines()
	if len(modelLines) == 0 {
		t.Fatalf("expected local model rows")
	}
	if got := lipgloss.Width(modelLines[0]); got != expectedWidth {
		t.Fatalf("selected model row width = %d, want %d: %q", got, expectedWidth, modelLines[0])
	}
	if !strings.HasPrefix(strings.TrimSpace(modelLines[0]), "> ") {
		t.Fatalf("selected model row should keep visible selected marker: %q", modelLines[0])
	}
}

func TestLaunchWizardHidesConfiguredNotWorkingBackends(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "backends.json")
	config := `{
  "version": 1,
  "runtimes": {
    "litert": {
      "gpu": {"working": true},
      "npu": {"working": false}
    },
    "llamacpp": {
      "cpu": {"working": true},
      "cuda13": {"working": false}
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write backend config: %v", err)
	}

	model := NewModel(ModelOptions{
		RunnerController:  newFakeRunnerController(nil),
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           testCatalogWithPresentModels(t),
		LlamaRuntimeRoot:  testLlamaRuntimeRoot(t, "llama-win-cpu-x64", "llama-win-cuda-13.3-x64"),
		BackendConfigPath: configPath,
	})
	model.setActiveTab("wizard")
	model.width = 140
	model.height = 36

	compactView := compactSpaces(model.View())
	if !strings.Contains(compactView, "LiteRT backend [cpu] gpu") {
		t.Fatalf("wizard should keep working/default LiteRT backends:\n%s", model.View())
	}
	if strings.Contains(compactView, "npu") {
		t.Fatalf("wizard should hide configured not-working LiteRT npu backend:\n%s", model.View())
	}

	model.toggleWizardRuntime()
	compactView = compactSpaces(model.View())
	if !strings.Contains(compactView, "llama type [cpu]") {
		t.Fatalf("wizard should keep working llama cpu backend:\n%s", model.View())
	}
	if strings.Contains(compactView, "cuda13") {
		t.Fatalf("wizard should hide configured not-working llama cuda13 backend:\n%s", model.View())
	}
}

func TestLaunchWizardClassifiesMacLlamaRuntimeAsMetal(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "backends.json")
	config := `{
  "version": 1,
  "runtimes": {
    "llamacpp": {
      "cpu": {"working": false},
      "metal": {"working": true}
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write backend config: %v", err)
	}

	model := NewModel(ModelOptions{
		RunnerController:  newFakeRunnerController(nil),
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           testCatalogWithPresentModels(t),
		LlamaRuntimeRoot:  testLlamaRuntimeRoot(t, "llama-macos-arm64"),
		BackendConfigPath: configPath,
	})
	model.setActiveTab("wizard")
	model.toggleWizardRuntime()
	if got := model.wizardBackend; got != "metal" {
		t.Fatalf("wizard backend for macOS llama runtime = %q, want metal", got)
	}
	variant, ok := model.selectedLlamaRuntimeVariant()
	if !ok {
		t.Fatal("expected selected macOS llama runtime variant")
	}
	if variant.Backend != "metal" {
		t.Fatalf("macOS llama variant backend = %q, want metal", variant.Backend)
	}
	if strings.Contains(compactSpaces(model.View()), "llama type cpu") {
		t.Fatalf("disabled cpu backend should not remain visible for macOS llama runtime:\n%s", model.View())
	}
	compactView := compactSpaces(model.View())
	if !strings.Contains(compactView, "llama type [metal]") {
		t.Fatalf("macOS llama runtime should only expose metal in wizard:\n%s", model.View())
	}
	for _, impossible := range []string{"cuda13", "cuda12", "openvino", "sycl"} {
		if strings.Contains(compactView, impossible) {
			t.Fatalf("macOS llama runtime should not expose %s in wizard:\n%s", impossible, model.View())
		}
	}
}

func TestSetupTabRendersOnlyInstalledLlamaRuntimeBackends(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "backends.json")
	config := `{
  "version": 1,
  "runtimes": {
    "litert": {
      "cpu": {"working": true},
      "gpu": {"working": false}
    },
    "llamacpp": {
      "cpu": {"working": false},
      "metal": {"working": true},
      "openvino": {"working": true},
      "cuda13": {"working": false}
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write backend config: %v", err)
	}

	model := NewModel(ModelOptions{
		RunnerController:  newFakeRunnerController(nil),
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           testCatalogWithPresentModels(t),
		LlamaRuntimeRoot:  testLlamaRuntimeRoot(t, "llama-macos-arm64"),
		BackendConfigPath: configPath,
	})
	model.setActiveTab("setup")
	model.width = 140
	model.height = 36

	view := model.View()
	compactView := compactSpaces(view)
	for _, expected := range []string{
		"3 Setup",
		"Backend Setup",
		"LiteRT cpu enabled",
		"LiteRT gpu disabled",
		"LiteRT npu enabled",
		"llama.cpp metal enabled",
		"Click a backend row to toggle it.",
	} {
		if !strings.Contains(compactView, expected) {
			t.Fatalf("setup tab missing %q:\n%s", expected, view)
		}
	}
	for _, impossible := range []string{
		"llama.cpp cpu",
		"llama.cpp openvino",
		"llama.cpp cuda13",
		"llama.cpp cuda12",
		"llama.cpp sycl",
	} {
		if strings.Contains(compactView, impossible) {
			t.Fatalf("setup tab should not show unavailable %q:\n%s", impossible, view)
		}
	}
}

func TestSetupAndWizardKeepInstalledCudaLlamaBackends(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
		LlamaRuntimeRoot: testLlamaRuntimeRoot(
			t,
			"llama-win-cuda-13.3-x64",
			"llama-linux-cuda-12-x64",
		),
		BackendConfigPath: filepath.Join(t.TempDir(), "missing-backends.json"),
	})
	model.setActiveTab("wizard")
	model.width = 140
	model.height = 36

	model.toggleWizardRuntime()
	compactView := compactSpaces(model.View())
	if !strings.Contains(compactView, "llama type [cuda13] cuda12") {
		t.Fatalf("wizard should expose installed CUDA llama runtimes:\n%s", model.View())
	}
	for _, unavailable := range []string{" metal", " openvino", " sycl"} {
		if strings.Contains(compactView, unavailable) {
			t.Fatalf("wizard should not expose unavailable llama backend %q:\n%s", unavailable, model.View())
		}
	}

	model.setActiveTab("setup")
	setupView := compactSpaces(model.View())
	for _, expected := range []string{"llama.cpp cuda13 enabled", "llama.cpp cuda12 enabled"} {
		if !strings.Contains(setupView, expected) {
			t.Fatalf("setup should expose installed CUDA backend %q:\n%s", expected, model.View())
		}
	}
	for _, unavailable := range []string{"llama.cpp metal", "llama.cpp openvino", "llama.cpp sycl"} {
		if strings.Contains(setupView, unavailable) {
			t.Fatalf("setup should not expose unavailable llama backend %q:\n%s", unavailable, model.View())
		}
	}
}

func TestSetupToggleWritesConfigAndUpdatesWizardVisibilityImmediately(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "nested", "backends.json")
	model := NewModel(ModelOptions{
		RunnerController:  newFakeRunnerController(nil),
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           testCatalogWithPresentModels(t),
		LlamaRuntimeRoot:  testLlamaRuntimeRoot(t, "llama-win-cpu-x64"),
		BackendConfigPath: configPath,
	})
	model.setActiveTab("setup")
	model.width = 140
	model.height = 36

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("setup down key returned unexpected command")
	}
	model = next.(Model)
	if model.setupSelection != 1 {
		t.Fatalf("setup selection = %d, want gpu row", model.setupSelection)
	}

	next, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("setup enter returned unexpected command")
	}
	model = next.(Model)
	if got := backendWorkingValue(t, configPath, "litert", "gpu"); got {
		t.Fatal("setup toggle should write litert gpu working=false")
	}
	if got := model.litertBackendOptions(); !reflect.DeepEqual(got, []string{"cpu", "npu"}) {
		t.Fatalf("litert backend options after disable = %#v, want cpu and npu", got)
	}
	model.setActiveTab("wizard")
	wizardChoices := compactSpaces(strings.Join(model.wizardChoiceLines(), " "))
	if strings.Contains(wizardChoices, "gpu") {
		t.Fatalf("wizard should hide disabled gpu immediately:\n%s", strings.Join(model.wizardChoiceLines(), "\n"))
	}

	model.setActiveTab("setup")
	next, cmd = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd != nil {
		t.Fatalf("setup space returned unexpected command")
	}
	model = next.(Model)
	if got := backendWorkingValue(t, configPath, "litert", "gpu"); !got {
		t.Fatal("setup toggle should write litert gpu working=true")
	}
	if got := model.litertBackendOptions(); !reflect.DeepEqual(got, []string{"cpu", "gpu", "npu"}) {
		t.Fatalf("litert backend options after enable = %#v, want cpu, gpu, and npu", got)
	}
	model.setActiveTab("wizard")
	wizardChoices = compactSpaces(strings.Join(model.wizardChoiceLines(), " "))
	if !strings.Contains(wizardChoices, "LiteRT backend [cpu] gpu npu") {
		t.Fatalf("wizard should show enabled gpu immediately:\n%s", strings.Join(model.wizardChoiceLines(), "\n"))
	}
}

func TestLaunchWizardOptionBarsDoNotUseDecorativeDashesOrTextFainting(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("read model.go: %v", err)
	}
	value := string(source)
	start := strings.Index(value, "func (m Model) wizardOptionBar(")
	end := strings.Index(value, "func (m Model) wizardStartLine()")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("could not locate wizard option renderer in model.go")
	}
	renderer := value[start:end]
	if strings.Contains(renderer, `Render(label + " ----")`) {
		t.Fatalf("wizard option renderer still emits decorative dashes")
	}
	if strings.Contains(renderer, "Faint(true)") {
		t.Fatalf("wizard option renderer should dim colors, not text")
	}
}

func TestRuntimeStatusBadgesUseGreenForActiveAndRedForIdle(t *testing.T) {
	t.Parallel()

	if got := runtimeUseBadgeColor(1); got != "82" {
		t.Fatalf("active runtime color = %q, want green", got)
	}
	if got := runtimeUseBadgeColor(0); got != "196" {
		t.Fatalf("idle runtime color = %q, want red", got)
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
		"Palette themes >",
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

func TestGlobalMenuPaletteActionOpensPaletteChoices(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 120
	model.height = 30

	next, _ := model.Update(leftClick(2, model.height-1))
	opened := next.(Model)
	view := opened.View()
	for _, expected := range []string{
		"Global menu",
		"Palette themes >",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("global menu missing %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "Amber") {
		t.Fatalf("palette choices should stay hidden until palette action is selected:\n%s", view)
	}

	next, cmd := opened.Update(leftClick(4, opened.globalMenuTopRow()+4))
	if cmd != nil {
		t.Fatalf("palette action click returned unexpected command")
	}
	view = next.(Model).View()
	for _, expected := range []string{
		"Palette choices",
		"Neon",
		"Amber",
		"Ocean",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("global menu missing %q:\n%s", expected, view)
		}
	}
}

func TestGlobalMenuPaletteChoiceCanBeClicked(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController(nil),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
		ManagedScreen:    true,
	})
	model.width = 120
	model.height = 30

	next, _ := model.Update(leftClick(2, model.height-1))
	opened := next.(Model)
	next, cmd := opened.Update(leftClick(4, opened.globalMenuTopRow()+4))
	if cmd != nil {
		t.Fatalf("palette action click returned unexpected command")
	}
	opened = next.(Model)
	paletteX := lipgloss.Width(firstRenderedLine(opened.globalMenuMainView())) + panelGridColumnGap + 3
	amberRow := opened.globalMenuTopRow() + 3
	next, cmd = opened.Update(leftClick(paletteX, amberRow))
	if cmd != nil {
		t.Fatalf("palette click returned unexpected command")
	}
	updated := next.(Model)
	if got := updated.paletteID; got != "amber" {
		t.Fatalf("palette = %q, want amber", got)
	}
	if updated.globalMenuOpen {
		t.Fatalf("palette click should close global menu")
	}
}

func TestRunnerBottomBarMouseActionsUseSharedController(t *testing.T) {
	t.Parallel()

	controller := newFakeRunnerController([]server.RunnerSnapshot{
		testRunner("LR-M-1", "litert", "main", "created"),
	})
	model := NewModel(ModelOptions{
		RunnerController: controller,
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 140
	model.height = 30
	model.setActiveTab("runner:LR-M-1")

	for _, tc := range []struct {
		name string
		x    int
		want string
	}{
		{name: "start", x: runnerBottomStartX, want: "start:LR-M-1"},
		{name: "stop", x: runnerBottomStopX, want: "stop:LR-M-1"},
		{name: "restart", x: runnerBottomRestartX, want: "restart:LR-M-1"},
	} {
		next, cmd := model.Update(leftClick(tc.x, model.height-1))
		if cmd == nil {
			t.Fatalf("%s click returned no command", tc.name)
		}
		message := cmd()
		afterAction, _ := next.(Model).Update(message)
		model = afterAction.(Model)
		if got := controller.calls[len(controller.calls)-1]; got != tc.want {
			t.Fatalf("%s call = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRunnerTabRendersAndEditsCommandPreview(t *testing.T) {
	t.Parallel()

	runner := testRunner("LR-M-1", "litert", "main", "created")
	runner.Command = []string{
		"litert-lm",
		"serve",
		"--host",
		"127.0.0.1",
		"--port",
		"9381",
	}
	controller := newFakeRunnerController([]server.RunnerSnapshot{runner})
	model := NewModel(ModelOptions{
		RunnerController: controller,
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 180
	model.height = 34
	model.setActiveTab("runner:LR-M-1")

	view := model.View()
	for _, expected := range []string{
		"Command",
		"litert-lm serve --host 127.0.0.1 --port 9381",
		"Edit Cmd",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("runner view missing %q:\n%s", expected, view)
		}
	}

	var editSegment bottomActionSegment
	for _, segment := range model.bottomActionSegments() {
		if segment.label == "Edit Cmd" {
			editSegment = segment
			break
		}
	}
	if editSegment.label == "" {
		t.Fatalf("runner bottom action segments missing Edit Cmd: %#v", model.bottomActionSegments())
	}

	next, cmd := model.Update(leftClick(editSegment.start, model.height-1))
	if cmd != nil {
		t.Fatalf("edit command click returned unexpected command")
	}
	editing := next.(Model)
	if editing.edit == nil || editing.edit.field != "commandLine" {
		t.Fatalf("edit state = %#v, want commandLine editor", editing.edit)
	}
	currentLine := runnerCommandLine(runner.Command)
	if editing.edit.value != currentLine {
		t.Fatalf("command editor value = %q, want current command %q", editing.edit.value, currentLine)
	}
	if !strings.Contains(editing.View(), "Editing Command") {
		t.Fatalf("runner command editor is not visible:\n%s", editing.View())
	}

	editedLine := "litert-lm serve --host 127.0.0.1 --port 9488 --verbose"
	for range currentLine {
		next, cmd = editing.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		if cmd != nil {
			t.Fatalf("clearing command returned unexpected command")
		}
		editing = next.(Model)
	}
	next, cmd = editing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(editedLine)})
	if cmd != nil {
		t.Fatalf("typing command returned unexpected command")
	}
	editing = next.(Model)
	next, cmd = editing.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("saving command edit returned no command")
	}
	message := cmd()
	afterUpdate, _ := next.(Model).Update(message)
	updated := afterUpdate.(Model)

	if got := controller.calls[len(controller.calls)-1]; got != "update:LR-M-1:commandLine:"+editedLine {
		t.Fatalf("update call = %q, want commandLine edit", got)
	}
	updatedRunner, ok := updated.runnerByID("LR-M-1")
	if !ok {
		t.Fatalf("runner LR-M-1 not found after command edit")
	}
	if got := strings.Join(updatedRunner.Command, " "); got != editedLine {
		t.Fatalf("updated command = %q, want %q", got, editedLine)
	}
}

func TestRunnerTabRendersBottomTerminalWithRunnerLogs(t *testing.T) {
	t.Parallel()

	runner := testRunner("LR-M-1", "litert", "main", "running")
	runner.Command = []string{
		"litert-lm",
		"serve",
		"--host",
		"127.0.0.1",
		"--port",
		"9381",
	}
	logs := server.NewLogBroadcaster(16)
	logs.Publish("runner:LR-M-1", "stdout", "model loaded")
	logs.Publish("runner:LR-M-1", "stderr", "retrying bind")

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController([]server.RunnerSnapshot{runner}),
		Logs:             logs,
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 180
	model.height = 40
	model.setActiveTab("runner:LR-M-1")

	view := model.View()
	terminal := runnerTerminalSection(t, view)
	for _, expected := range []string{
		"Command",
		"litert-lm serve --host 127.0.0.1 --port 9381",
		"stdout model loaded",
		"stderr retrying bind",
	} {
		if !strings.Contains(terminal, expected) {
			t.Fatalf("runner terminal missing %q:\n%s", expected, view)
		}
	}
	commandIndex := strings.Index(terminal, "Command")
	logIndex := strings.Index(terminal, "stdout model loaded")
	if commandIndex < 0 || logIndex < commandIndex {
		t.Fatalf("runner terminal should render command before logs:\n%s", terminal)
	}
	terminalIndex := strings.LastIndex(view, "Terminal")
	controlsIndex := strings.LastIndex(view, "Routes / Controls")
	if terminalIndex < controlsIndex {
		t.Fatalf("runner terminal should render below route/control panels:\n%s", view)
	}
}

func TestRunnerTabFiltersTerminalLogsToActiveRunner(t *testing.T) {
	t.Parallel()

	runner := testRunner("LR-M-1", "litert", "main", "running")
	runner.Command = []string{"litert-lm", "serve"}
	logs := server.NewLogBroadcaster(16)
	logs.Publish("runner:LR-M-2", "stdout", "other runner output")
	logs.Publish("runtime", "stderr", "runtime output")

	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController([]server.RunnerSnapshot{runner}),
		Logs:             logs,
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 160
	model.height = 36
	model.setActiveTab("runner:LR-M-1")

	terminal := runnerTerminalSection(t, model.View())
	for _, unexpected := range []string{
		"other runner output",
		"runtime output",
	} {
		if strings.Contains(terminal, unexpected) {
			t.Fatalf("runner terminal should filter %q:\n%s", unexpected, terminal)
		}
	}
}

func TestRunnerTabTerminalEmptyStateIncludesCommand(t *testing.T) {
	t.Parallel()

	runner := testRunner("LR-M-1", "litert", "main", "created")
	runner.Command = []string{"litert-lm", "serve", "--port", "9381"}
	model := NewModel(ModelOptions{
		RunnerController: newFakeRunnerController([]server.RunnerSnapshot{runner}),
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 140
	model.height = 34
	model.setActiveTab("runner:LR-M-1")

	terminal := runnerTerminalSection(t, model.View())
	for _, expected := range []string{
		"Command",
		"litert-lm serve --port 9381",
		"No runner logs yet.",
	} {
		if !strings.Contains(terminal, expected) {
			t.Fatalf("empty runner terminal missing %q:\n%s", expected, terminal)
		}
	}
}

func TestRunnerCloseBottomActionCallsControllerAndRemovesTab(t *testing.T) {
	t.Parallel()

	controller := newFakeRunnerController([]server.RunnerSnapshot{
		testRunner("LR-M-1", "litert", "main", "running"),
	})
	model := NewModel(ModelOptions{
		RunnerController: controller,
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          testCatalogWithPresentModels(t),
	})
	model.width = 140
	model.height = 30
	model.setActiveTab("runner:LR-M-1")

	bottom := lastNonEmptyLine(model.View())
	if !strings.Contains(bottom, "X Close") {
		t.Fatalf("runner bottom action bar missing X close action in %q:\n%s", bottom, model.View())
	}
	var closeSegment bottomActionSegment
	for _, segment := range model.bottomActionSegments() {
		if segment.label == "X Close" {
			closeSegment = segment
			break
		}
	}
	if closeSegment.label == "" {
		t.Fatalf("runner bottom action segments missing X Close: %#v", model.bottomActionSegments())
	}

	next, cmd := model.Update(leftClick(closeSegment.start, model.height-1))
	if cmd == nil {
		t.Fatalf("close click returned no command")
	}
	message := cmd()
	afterAction, _ := next.(Model).Update(message)
	updated := afterAction.(Model)

	if got := controller.calls[len(controller.calls)-1]; got != "close:LR-M-1" {
		t.Fatalf("close call = %q, want close:LR-M-1", got)
	}
	if _, ok := updated.runnerByID("LR-M-1"); ok {
		t.Fatalf("runner LR-M-1 should be removed after close")
	}
	if got := updated.activeTabID(); got != "dashboard" {
		t.Fatalf("active tab = %q, want dashboard after closing last runner", got)
	}
}

func TestLaunchWizardClickStartCreatesAndStartsNumberedRunner(t *testing.T) {
	t.Parallel()

	controller := newFakeRunnerController(nil)
	model := NewModel(ModelOptions{
		RunnerController:  controller,
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           testCatalogWithPresentModels(t),
		LlamaRuntimeRoot:  testLlamaRuntimeRoot(t, "llama-win-cuda-13.3-x64"),
		BackendConfigPath: filepath.Join(t.TempDir(), "missing-backends.json"),
	})
	model.width = 140
	model.height = 36
	model.setActiveTab("wizard")

	runtimeRow := lineNumberContaining(model.View(), "runtime")
	next, _ := model.Update(leftClick(wizardRuntimeLlamaX, runtimeRow))
	model = next.(Model)
	if !strings.Contains(compactSpaces(model.View()), "runtime litert [llama.cpp]") {
		t.Fatalf("wizard did not select llama.cpp by mouse:\n%s", model.View())
	}

	roleRow := lineNumberContaining(model.View(), "model role")
	next, _ = model.Update(leftClick(wizardRoleRerankingX, roleRow))
	model = next.(Model)
	view := model.View()
	compactView := compactSpaces(view)
	for _, expected := range []string{
		"llama type [cuda13]",
		"model role main embedding [reranking]",
		"Qwen3-Reranker-0.6B-Q4_K_M.gguf",
		"[ START ]",
	} {
		if !strings.Contains(compactView, expected) {
			t.Fatalf("wizard missing %q:\n%s", expected, view)
		}
	}

	startRow := lineNumberContaining(model.View(), "[ START ]")
	nextModel, cmd := model.Update(leftClick(wizardStartX, startRow))
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
	if !strings.Contains(updated.View(), "4 ● LM-R-1") {
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
		"3 Setup",
		"4 ● LR-M-1",
		"5 ● LM-E-1",
		"6 ◐ LM-M-2",
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

func lastRenderedLineWithContent(view string) string {
	lines := strings.Split(view, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if strings.TrimSpace(lines[index]) != "" {
			return lines[index]
		}
	}
	return ""
}

func lineNumberContaining(view string, text string) int {
	for index, line := range strings.Split(view, "\n") {
		if strings.Contains(line, text) {
			return index
		}
	}
	return -1
}

func lineContainsAll(view string, parts ...string) bool {
	for _, line := range strings.Split(view, "\n") {
		matches := true
		for _, part := range parts {
			if !strings.Contains(line, part) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func runnerTerminalSection(t *testing.T, view string) string {
	t.Helper()

	index := strings.LastIndex(view, "Terminal")
	if index < 0 {
		t.Fatalf("runner terminal panel missing:\n%s", view)
	}
	return view[index:]
}

func compactSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func backendWorkingValue(
	t *testing.T,
	path string,
	runtimeName string,
	backend string,
) bool {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read backend config: %v", err)
	}
	var decoded struct {
		Runtimes map[string]map[string]struct {
			Working bool `json:"working"`
		} `json:"runtimes"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode backend config: %v", err)
	}
	runtimeResults, ok := decoded.Runtimes[runtimeName]
	if !ok {
		t.Fatalf("runtime %q missing from config %#v", runtimeName, decoded.Runtimes)
	}
	result, ok := runtimeResults[backend]
	if !ok {
		t.Fatalf("backend %q missing from config %#v", backend, runtimeResults)
	}
	return result.Working
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
	_ context.Context,
	id string,
	patch server.RunnerPatch,
) (server.RunnerSnapshot, error) {
	for index := range c.runners {
		if c.runners[index].ID != id {
			continue
		}
		switch {
		case patch.CommandLine != nil:
			c.calls = append(c.calls, "update:"+id+":commandLine:"+*patch.CommandLine)
			c.runners[index].Command = strings.Fields(*patch.CommandLine)
		case patch.Command != nil:
			c.calls = append(c.calls, "update:"+id+":command:"+strings.Join(patch.Command, " "))
			c.runners[index].Command = append([]string(nil), patch.Command...)
		default:
			c.calls = append(c.calls, "update:"+id)
		}
		return c.runners[index], nil
	}
	c.calls = append(c.calls, "update:"+id)
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
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "stop:"+id)
	for index := range c.runners {
		if c.runners[index].ID == id {
			c.runners[index].State = "stopped"
			return c.runners[index], nil
		}
	}
	return server.RunnerSnapshot{}, nil
}

func (c *fakeRunnerController) RestartRunner(
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "restart:"+id)
	for index := range c.runners {
		if c.runners[index].ID == id {
			c.runners[index].State = "running"
			return c.runners[index], nil
		}
	}
	return server.RunnerSnapshot{}, nil
}

func (c *fakeRunnerController) CloseRunner(
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "close:"+id)
	for index, runner := range c.runners {
		if runner.ID == id {
			c.runners = append(c.runners[:index], c.runners[index+1:]...)
			return runner, nil
		}
	}
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
