package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

func TestModelRendersRichVisualShell(t *testing.T) {
	t.Parallel()

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runner:main-litert", "stdout", "runtime ready")
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              logs,
	})
	model.width = 132
	model.height = 42
	view := model.View()

	for _, expected := range []string{
		"◆ LiteRT sidecar",
		"Runtime:",
		"Runners:",
		"Routes:",
		"Logs:",
		"Viewport:",
		"╭",
		"╰",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("rich visual shell missing %q:\n%s", expected, view)
		}
	}
}

func TestModelRendersStatusRichTabBar(t *testing.T) {
	t.Parallel()

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runner:main-litert", "stdout", "runtime ready")
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              logs,
		Catalog:           testCatalog(t),
	})
	view := model.View()

	for _, expected := range []string{
		"1 ◆ Dashboard 1/2 running",
		"2 ● main-litert",
		"3 ◐ embed-qwen",
		"4 ● Models 4/4",
		"5 ● Logs 1",
		"6 ● Settings API",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("status-rich tab bar missing %q:\n%s", expected, view)
		}
	}
}

func TestModelRendersContextCommandRail(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})

	dashboardView := model.View()
	for _, expected := range []string{
		"Command rail",
		"Global: Tab/Right next | Shift+Tab/Left previous | 1-6 jump | Esc quit",
		"Dashboard: specs + topology + runnable backends",
		"API: status.get + /sidecar/v1/status",
	} {
		if !strings.Contains(dashboardView, expected) {
			t.Fatalf("dashboard command rail missing %q:\n%s", expected, dashboardView)
		}
	}

	model.setActiveTab("runner:main-litert")
	runnerView := model.View()
	for _, expected := range []string{
		"Runner main-litert: s Start | x Stop | r Restart",
		"Edit: b/p/h/i/m/e/u/f/l/v/t/o",
		"API: RunnerController + /sidecar/v1/runners/main-litert",
	} {
		if !strings.Contains(runnerView, expected) {
			t.Fatalf("runner command rail missing %q:\n%s", expected, runnerView)
		}
	}

	model.setActiveTab("models")
	modelsView := model.View()
	for _, expected := range []string{
		"Models: d Download | m Main | e Embedding | r Rerank",
		"API: Catalog.Download + POST /sidecar/v1/models/download",
	} {
		if !strings.Contains(modelsView, expected) {
			t.Fatalf("models command rail missing %q:\n%s", expected, modelsView)
		}
	}

	model.setActiveTab("logs")
	logsView := model.View()
	for _, expected := range []string{
		"Logs: live broadcaster cache | WebSocket logs.subscribe parity",
		"API: LogBroadcaster + logs.subscribe",
	} {
		if !strings.Contains(logsView, expected) {
			t.Fatalf("logs command rail missing %q:\n%s", expected, logsView)
		}
	}

	model.setActiveTab("settings")
	settingsView := model.View()
	for _, expected := range []string{
		"Settings: s/d/r/g/x runtime | e/h/p/m/i/u/f edit | l/a/v toggle",
		"API: RuntimeController + WebSocket runtime.*",
	} {
		if !strings.Contains(settingsView, expected) {
			t.Fatalf("settings command rail missing %q:\n%s", expected, settingsView)
		}
	}
}

func viewLineContainsAll(view string, needles ...string) bool {
	for _, line := range strings.Split(view, "\n") {
		matched := true
		for _, needle := range needles {
			if !strings.Contains(line, needle) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
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

func TestDashboardRendersRichOperationalOverview(t *testing.T) {
	t.Parallel()

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runner:main-litert", "stdout", "runtime ready")
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              logs,
	})
	view := model.View()

	for _, expected := range []string{
		"System health",
		"Runtime topology",
		"Backend matrix",
		"Route map",
		"Recent activity",
		"configured runners",
		"runnable routes",
		"Sidecar API",
		"runtime upstream",
		"main-litert => http://127.0.0.1:9381",
		"embed-qwen",
		"chat=openai-compatible",
		"runtime ready",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("dashboard overview missing %q:\n%s", expected, view)
		}
	}
}

