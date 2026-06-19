package tui

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
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

type modelDownloadMsg struct {
	id    string
	entry catalog.Entry
	err   error
}

type runnerUpdateMsg struct {
	field  string
	value  string
	runner server.RunnerSnapshot
	err    error
}

type runnerEdit struct {
	runner  server.RunnerSnapshot
	field   string
	label   string
	current string
	value   string
	numeric bool
	secret  bool
}

type runtimeEdit struct {
	field   string
	label   string
	current string
	value   string
	numeric bool
	secret  bool
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

	active       int
	width        int
	height       int
	snapshot     server.RunnerSnapshotResponse
	runtime      server.RuntimeStatus
	logEntries   []server.LogEntry
	models       []catalog.Entry
	notice       string
	edit         *runnerEdit
	runtimeEdit  *runtimeEdit
	runtimeDraft server.RuntimeControlConfig
}

var panelBorder = lipgloss.RoundedBorder()

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
		if m.runtimeEdit != nil {
			return m.updateRuntimeEditKey(msg)
		}
		if m.edit != nil {
			return m.updateEditKey(msg)
		}
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
	case modelDownloadMsg:
		m.refresh()
		m.setActiveTab("models")
		m.notice = m.modelDownloadNotice(msg)
	case runnerUpdateMsg:
		m.refresh()
		m.setActiveTab("runner:" + msg.runner.ID)
		m.notice = m.runnerUpdateNotice(msg)
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
			case "g":
				return m, m.runtimeActionCmd("restart", server.RuntimeModeDebug)
			case "e":
				m.runtimeEdit = m.newRuntimeEdit("runtimeExe", "Runtime exe", m.runtimeConfigValue("runtimeExe"), false)
				return m, nil
			case "h":
				m.runtimeEdit = m.newRuntimeEdit("runtimeHost", "Runtime host", m.runtimeConfigValue("runtimeHost"), false)
				return m, nil
			case "p":
				m.runtimeEdit = m.newRuntimeEdit("runtimePort", "Runtime port", m.runtimeConfigValue("runtimePort"), true)
				return m, nil
			case "m":
				m.runtimeEdit = m.newRuntimeEdit("modelFile", "Model file", m.runtimeConfigValue("modelFile"), false)
				return m, nil
			case "i":
				m.runtimeEdit = m.newRuntimeEdit("modelId", "Model ID", m.runtimeConfigValue("modelId"), false)
				return m, nil
			case "u":
				m.runtimeEdit = m.newRuntimeEdit("upstream", "Upstream", m.runtimeConfigValue("upstream"), false)
				return m, nil
			case "f":
				m.runtimeEdit = m.newSecretRuntimeEdit("huggingFaceToken", "HF token", m.runtimeConfigValue("huggingFaceToken"))
				return m, nil
			case "l":
				current := boolPointerValue(m.runtimeDraft.LaunchRuntime, true)
				next := !current
				m.runtimeDraft.LaunchRuntime = &next
				m.notice = "runtime config launchRuntime " + strconv.FormatBool(next)
				return m, nil
			case "a":
				current := boolPointerValue(m.runtimeDraft.ImportModel, true)
				next := !current
				m.runtimeDraft.ImportModel = &next
				m.notice = "runtime config importModel " + strconv.FormatBool(next)
				return m, nil
			case "v":
				current := boolPointerValue(m.runtimeDraft.RuntimeVerbose, false)
				next := !current
				m.runtimeDraft.RuntimeVerbose = &next
				m.notice = "runtime config runtimeVerbose " + strconv.FormatBool(next)
				return m, nil
			}
		}
		if m.activeTabID() == "models" {
			switch strings.ToLower(value) {
			case "d":
				return m, m.modelDownloadCmd()
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
			case "b":
				backend := nextBackend(runner.Backend)
				return m, m.runnerUpdateCmd(
					runner,
					"backend",
					backend,
					server.RunnerPatch{Backend: backend},
				)
			case "p":
				m.edit = newRunnerEdit(runner, "port", "Port", fallbackInt(runner.Port, ""), true)
				return m, nil
			case "h":
				m.edit = newRunnerEdit(runner, "host", "Host", runner.Host, false)
				return m, nil
			case "i":
				m.edit = newRunnerEdit(runner, "modelId", "Model ID", runner.ModelID, false)
				return m, nil
			case "m":
				m.edit = newRunnerEdit(runner, "modelPath", "Model path", runner.ModelPath, false)
				return m, nil
			case "e":
				m.edit = newRunnerEdit(runner, "executable", "Executable", runner.Executable, false)
				return m, nil
			case "u":
				m.edit = newRunnerEdit(runner, "upstream", "Upstream", runner.Upstream, false)
				return m, nil
			case "f":
				m.edit = newSecretRunnerEdit(runner, "huggingFaceToken", "HF token", "not shown")
				return m, nil
			case "l":
				launch := !runner.Launch
				value := "external"
				if launch {
					value = "managed"
				}
				return m, m.runnerUpdateCmd(
					runner,
					"launch",
					value,
					server.RunnerPatch{Launch: &launch},
				)
			case "v":
				verbose := !runner.Verbose
				return m, m.runnerUpdateCmd(
					runner,
					"verbose",
					strconv.FormatBool(verbose),
					server.RunnerPatch{Verbose: &verbose},
				)
			case "t":
				runtimeName := nextRuntime(runner.Runtime)
				return m, m.runnerUpdateCmd(
					runner,
					"runtime",
					runtimeName,
					server.RunnerPatch{Runtime: runtimeName},
				)
			case "o":
				role := nextRole(runner.Role)
				return m, m.runnerUpdateCmd(
					runner,
					"role",
					role,
					server.RunnerPatch{Role: role},
				)
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

func (m Model) updateEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	edit := *m.edit
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.edit = nil
		return m, nil
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.edit.value) > 0 {
			m.edit.value = m.edit.value[:len(m.edit.value)-1]
		}
		return m, nil
	case tea.KeyEnter:
		m.edit = nil
		patch, value, err := runnerEditPatch(edit)
		if err != nil {
			return m, func() tea.Msg {
				return runnerUpdateMsg{
					field:  edit.field,
					value:  edit.value,
					runner: edit.runner,
					err:    err,
				}
			}
		}
		return m, m.runnerUpdateCmd(edit.runner, edit.field, value, patch)
	case tea.KeyRunes:
		input := msg.String()
		if edit.numeric && !isDigits(input) {
			return m, nil
		}
		m.edit.value += input
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) updateRuntimeEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	edit := *m.runtimeEdit
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.runtimeEdit = nil
		return m, nil
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.runtimeEdit.value) > 0 {
			m.runtimeEdit.value = m.runtimeEdit.value[:len(m.runtimeEdit.value)-1]
		}
		return m, nil
	case tea.KeyEnter:
		m.runtimeEdit = nil
		value, err := m.applyRuntimeEdit(edit)
		if err != nil {
			m.notice = fmt.Sprintf("runtime config %s failed: %v", edit.field, err)
			return m, nil
		}
		m.notice = fmt.Sprintf("runtime config %s %s", edit.field, value)
		return m, nil
	case tea.KeyRunes:
		input := msg.String()
		if edit.numeric && !isDigits(input) {
			return m, nil
		}
		m.runtimeEdit.value += input
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) View() string {
	var builder strings.Builder
	builder.WriteString(m.headerView())
	builder.WriteString("\n\n")
	builder.WriteString(m.tabBar())
	builder.WriteString("\n\n")
	builder.WriteString(m.missionControlView())
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
	builder.WriteString("\n\n")
	builder.WriteString(m.commandRailView())

	return builder.String()
}

func newRunnerEdit(
	runner server.RunnerSnapshot,
	field string,
	label string,
	value string,
	numeric bool,
) *runnerEdit {
	return &runnerEdit{
		runner:  runner,
		field:   field,
		label:   label,
		current: value,
		value:   "",
		numeric: numeric,
	}
}

