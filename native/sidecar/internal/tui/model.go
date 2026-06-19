package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"litert-sidecar/internal/catalog"
	"litert-sidecar/internal/server"
)

const refreshInterval = time.Second

type tab struct {
	id    string
	label string
}

type ModelOptions struct {
	RuntimeController server.RuntimeController
	RunnerController  server.RunnerController
	Logs              *server.LogBroadcaster
	Catalog           *catalog.Catalog
	Context           context.Context
}

type tickMsg time.Time

type runnerActionMsg struct {
	action string
	id     string
	err    error
}

type runtimeActionMsg struct {
	action string
	mode   server.RuntimeMode
	err    error
}

type runnerCreateMsg struct {
	label  string
	runner server.RunnerSnapshot
	err    error
}

type runnerPreset struct {
	id      string
	role    string
	modelID string
	port    int
}

type Model struct {
	runtimeController server.RuntimeController
	runnerController  server.RunnerController
	logs              *server.LogBroadcaster
	catalog           *catalog.Catalog
	ctx               context.Context

	active     int
	width      int
	height     int
	snapshot   server.RunnerSnapshotResponse
	runtime    server.RuntimeStatus
	logEntries []server.LogEntry
	models     []catalog.Entry
	notice     string
}

var asciiBorder = lipgloss.Border{
	Top:         "-",
	Bottom:      "-",
	Left:        "|",
	Right:       "|",
	TopLeft:     "+",
	TopRight:    "+",
	BottomLeft:  "+",
	BottomRight: "+",
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("45"))
	subtitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))
	tabStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("236"))
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(lipgloss.Color("16")).
			Background(lipgloss.Color("39"))
	noticeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("16")).
			Background(lipgloss.Color("82")).
			Padding(0, 1)
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
)

func NewModel(options ModelOptions) Model {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	model := Model{
		runtimeController: options.RuntimeController,
		runnerController:  options.RunnerController,
		logs:              options.Logs,
		catalog:           options.Catalog,
		ctx:               ctx,
		active:            0,
	}
	model.refresh()
	return model
}