func TestDashboardRendersSignalBoardWithReadinessMeters(t *testing.T) {
	t.Parallel()

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runner:main-litert", "stdout", "runtime ready")
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              logs,
		Catalog:           testCatalog(t),
	})
	view := model.View()

	for _, expected := range []string{
		"Signal board / Readiness",
		"Runtime     ● running     [##########] serving",
		"Runners     ● 1/2 active  [#####-----] 1/2 running",
		"Routes      ● 2 wired     [##########] 2/2 routed",
		"Models      ● 4/4 present [##########] required ready",
		"Logs        ● 1 cached    latest: runner:main-litert/stdout",
		"Next action  open runner tab 2 main-litert or Models for downloads",
		"Legend      ● ready  ◐ partial  ! attention",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("dashboard signal board missing %q:\n%s", expected, view)
		}
	}
}

func TestDashboardRendersReadableRunnerBackendCards(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	view := model.View()

	for _, expected := range []string{
		"Runner backend cards",
		"● main-litert",
		"Runtime/Role:",
		"litert / main",
		"Backend:",
		"cpu",
		"Launch:",
		"managed by sidecar",
		"Health:",
		"[##########] serving",
		"Route:",
		"http://127.0.0.1:9381",
		"Caps:",
		"chat=openai-compatible",
		"◐ embed-qwen",
		"llamacpp / embedding",
		"[#####-----] ready to start",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("dashboard backend cards missing %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "main-litert |") {
		t.Fatalf("dashboard still renders dense pipe-delimited runner rows:\n%s", view)
	}
}

func TestDashboardRendersTopologyGraphWithRouteAuthority(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	view := model.View()

	for _, expected := range []string{
		"Topology graph",
		"Visual route authority",
		"◉ Browser UI",
		"api.request / ws://127.0.0.1:9379/sidecar/v1/ws",
		"◆ Sidecar API",
		"127.0.0.1:9379 /sidecar/v1/*",
		"◇ Runner supervisor",
		"routes=2 runners=2",
		"├─ embedding => ◐ embed-qwen",
		"│  http://127.0.0.1:9382",
		"└─ main => ● main-litert",
		"   http://127.0.0.1:9381",
		"Legend: ● running  ◐ configured  ! attention  ○ idle",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("dashboard topology graph missing %q:\n%s", expected, view)
		}
	}
}

func TestDashboardUsesWideTwoColumnLayout(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	model.width = 180
	model.height = 48
	view := model.View()

	if !viewLineContainsAll(view, "System health / Specs", "Topology graph / Visual route authority") {
		t.Fatalf("wide dashboard did not place health and topology graph in one row:\n%s", view)
	}
	if !viewLineContainsAll(view, "Runtime topology", "Backend matrix / Runnable backends") {
		t.Fatalf("wide dashboard did not place topology and backend matrix in one row:\n%s", view)
	}
	if !viewLineContainsAll(view, "Route map / Routes", "Recent activity") {
		t.Fatalf("wide dashboard did not place routes and recent activity in one row:\n%s", view)
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
		"p Port",
		"h Host",
		"i Model ID",
		"m Model path",
		"e Executable",
		"u Upstream",
		"f HF token",
		"l Launch",
		"v Verbose",
		"t Runtime",
		"o Role",
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
		"Verbose",
		"Upstream",
		"HF token",
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

func TestRunnerTabRendersRichOperationalPanels(t *testing.T) {
	t.Parallel()

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runner:main-litert", "stdout", "loaded model")
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              logs,
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := next.(Model)
	view := updated.View()

	for _, expected := range []string{
		"Runner health",
		"Endpoint map",
		"Control surface",
		"Runtime command",
		"Capabilities matrix",
		"Recent runner logs",
		"PATCH /sidecar/v1/runners/main-litert",
		"POST /sidecar/v1/runners/main-litert/start",
		"POST /sidecar/v1/runners/main-litert/restart",
		"/v1/chat/completions",
		"/sidecar/v1/runners/main-litert/stop",
		"chat=openai-compatible",
		"loaded model",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("runner operational panels missing %q:\n%s", expected, view)
		}
	}
}

func TestRunnerTabRendersSignalBoardWithRunnerReadiness(t *testing.T) {
	t.Parallel()

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runner:main-litert", "stdout", "loaded model")
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              logs,
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := next.(Model)
	view := updated.View()

	for _, expected := range []string{
		"Runner signal board / Readiness",
		"State       ● running     [##########] serving",
		"Route       ● main        /v1/chat/completions -> http://127.0.0.1:9381",
		"Process     ● pid 1234    managed by sidecar",
		"Model       ● gemma4-e2b  /models/litert/gemma-4-E2B-it.litertlm",
		"Caps        ● 2 advertised chat=openai-compatible, multimodal=litert-run",
		"Logs        ● seq 41      cached entries: 1",
		"Next action  use x/r for process control or edit b/p/h/i/m/e/u/f/l/v/t/o",
		"Legend      ● ready  ◐ configured  ! attention",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("runner signal board missing %q:\n%s", expected, view)
		}
	}
}