func newSecretRunnerEdit(
	runner server.RunnerSnapshot,
	field string,
	label string,
	value string,
) *runnerEdit {
	edit := newRunnerEdit(runner, field, label, value, false)
	edit.secret = true
	return edit
}

func (m Model) newRuntimeEdit(
	field string,
	label string,
	value string,
	numeric bool,
) *runtimeEdit {
	return &runtimeEdit{
		field:   field,
		label:   label,
		current: value,
		value:   "",
		numeric: numeric,
	}
}

func (m Model) newSecretRuntimeEdit(
	field string,
	label string,
	value string,
) *runtimeEdit {
	edit := m.newRuntimeEdit(field, label, value, false)
	edit.secret = true
	return edit
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
	result := []tab{{id: "dashboard", label: m.dashboardTabLabel()}}
	for _, runner := range m.snapshot.Runners {
		label := runner.ID
		if len(label) > 18 {
			label = label[:17] + "."
		}
		result = append(result, tab{
			id:    "runner:" + runner.ID,
			label: runnerStateGlyph(runner.State) + " " + label,
		})
	}
	result = append(
		result,
		tab{id: "models", label: m.modelsTabLabel()},
		tab{id: "logs", label: m.logsTabLabel()},
		tab{id: "settings", label: m.settingsTabLabel()},
	)
	return result
}

func (m Model) dashboardTabLabel() string {
	return fmt.Sprintf(
		"◆ Dashboard %d/%d running",
		m.runningRunnerCount(),
		len(m.snapshot.Runners),
	)
}

func (m Model) modelsTabLabel() string {
	required, present := m.requiredModelCounts()
	if required <= 0 {
		return "○ Models no catalog"
	}
	return fmt.Sprintf(
		"%s Models %d/%d",
		modelSignalGlyph(present, required),
		present,
		required,
	)
}

func (m Model) logsTabLabel() string {
	return fmt.Sprintf("%s Logs %d", logSignalGlyph(len(m.logEntries)), len(m.logEntries))
}

func (m Model) settingsTabLabel() string {
	return settingsGlyph(m.runtimeController, m.runnerController) + " Settings API"
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
	return m.runnerByID(id)
}

func (m Model) headerView() string {
	parts := []string{
		titleStyle.Render("◆ LiteRT sidecar"),
		"Runtime: " + statusBadge(m.runtime.State),
		fmt.Sprintf("Runners: %d", len(m.snapshot.Runners)),
		fmt.Sprintf("Routes: %d", len(m.snapshot.Routes)),
		fmt.Sprintf("Logs: %d", len(m.logEntries)),
	}
	if m.width > 0 || m.height > 0 {
		parts = append(parts, fmt.Sprintf("Viewport: %dx%d", m.width, m.height))
	}

	return lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(lipgloss.Color("45")).
		Padding(0, 1).
		Render(strings.Join(parts, "  "))
}

func (m Model) runnerByID(id string) (server.RunnerSnapshot, bool) {
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

func (m Model) missionControlView() string {
	return renderPanel("Mission control / Live state", m.missionControlLines(), "45")
}

func (m Model) missionControlLines() []string {
	runningRunners := m.runningRunnerCount()
	totalRunners := len(m.snapshot.Runners)
	requiredModels, presentModels := m.requiredModelCounts()

	return []string{
		"Active      " + m.activeMissionLine(),
		formatSignalLine(
			"Runtime",
			runtimeSignalGlyph(m.runtime.State),
			fallback(m.runtime.State, "unknown"),
			runtimeSignalMeter(m.runtime.State),
		),
		formatSignalLine(
			"Runners",
			runnerSignalGlyph(runningRunners, totalRunners),
			fmt.Sprintf("%d/%d active", runningRunners, totalRunners),
			statusMeter(runningRunners, totalRunners),
		),
		formatSignalLine(
			"Routes",
			routeSignalGlyph(len(m.snapshot.Routes)),
			fmt.Sprintf("%d wired", len(m.snapshot.Routes)),
			m.compactRouteSummary(),
		),
		formatSignalLine(
			"Models",
			modelSignalGlyph(presentModels, requiredModels),
			fmt.Sprintf("%d/%d present", presentModels, requiredModels),
			modelSignalMeter(presentModels, requiredModels),
		),
		"API        WebSocket api.request + shared controllers",
	}
}

func (m Model) activeMissionLine() string {
	switch m.activeTabID() {
	case "dashboard":
		return "◆ Dashboard / status.get + /sidecar/v1/status"
	case "models":
		return "● Models / Catalog.Download + RunnerController.CreateRunner"
	case "logs":
		return "● Logs / LogBroadcaster.Subscribe"
	case "settings":
		return "● Settings / RuntimeController + RunnerController"
	default:
		runner, ok := m.activeRunner()
		if !ok {
			return "! Runner / no active runner"
		}
		return fmt.Sprintf(
			"%s Runner %s / %s -> %s",
			runnerStateGlyph(runner.State),
			runner.ID,
			runnerRoleRoute(runner.Role),
			fallback(runner.Upstream, "unavailable"),
		)
	}
}

func (m Model) compactRouteSummary() string {
	if len(m.snapshot.Routes) == 0 {
		return "Routes: none"
	}

	keys := make([]string, 0, len(m.snapshot.Routes))
	for key := range m.snapshot.Routes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+" -> "+m.snapshot.Routes[key])
	}
	return strings.Join(parts, ", ")
}

func (m Model) commandRailView() string {
	return renderPanel("Command rail", m.commandRailLines(), "39")
}

func (m Model) commandRailLines() []string {
	lines := []string{
		"Global: Tab/Right next | Shift+Tab/Left previous | 1-6 jump | Esc quit",
	}

	switch m.activeTabID() {
	case "dashboard":
		lines = append(
			lines,
			"Dashboard: specs + topology + runnable backends",
			"API: status.get + /sidecar/v1/status",
		)
	case "models":
		lines = append(
			lines,
			"Models: d Download | m Main | e Embedding | r Rerank",
			"API: Catalog.Download + POST /sidecar/v1/models/download",
			"API: RunnerController.CreateRunner + POST /sidecar/v1/runners",
		)
	case "logs":
		lines = append(
			lines,
			"Logs: live broadcaster cache | WebSocket logs.subscribe parity",
			"API: LogBroadcaster + logs.subscribe",
		)
	case "settings":
		lines = append(
			lines,
			"Settings: s/d/r/g/x runtime | e/h/p/m/i/u/f edit | l/a/v toggle",
			"API: RuntimeController + WebSocket runtime.*",
			"API: api.request GET/PATCH/POST /sidecar/v1/*",
		)
	default:
		runner, ok := m.activeRunner()
		if !ok {
			lines = append(lines, "Runner: no active runner")
			return lines
		}
		basePath := "/sidecar/v1/runners/" + runner.ID
		lines = append(
			lines,
			fmt.Sprintf("Runner %s: s Start | x Stop | r Restart", runner.ID),
			"Edit: b/p/h/i/m/e/u/f/l/v/t/o",
			"API: RunnerController + "+basePath,
			"Route: "+runnerRoleRoute(runner.Role)+" -> "+fallback(runner.Upstream, "unavailable"),
		)
	}

	return lines
}

func (m Model) dashboardView() string {
	health := renderPanel("System health / Specs", m.systemHealthLines(), "82")
	signalBoard := renderPanel("Signal board / Readiness", m.signalBoardLines(), "82")
	runtimeTopology := renderPanel("Runtime topology", m.runtimeTopologyLines(), "45")
	topologyGraph := renderPanel("Topology graph / Visual route authority", m.topologyGraphLines(), "39")
	backendMatrix := renderPanel("Backend matrix / Runnable backends", m.backendMatrixLines(), "214")
	routeMap := renderPanel("Route map / Routes", m.routeMapLines(), "205")
	recentActivity := renderPanel("Recent activity", m.recentActivityLines(6), "244")
	hotkeys := renderPanel("Hotkeys", []string{
		"Tab/Right: next tab",
		"Shift+Tab/Left: previous tab",
		"Number keys: jump tabs",
		"Runner tabs: s Start, x Stop, r Restart",
		"Esc/Ctrl+C: quit",
	}, "205")

	if m.width >= 150 {
		return joinPanels(
			joinPanelRow(health, topologyGraph),
			signalBoard,
			joinPanelRow(runtimeTopology, backendMatrix),
			joinPanelRow(routeMap, recentActivity),
			hotkeys,
		)
	}

	return joinPanels(
		health,
		signalBoard,
		runtimeTopology,
		topologyGraph,
		backendMatrix,
		routeMap,
		recentActivity,
		hotkeys,
	)
}

