package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"litert-sidecar/internal/server"
	"litert-sidecar/internal/supervisor"
)

func TestModelRendersRequiredTabs(t *testing.T) {
	t.Parallel()

	model := NewModel(testSupervisor(), server.NewLogBroadcaster(8))
	view := model.View()

	for _, label := range []string{
		"Dashboard",
		"Runners",
		"Launch Wizard",
		"Chat",
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

func TestModelSwitchesTabsWithKeys(t *testing.T) {
	t.Parallel()

	model := NewModel(testSupervisor(), server.NewLogBroadcaster(8))
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := next.(Model)

	if updated.activeTabID() != "runners" {
		t.Fatalf("active tab = %q, want runners", updated.activeTabID())
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	updated = next.(Model)
	if updated.activeTabID() != "models" {
		t.Fatalf("active tab = %q, want models", updated.activeTabID())
	}
}

func TestModelLogsViewShowsRecentEntries(t *testing.T) {
	t.Parallel()

	logs := server.NewLogBroadcaster(8)
	logs.Publish("runner:main-litert", "stdout", "runtime ready")
	model := NewModel(testSupervisor(), logs)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	updated := next.(Model)
	view := updated.View()

	if !strings.Contains(view, "runtime ready") {
		t.Fatalf("logs view missing log entry:\n%s", view)
	}
}

func testSupervisor() *supervisor.Supervisor {
	return supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch:   false,
			Host:     "127.0.0.1",
			Port:     9381,
			ModelID:  "gemma4-e2b",
			Upstream: "http://127.0.0.1:9381",
		},
	})
}