func TestRunnerTabUsesWideTwoColumnLayout(t *testing.T) {
	t.Parallel()

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runner:main-litert", "stdout", "loaded model")
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              logs,
	})
	model.width = 180
	model.height = 48
	model.setActiveTab("runner:main-litert")
	view := model.View()

	for _, tc := range []struct {
		left  string
		right string
	}{
		{left: "Runner main-litert / Runner health", right: "Runner signal board / Readiness"},
		{left: "Endpoint map", right: "Operation flow"},
		{left: "Control surface", right: "Runtime command"},
		{left: "Settings matrix", right: "Settings"},
		{left: "Details", right: "Recent runner logs"},
	} {
		if !viewLineContainsAll(view, tc.left, tc.right) {
			t.Fatalf("wide runner tab did not place %q beside %q:\n%s", tc.left, tc.right, view)
		}
	}
}

func TestRunnerTabShowsOperationFlowAndSharedMethodParity(t *testing.T) {
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
		"Operation flow",
		"● main-litert",
		"Model file -> Runtime -> Upstream -> Route",
		"Model file:    /models/litert/gemma-4-E2B-it.litertlm",
		"Runtime:       litert / main / cpu",
		"Upstream:      http://127.0.0.1:9381",
		"API route:     /v1/chat/completions",
		"Controller parity:",
		"RunnerController.StartRunner / StopRunner / RestartRunner / UpdateRunner",
		"WebSocket api.request parity:",
		"POST /sidecar/v1/runners/main-litert/start",
		"POST /sidecar/v1/runners/main-litert/stop",
		"POST /sidecar/v1/runners/main-litert/restart",
		"PATCH /sidecar/v1/runners/main-litert",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("runner operation flow missing %q:\n%s", expected, view)
		}
	}
}

func TestRunnerTabShowsSettingsMatrixWithPatchFields(t *testing.T) {
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
		"Settings matrix",
		"Key  Setting       Current                         Patch/API",
		"b    Backend       cpu                             backend -> RunnerController.UpdateRunner",
		"p    Port          9381                            port -> RunnerController.UpdateRunner",
		"h    Host          127.0.0.1                       host -> RunnerController.UpdateRunner",
		"i    Model ID      gemma4-e2b                      modelId -> RunnerController.UpdateRunner",
		"m    Model path    /models/litert/gemma-4-E2B-it.litertlm modelPath -> RunnerController.UpdateRunner",
		"e    Executable    /opt/litert-lm                  executable -> RunnerController.UpdateRunner",
		"u    Upstream      http://127.0.0.1:9381           upstream -> RunnerController.UpdateRunner",
		"f    HF token      not shown                       huggingFaceToken -> RunnerController.UpdateRunner",
		"l    Launch        managed by sidecar              launch -> RunnerController.UpdateRunner",
		"v    Verbose       false                           verbose -> RunnerController.UpdateRunner",
		"t    Runtime       litert                          runtime -> RunnerController.UpdateRunner",
		"o    Role          main                            role -> RunnerController.UpdateRunner",
		"PATCH /sidecar/v1/runners/main-litert",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("runner settings matrix missing %q:\n%s", expected, view)
		}
	}
}