func (m Model) signalBoardLines() []string {
	runningRunners := m.runningRunnerCount()
	totalRunners := len(m.snapshot.Runners)
	routedCount := len(m.snapshot.Routes)
	requiredModels, presentModels := m.requiredModelCounts()

	return []string{
		formatSignalLine(
			"Runtime",
			runtimeSignalGlyph(m.runtime.State),
			fallback(m.runtime.State, "unknown"),
			runtimeSignalMeter(m.runtime.State),
		),
		formatSignalLine(
			"Runners",
			runnerSignalGlyph(runningRunners, totalRunners),
			fmt.Sprintf("%d/%d active", runningRunners, totalRunners),
			statusMeter(runningRunners, totalRunners),
		),
		formatSignalLine(
			"Routes",
			routeSignalGlyph(routedCount),
			fmt.Sprintf("%d wired", routedCount),
			routeSignalMeter(routedCount, totalRunners),
		),
		formatSignalLine(
			"Models",
			modelSignalGlyph(presentModels, requiredModels),
			fmt.Sprintf("%d/%d present", presentModels, requiredModels),
			modelSignalMeter(presentModels, requiredModels),
		),
		formatSignalLine(
			"Logs",
			logSignalGlyph(len(m.logEntries)),
			fmt.Sprintf("%d cached", len(m.logEntries)),
			"latest: "+m.latestLogSource(),
		),
		"Next action  " + m.dashboardNextAction(),
		"Legend      ● ready  ◐ partial  ! attention",
	}
}

func (m Model) runningRunnerCount() int {
	running := 0
	for _, runner := range m.snapshot.Runners {
		if strings.EqualFold(runner.State, "running") {
			running++
		}
	}
	return running
}

func (m Model) requiredModelCounts() (int, int) {
	required := 0
	present := 0
	for _, entry := range m.models {
		if !entry.Required {
			continue
		}
		required++
		if entry.State == catalog.StatePresent {
			present++
		}
	}
	return required, present
}

func (m Model) latestLogSource() string {
	if len(m.logEntries) == 0 {
		return "none"
	}
	latest := m.logEntries[len(m.logEntries)-1]
	return latest.Source + "/" + latest.Stream
}

func (m Model) dashboardNextAction() string {
	for index, runner := range m.snapshot.Runners {
		if strings.EqualFold(runner.State, "running") {
			return fmt.Sprintf("open runner tab %d %s or Models for downloads", index+2, runner.ID)
		}
	}
	for index, runner := range m.snapshot.Runners {
		if strings.TrimSpace(runner.ID) != "" {
			return fmt.Sprintf("open runner tab %d %s and press s to start", index+2, runner.ID)
		}
	}
	return "open Models and create a runner"
}

func (m Model) systemHealthLines() []string {
	configured := len(m.snapshot.Runners)
	routes := len(m.snapshot.Routes)
	running := 0
	for _, runner := range m.snapshot.Runners {
		if strings.EqualFold(runner.State, "running") {
			running++
		}
	}

	lines := []string{
		"Specs",
		formatKV("Runtime", statusBadge(m.runtime.State)),
		formatKV("Mode", fallback(m.runtime.Mode, "release")),
		formatKV("Version", fallback(m.runtime.Version, "unknown")),
		formatKV("Model", fallback(m.runtime.ModelID, "unconfigured")),
		formatKV("Model file", fallback(m.runtime.ModelFile, "not configured")),
		formatKV("Health", statusMeter(running, configured)),
		fmt.Sprintf("configured runners: %d", configured),
		fmt.Sprintf("runnable routes: %d", routes),
		formatKV("Logs", fmt.Sprintf("%d entries", len(m.logEntries))),
	}
	if m.runtime.Detail != "" {
		lines = append(lines, formatKV("Detail", m.runtime.Detail))
	}
	return lines
}

func (m Model) runtimeTopologyLines() []string {
	return []string{
		formatKV("Sidecar API", "127.0.0.1:9379 /sidecar/v1/*"),
		formatKV("Control WS", "ws://127.0.0.1:9379/sidecar/v1/ws"),
		formatKV("runtime upstream", fallback(m.runtime.Upstream, "unavailable")),
		formatKV("Executable", fallback(m.runtime.Executable, "not configured")),
		"browser api.request => sidecar controllers => runner supervisor",
	}
}

func (m Model) topologyGraphLines() []string {
	lines := []string{
		"◉ Browser UI",
		"│  api.request / ws://127.0.0.1:9379/sidecar/v1/ws",
		"▼",
		"◆ Sidecar API",
		"│  127.0.0.1:9379 /sidecar/v1/*",
		"▼",
		"◇ Runner supervisor",
		fmt.Sprintf("│  routes=%d runners=%d", len(m.snapshot.Routes), len(m.snapshot.Runners)),
	}

	if len(m.snapshot.Routes) == 0 {
		return append(lines,
			"└─ no routes wired",
			"Legend: ● running  ◐ configured  ! attention  ○ idle",
		)
	}

	keys := make([]string, 0, len(m.snapshot.Routes))
	for key := range m.snapshot.Routes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for index, key := range keys {
		branch := "├─"
		detailPrefix := "│ "
		if index == len(keys)-1 {
			branch = "└─"
			detailPrefix = "  "
		}

		runnerID := m.snapshot.Routes[key]
		glyph := "○"
		upstream := "unavailable"
		if runner, ok := m.runnerByID(runnerID); ok {
			glyph = runnerStateGlyph(runner.State)
			upstream = fallback(runner.Upstream, "unavailable")
		}

		lines = append(
			lines,
			fmt.Sprintf("%s %s => %s %s", branch, key, glyph, runnerID),
			detailPrefix+" "+upstream,
		)
	}

	return append(lines, "Legend: ● running  ◐ configured  ! attention  ○ idle")
}

func (m Model) backendMatrixLines() []string {
	if len(m.snapshot.Runners) == 0 {
		return []string{"No runners configured."}
	}

	lines := make([]string, 0, 1+len(m.snapshot.Runners)*9)
	lines = append(lines, "Runnable backends - Runner backend cards")
	for index, runner := range m.snapshot.Runners {
		if index > 0 {
			lines = append(lines, "")
		}

		runtimeRole := fmt.Sprintf(
			"%s / %s",
			fallback(runner.Runtime, "runtime"),
			fallback(runner.Role, "role"),
		)
		lines = append(lines,
			fmt.Sprintf(
				"%s %s  %s",
				runnerStateGlyph(runner.State),
				runner.ID,
				statusBadge(runner.State),
			),
			formatKV("Runtime/Role", runtimeRole),
			formatKV("Backend", fallback(runner.Backend, "backend")),
			formatKV("Model", fallback(runner.ModelID, fallback(runner.ModelPath, "model"))),
			formatKV("Launch", runnerLaunchMode(runner)),
			formatKV("Health", runnerHealthMeter(runner)),
			formatKV("Route", fallback(runner.Upstream, "unavailable")),
			formatKV("Caps", capabilitiesLine(runner.Capabilities)),
		)
	}
	return lines
}

func (m Model) routeMapLines() []string {
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
		runnerID := m.snapshot.Routes[key]
		upstream := "unavailable"
		if runner, ok := m.runnerByID(runnerID); ok {
			upstream = fallback(runner.Upstream, "unavailable")
		}
		lines = append(lines, fmt.Sprintf("%s -> %s => %s", key, runnerID, upstream))
	}
	return lines
}

