package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"g0litellama/internal/catalog"
	"g0litellama/internal/server"
	"g0litellama/internal/tui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, err := os.MkdirTemp("", "g0litellama-tui-fixture-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create fixture temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(root)

	modelCatalog, err := fixtureCatalog(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create fixture catalog: %v\n", err)
		os.Exit(1)
	}
	llamaRoot, err := fixtureLlamaRuntimeRoot(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create fixture llama runtime: %v\n", err)
		os.Exit(1)
	}

	model := tui.NewModel(tui.ModelOptions{
		RuntimeController: fakeRuntimeController{},
		RunnerController:  newFakeRunnerController(),
		Logs:              server.NewLogBroadcaster(128),
		Catalog:           modelCatalog,
		Context:           ctx,
		ManagedScreen:     true,
		LlamaRuntimeRoot:  llamaRoot,
		BackendConfigPath: filepath.Join(root, "backends.json"),
	})

	program := tea.NewProgram(model, tea.WithContext(ctx))
	if _, err := program.Run(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "run fixture TUI: %v\n", err)
		os.Exit(1)
	}
}

func fixtureCatalog(root string) (*catalog.Catalog, error) {
	modelCatalog := catalog.NewDefault(filepath.Join(root, "models"))
	for _, entry := range modelCatalog.Entries() {
		if err := os.MkdirAll(filepath.Dir(entry.TargetPath), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(entry.TargetPath, []byte("fixture"), 0o644); err != nil {
			return nil, err
		}
	}
	return modelCatalog, nil
}

func fixtureLlamaRuntimeRoot(root string) (string, error) {
	bin := filepath.Join(root, "llama-runtimes", "llama-cpu", "bin", "llama-server")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		return "", err
	}
	return filepath.Join(root, "llama-runtimes"), nil
}

type fakeRuntimeController struct{}

func (fakeRuntimeController) Start(context.Context, server.RuntimeMode, server.RuntimeControlConfig) error {
	return nil
}

func (fakeRuntimeController) Stop(context.Context) error {
	return nil
}

func (fakeRuntimeController) Restart(context.Context, server.RuntimeMode, server.RuntimeControlConfig) error {
	return nil
}

func (fakeRuntimeController) Status() server.RuntimeStatus {
	return server.RuntimeStatus{
		State:    "idle",
		Upstream: "fixture",
		Detail:   "fixture runtime only",
	}
}

type fakeRunnerController struct {
	mu      sync.Mutex
	runners []server.RunnerSnapshot
}

func newFakeRunnerController() *fakeRunnerController {
	return &fakeRunnerController{}
}

func (c *fakeRunnerController) Snapshot() server.RunnerSnapshotResponse {
	c.mu.Lock()
	defer c.mu.Unlock()

	runners := append([]server.RunnerSnapshot{}, c.runners...)
	routes := map[string]string{}
	for _, runner := range runners {
		if runner.State == "running" {
			routes[runner.Role] = runner.ID
		}
	}
	return server.RunnerSnapshotResponse{Runners: runners, Routes: routes}
}

func (c *fakeRunnerController) CreateRunner(
	_ context.Context,
	spec server.RunnerSpec,
) (server.RunnerSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	runner := snapshotFromSpec(spec, "created")
	c.runners = append(c.runners, runner)
	return runner, nil
}

func (c *fakeRunnerController) UpdateRunner(
	_ context.Context,
	id string,
	patch server.RunnerPatch,
) (server.RunnerSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	index := c.index(id)
	if index < 0 {
		return server.RunnerSnapshot{}, server.ErrRunnerNotFound
	}
	if patch.CommandLine != nil {
		c.runners[index].Command = strings.Fields(*patch.CommandLine)
	}
	return c.runners[index], nil
}

func (c *fakeRunnerController) StartRunner(_ context.Context, id string) (server.RunnerSnapshot, error) {
	return c.setState(id, "running")
}

func (c *fakeRunnerController) StopRunner(_ context.Context, id string) (server.RunnerSnapshot, error) {
	return c.setState(id, "stopped")
}

func (c *fakeRunnerController) RestartRunner(_ context.Context, id string) (server.RunnerSnapshot, error) {
	return c.setState(id, "running")
}

func (c *fakeRunnerController) CloseRunner(_ context.Context, id string) (server.RunnerSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	index := c.index(id)
	if index < 0 {
		return server.RunnerSnapshot{}, server.ErrRunnerNotFound
	}
	runner := c.runners[index]
	c.runners = append(c.runners[:index], c.runners[index+1:]...)
	return runner, nil
}

func (c *fakeRunnerController) setState(id string, state string) (server.RunnerSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	index := c.index(id)
	if index < 0 {
		return server.RunnerSnapshot{}, server.ErrRunnerNotFound
	}
	c.runners[index].State = state
	return c.runners[index], nil
}

func (c *fakeRunnerController) index(id string) int {
	for index, runner := range c.runners {
		if runner.ID == id {
			return index
		}
	}
	return -1
}

func snapshotFromSpec(spec server.RunnerSpec, state string) server.RunnerSnapshot {
	return server.RunnerSnapshot{
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
		Verbose:    spec.Verbose,
		State:      state,
		Upstream:   spec.Upstream,
		Command:    append([]string{}, spec.Command...),
	}
}