func TestRunnerTabEditsPortThroughSharedRunnerController(t *testing.T) {
	t.Parallel()

	runners := testRunnerController()
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  runners,
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	updated := next.(Model)

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	updated = next.(Model)
	if !strings.Contains(updated.View(), "Editing Port for embed-qwen") {
		t.Fatalf("view missing port editor:\n%s", updated.View())
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9599")})
	updated = next.(Model)
	nextModel, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("port editor enter returned no command")
	}

	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if got := runners.lastCall(); got != "update:embed-qwen:port=9599" {
		t.Fatalf("last call = %q, want port update", got)
	}
	view := updated.View()
	if !strings.Contains(view, "updated embed-qwen port 9599") {
		t.Fatalf("view missing port update notice:\n%s", view)
	}
	if !strings.Contains(view, "Port:          9599") {
		t.Fatalf("view missing updated port:\n%s", view)
	}
}

func TestRunnerTabEditsModelIDThroughSharedRunnerController(t *testing.T) {
	t.Parallel()

	runners := testRunnerController()
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  runners,
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	updated := next.(Model)

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	updated = next.(Model)
	if !strings.Contains(updated.View(), "Editing Model ID for embed-qwen") {
		t.Fatalf("view missing model id editor:\n%s", updated.View())
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("custom-embedding")})
	updated = next.(Model)
	nextModel, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("model id editor enter returned no command")
	}

	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if got := runners.lastCall(); got != "update:embed-qwen:modelId=custom-embedding" {
		t.Fatalf("last call = %q, want model id update", got)
	}
	view := updated.View()
	if !strings.Contains(view, "updated embed-qwen modelId custom-embedding") {
		t.Fatalf("view missing model id update notice:\n%s", view)
	}
	if !strings.Contains(view, "Model ID:      custom-embedding") {
		t.Fatalf("view missing updated model id:\n%s", view)
	}
}

func TestRunnerTabEditsHuggingFaceTokenThroughSharedRunnerControllerWithoutRenderingSecret(t *testing.T) {
	t.Parallel()

	const secret = "hf_test_runner_secret"

	runners := testRunnerController()
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  runners,
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	updated := next.(Model)

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	updated = next.(Model)
	view := updated.View()
	if !strings.Contains(view, "Editing HF token for embed-qwen") {
		t.Fatalf("view missing HF token editor")
	}
	if strings.Contains(view, secret) {
		t.Fatalf("view leaked runner HF token before typing")
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(secret)})
	updated = next.(Model)
	if strings.Contains(updated.View(), secret) {
		t.Fatalf("view leaked runner HF token while editing")
	}

	nextModel, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("HF token editor enter returned no command")
	}

	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if got := runners.lastCall(); got != "update:embed-qwen:huggingFaceToken=set" {
		t.Fatalf("last call = %q, want masked HF token update", got)
	}
	if strings.Contains(runners.lastCall(), secret) {
		t.Fatalf("runner controller call leaked HF token")
	}
	view = updated.View()
	if strings.Contains(view, secret) {
		t.Fatalf("view leaked runner HF token after save")
	}
	if !strings.Contains(view, "updated embed-qwen huggingFaceToken set") {
		t.Fatalf("view missing masked HF token update notice:\n%s", view)
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

func TestRunnerTabTogglesLaunchAndVerboseThroughSharedRunnerController(t *testing.T) {
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
		Runes: []rune("l"),
	})
	if cmd == nil {
		t.Fatalf("launch edit returned no command")
	}

	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if got := runners.lastCall(); got != "update:embed-qwen:launch=false" {
		t.Fatalf("last call = %q, want launch update", got)
	}
	view := updated.View()
	if !strings.Contains(view, "updated embed-qwen launch external") {
		t.Fatalf("view missing launch update notice:\n%s", view)
	}
	if !strings.Contains(view, "external upstream") {
		t.Fatalf("view missing updated launch mode:\n%s", view)
	}

	nextModel, cmd = updated.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("v"),
	})
	if cmd == nil {
		t.Fatalf("verbose edit returned no command")
	}

	message = cmd()
	afterAction, _ = nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if got := runners.lastCall(); got != "update:embed-qwen:verbose=true" {
		t.Fatalf("last call = %q, want verbose update", got)
	}
	view = updated.View()
	if !strings.Contains(view, "updated embed-qwen verbose true") {
		t.Fatalf("view missing verbose update notice:\n%s", view)
	}
	if !strings.Contains(view, "Verbose:") || !strings.Contains(view, "true") {
		t.Fatalf("view missing updated verbose setting:\n%s", view)
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
	for _, expected := range []string{
		"Download models",
		"d Download next missing required model",
		"POST /sidecar/v1/models/download",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("models view missing download control %q:\n%s", expected, view)
		}
	}
}