func (m Model) recentActivityLines(limit int) []string {
	if len(m.logEntries) == 0 {
		return []string{"No runtime output yet."}
	}

	start := len(m.logEntries) - limit
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, len(m.logEntries[start:]))
	for _, entry := range m.logEntries[start:] {
		lines = append(lines, formatLogEntry(entry))
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
		formatKV("Verbose", strconv.FormatBool(runner.Verbose)),
		formatKV("Upstream", fallback(runner.Upstream, "unavailable")),
		formatKV("HF token", "not shown"),
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

	health := renderPanel("Runner "+runner.ID+" / Runner health", m.runnerHealthLines(runner), "82")
	signalBoard := renderPanel("Runner signal board / Readiness", m.runnerSignalLines(runner), "82")
	endpointMap := renderPanel("Endpoint map", m.runnerEndpointLines(runner), "45")
	operationFlow := renderPanel("Operation flow", runnerOperationLines(runner), "214")
	controlSurface := renderPanel("Control surface", m.runnerControlLines(runner), "39")
	runtimeCommand := renderPanel("Runtime command", []string{commandLine(runner.Command)}, "214")
	capabilities := renderPanel("Capabilities matrix", runnerCapabilityLines(runner), "205")
	editor := m.runnerEditorView(runner)
	settingsMatrix := renderPanel("Settings matrix", runnerSettingsMatrixLines(runner), "39")
	settingsPanel := renderPanel("Settings", settings, "45")
	detailsPanel := renderPanel("Details", details, "214")
	logsPanel := renderPanel("Recent runner logs", m.runnerLogLines(runner.ID, 6), "244")

	if m.width >= 150 {
		return joinPanels(
			joinPanelRow(health, signalBoard),
			joinPanelRow(endpointMap, operationFlow),
			joinPanelRow(controlSurface, runtimeCommand),
			capabilities,
			editor,
			joinPanelRow(settingsMatrix, settingsPanel),
			joinPanelRow(detailsPanel, logsPanel),
		)
	}

	return joinPanels(
		health,
		signalBoard,
		endpointMap,
		operationFlow,
		controlSurface,
		runtimeCommand,
		capabilities,
		editor,
		settingsMatrix,
		settingsPanel,
		detailsPanel,
		logsPanel,
	)
}

func (m Model) runnerSignalLines(runner server.RunnerSnapshot) []string {
	logCount := m.runnerLogCount(runner.ID)

	return []string{
		formatSignalLine(
			"State",
			runnerStateGlyph(runner.State),
			fallback(runner.State, "unknown"),
			runnerHealthMeter(runner),
		),
		formatSignalLine(
			"Route",
			runnerRouteGlyph(runner),
			fallback(runner.Role, "unknown"),
			runnerRoleRoute(runner.Role)+" -> "+fallback(runner.Upstream, "unavailable"),
		),
		formatSignalLine(
			"Process",
			runnerProcessGlyph(runner),
			runnerProcessState(runner),
			runnerLaunchMode(runner),
		),
		formatSignalLine(
			"Model",
			runnerModelGlyph(runner),
			fallback(runner.ModelID, "model"),
			fallback(runner.ModelPath, "not configured"),
		),
		formatSignalLine(
			"Caps",
			runnerCapabilitiesGlyph(runner),
			fmt.Sprintf("%d advertised", len(runner.Capabilities)),
			capabilitiesLine(runner.Capabilities),
		),
		formatSignalLine(
			"Logs",
			runnerLogsGlyph(runner, logCount),
			"seq "+fallbackUint(runner.LogSequence, "none"),
			fmt.Sprintf("cached entries: %d", logCount),
		),
		"Next action  " + runnerNextAction(runner),
		"Legend      ● ready  ◐ configured  ! attention",
	}
}

func (m Model) runnerLogCount(runnerID string) int {
	count := 0
	source := "runner:" + runnerID
	for _, entry := range m.logEntries {
		if entry.Source == source {
			count++
		}
	}
	return count
}

func (m Model) runnerHealthLines(runner server.RunnerSnapshot) []string {
	return []string{
		formatKV("State", statusBadge(runner.State)),
		formatKV("Runtime", fallback(runner.Runtime, "unknown")),
		formatKV("Role", fallback(runner.Role, "unknown")),
		formatKV("Backend", fallback(runner.Backend, "default")),
		formatKV("Launch", runnerLaunchMode(runner)),
		formatKV("Verbose", strconv.FormatBool(runner.Verbose)),
		formatKV("Process", fallbackInt(runner.PID, "not running")),
		formatKV("Health", runnerHealthMeter(runner)),
	}
}

func (m Model) runnerEndpointLines(runner server.RunnerSnapshot) []string {
	upstream := fallback(runner.Upstream, "unavailable")
	lines := []string{
		formatKV("Upstream", upstream),
		formatKV("Models", endpointPath(upstream, "/v1/models")),
	}
	switch runner.Role {
	case "main":
		lines = append(lines, formatKV("Chat", runnerRoleRoute(runner.Role)))
	case "embedding":
		lines = append(lines, formatKV("Embeddings", runnerRoleRoute(runner.Role)))
	case "reranking":
		lines = append(lines, formatKV("Rerank", runnerRoleRoute(runner.Role)))
	default:
		lines = append(lines, formatKV("OpenAI", runnerRoleRoute(runner.Role)))
	}
	return lines
}

func runnerOperationLines(runner server.RunnerSnapshot) []string {
	basePath := "/sidecar/v1/runners/" + runner.ID
	runtimeRoleBackend := fmt.Sprintf(
		"%s / %s / %s",
		fallback(runner.Runtime, "runtime"),
		fallback(runner.Role, "role"),
		fallback(runner.Backend, "backend"),
	)

	return []string{
		fmt.Sprintf(
			"%s %s  %s",
			runnerStateGlyph(runner.State),
			runner.ID,
			statusBadge(runner.State),
		),
		"Model file -> Runtime -> Upstream -> Route",
		formatKV("Model file", fallback(runner.ModelPath, "not configured")),
		formatKV("Runtime", runtimeRoleBackend),
		formatKV("Upstream", fallback(runner.Upstream, "unavailable")),
		formatKV("API route", runnerRoleRoute(runner.Role)),
		"",
		"Controller parity:",
		"RunnerController.StartRunner / StopRunner / RestartRunner / UpdateRunner",
		"WebSocket api.request parity:",
		"POST " + basePath + "/start",
		"POST " + basePath + "/stop",
		"POST " + basePath + "/restart",
		"PATCH " + basePath,
	}
}

func runnerSettingsMatrixLines(runner server.RunnerSnapshot) []string {
	return []string{
		"Key  Setting       Current                         Patch/API",
		runnerSettingsMatrixLine(
			"b",
			"Backend",
			fallback(runner.Backend, "default"),
			"backend",
		),
		runnerSettingsMatrixLine(
			"p",
			"Port",
			fallbackInt(runner.Port, "auto"),
			"port",
		),
		runnerSettingsMatrixLine(
			"h",
			"Host",
			fallback(runner.Host, "127.0.0.1"),
			"host",
		),
		runnerSettingsMatrixLine(
			"i",
			"Model ID",
			fallback(runner.ModelID, "not configured"),
			"modelId",
		),
		runnerSettingsMatrixLine(
			"m",
			"Model path",
			fallback(runner.ModelPath, "not configured"),
			"modelPath",
		),
		runnerSettingsMatrixLine(
			"e",
			"Executable",
			fallback(runner.Executable, "auto-discover"),
			"executable",
		),
		runnerSettingsMatrixLine(
			"u",
			"Upstream",
			fallback(runner.Upstream, "unavailable"),
			"upstream",
		),
		runnerSettingsMatrixLine(
			"f",
			"HF token",
			"not shown",
			"huggingFaceToken",
		),
		runnerSettingsMatrixLine(
			"l",
			"Launch",
			runnerLaunchMode(runner),
			"launch",
		),
		runnerSettingsMatrixLine(
			"v",
			"Verbose",
			strconv.FormatBool(runner.Verbose),
			"verbose",
		),
		runnerSettingsMatrixLine(
			"t",
			"Runtime",
			fallback(runner.Runtime, "unknown"),
			"runtime",
		),
		runnerSettingsMatrixLine(
			"o",
			"Role",
			fallback(runner.Role, "unknown"),
			"role",
		),
		"PATCH /sidecar/v1/runners/" + runner.ID,
	}
}