func Run(
	ctx context.Context,
	runtimeController server.RuntimeController,
	runnerController server.RunnerController,
	logs *server.LogBroadcaster,
	modelCatalog *catalog.Catalog,
) error {
	program := tea.NewProgram(
		NewModel(ModelOptions{
			RuntimeController: runtimeController,
			RunnerController:  runnerController,
			Logs:              logs,
			Catalog:           modelCatalog,
			Context:           ctx,
		}),
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
		return m.updateKey(msg)
	case tickMsg:
		m.refresh()
		return m, tick()
	case runnerActionMsg:
		m.refresh()
		m.notice = m.actionNotice(msg)
	case runtimeActionMsg:
		m.refresh()
		m.notice = m.runtimeActionNotice(msg)
	case runnerCreateMsg:
		m.refresh()
		m.setActiveTab("models")
		m.notice = m.runnerCreateNotice(msg)
	}

	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyRight, tea.KeyTab:
		m.active = (m.active + 1) % len(m.tabs())
		return m, nil
	case tea.KeyLeft, tea.KeyShiftTab:
		m.active = (m.active + len(m.tabs()) - 1) % len(m.tabs())
		return m, nil
	case tea.KeyRunes:
		value := msg.String()
		if m.selectRuneTab(value) {
			return m, nil
		}
		if m.activeTabID() == "settings" {
			switch strings.ToLower(value) {
			case "s":
				return m, m.runtimeActionCmd("start", server.RuntimeModeRelease)
			case "d":
				return m, m.runtimeActionCmd("start", server.RuntimeModeDebug)
			case "x":
				return m, m.runtimeActionCmd("stop", "")
			case "r":
				return m, m.runtimeActionCmd("restart", server.RuntimeModeRelease)
			}
		}
		if m.activeTabID() == "models" {
			switch strings.ToLower(value) {
			case "m":
				return m, m.runnerCreateCmd("main")
			case "e":
				return m, m.runnerCreateCmd("embedding")
			case "r":
				return m, m.runnerCreateCmd("reranking")
			}
		}
		if runner, ok := m.activeRunner(); ok {
			switch strings.ToLower(value) {
			case "s":
				return m, m.runnerActionCmd("start", runner.ID)
			case "x":
				return m, m.runnerActionCmd("stop", runner.ID)
			case "r":
				return m, m.runnerActionCmd("restart", runner.ID)
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	var builder strings.Builder
	builder.WriteString(titleStyle.Render("LiteRT sidecar"))
	if m.width > 0 || m.height > 0 {
		builder.WriteString(mutedStyle.Render(fmt.Sprintf("  %dx%d", m.width, m.height)))
	}
	builder.WriteString("\n")
	builder.WriteString(m.tabBar())
	builder.WriteString("\n\n")
	if strings.TrimSpace(m.notice) != "" {
		builder.WriteString(noticeStyle.Render(m.notice))
		builder.WriteString("\n\n")
	}

	switch m.activeTabID() {
	case "dashboard":
		builder.WriteString(m.dashboardView())
	case "models":
		builder.WriteString(m.modelsView())
	case "logs":
		builder.WriteString(m.logsView())
	case "settings":
		builder.WriteString(m.settingsView())
	default:
		if runner, ok := m.activeRunner(); ok {
			builder.WriteString(m.runnerView(runner))
		} else {
			builder.WriteString(renderPanel("Runner", []string{"No runner selected."}, "196"))
		}
	}

	return builder.String()
}

func (m Model) activeTabID() string {
	tabs := m.tabs()
	if len(tabs) == 0 {
		return "dashboard"
	}
	if m.active < 0 || m.active >= len(tabs) {
		return tabs[0].id
	}
	return tabs[m.active].id
}

func (m Model) tabs() []tab {
	result := []tab{{id: "dashboard", label: "Dashboard"}}
	for _, runner := range m.snapshot.Runners {
		label := runner.ID
		if len(label) > 18 {
			label = label[:17] + "."
		}
		result = append(result, tab{
			id:    "runner:" + runner.ID,
			label: label,
		})
	}
	result = append(
		result,
		tab{id: "models", label: "Models"},
		tab{id: "logs", label: "Logs"},
		tab{id: "settings", label: "Settings"},
	)
	return result
}

func (m *Model) refresh() {
	if m.runtimeController != nil {
		m.runtime = m.runtimeController.Status()
	}
	if m.runnerController != nil {
		m.snapshot = m.runnerController.Snapshot()
	}
	if m.logs != nil {
		m.logEntries = m.logs.Snapshot()
	}
	if m.catalog != nil {
		m.models = m.catalog.Entries()
	}
	if m.active >= len(m.tabs()) {
		m.active = 0
	}
}

func (m *Model) selectRuneTab(value string) bool {
	if len(value) != 1 {
		return false
	}
	index := int(value[0] - '1')
	if index < 0 || index >= len(m.tabs()) {
		return false
	}
	m.active = index
	return true
}

func (m Model) activeRunner() (server.RunnerSnapshot, bool) {
	id := strings.TrimPrefix(m.activeTabID(), "runner:")
	if id == m.activeTabID() {
		return server.RunnerSnapshot{}, false
	}
	for _, runner := range m.snapshot.Runners {
		if runner.ID == id {
			return runner, true
		}
	}
	return server.RunnerSnapshot{}, false
}

func (m *Model) setActiveTab(id string) {
	for index, item := range m.tabs() {
		if item.id == id {
			m.active = index
			return
		}
	}
}

func (m Model) tabBar() string {
	tabs := m.tabs()
	parts := make([]string, 0, len(tabs))
	for index, item := range tabs {
		label := fmt.Sprintf("%d %s", index+1, item.label)
		if index == m.active {
			parts = append(parts, activeTabStyle.Render(label))
			continue
		}
		parts = append(parts, tabStyle.Render(label))
	}
	return strings.Join(parts, " ")
}

func (m Model) dashboardView() string {
	specs := []string{
		formatKV("State", statusBadge(m.runtime.State)),
		formatKV("Executable", fallback(m.runtime.Executable, "not configured")),
		formatKV("Version", fallback(m.runtime.Version, "unknown")),
		formatKV("Model", fallback(m.runtime.ModelID, "unconfigured")),
		formatKV("Model file", fallback(m.runtime.ModelFile, "not configured")),
		formatKV("Upstream", fallback(m.runtime.Upstream, "unavailable")),
		formatKV("Mode", fallback(m.runtime.Mode, "release")),
		formatKV("Logs", fmt.Sprintf("%d entries", len(m.logEntries))),
	}
	if m.runtime.Detail != "" {
		specs = append(specs, formatKV("Detail", m.runtime.Detail))
	}

	return strings.Join([]string{
		renderPanel("Dashboard", []string{
			"Specs",
			strings.Join(specs, "\n"),
		}, "45"),
		renderPanel("Runnable backends", m.backendLines(), "82"),
		renderPanel("Routes", m.routeLines(), "214"),
		renderPanel("Hotkeys", []string{
			"Tab/Right: next tab",
			"Shift+Tab/Left: previous tab",
			"Number keys: jump tabs",
			"Runner tabs: s Start, x Stop, r Restart",
			"Esc/Ctrl+C: quit",
		}, "205"),
	}, "\n\n")
}

func (m Model) backendLines() []string {
	if len(m.snapshot.Runners) == 0 {
		return []string{"No runners configured."}
	}

	lines := make([]string, 0, len(m.snapshot.Runners))
	for _, runner := range m.snapshot.Runners {
		lines = append(lines, fmt.Sprintf(
			"%s  %s/%s  %s  %s",
			statusBadge(runner.State),
			fallback(runner.Runtime, "runtime"),
			fallback(runner.Role, "role"),
			fallback(runner.Backend, "backend"),
			runner.ID,
		))
	}
	return lines
}

func (m Model) routeLines() []string {
	if len(m.snapshot.Routes) == 0 {
		return []string{"No route authority configured."}
	}

	keys := make([]string, 0, len(m.snapshot.Routes))
	for key := range m.snapshot.Routes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s -> %s", key, m.snapshot.Routes[key]))
	}
	return lines
}

func (m Model) runnerView(runner server.RunnerSnapshot) string {
	settings := []string{
		formatKV("Runtime", fallback(runner.Runtime, "unknown")),
		formatKV("Role", fallback(runner.Role, "unknown")),
		formatKV("Backend", fallback(runner.Backend, "default")),
		formatKV("Executable", fallback(runner.Executable, "auto-discover")),
		formatKV("Version", fallback(runner.Version, "unknown")),
		formatKV("Model path", fallback(runner.ModelPath, "not configured")),
		formatKV("Model ID", fallback(runner.ModelID, "not configured")),
		formatKV("Host", fallback(runner.Host, "127.0.0.1")),
		formatKV("Port", fallbackInt(runner.Port, "auto")),
		formatKV("Launch", runnerLaunchMode(runner)),
		formatKV("Upstream", fallback(runner.Upstream, "unavailable")),
	}

	details := []string{
		formatKV("State", statusBadge(runner.State)),
		formatKV("PID", fallbackInt(runner.PID, "not running")),
		formatKV("Command", commandLine(runner.Command)),
		formatKV("Capabilities", capabilitiesLine(runner.Capabilities)),
		formatKV("Last error", fallback(runner.LastError, "none")),
		formatKV("Log sequence", fallbackUint(runner.LogSequence, "none")),
	}
	if runner.Detail != "" {
		details = append(details, formatKV("Detail", runner.Detail))
	}

	return strings.Join([]string{
		renderPanel("Runner "+runner.ID, []string{
			"Controls",
			"s Start   x Stop   r Restart",
		}, "39"),
		renderPanel("Settings", settings, "45"),
		renderPanel("Details", details, "214"),
	}, "\n\n")
}

func (m Model) modelsView() string {
	lines := []string{
		"Create runners",
		"m Main llama.cpp   e Embedding llama.cpp   r Rerank llama.cpp",
		"",
	}
	if len(m.models) == 0 {
		lines = append(lines, "No model catalog configured.")
	} else {
		for _, entry := range m.models {
			lines = append(lines, fmt.Sprintf(
				"%s  %s  %s",
				statusBadge(string(entry.State)),
				entry.ID,
				fallback(entry.TargetPath, "no target path"),
			))
		}
	}
	lines = append(lines, "", "Download actions use POST /sidecar/v1/models/download through api.request.")
	return renderPanel("Models", lines, "82")
}

func (m Model) logsView() string {
	if len(m.logEntries) == 0 {
		return renderPanel("Logs", []string{"No logs yet."}, "244")
	}

	start := len(m.logEntries) - 16
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, len(m.logEntries[start:]))
	for _, entry := range m.logEntries[start:] {
		lines = append(lines, fmt.Sprintf(
			"#%d %s %s %s",
			entry.Seq,
			entry.Source,
			entry.Stream,
			entry.Line,
		))
	}
	return renderPanel("Logs", lines, "244")
}