func TestModelsTabDownloadsNextMissingRequiredModelThroughSharedCatalog(t *testing.T) {
	t.Parallel()

	var requestedPath string
	modelHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("content-length", "10")
		_, _ = w.Write([]byte("model-data"))
	}))
	defer modelHost.Close()

	modelCatalog := catalog.NewDefault(t.TempDir(), catalog.WithBaseURL(modelHost.URL))
	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           modelCatalog,
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	updated := next.(Model)

	nextModel, cmd := updated.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("d"),
	})
	if cmd == nil {
		t.Fatalf("download key returned no command")
	}

	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if requestedPath != "/unsloth/gemma-4-E2B-it-qat-GGUF/resolve/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf" {
		t.Fatalf("requested path = %q, want first missing catalog artifact", requestedPath)
	}
	view := updated.View()
	for _, expected := range []string{
		"downloaded model gemma4-gguf",
		"gemma4-gguf",
		"present",
		"10/10 B",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("models view missing download result %q:\n%s", expected, view)
		}
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
		"g Restart debug",
		"status.get",
		"runtime.start",
		"runtime.stop",
		"runtime.restart",
		"api.request POST /sidecar/v1/multimodal",
		"api.request * /v1/* upstream proxy",
		"api.request GET /sidecar/v1/runners",
		"api.request PATCH /sidecar/v1/runners/{id}",
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

func TestSettingsViewShowsRunnerAPIParityFromLiveSnapshot(t *testing.T) {
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
		"Runner API parity / Live snapshot",
		"Runner role/state/route comes from RunnerController.Snapshot()",
		"Routes: embedding -> embed-qwen, main -> main-litert",
		"main-litert  main       running  /v1/chat/completions",
		"TUI: s/x/r + b/p/h/i/m/e/u/f/l/v/t/o",
		"Controller: RunnerController.StartRunner/StopRunner/RestartRunner/UpdateRunner",
		"WS: api.request PATCH /sidecar/v1/runners/main-litert",
		"WS: api.request POST /sidecar/v1/runners/main-litert/start|stop|restart",
		"embed-qwen   embedding  created  /v1/embeddings",
		"WS: api.request PATCH /sidecar/v1/runners/embed-qwen",
		"WS: api.request POST /sidecar/v1/runners/embed-qwen/start|stop|restart",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("settings runner API parity missing %q:\n%s", expected, view)
		}
	}
}

func TestSettingsViewShowsSharedActionMethodMap(t *testing.T) {
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
		"Shared action map",
		"TUI key -> shared method -> WebSocket/API",
		"s Start release -> RuntimeController.Start(release) -> runtime.start",
		"d Start debug -> RuntimeController.Start(debug) -> runtime.start",
		"x Stop runtime -> RuntimeController.Stop() -> runtime.stop",
		"r Restart release -> RuntimeController.Restart(release) -> runtime.restart",
		"g Restart debug -> RuntimeController.Restart(debug) -> runtime.restart",
		"Runner s/x/r -> RunnerController.StartRunner/StopRunner/RestartRunner",
		"POST /sidecar/v1/runners/{id}/start|stop|restart",
		"Runner edits -> RunnerController.UpdateRunner -> PATCH /sidecar/v1/runners/{id}",
		"Models d -> Catalog.Download -> POST /sidecar/v1/models/download",
		"Models m/e/r -> RunnerController.CreateRunner -> POST /sidecar/v1/runners",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("settings action map missing %q:\n%s", expected, view)
		}
	}
}