func runnerSettingsMatrixLine(
	key string,
	setting string,
	current string,
	patchField string,
) string {
	return fmt.Sprintf(
		"%-4s %-13s %-31s %s -> RunnerController.UpdateRunner",
		key,
		setting,
		current,
		patchField,
	)
}

func (m Model) runnerControlLines(runner server.RunnerSnapshot) []string {
	basePath := "/sidecar/v1/runners/" + runner.ID
	return []string{
		"Controls",
		"Actions",
		"s Start   x Stop   r Restart",
		"POST " + basePath + "/start",
		"POST " + basePath + "/stop",
		"POST " + basePath + "/restart",
		"",
		"Edit settings",
		"Settings editor",
		"b Backend CPU/GPU   l Launch managed/external   v Verbose",
		"t Runtime   o Role",
		"p Port   h Host   i Model ID",
		"m Model path   e Executable   u Upstream   f HF token",
		"PATCH " + basePath,
	}
}

func runnerCapabilityLines(runner server.RunnerSnapshot) []string {
	if len(runner.Capabilities) == 0 {
		return []string{"none advertised"}
	}

	keys := make([]string, 0, len(runner.Capabilities))
	for key := range runner.Capabilities {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+runner.Capabilities[key])
	}
	return lines
}

func (m Model) runnerLogLines(runnerID string, limit int) []string {
	filtered := make([]server.LogEntry, 0, len(m.logEntries))
	source := "runner:" + runnerID
	for _, entry := range m.logEntries {
		if entry.Source == source {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) == 0 {
		return []string{"No runner logs yet."}
	}

	start := len(filtered) - limit
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, len(filtered[start:]))
	for _, entry := range filtered[start:] {
		lines = append(lines, formatLogEntry(entry))
	}
	return lines
}

func (m Model) runnerEditorView(runner server.RunnerSnapshot) string {
	if m.edit == nil || m.edit.runner.ID != runner.ID {
		return ""
	}
	newValue := m.edit.value
	if m.edit.secret {
		newValue = secretEditValue(m.edit.value)
	}
	return renderPanel(
		"Editing "+m.edit.label+" for "+runner.ID,
		[]string{
			formatKV("Current", fallback(m.edit.current, "not configured")),
			"",
			"New value",
			newValue,
			"Enter saves through PATCH /sidecar/v1/runners/{id}; Esc cancels.",
		},
		"205",
	)
}

func (m Model) modelsView() string {
	readiness := renderPanel(
		"Model readiness / Required artifacts",
		m.modelReadinessLines(),
		"82",
	)
	actions := renderPanel(
		"Runner creation / Catalog presets",
		modelActionLines(),
		"214",
	)
	cards := renderPanel(
		"Catalog cards / Download state",
		m.modelCatalogCardLines(),
		"45",
	)

	if m.width >= 150 {
		return joinPanels(
			joinPanelRow(readiness, actions),
			cards,
		)
	}

	return joinPanels(
		readiness,
		actions,
		cards,
	)
}

func (m Model) modelReadinessLines() []string {
	if len(m.models) == 0 {
		return []string{"No model catalog configured."}
	}

	required, present := m.requiredModelCounts()
	missing := required - present
	nextAction := "all required models ready"
	if entry, ok := m.nextDownloadEntry(); ok {
		nextAction = "d Download " + entry.ID + " via POST /sidecar/v1/models/download"
	}

	return []string{
		formatSignalLine(
			"Required",
			modelSignalGlyph(present, required),
			fmt.Sprintf("%d/%d present", present, required),
			modelSignalMeter(present, required),
		),
		formatSignalLine(
			"Missing",
			modelMissingGlyph(missing),
			fmt.Sprintf("%d missing", missing),
			"download queue",
		),
		formatSignalLine(
			"Catalog",
			"●",
			fmt.Sprintf("%d entries", len(m.models)),
			"required artifacts",
		),
		fmt.Sprintf("%-11s %s", "Next", nextAction),
		"Legend      ● present  ◉ downloading  ! error  ○ missing",
	}
}

func modelActionLines() []string {
	return []string{
		"Create runners",
		"m Main llama.cpp -> main-llamacpp /v1/chat/completions",
		"e Embedding llama.cpp -> embedding-llamacpp /v1/embeddings",
		"r Rerank llama.cpp -> rerank-llamacpp /v1/rerank",
		"RunnerController.CreateRunner",
		"POST /sidecar/v1/runners",
		"",
		"Download models",
		"d Download next missing required model",
		"Catalog.Download",
		"POST /sidecar/v1/models/download",
		"WebSocket api.request parity for both actions",
	}
}

func (m Model) modelCatalogCardLines() []string {
	if len(m.models) == 0 {
		return []string{"No model catalog configured."}
	}

	lines := make([]string, 0, len(m.models)*6)
	for index, entry := range m.models {
		if index > 0 {
			lines = append(lines, "")
		}
		lines = append(
			lines,
			modelCatalogCardHeader(entry),
			formatKV("File", entry.Filename),
			formatKV("Target", entry.TargetPath),
			formatKV("Progress", modelProgress(entry)),
		)
		if entry.LastError != "" {
			lines = append(lines, formatKV("Last error", entry.LastError))
		}
	}
	return lines
}

func (m Model) logsView() string {
	signal := renderPanel("Log signal / Live cache", m.logSignalLines(), "82")
	sources := renderPanel("Source activity / Streams", m.logSourceActivityLines(), "45")
	recent := renderPanel("Recent log events", m.recentLogEventLines(16), "244")

	if m.width >= 150 {
		return joinPanels(
			joinPanelRow(signal, sources),
			recent,
		)
	}

	return joinPanels(
		signal,
		sources,
		recent,
	)
}

func (m Model) logSignalLines() []string {
	sourceCount, streamCount := m.logSourceAndStreamCounts()
	latest := "none"
	if len(m.logEntries) > 0 {
		entry := m.logEntries[len(m.logEntries)-1]
		latest = fmt.Sprintf("#%d %s/%s", entry.Seq, entry.Source, entry.Stream)
	}

	return []string{
		formatSignalLine(
			"Entries",
			logSignalGlyph(len(m.logEntries)),
			fmt.Sprintf("%d cached", len(m.logEntries)),
			"latest: "+m.latestLogSource(),
		),
		formatSignalLine(
			"Sources",
			logSignalGlyph(sourceCount),
			fmt.Sprintf("%d sources", sourceCount),
			"LogBroadcaster.Snapshot",
		),
		formatSignalLine(
			"Streams",
			logSignalGlyph(streamCount),
			fmt.Sprintf("%d channels", streamCount),
			"logs.subscribe parity",
		),
		fmt.Sprintf("%-11s %s", "Latest", latest),
		"Legend      ● active  ◐ empty  ! attention",
	}
}

func (m Model) logSourceAndStreamCounts() (int, int) {
	sources := map[string]struct{}{}
	streams := map[string]struct{}{}
	for _, entry := range m.logEntries {
		sources[entry.Source] = struct{}{}
		streams[entry.Source+"/"+entry.Stream] = struct{}{}
	}
	return len(sources), len(streams)
}

type logSourceStat struct {
	source string
	stream string
	count  int
	latest uint64
}