func (m Model) settingsView() string {
	runtimeState := "not configured"
	if m.runtimeController != nil {
		runtimeState = "RuntimeController connected"
	}
	runnerState := "not configured"
	if m.runnerController != nil {
		runnerState = "RunnerController connected"
	}

	return strings.Join([]string{
		renderPanel("Settings", []string{
			"Controls",
			"s Start release   d Start debug   r Restart release   x Stop runtime",
			"",
			formatKV("Runtime controller", runtimeState),
			formatKV("Runner controller", runnerState),
			formatKV("HTTP listen", "configured by -addr"),
			formatKV("Default upstream", fallback(m.runtime.Upstream, "unavailable")),
			formatKV("Runtime mode", fallback(m.runtime.Mode, "release")),
			formatKV("Log entries", fmt.Sprintf("%d cached", len(m.logEntries))),
		}, "45"),
		renderPanel("WebSocket API parity", []string{
			"status.get",
			"runtime.start",
			"runtime.stop",
			"runtime.restart",
			"api.request GET /sidecar/v1/status",
			"api.request GET /sidecar/v1/models",
			"api.request POST /sidecar/v1/models/download",
			"api.request GET /sidecar/v1/runners",
			"api.request POST /sidecar/v1/runners",
			"api.request POST /sidecar/v1/runners/{id}/start",
			"api.request POST /sidecar/v1/runners/{id}/stop",
			"api.request POST /sidecar/v1/runners/{id}/restart",
			"TUI controls call the same methods underneath: RuntimeController and RunnerController.",
		}, "205"),
	}, "\n\n")
}

