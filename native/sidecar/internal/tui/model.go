package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"litert-sidecar/internal/catalog"
	"litert-sidecar/internal/server"
	"litert-sidecar/internal/supervisor"
)

const refreshInterval = time.Second

type tab struct {
	id    string
	label string
}

var tabs = []tab{
	{id: "dashboard", label: "Dashboard"},
	{id: "runners", label: "Runners"},
	{id: "launch", label: "Launch Wizard"},
	{id: "chat", label: "Chat"},
	{id: "models", label: "Models"},
	{id: "logs", label: "Logs"},
	{id: "settings", label: "Settings"},
}

type tickMsg time.Time

type Model struct {
	supervisor *supervisor.Supervisor
	logs       *server.LogBroadcaster
	catalog    *catalog.Catalog
	active     int
	width      int
	height     int
	snapshot   supervisor.Snapshot
	runtime    supervisor.RuntimeStatus
	logEntries []server.LogEntry
	models     []catalog.Entry
}

func NewModel(
	runtimeSupervisor *supervisor.Supervisor,
	logs *server.LogBroadcaster,
	modelCatalog ...*catalog.Catalog,
) Model {
	model := Model{
		supervisor: runtimeSupervisor,
		logs:       logs,
		active:     0,
	}
	if len(modelCatalog) > 0 {
		model.catalog = modelCatalog[0]
	}
	model.refresh()
	return model
}

func Run(
	ctx context.Context,
	runtimeSupervisor *supervisor.Supervisor,
	logs *server.LogBroadcaster,
	modelCatalog *catalog.Catalog,
) error {
	program := tea.NewProgram(
		NewModel(runtimeSupervisor, logs, modelCatalog),
		tea.WithContext(ctx),
	)
	_, err := program.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tick()
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyRight, tea.KeyTab:
			m.active = (m.active + 1) % len(tabs)
		case tea.KeyLeft, tea.KeyShiftTab:
			m.active = (m.active + len(tabs) - 1) % len(tabs)
		case tea.KeyRunes:
			m.selectRuneTab(msg.String())
		}
	case tickMsg:
		m.refresh()
		return m, tick()
	}

	return m, nil
}

func (m Model) View() string {
	var builder strings.Builder
	builder.WriteString("LiteRT sidecar\n")
	builder.WriteString(m.tabBar())
	builder.WriteString("\n\n")

	switch m.activeTabID() {
	case "dashboard":
		builder.WriteString(m.dashboardView())
	case "runners":
		builder.WriteString(m.runnersView())
	case "launch":
		builder.WriteString(m.launchView())
	case "chat":
		builder.WriteString(m.chatView())
	case "models":
		builder.WriteString(m.modelsView())
	case "logs":
		builder.WriteString(m.logsView())
	case "settings":
		builder.WriteString(m.settingsView())
	}

	return builder.String()
}

func (m Model) activeTabID() string {
	if m.active < 0 || m.active >= len(tabs) {
		return tabs[0].id
	}
	return tabs[m.active].id
}

func (m *Model) refresh() {
	if m.supervisor != nil {
		m.snapshot = m.supervisor.Snapshot()
		m.runtime = m.supervisor.LegacyStatus()
	}
	if m.logs != nil {
		m.logEntries = m.logs.Snapshot()
	}
	if m.catalog != nil {
		m.models = m.catalog.Entries()
	}
}

func (m *Model) selectRuneTab(value string) {
	switch value {
	case "1":
		m.active = 0
	case "2":
		m.active = 1
	case "3":
		m.active = 2
	case "4":
		m.active = 3
	case "5":
		m.active = 4
	case "6":
		m.active = 5
	case "7":
		m.active = 6
	}
}

func (m Model) tabBar() string {
	parts := make([]string, 0, len(tabs))
	for index, item := range tabs {
		label := fmt.Sprintf("%d %s", index+1, item.label)
		if index == m.active {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

func (m Model) dashboardView() string {
	var builder strings.Builder
	builder.WriteString("Dashboard\n")
	builder.WriteString(fmt.Sprintf("Runtime: %s\n", fallback(m.runtime.State, "unknown")))
	builder.WriteString(fmt.Sprintf("Model: %s\n", fallback(m.runtime.ModelID, "unconfigured")))
	builder.WriteString(fmt.Sprintf("Upstream: %s\n", fallback(m.runtime.Upstream, "unavailable")))
	builder.WriteString(fmt.Sprintf("Logs: %d entries\n", len(m.logEntries)))
	return builder.String()
}

func (m Model) runnersView() string {
	var builder strings.Builder
	builder.WriteString("Runners\n")
	if len(m.snapshot.Runners) == 0 {
		builder.WriteString("No runners configured.\n")
		return builder.String()
	}
	for _, runner := range m.snapshot.Runners {
		builder.WriteString(fmt.Sprintf(
			"- %s %s/%s %s %s\n",
			runner.ID,
			runner.Runtime,
			runner.Role,
			runner.State,
			fallback(runner.Upstream, "no upstream"),
		))
	}
	return builder.String()
}

func (m Model) launchView() string {
	return "Launch Wizard\nRole, runtime, backend, model, port, and advanced arguments will be launched through the supervisor.\n"
}

func (m Model) chatView() string {
	return "Chat\nSelect a running main runner to send OpenAI-compatible chat requests.\n"
}

func (m Model) modelsView() string {
	var builder strings.Builder
	builder.WriteString("Models\n")
	if len(m.models) == 0 {
		builder.WriteString("No model catalog configured.\n")
		return builder.String()
	}
	for _, entry := range m.models {
		builder.WriteString(fmt.Sprintf(
			"- %s %s %s\n",
			entry.ID,
			entry.State,
			entry.TargetPath,
		))
	}
	return builder.String()
}

func (m Model) logsView() string {
	var builder strings.Builder
	builder.WriteString("Logs\n")
	if len(m.logEntries) == 0 {
		builder.WriteString("No logs yet.\n")
		return builder.String()
	}

	start := len(m.logEntries) - 12
	if start < 0 {
		start = 0
	}
	for _, entry := range m.logEntries[start:] {
		builder.WriteString(fmt.Sprintf(
			"#%d %s %s %s\n",
			entry.Seq,
			entry.Source,
			entry.Stream,
			entry.Line,
		))
	}
	return builder.String()
}

func (m Model) settingsView() string {
	return "Settings\nUse --headless to run only the HTTP/WebSocket sidecar for browser automation.\n"
}

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}