func (m Model) logSourceActivityLines() []string {
	if len(m.logEntries) == 0 {
		return []string{"No logs yet."}
	}

	stats := map[string]logSourceStat{}
	for _, entry := range m.logEntries {
		key := entry.Source + "\x00" + entry.Stream
		stat := stats[key]
		stat.source = entry.Source
		stat.stream = entry.Stream
		stat.count++
		stat.latest = entry.Seq
		stats[key] = stat
	}

	keys := make([]string, 0, len(stats))
	for key := range stats {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		stat := stats[key]
		lines = append(lines, fmt.Sprintf(
			"%s %s %s %d entries latest #%d",
			logSignalGlyph(stat.count),
			stat.source,
			stat.stream,
			stat.count,
			stat.latest,
		))
	}
	lines = append(lines, "", "WebSocket logs.subscribe -> LogBroadcaster.Subscribe")
	return lines
}

func (m Model) recentLogEventLines(limit int) []string {
	if len(m.logEntries) == 0 {
		return []string{"No logs yet."}
	}
	start := len(m.logEntries) - 16
	if start < 0 {
		start = 0
	}
	if limit > 0 {
		start = len(m.logEntries) - limit
		if start < 0 {
			start = 0
		}
	}
	lines := make([]string, 0, len(m.logEntries[start:]))
	for _, entry := range m.logEntries[start:] {
		lines = append(lines, formatLogEntry(entry))
	}
	lines = append(lines, "", "WebSocket logs.subscribe -> LogBroadcaster.Subscribe")
	return lines
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

	settingsPanel := renderPanel("Settings", []string{
		"Controls",
		"s Start release   d Start debug   r Restart release   g Restart debug   x Stop runtime",
		"",
		formatKV("Runtime controller", runtimeState),
		formatKV("Runner controller", runnerState),
		formatKV("HTTP listen", "configured by -addr"),
		formatKV("Default upstream", fallback(m.runtime.Upstream, "unavailable")),
		formatKV("Runtime mode", fallback(m.runtime.Mode, "release")),
		formatKV("Log entries", fmt.Sprintf("%d cached", len(m.logEntries))),
	}, "45")
	configEditor := renderPanel("Runtime config editor", m.runtimeConfigLines(), "82")
	activeEditor := m.runtimeEditorView()
	actionMap := renderPanel("Shared action map", settingsActionMapLines(), "214")
	runnerParity := renderPanel("Runner API parity / Live snapshot", m.settingsRunnerParityLines(), "39")
	webSocketParity := renderPanel("WebSocket API parity", settingsAPIParityLines(), "205")

	if m.width >= 150 {
		return joinPanels(
			joinPanelRow(settingsPanel, configEditor),
			activeEditor,
			joinPanelRow(actionMap, runnerParity),
			webSocketParity,
		)
	}

	return joinPanels(
		settingsPanel,
		configEditor,
		activeEditor,
		actionMap,
		runnerParity,
		webSocketParity,
	)
}

func (m Model) settingsRunnerParityLines() []string {
	lines := []string{
		"Runner role/state/route comes from RunnerController.Snapshot()",
		m.settingsRouteSummaryLine(),
	}
	if len(m.snapshot.Runners) == 0 {
		return append(lines, "No runners configured.")
	}

	for index, runner := range m.snapshot.Runners {
		if index > 0 {
			lines = append(lines, "")
		}
		basePath := "/sidecar/v1/runners/" + runner.ID
		lines = append(
			lines,
			fmt.Sprintf(
				"%-12s %-10s %-8s %s",
				runner.ID,
				fallback(runner.Role, "unknown"),
				fallback(runner.State, "unknown"),
				runnerRoleRoute(runner.Role),
			),
			"TUI: s/x/r + b/p/h/i/m/e/u/f/l/v/t/o",
			"Controller: RunnerController.StartRunner/StopRunner/RestartRunner/UpdateRunner",
			"WS: api.request PATCH "+basePath,
			"WS: api.request POST "+basePath+"/start|stop|restart",
		)
	}
	return lines
}

func (m Model) settingsRouteSummaryLine() string {
	if len(m.snapshot.Routes) == 0 {
		return "Routes: none"
	}

	keys := make([]string, 0, len(m.snapshot.Routes))
	for key := range m.snapshot.Routes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+" -> "+m.snapshot.Routes[key])
	}
	return "Routes: " + strings.Join(parts, ", ")
}

type settingsAPIParityGroup struct {
	title string
	rows  []settingsAPIParityRow
}

type settingsAPIParityRow struct {
	surface string
	backing string
}

func settingsAPIParityGroups() []settingsAPIParityGroup {
	return []settingsAPIParityGroup{
		{
			title: "Direct WebSocket messages",
			rows: []settingsAPIParityRow{
				{surface: "status.get", backing: "RuntimeController.Status"},
				{surface: "runtime.start", backing: "RuntimeController.Start"},
				{surface: "runtime.start config", backing: "RuntimeController.Start"},
				{surface: "runtime.stop", backing: "RuntimeController.Stop"},
				{surface: "runtime.restart", backing: "RuntimeController.Restart"},
				{surface: "runtime.restart config", backing: "RuntimeController.Restart"},
				{surface: "logs.subscribe", backing: "LogBroadcaster.Subscribe"},
			},
		},
		{
			title: "Sidecar api.request routes",
			rows: []settingsAPIParityRow{
				{surface: "api.request GET /sidecar/v1/status", backing: "RuntimeController.Status"},
				{surface: "api.request GET /sidecar/v1/models", backing: "Catalog.Entries"},
				{surface: "api.request POST /sidecar/v1/models/download", backing: "Catalog.Download"},
				{surface: "api.request GET /sidecar/v1/runners", backing: "RunnerController.Snapshot"},
				{surface: "api.request POST /sidecar/v1/runners", backing: "RunnerController.CreateRunner"},
				{surface: "api.request PATCH /sidecar/v1/runners/{id}", backing: "RunnerController.UpdateRunner"},
				{surface: "api.request POST /sidecar/v1/runners/{id}/start", backing: "RunnerController.StartRunner"},
				{surface: "api.request POST /sidecar/v1/runners/{id}/stop", backing: "RunnerController.StopRunner"},
				{
					surface: "api.request POST /sidecar/v1/runners/{id}/restart",
					backing: "RunnerController.RestartRunner",
				},
				{surface: "api.request POST /sidecar/v1/multimodal", backing: "RuntimeController.Multimodal"},
			},
		},
		{
			title: "OpenAI upstream proxy",
			rows: []settingsAPIParityRow{
				{surface: "api.request * /v1/*", backing: "runner supervisor route authority"},
			},
		},
	}
}

func settingsAPIParityLines() []string {
	groups := settingsAPIParityGroups()
	lines := make([]string, 0, 32)
	for groupIndex, group := range groups {
		if groupIndex > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, group.title)
		for _, row := range group.rows {
			lines = append(lines, row.surface+" -> "+row.backing)
		}
	}
	lines = append(lines, "", "TUI uses the same backing methods as WebSocket and HTTP handlers.")
	return lines
}

func settingsActionMapLines() []string {
	return []string{
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
	}
}

func (m Model) runtimeConfigLines() []string {
	return []string{
		"Edit runtime config used by s/d/r runtime actions",
		"e Runtime exe   h Runtime host   p Runtime port",
		"m Model file    i Model ID       u Upstream",
		"f HF token",
		"l Launch runtime   a Import model   v Runtime verbose",
		"",
		formatKV("Runtime exe", m.runtimeConfigValue("runtimeExe")),
		formatKV("Runtime host", m.runtimeConfigValue("runtimeHost")),
		formatKV("Runtime port", m.runtimeConfigValue("runtimePort")),
		formatKV("Model file", m.runtimeConfigValue("modelFile")),
		formatKV("Model ID", m.runtimeConfigValue("modelId")),
		formatKV("Upstream", m.runtimeConfigValue("upstream")),
		formatKV("HF token", m.runtimeConfigValue("huggingFaceToken")),
		formatKV("Launch runtime", strconv.FormatBool(boolPointerValue(m.runtimeDraft.LaunchRuntime, true))),
		formatKV("Import model", strconv.FormatBool(boolPointerValue(m.runtimeDraft.ImportModel, true))),
		formatKV("Runtime verbose", strconv.FormatBool(boolPointerValue(m.runtimeDraft.RuntimeVerbose, false))),
	}
}