func (m Model) runnerActionCmd(action string, id string) tea.Cmd {
	return func() tea.Msg {
		if m.runnerController == nil {
			return runnerActionMsg{
				action: action,
				id:     id,
				err:    fmt.Errorf("runner controller is not configured"),
			}
		}

		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()

		var err error
		switch action {
		case "start":
			_, err = m.runnerController.StartRunner(ctx, id)
		case "stop":
			_, err = m.runnerController.StopRunner(ctx, id)
		case "restart":
			_, err = m.runnerController.RestartRunner(ctx, id)
		default:
			err = fmt.Errorf("unknown runner action %q", action)
		}
		return runnerActionMsg{
			action: action,
			id:     id,
			err:    err,
		}
	}
}

func (m Model) runtimeActionCmd(action string, mode server.RuntimeMode) tea.Cmd {
	return func() tea.Msg {
		if m.runtimeController == nil {
			return runtimeActionMsg{
				action: action,
				mode:   mode,
				err:    fmt.Errorf("runtime controller is not configured"),
			}
		}

		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()

		var err error
		switch action {
		case "start":
			err = m.runtimeController.Start(ctx, mode, server.RuntimeControlConfig{})
		case "stop":
			err = m.runtimeController.Stop(ctx)
		case "restart":
			err = m.runtimeController.Restart(ctx, mode, server.RuntimeControlConfig{})
		default:
			err = fmt.Errorf("unknown runtime action %q", action)
		}
		return runtimeActionMsg{
			action: action,
			mode:   mode,
			err:    err,
		}
	}
}

func (m Model) runnerCreateCmd(role string) tea.Cmd {
	return func() tea.Msg {
		if m.runnerController == nil {
			return runnerCreateMsg{
				label: role,
				err:   fmt.Errorf("runner controller is not configured"),
			}
		}

		spec, label, err := m.runnerSpecForRole(role)
		if err != nil {
			return runnerCreateMsg{
				label: label,
				err:   err,
			}
		}

		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()

		runner, err := m.runnerController.CreateRunner(ctx, spec)
		return runnerCreateMsg{
			label:  label,
			runner: runner,
			err:    err,
		}
	}
}

func (m Model) actionNotice(message runnerActionMsg) string {
	if message.err != nil {
		return fmt.Sprintf("%s %s failed: %v", message.action, message.id, message.err)
	}
	switch message.action {
	case "start":
		return "started " + message.id
	case "stop":
		return "stopped " + message.id
	case "restart":
		return "restarted " + message.id
	default:
		return message.action + " " + message.id
	}
}

func (m Model) runnerCreateNotice(message runnerCreateMsg) string {
	if message.err != nil {
		return fmt.Sprintf("create %s runner failed: %v", message.label, message.err)
	}
	return "created runner " + message.runner.ID
}

