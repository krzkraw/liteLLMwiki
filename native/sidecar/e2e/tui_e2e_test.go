package e2e

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"litert-sidecar/internal/catalog"
	"litert-sidecar/internal/server"
	"litert-sidecar/internal/tui"
)

func TestTUIStartsAndCreatesRunnerThroughLaunchWizard(t *testing.T) {
	t.Parallel()

	controller := newHarnessRunnerController()
	modelCatalog := harnessCatalogWithPresentModels(t, "gemma4-litert")
	model := tui.NewModel(tui.ModelOptions{
		RunnerController: controller,
		Logs:             server.NewLogBroadcaster(8),
		Catalog:          modelCatalog,
	})

	assertBubbleTeaProgramStarts(t, model)

	next, cmd := model.Update(tea.WindowSizeMsg{Width: 140, Height: 36})
	if cmd != nil {
		t.Fatalf("window size update returned unexpected command")
	}
	model = next.(tui.Model)
	next, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if cmd != nil {
		t.Fatalf("wizard tab key returned unexpected command")
	}
	model = next.(tui.Model)
	if !strings.Contains(model.View(), "Launch Wizard") {
		t.Fatalf("wizard tab did not render:\n%s", model.View())
	}

	next, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("wizard enter returned no create command")
	}
	message := cmd()
	afterCreate, _ := next.(tui.Model).Update(message)
	model = afterCreate.(tui.Model)

	if got := strings.Join(controller.calls, ","); got != "create:LR-M-1:litert:main:gemma4-litert,start:LR-M-1" {
		t.Fatalf("controller calls = %q", got)
	}
	if !strings.Contains(model.View(), "4 ● LR-M-1") {
		t.Fatalf("created runner tab did not render:\n%s", model.View())
	}
}

func assertBubbleTeaProgramStarts(t *testing.T, model tui.Model) {
	t.Helper()

	program := tea.NewProgram(
		model,
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
	)
	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()

	program.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	time.Sleep(25 * time.Millisecond)
	program.Quit()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("start Bubble Tea program: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Bubble Tea program did not stop after start/quit")
	}
}

func harnessCatalogWithPresentModels(t *testing.T, ids ...string) *catalog.Catalog {
	t.Helper()

	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}

	root := t.TempDir()
	modelCatalog := catalog.NewDefault(root)
	for _, entry := range modelCatalog.Entries() {
		if !wanted[entry.ID] {
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

type harnessRunnerController struct {
	runners []server.RunnerSnapshot
	calls   []string
}

func newHarnessRunnerController() *harnessRunnerController {
	return &harnessRunnerController{}
}

func (c *harnessRunnerController) Snapshot() server.RunnerSnapshotResponse {
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

func (c *harnessRunnerController) CreateRunner(
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

func (c *harnessRunnerController) UpdateRunner(
	_ context.Context,
	id string,
	_ server.RunnerPatch,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "update:"+id)
	return server.RunnerSnapshot{}, nil
}

func (c *harnessRunnerController) StartRunner(
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

func (c *harnessRunnerController) StopRunner(
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "stop:"+id)
	return server.RunnerSnapshot{}, nil
}

func (c *harnessRunnerController) RestartRunner(
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "restart:"+id)
	return server.RunnerSnapshot{}, nil
}

func (c *harnessRunnerController) CloseRunner(
	_ context.Context,
	id string,
) (server.RunnerSnapshot, error) {
	c.calls = append(c.calls, "close:"+id)
	return server.RunnerSnapshot{}, nil
}