func (m Model) runtimeEditorView() string {
	if m.runtimeEdit == nil {
		return ""
	}
	newValue := m.runtimeEdit.value
	if m.runtimeEdit.secret {
		newValue = secretEditValue(m.runtimeEdit.value)
	}
	return renderPanel(
		"Editing "+m.runtimeEdit.label,
		[]string{
			formatKV("Current", fallback(m.runtimeEdit.current, "not configured")),
			"",
			"New value",
			newValue,
			"Enter stores this config for runtime.start/runtime.restart; Esc cancels.",
		},
		"205",
	)
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
			err = m.runtimeController.Start(ctx, mode, m.runtimeDraft)
		case "stop":
			err = m.runtimeController.Stop(ctx)
		case "restart":
			err = m.runtimeController.Restart(ctx, mode, m.runtimeDraft)
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

func (m Model) modelDownloadCmd() tea.Cmd {
	entry, ok := m.nextDownloadEntry()
	if !ok {
		return func() tea.Msg {
			return modelDownloadMsg{
				err: fmt.Errorf("no missing required models to download"),
			}
		}
	}

	return func() tea.Msg {
		if m.catalog == nil {
			return modelDownloadMsg{
				id:  entry.ID,
				err: fmt.Errorf("model catalog is not configured"),
			}
		}

		downloaded, err := m.catalog.Download(m.ctx, entry.ID)
		return modelDownloadMsg{
			id:    entry.ID,
			entry: downloaded,
			err:   err,
		}
	}
}

func (m Model) runnerUpdateCmd(
	runner server.RunnerSnapshot,
	field string,
	value string,
	patch server.RunnerPatch,
) tea.Cmd {
	return func() tea.Msg {
		if m.runnerController == nil {
			return runnerUpdateMsg{
				field:  field,
				value:  value,
				runner: runner,
				err:    fmt.Errorf("runner controller is not configured"),
			}
		}

		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()

		updated, err := m.runnerController.UpdateRunner(ctx, runner.ID, patch)
		if updated.ID == "" {
			updated = runner
		}
		return runnerUpdateMsg{
			field:  field,
			value:  value,
			runner: updated,
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

func (m Model) modelDownloadNotice(message modelDownloadMsg) string {
	if message.err != nil {
		if message.id == "" {
			return fmt.Sprintf("download model failed: %v", message.err)
		}
		return fmt.Sprintf("download %s failed: %v", message.id, message.err)
	}
	return fmt.Sprintf(
		"downloaded model %s %s",
		message.entry.ID,
		modelProgress(message.entry),
	)
}

func (m Model) runnerUpdateNotice(message runnerUpdateMsg) string {
	if message.err != nil {
		return fmt.Sprintf("update %s %s failed: %v", message.runner.ID, message.field, message.err)
	}
	return fmt.Sprintf("updated %s %s %s", message.runner.ID, message.field, message.value)
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

func nextBackend(current string) string {
	if strings.EqualFold(current, "cpu") {
		return "gpu"
	}
	return "cpu"
}

func nextRuntime(current string) string {
	if strings.EqualFold(current, "litert") {
		return "llamacpp"
	}
	return "litert"
}

func nextRole(current string) string {
	switch strings.ToLower(current) {
	case "main":
		return "embedding"
	case "embedding":
		return "reranking"
	default:
		return "main"
	}
}

func runnerEditPatch(edit runnerEdit) (server.RunnerPatch, string, error) {
	value := strings.TrimSpace(edit.value)
	switch edit.field {
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil || port <= 0 {
			return server.RunnerPatch{}, value, fmt.Errorf("port must be a positive integer")
		}
		return server.RunnerPatch{Port: port}, strconv.Itoa(port), nil
	case "host":
		return server.RunnerPatch{Host: value}, value, nil
	case "modelId":
		return server.RunnerPatch{ModelID: value}, value, nil
	case "modelPath":
		return server.RunnerPatch{ModelPath: value}, value, nil
	case "executable":
		return server.RunnerPatch{Executable: value}, value, nil
	case "upstream":
		return server.RunnerPatch{Upstream: value}, value, nil
	case "huggingFaceToken":
		return server.RunnerPatch{HuggingFaceToken: &value}, tokenNoticeValue(value), nil
	default:
		return server.RunnerPatch{}, value, fmt.Errorf("unknown runner field %q", edit.field)
	}
}

func (m *Model) applyRuntimeEdit(edit runtimeEdit) (string, error) {
	value := strings.TrimSpace(edit.value)
	switch edit.field {
	case "runtimeExe":
		m.runtimeDraft.RuntimeExe = value
	case "runtimeHost":
		m.runtimeDraft.RuntimeHost = value
	case "runtimePort":
		port, err := strconv.Atoi(value)
		if err != nil || port <= 0 {
			return value, fmt.Errorf("runtime port must be a positive integer")
		}
		m.runtimeDraft.RuntimePort = port
		value = strconv.Itoa(port)
	case "modelFile":
		m.runtimeDraft.ModelFile = value
	case "modelId":
		m.runtimeDraft.ModelID = value
	case "upstream":
		m.runtimeDraft.Upstream = value
	case "huggingFaceToken":
		m.runtimeDraft.HuggingFaceToken = &value
		value = tokenNoticeValue(value)
	default:
		return value, fmt.Errorf("unknown runtime config field %q", edit.field)
	}
	return value, nil
}

func (m Model) runtimeConfigValue(field string) string {
	switch field {
	case "runtimeExe":
		return fallback(m.runtimeDraft.RuntimeExe, fallback(m.runtime.Executable, "not configured"))
	case "runtimeHost":
		return fallback(m.runtimeDraft.RuntimeHost, fallback(upstreamHost(m.runtime.Upstream), "127.0.0.1"))
	case "runtimePort":
		return fallbackInt(m.runtimeDraft.RuntimePort, fallback(upstreamPort(m.runtime.Upstream), "auto"))
	case "modelFile":
		return fallback(m.runtimeDraft.ModelFile, fallback(m.runtime.ModelFile, "not configured"))
	case "modelId":
		return fallback(m.runtimeDraft.ModelID, fallback(m.runtime.ModelID, "not configured"))
	case "upstream":
		return fallback(m.runtimeDraft.Upstream, fallback(m.runtime.Upstream, "unavailable"))
	case "huggingFaceToken":
		if m.runtimeDraft.HuggingFaceToken == nil {
			return "not set"
		}
		return tokenNoticeValue(*m.runtimeDraft.HuggingFaceToken)
	default:
		return "unknown"
	}
}

func secretEditValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[empty clears token]"
	}
	return "[hidden]"
}

func tokenNoticeValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "cleared"
	}
	return "set"
}

func upstreamHost(upstream string) string {
	parsed, err := url.Parse(upstream)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func upstreamPort(upstream string) string {
	parsed, err := url.Parse(upstream)
	if err != nil {
		return ""
	}
	return parsed.Port()
}

func boolPointerValue(value *bool, fallbackValue bool) bool {
	if value == nil {
		return fallbackValue
	}
	return *value
}

func isDigits(value string) bool {
	if value == "" {
		return true
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
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

func (m Model) nextDownloadEntry() (catalog.Entry, bool) {
	for _, entry := range m.models {
		if !entry.Required {
			continue
		}
		switch entry.State {
		case catalog.StateMissing, catalog.StateError:
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

func modelCatalogLine(entry catalog.Entry) string {
	return fmt.Sprintf(
		"%s %s  %s/%s  %s  %s",
		modelStateGlyph(entry.State),
		statusBadge(string(entry.State)),
		fallback(entry.Runtime, "runtime"),
		fallback(entry.Role, "role"),
		entry.ID,
		modelProgress(entry),
	)
}

func modelCatalogCardHeader(entry catalog.Entry) string {
	return fmt.Sprintf(
		"%s %-11s %-18s %s",
		modelStateGlyph(entry.State),
		string(entry.State),
		fallback(entry.Runtime, "runtime")+"/"+fallback(entry.Role, "role"),
		entry.ID,
	)
}

func modelMissingGlyph(missing int) string {
	if missing == 0 {
		return "●"
	}
	return "!"
}

func modelStateGlyph(state catalog.State) string {
	switch state {
	case catalog.StatePresent:
		return "●"
	case catalog.StateDownloading:
		return "◉"
	case catalog.StateError:
		return "!"
	default:
		return "○"
	}
}

func modelProgress(entry catalog.Entry) string {
	if entry.SizeBytes <= 0 {
		return fallback(entry.TargetPath, "no target path")
	}
	if entry.BytesDownloaded < 1024 && entry.SizeBytes < 1024 {
		return fmt.Sprintf("%d/%d B", entry.BytesDownloaded, entry.SizeBytes)
	}
	return fmt.Sprintf(
		"%s/%s",
		formatBytes(entry.BytesDownloaded),
		formatBytes(entry.SizeBytes),
	)
}

func formatBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(value)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(value)/(1024*1024))
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
		Border(panelBorder).
		BorderForeground(lipgloss.Color(color)).
		Padding(0, 1).
		Render(subtitleStyle.Render(title) + "\n" + body)
}

func joinPanels(panels ...string) string {
	visible := make([]string, 0, len(panels))
	for _, panel := range panels {
		if strings.TrimSpace(panel) == "" {
			continue
		}
		visible = append(visible, panel)
	}
	return strings.Join(visible, "\n\n")
}

func joinPanelRow(left string, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
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

func runnerStateGlyph(state string) string {
	switch strings.ToLower(state) {
	case "running", "available", "ready":
		return "●"
	case "starting":
		return "◉"
	case "created":
		return "◐"
	case "external":
		return "◆"
	case "unavailable", "error", "missing":
		return "!"
	default:
		return "○"
	}
}

func statusMeter(active int, total int) string {
	if total <= 0 {
		return "[----------] 0/0 running"
	}

	width := 10
	filled := active * width / total
	if active > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return fmt.Sprintf(
		"[%s%s] %d/%d running",
		strings.Repeat("#", filled),
		strings.Repeat("-", width-filled),
		active,
		total,
	)
}

func formatSignalLine(label string, glyph string, state string, detail string) string {
	return fmt.Sprintf("%-11s %s %-11s %s", label, glyph, state, detail)
}

func runtimeSignalGlyph(state string) string {
	switch strings.ToLower(state) {
	case "running", "available", "ready":
		return "●"
	case "unavailable", "error", "missing":
		return "!"
	default:
		return "◐"
	}
}

func runtimeSignalMeter(state string) string {
	switch strings.ToLower(state) {
	case "running", "available", "ready":
		return "[##########] serving"
	case "starting":
		return "[######----] starting"
	case "unavailable", "error", "missing":
		return "[##--------] needs attention"
	default:
		return "[#####-----] waiting"
	}
}

func runnerSignalGlyph(active int, total int) string {
	if active > 0 {
		return "●"
	}
	if total > 0 {
		return "◐"
	}
	return "!"
}

func routeSignalGlyph(routes int) string {
	if routes > 0 {
		return "●"
	}
	return "!"
}

func routeSignalMeter(routes int, runners int) string {
	total := runners
	if total < routes {
		total = routes
	}
	if total <= 0 {
		return "[----------] 0/0 routed"
	}

	filled := routes * 10 / total
	if routes > 0 && filled == 0 {
		filled = 1
	}
	if filled > 10 {
		filled = 10
	}
	return fmt.Sprintf(
		"[%s%s] %d/%d routed",
		strings.Repeat("#", filled),
		strings.Repeat("-", 10-filled),
		routes,
		total,
	)
}

func modelSignalGlyph(present int, required int) string {
	if required > 0 && present == required {
		return "●"
	}
	if present > 0 {
		return "◐"
	}
	return "!"
}

func modelSignalMeter(present int, required int) string {
	if required <= 0 {
		return "[----------] no required models"
	}

	filled := present * 10 / required
	if present > 0 && filled == 0 {
		filled = 1
	}
	if filled > 10 {
		filled = 10
	}
	return fmt.Sprintf(
		"[%s%s] required ready",
		strings.Repeat("#", filled),
		strings.Repeat("-", 10-filled),
	)
}

func logSignalGlyph(count int) string {
	if count > 0 {
		return "●"
	}
	return "◐"
}

func settingsGlyph(
	runtimeController server.RuntimeController,
	runnerController server.RunnerController,
) string {
	if runtimeController != nil && runnerController != nil {
		return "●"
	}
	if runtimeController != nil || runnerController != nil {
		return "◐"
	}
	return "!"
}

func runnerRouteGlyph(runner server.RunnerSnapshot) string {
	if strings.TrimSpace(runner.Upstream) == "" {
		return "!"
	}
	if runnerRoleRoute(runner.Role) == "/v1/*" {
		return "◐"
	}
	return "●"
}

func runnerProcessGlyph(runner server.RunnerSnapshot) string {
	if runner.PID > 0 {
		return "●"
	}
	if runner.Launch {
		return "◐"
	}
	return "!"
}

func runnerProcessState(runner server.RunnerSnapshot) string {
	if runner.PID > 0 {
		return "pid " + strconv.Itoa(runner.PID)
	}
	if runner.Launch {
		return "not running"
	}
	return "external"
}

func runnerModelGlyph(runner server.RunnerSnapshot) string {
	if strings.TrimSpace(runner.ModelID) != "" && strings.TrimSpace(runner.ModelPath) != "" {
		return "●"
	}
	if strings.TrimSpace(runner.ModelID) != "" || strings.TrimSpace(runner.ModelPath) != "" {
		return "◐"
	}
	return "!"
}

func runnerCapabilitiesGlyph(runner server.RunnerSnapshot) string {
	if len(runner.Capabilities) > 0 {
		return "●"
	}
	return "◐"
}

func runnerLogsGlyph(runner server.RunnerSnapshot, cached int) string {
	if runner.LogSequence > 0 || cached > 0 {
		return "●"
	}
	return "◐"
}

func runnerNextAction(runner server.RunnerSnapshot) string {
	switch strings.ToLower(runner.State) {
	case "running":
		return "use x/r for process control or edit b/p/h/i/m/e/u/f/l/v/t/o"
	case "starting":
		return "wait for health, then inspect logs if it stalls"
	case "created", "stopped", "exited":
		return "press s to start or edit b/p/h/i/m/e/u/f/l/v/t/o"
	case "unavailable", "error", "missing":
		return "inspect Last error, model path, executable, and logs"
	default:
		return "review settings, route, and logs"
	}
}

func runnerHealthMeter(runner server.RunnerSnapshot) string {
	switch strings.ToLower(runner.State) {
	case "running":
		return "[##########] serving"
	case "starting":
		return "[######----] starting"
	case "created":
		return "[#####-----] ready to start"
	case "external":
		return "[#####-----] external upstream"
	case "unavailable", "error":
		return "[##--------] needs attention"
	default:
		return "[---.......] idle"
	}
}

func runnerLaunchMode(runner server.RunnerSnapshot) string {
	if !runner.Launch {
		return "external upstream"
	}
	return "managed by sidecar"
}

func runnerRoleRoute(role string) string {
	switch strings.ToLower(role) {
	case "main":
		return "/v1/chat/completions"
	case "embedding":
		return "/v1/embeddings"
	case "reranking":
		return "/v1/rerank"
	default:
		return "/v1/*"
	}
}

func endpointPath(upstream string, path string) string {
	if strings.TrimSpace(upstream) == "" || upstream == "unavailable" {
		return "unavailable"
	}
	return strings.TrimRight(upstream, "/") + path
}

func formatLogEntry(entry server.LogEntry) string {
	return fmt.Sprintf(
		"#%d %s %s %s",
		entry.Seq,
		entry.Source,
		entry.Stream,
		entry.Line,
	)
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