func (m Model) runtimeActionNotice(message runtimeActionMsg) string {
	if message.err != nil {
		return fmt.Sprintf("%s runtime failed: %v", message.action, message.err)
	}
	switch message.action {
	case "start":
		return "started runtime " + string(message.mode)
	case "stop":
		return "stopped runtime"
	case "restart":
		return "restarted runtime " + string(message.mode)
	default:
		return message.action + " runtime"
	}
}

func (m Model) runnerSpecForRole(role string) (server.RunnerSpec, string, error) {
	switch role {
	case "main":
		entry, ok := m.catalogEntry("gemma4-gguf")
		if !ok {
			return server.RunnerSpec{}, role, fmt.Errorf("catalog entry gemma4-gguf is not available")
		}
		return catalogRunnerSpec(entry, runnerPreset{
			id:      "main-llamacpp",
			role:    "main",
			modelID: "gemma4-gguf",
			port:    9482,
		}), role, nil
	case "embedding":
		entry, ok := m.catalogEntry("qwen3-embedding-gguf")
		if !ok {
			return server.RunnerSpec{}, role, fmt.Errorf("catalog entry qwen3-embedding-gguf is not available")
		}
		return catalogRunnerSpec(entry, runnerPreset{
			id:      "embedding-llamacpp",
			role:    "embedding",
			modelID: "qwen3-embedding",
			port:    9483,
		}), role, nil
	case "reranking":
		entry, ok := m.catalogEntry("qwen3-embedding-gguf")
		if !ok {
			return server.RunnerSpec{}, role, fmt.Errorf("catalog entry qwen3-embedding-gguf is not available")
		}
		return catalogRunnerSpec(entry, runnerPreset{
			id:      "rerank-llamacpp",
			role:    "reranking",
			modelID: "qwen3-rerank-probe",
			port:    9484,
		}), role, nil
	default:
		return server.RunnerSpec{}, role, fmt.Errorf("unknown runner role %q", role)
	}
}

func (m Model) catalogEntry(id string) (catalog.Entry, bool) {
	for _, entry := range m.models {
		if entry.ID == id {
			return entry, true
		}
	}
	return catalog.Entry{}, false
}

func catalogRunnerSpec(entry catalog.Entry, preset runnerPreset) server.RunnerSpec {
	return server.RunnerSpec{
		ID:        preset.id,
		Runtime:   entry.Runtime,
		Role:      preset.role,
		Backend:   "cpu",
		ModelPath: entry.TargetPath,
		ModelID:   preset.modelID,
		Host:      "127.0.0.1",
		Port:      preset.port,
		Launch:    true,
	}
}

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func renderPanel(title string, lines []string, color string) string {
	body := strings.Join(lines, "\n")
	if strings.TrimSpace(body) == "" {
		body = "No data."
	}
	return lipgloss.NewStyle().
		Border(asciiBorder).
		BorderForeground(lipgloss.Color(color)).
		Padding(0, 1).
		Render(subtitleStyle.Render(title) + "\n" + body)
}

func formatKV(label string, value string) string {
	return fmt.Sprintf("%-14s %s", label+":", value)
}

func statusBadge(state string) string {
	state = fallback(state, "unknown")
	color := "244"
	switch strings.ToLower(state) {
	case "running", "available", "ready":
		color = "82"
	case "starting", "created":
		color = "214"
	case "external":
		color = "45"
	case "stopped", "exited":
		color = "244"
	case "unavailable", "error", "missing":
		color = "196"
	}
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(color)).
		Render(state)
}

func runnerLaunchMode(runner server.RunnerSnapshot) string {
	switch strings.ToLower(runner.State) {
	case "external":
		return "external upstream"
	default:
		return "managed or ready"
	}
}

func commandLine(command []string) string {
	if len(command) == 0 {
		return "not started"
	}
	return strings.Join(command, " ")
}

func capabilitiesLine(capabilities map[string]string) string {
	if len(capabilities) == 0 {
		return "none advertised"
	}

	keys := make([]string, 0, len(capabilities))
	for key := range capabilities {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+capabilities[key])
	}
	return strings.Join(parts, ", ")
}

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}

func fallbackInt(value int, fallbackValue string) string {
	if value == 0 {
		return fallbackValue
	}
	return fmt.Sprintf("%d", value)
}

func fallbackUint(value uint64, fallbackValue string) string {
	if value == 0 {
		return fallbackValue
	}
	return fmt.Sprintf("%d", value)
}