func TestSettingsViewUsesWideTwoColumnLayout(t *testing.T) {
	t.Parallel()

	model := NewModel(ModelOptions{
		RuntimeController: testRuntimeController(),
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	model.width = 180
	model.height = 48
	model.setActiveTab("settings")
	view := model.View()

	for _, tc := range []struct {
		left  string
		right string
	}{
		{left: "Settings", right: "Runtime config editor"},
		{left: "Shared action map", right: "Runner API parity / Live snapshot"},
	} {
		if !viewLineContainsAll(view, tc.left, tc.right) {
			t.Fatalf("wide settings tab did not place %q beside %q:\n%s", tc.left, tc.right, view)
		}
	}
}

func TestSettingsViewShowsRuntimeConfigEditor(t *testing.T) {
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
		"Runtime config editor",
		"e Runtime exe",
		"h Runtime host",
		"p Runtime port",
		"m Model file",
		"i Model ID",
		"u Upstream",
		"f HF token",
		"l Launch runtime",
		"a Import model",
		"v Runtime verbose",
		"Runtime exe:",
		"Runtime host:",
		"Runtime port:",
		"Model file:",
		"Model ID:",
		"Upstream:",
		"HF token:",
		"Launch runtime:",
		"Import model:",
		"Runtime verbose:",
		"runtime.start config",
		"runtime.restart config",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("settings config editor missing %q:\n%s", expected, view)
		}
	}
}

func TestSettingsRuntimeConfigEditorSetsHuggingFaceTokenWithoutRenderingSecret(t *testing.T) {
	t.Parallel()

	const secret = "hf_test_runtime_secret"

	runtimeControl := testRuntimeController()
	model := NewModel(ModelOptions{
		RuntimeController: runtimeControl,
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	updated := next.(Model)

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	updated = next.(Model)
	view := updated.View()
	if !strings.Contains(view, "Editing HF token") {
		t.Fatalf("settings view missing HF token editor")
	}
	if strings.Contains(view, secret) {
		t.Fatalf("settings view leaked HF token before typing")
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(secret)})
	updated = next.(Model)
	if strings.Contains(updated.View(), secret) {
		t.Fatalf("settings view leaked HF token while editing")
	}

	nextModel, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("settings HF token edit unexpectedly returned command")
	}
	updated = nextModel.(Model)
	if strings.Contains(updated.View(), secret) {
		t.Fatalf("settings view leaked HF token after save")
	}

	nextModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatalf("start runtime returned no command")
	}
	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if got := runtimeControl.lastCall(); got != "start:release:huggingFaceToken=set" {
		t.Fatalf("last call = %q, want masked runtime HF token config", got)
	}
	if strings.Contains(runtimeControl.lastCall(), secret) {
		t.Fatalf("runtime controller call leaked HF token")
	}
	if strings.Contains(updated.View(), secret) {
		t.Fatalf("settings view leaked HF token after runtime start")
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
		{key: "g", wantCall: "restart:debug", wantToast: "restarted runtime debug"},
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

func TestSettingsRuntimeConfigEditorFeedsSharedRuntimeController(t *testing.T) {
	t.Parallel()

	runtimeControl := testRuntimeController()
	model := NewModel(ModelOptions{
		RuntimeController: runtimeControl,
		RunnerController:  testRunnerController(),
		Logs:              server.NewLogBroadcaster(8),
	})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	updated := next.(Model)

	for _, edit := range []struct {
		key   string
		value string
		label string
	}{
		{key: "p", value: "9499", label: "Runtime port"},
		{key: "u", value: "http://127.0.0.1:9499", label: "Upstream"},
		{key: "i", value: "gemma-custom", label: "Model ID"},
	} {
		next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(edit.key)})
		updated = next.(Model)
		if !strings.Contains(updated.View(), "Editing "+edit.label) {
			t.Fatalf("settings view missing editor for %q:\n%s", edit.label, updated.View())
		}
		next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(edit.value)})
		updated = next.(Model)
		nextModel, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil {
			t.Fatalf("settings edit %q unexpectedly returned command", edit.label)
		}
		updated = nextModel.(Model)
	}

	for _, key := range []string{"l", "v"} {
		next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		updated = next.(Model)
	}

	nextModel, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatalf("start runtime returned no command")
	}
	message := cmd()
	afterAction, _ := nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if got := runtimeControl.lastCall(); got != "start:release:runtimePort=9499:upstream=http://127.0.0.1:9499:modelId=gemma-custom:launchRuntime=false:runtimeVerbose=true" {
		t.Fatalf("last call = %q, want start with edited runtime config", got)
	}
	if !strings.Contains(updated.View(), "started runtime release") {
		t.Fatalf("view missing runtime start notice:\n%s", updated.View())
	}

	nextModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatalf("restart runtime returned no command")
	}
	message = cmd()
	afterAction, _ = nextModel.(Model).Update(message)
	updated = afterAction.(Model)

	if got := runtimeControl.lastCall(); got != "restart:release:runtimePort=9499:upstream=http://127.0.0.1:9499:modelId=gemma-custom:launchRuntime=false:runtimeVerbose=true" {
		t.Fatalf("last call = %q, want restart with edited runtime config", got)
	}
}

func testCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()

	root := t.TempDir()
	modelCatalog := catalog.NewDefault(root)
	for _, entry := range modelCatalog.Entries() {
		if err := os.MkdirAll(filepath.Dir(entry.TargetPath), 0o755); err != nil {
			t.Fatalf("create model fixture directory: %v", err)
		}
		if err := os.WriteFile(entry.TargetPath, []byte("x"), 0o644); err != nil {
			t.Fatalf("write model fixture: %v", err)
		}
	}
	return modelCatalog
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
	config server.RuntimeControlConfig,
) error {
	c.calls = append(c.calls, runtimeControlCall("start", mode, config))
	return nil
}

func (c *fakeRuntimeController) Stop(context.Context) error {
	c.calls = append(c.calls, "stop")
	return nil
}

func (c *fakeRuntimeController) Restart(
	_ context.Context,
	mode server.RuntimeMode,
	config server.RuntimeControlConfig,
) error {
	c.calls = append(c.calls, runtimeControlCall("restart", mode, config))
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

func maskedTokenState(value string) string {
	if strings.TrimSpace(value) == "" {
		return "cleared"
	}
	return "set"
}

func runtimeControlCall(
	action string,
	mode server.RuntimeMode,
	config server.RuntimeControlConfig,
) string {
	parts := []string{action, string(mode)}
	if config.RuntimeExe != "" {
		parts = append(parts, "runtimeExe="+config.RuntimeExe)
	}
	if config.RuntimeHost != "" {
		parts = append(parts, "runtimeHost="+config.RuntimeHost)
	}
	if config.RuntimePort > 0 {
		parts = append(parts, "runtimePort="+strconv.Itoa(config.RuntimePort))
	}
	if config.Upstream != "" {
		parts = append(parts, "upstream="+config.Upstream)
	}
	if config.ModelFile != "" {
		parts = append(parts, "modelFile="+config.ModelFile)
	}
	if config.ModelID != "" {
		parts = append(parts, "modelId="+config.ModelID)
	}
	if config.HuggingFaceToken != nil {
		parts = append(parts, "huggingFaceToken="+maskedTokenState(*config.HuggingFaceToken))
	}
	if config.ImportModel != nil {
		parts = append(parts, "importModel="+strconv.FormatBool(*config.ImportModel))
	}
	if config.LaunchRuntime != nil {
		parts = append(parts, "launchRuntime="+strconv.FormatBool(*config.LaunchRuntime))
	}
	if config.RuntimeVerbose != nil {
		parts = append(parts, "runtimeVerbose="+strconv.FormatBool(*config.RuntimeVerbose))
	}
	return strings.Join(parts, ":")
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
			Launch:     true,
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
			Launch:     true,
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
		Launch:    spec.Launch,
		Verbose:   spec.Verbose,
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
	if patch.Port > 0 {
		parts = append(parts, "port="+strconv.Itoa(patch.Port))
	}
	if patch.ModelID != "" {
		parts = append(parts, "modelId="+patch.ModelID)
	}
	if patch.HuggingFaceToken != nil {
		parts = append(parts, "huggingFaceToken="+maskedTokenState(*patch.HuggingFaceToken))
	}
	if patch.Launch != nil {
		parts = append(parts, "launch="+strconv.FormatBool(*patch.Launch))
	}
	if patch.Verbose != nil {
		parts = append(parts, "verbose="+strconv.FormatBool(*patch.Verbose))
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
	if patch.Launch != nil {
		runner.Launch = *patch.Launch
		if runner.Launch {
			runner.State = "created"
		} else {
			runner.State = "external"
		}
	}
	if patch.Verbose != nil {
		runner.Verbose = *patch.Verbose
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
