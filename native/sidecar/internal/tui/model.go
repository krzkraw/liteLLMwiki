package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
const defaultChatEndpoint = "http://127.0.0.1:9379/v1/chat/completions"

const (
	dashboardModelMainX   = 23
	dashboardModelEmbedX  = 33
	dashboardModelRerankX = 49

	wizardRuntimeLlamaX  = 24
	wizardRoleMainX      = 18
	wizardRoleEmbeddingX = 25
	wizardRoleRerankingX = 37
	wizardStartX         = 6

	runnerBottomStartX   = 63
	runnerBottomStopX    = 73
	runnerBottomRestartX = 82
)

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
	ChatEndpoint      string
	ManagedScreen     bool
	LlamaRuntimeRoot  string
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

type chatCompletionMsg struct {
	prompt   string
	response string
	err      error
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

type chatMessage struct {
	role    string
	content string
}

type runnerPreset struct {
	id      string
	role    string
	backend string
	exe     string
	modelID string
	port    int
}

type llamaRuntimeVariant struct {
	Name       string
	Backend    string
	Executable string
}

type Model struct {
	runtimeController server.RuntimeController
	runnerController  server.RunnerController
	logs              *server.LogBroadcaster
	catalog           *catalog.Catalog
	ctx               context.Context
	chatEndpoint      string

	active        int
	width         int
	height        int
	snapshot      server.RunnerSnapshotResponse
	runtime       server.RuntimeStatus
	logEntries    []server.LogEntry
	models        []catalog.Entry
	notice        string
	edit          *runnerEdit
	runtimeEdit   *runtimeEdit
	runtimeDraft  server.RuntimeControlConfig
	chatDraft     string
	chatMessages  []chatMessage
	chatBusy      bool
	managedScreen bool
	scrollOffset  int

	wizardRuntime          string
	wizardBackend          string
	wizardRole             string
	wizardVariantSelection int
	wizardModelSelection   int
	llamaRuntimeRoot       string
	llamaRuntimeVariants   []llamaRuntimeVariant
	dashboardModelDropdown string
	globalMenuOpen         bool
	paletteID              string
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

type tuiPalette struct {
	id       string
	label    string
	header   string
	footerBG string
	footerFG string
	menu     string

	runtimeBG       string
	runtimeHeader   string
	runtimeButton   string
	runtimeSelected string

	variantBG       string
	variantHeader   string
	variantButton   string
	variantSelected string

	roleBG       string
	roleHeader   string
	roleButton   string
	roleSelected string

	modelBG       string
	modelSelected string
	actionBG      string
	actionFG      string
}

func tuiPalettes() []tuiPalette {
	return []tuiPalette{
		{
			id:              "neon",
			label:           "Neon",
			header:          "45",
			footerBG:        "17",
			footerFG:        "250",
			menu:            "39",
			runtimeBG:       "17",
			runtimeHeader:   "51",
			runtimeButton:   "80",
			runtimeSelected: "45",
			variantBG:       "53",
			variantHeader:   "219",
			variantButton:   "213",
			variantSelected: "201",
			roleBG:          "58",
			roleHeader:      "229",
			roleButton:      "222",
			roleSelected:    "214",
			modelBG:         "22",
			modelSelected:   "82",
			actionBG:        "28",
			actionFG:        "16",
		},
		{
			id:              "amber",
			label:           "Amber",
			header:          "214",
			footerBG:        "94",
			footerFG:        "230",
			menu:            "214",
			runtimeBG:       "58",
			runtimeHeader:   "229",
			runtimeButton:   "222",
			runtimeSelected: "214",
			variantBG:       "94",
			variantHeader:   "230",
			variantButton:   "216",
			variantSelected: "208",
			roleBG:          "52",
			roleHeader:      "217",
			roleButton:      "180",
			roleSelected:    "174",
			modelBG:         "22",
			modelSelected:   "148",
			actionBG:        "34",
			actionFG:        "16",
		},
		{
			id:              "ocean",
			label:           "Ocean",
			header:          "81",
			footerBG:        "24",
			footerFG:        "231",
			menu:            "81",
			runtimeBG:       "23",
			runtimeHeader:   "159",
			runtimeButton:   "117",
			runtimeSelected: "45",
			variantBG:       "24",
			variantHeader:   "153",
			variantButton:   "110",
			variantSelected: "75",
			roleBG:          "18",
			roleHeader:      "147",
			roleButton:      "111",
			roleSelected:    "69",
			modelBG:         "22",
			modelSelected:   "84",
			actionBG:        "35",
			actionFG:        "16",
		},
	}
}

func NewModel(options ModelOptions) Model {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	chatEndpoint := strings.TrimSpace(options.ChatEndpoint)
	if chatEndpoint == "" {
		chatEndpoint = defaultChatEndpoint
	}
	model := Model{
		runtimeController: options.RuntimeController,
		runnerController:  options.RunnerController,
		logs:              options.Logs,
		catalog:           options.Catalog,
		ctx:               ctx,
		chatEndpoint:      chatEndpoint,
		active:            0,
		managedScreen:     options.ManagedScreen,
		wizardRuntime:     "litert",
		wizardBackend:     "cpu",
		wizardRole:        "main",
		llamaRuntimeRoot:  options.LlamaRuntimeRoot,
		paletteID:         "neon",
	}
	model.refresh()
	model.normalizeWizardSelection()
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
			ManagedScreen:     true,
		}),
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
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
		m.clampScrollOffset()
	case tea.KeyMsg:
		if m.runtimeEdit != nil {
			return m.updateRuntimeEditKey(msg)
		}
		if m.edit != nil {
			return m.updateEditKey(msg)
		}
		return m.updateKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tickMsg:
		m.refresh()
		m.clampScrollOffset()
		return m, tick()
	case runnerActionMsg:
		m.refresh()
		m.notice = m.actionNotice(msg)
	case runtimeActionMsg:
		m.refresh()
		m.notice = m.runtimeActionNotice(msg)
	case runnerCreateMsg:
		m.refresh()
		if msg.err == nil {
			m.setActiveTab("runner:" + msg.runner.ID)
		}
		m.notice = m.runnerCreateNotice(msg)
	case modelDownloadMsg:
		m.refresh()
		m.setActiveTab("models")
		m.notice = m.modelDownloadNotice(msg)
	case chatCompletionMsg:
		m.chatBusy = false
		if msg.err != nil {
			m.notice = fmt.Sprintf("chat prompt failed: %v", msg.err)
			break
		}
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "assistant",
			content: msg.response,
		})
		m.notice = "sent chat prompt through /v1/chat/completions"
	case runnerUpdateMsg:
		m.refresh()
		m.setActiveTab("runner:" + msg.runner.ID)
		m.notice = m.runnerUpdateNotice(msg)
	}

	return m, nil
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Type != tea.MouseLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}

	if next, cmd, ok := m.handleBottomBarAction(msg.X, msg.Y); ok {
		return next, cmd
	}
	if next, cmd, ok := m.handleGlobalMenuClick(msg.X, msg.Y); ok {
		return next, cmd
	}
	if m.handleTabClick(msg.X, msg.Y) {
		return m, nil
	}

	switch m.activeTabID() {
	case "dashboard":
		return m.updateDashboardMouse(msg.X, msg.Y), nil
	case "wizard":
		return m.updateWizardMouse(msg.X, msg.Y)
	default:
		return m, nil
	}
}

func (m Model) handleBottomBarAction(x int, y int) (Model, tea.Cmd, bool) {
	if m.height <= 0 || y < m.height-1 {
		return m, nil, false
	}
	segment, ok := m.bottomActionAt(x)
	if !ok {
		return m, nil, false
	}
	switch segment.id {
	case "menu":
		m.globalMenuOpen = !m.globalMenuOpen
		return m, nil, true
	case "next":
		m.active = (m.active + 1) % len(m.tabs())
		m.resetScroll()
		return m, nil, true
	case "previous":
		m.active = (m.active + len(m.tabs()) - 1) % len(m.tabs())
		m.resetScroll()
		return m, nil, true
	case "quit":
		return m, tea.Quit, true
	case "runner-start":
		if runner, ok := m.activeRunner(); ok {
			return m, m.runnerActionCmd("start", runner.ID), true
		}
	case "runner-stop":
		if runner, ok := m.activeRunner(); ok {
			return m, m.runnerActionCmd("stop", runner.ID), true
		}
	case "runner-restart":
		if runner, ok := m.activeRunner(); ok {
			return m, m.runnerActionCmd("restart", runner.ID), true
		}
	}
	return m, nil, false
}

func (m Model) bottomActionAt(x int) (bottomActionSegment, bool) {
	for _, segment := range m.bottomActionSegments() {
		if x >= segment.start && x < segment.end {
			return segment, true
		}
	}
	return bottomActionSegment{}, false
}

func (m Model) handleGlobalMenuClick(x int, y int) (Model, tea.Cmd, bool) {
	if !m.globalMenuOpen || m.height <= 0 {
		return m, nil, false
	}
	menuTop := m.globalMenuTopRow()
	if y < menuTop || y >= m.height-1 {
		return m, nil, false
	}
	menuWidth := lipgloss.Width(firstRenderedLine(m.globalMenuMainView()))
	paletteX := menuWidth + panelGridColumnGap
	if x >= paletteX {
		index := y - menuTop - 2
		palettes := tuiPalettes()
		if index >= 0 && index < len(palettes) {
			m.paletteID = palettes[index].id
			m.globalMenuOpen = false
			return m, nil, true
		}
		return m, nil, false
	}

	switch y - menuTop {
	case 2:
		m.active = (m.active + 1) % len(m.tabs())
		m.resetScroll()
		m.globalMenuOpen = false
		return m, nil, true
	case 3:
		m.active = (m.active + len(m.tabs()) - 1) % len(m.tabs())
		m.resetScroll()
		m.globalMenuOpen = false
		return m, nil, true
	case 4:
		return m, nil, true
	case 5:
		return m, tea.Quit, true
	default:
		return m, nil, false
	}
}

func (m Model) globalMenuTopRow() int {
	return maxInt(0, m.height-viewLineCount(m.footerView()))
}

func (m *Model) handleTabClick(x int, y int) bool {
	if y != m.tabBarRow() {
		return false
	}
	position := 0
	for index, item := range m.tabs() {
		label := fmt.Sprintf("%d %s", index+1, item.label)
		width := lipgloss.Width(label) + 2
		if x >= position && x < position+width {
			m.active = index
			m.resetScroll()
			return true
		}
		position += width + 1
	}
	return false
}

func (m Model) updateDashboardMouse(x int, y int) Model {
	if y != m.dashboardModelRow() {
		return m
	}
	switch {
	case x >= dashboardModelRerankX:
		m.dashboardModelDropdown = "reranking"
	case x >= dashboardModelEmbedX:
		m.dashboardModelDropdown = "embedding"
	case x >= dashboardModelMainX:
		m.dashboardModelDropdown = "main"
	default:
		m.dashboardModelDropdown = ""
	}
	return m
}

func (m Model) updateWizardMouse(x int, y int) (tea.Model, tea.Cmd) {
	if index, ok := m.wizardModelIndexFromMouse(x, y); ok {
		m.setWizardModelSelection(index)
		return m, nil
	}

	switch y {
	case m.wizardChoiceRow(0):
		if x >= wizardRuntimeLlamaX {
			m.wizardRuntime = "llamacpp"
			m.wizardBackend = m.firstAvailableLlamaType()
		} else {
			m.wizardRuntime = "litert"
			m.wizardBackend = "cpu"
		}
		m.wizardModelSelection = 0
		m.normalizeWizardSelection()
	case m.wizardChoiceRow(2):
		m.setWizardVariantFromMouse(x)
	case m.wizardChoiceRow(4):
		switch {
		case x >= wizardRoleRerankingX:
			m.setWizardRole("reranking")
		case x >= wizardRoleEmbeddingX:
			m.setWizardRole("embedding")
		case x >= wizardRoleMainX:
			m.setWizardRole("main")
		}
	case m.wizardChoiceRow(6):
		if x >= wizardStartX && x < wizardStartX+12 {
			return m, m.wizardCreateCmd()
		}
	}
	return m, nil
}

func (m Model) tabBarRow() int {
	return viewLineCount(m.headerView())
}

func (m Model) contentTopRow() int {
	return viewLineCount(m.managedTopView()) + 1
}

func (m Model) dashboardModelRow() int {
	return panelContentRow(m.contentTopRow(), 9)
}

func (m Model) wizardChoiceRow(line int) int {
	return panelContentRow(m.contentTopRow(), line)
}

func (m Model) wizardModelIndexFromMouse(x int, y int) (int, bool) {
	startX := 0
	startY := m.wizardLocalModelStartRow()
	if m.usesWidePanelGrid() {
		startX = m.panelGridColumnWidth() + panelGridColumnGap
	}
	if x < startX || y < startY {
		return 0, false
	}
	candidates := m.wizardCandidateModels()
	if len(candidates) == 0 {
		return 0, false
	}
	index := y - startY
	if index < 0 || index >= len(candidates) {
		return 0, false
	}
	return index, true
}

func (m Model) wizardLocalModelStartRow() int {
	if m.usesWidePanelGrid() {
		return panelContentRow(m.contentTopRow(), 0)
	}
	choicePanel := renderPanelSpec(
		panelSpec{"Launch Wizard", m.wizardChoiceLines(), "214"},
		m.width,
	)
	localPanelTop := m.contentTopRow() + viewLineCount(choicePanel) + 1
	return panelContentRow(localPanelTop, 0)
}

func (m Model) usesWidePanelGrid() bool {
	return m.width >= widePanelGridMinWidth
}

func (m Model) panelGridColumnWidth() int {
	return (m.width - panelGridColumnGap) / 2
}

func panelContentRow(panelTop int, line int) int {
	return panelTop + 2 + line
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.managedScreen {
		if next, ok := m.updateScrollKey(msg); ok {
			return next, nil
		}
	}

	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyF1:
		m.globalMenuOpen = !m.globalMenuOpen
		return m, nil
	case tea.KeyRight, tea.KeyTab:
		m.active = (m.active + 1) % len(m.tabs())
		m.resetScroll()
		return m, nil
	case tea.KeyLeft, tea.KeyShiftTab:
		m.active = (m.active + len(m.tabs()) - 1) % len(m.tabs())
		m.resetScroll()
		return m, nil
	case tea.KeyBackspace, tea.KeyCtrlH, tea.KeyEnter:
		if m.activeTabID() == "wizard" && msg.Type == tea.KeyEnter {
			return m, m.wizardCreateCmd()
		}
		if m.activeTabID() == "chat" {
			return m.updateChatKey(msg)
		}
	case tea.KeyRunes:
		value := msg.String()
		if m.selectRuneTab(value) {
			return m, nil
		}
		if m.activeTabID() == "chat" {
			return m.updateChatKey(msg)
		}
		if m.activeTabID() == "wizard" {
			switch strings.ToLower(value) {
			case "t":
				m.toggleWizardRuntime()
				return m, nil
			case "b":
				m.cycleWizardVariant()
				return m, nil
			case "m":
				m.setWizardRole("main")
				return m, nil
			case "e":
				m.setWizardRole("embedding")
				return m, nil
			case "r":
				m.setWizardRole("reranking")
				return m, nil
			case "n":
				m.cycleWizardModel(1)
				return m, nil
			case "p":
				m.cycleWizardModel(-1)
				return m, nil
			}
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
			case "w":
				m.setActiveTab("wizard")
				m.resetScroll()
				return m, nil
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

func (m Model) updateScrollKey(msg tea.KeyMsg) (Model, bool) {
	switch msg.Type {
	case tea.KeyUp:
		m.scrollOffset--
	case tea.KeyDown:
		m.scrollOffset++
	case tea.KeyPgUp:
		m.scrollOffset -= m.scrollPageSize()
	case tea.KeyPgDown:
		m.scrollOffset += m.scrollPageSize()
	case tea.KeyHome:
		m.scrollOffset = 0
	case tea.KeyEnd:
		m.scrollOffset = m.maxScrollOffset()
	default:
		return m, false
	}
	m.clampScrollOffset()
	return m, true
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

func (m Model) updateChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.chatDraft) > 0 {
			m.chatDraft = m.chatDraft[:len(m.chatDraft)-1]
		}
		return m, nil
	case tea.KeyEnter:
		prompt := strings.TrimSpace(m.chatDraft)
		if prompt == "" || m.chatBusy {
			return m, nil
		}
		runner, ok := m.selectedChatRunner()
		if !ok {
			m.notice = "chat prompt failed: no main runner configured"
			return m, nil
		}
		m.chatDraft = ""
		m.chatBusy = true
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "user",
			content: prompt,
		})
		return m, m.chatCompletionCmd(prompt, runner)
	case tea.KeyRunes:
		m.chatDraft += msg.String()
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) View() string {
	if m.managedScreen {
		if m.height <= 0 {
			return m.managedStartupView()
		}
		return m.managedScreenView()
	}
	return m.fullView()
}

func (m Model) fullView() string {
	var builder strings.Builder
	builder.WriteString(m.headerView())
	builder.WriteString("\n")
	builder.WriteString(m.tabBar())
	builder.WriteString("\n\n")
	if strings.TrimSpace(m.notice) != "" {
		builder.WriteString(noticeStyle.Render(m.notice))
		builder.WriteString("\n\n")
	}

	builder.WriteString(m.activeContentView())
	builder.WriteString("\n\n")
	builder.WriteString(m.footerView())

	return builder.String()
}

func (m Model) managedScreenView() string {
	top := m.managedTopView()
	footer := m.footerView()
	bodyHeight := managedBodyHeight(m.height, top, footer)
	if bodyHeight <= 0 {
		topHeight := m.height - viewLineCount(footer) - 2
		if topHeight < 0 {
			topHeight = 0
		}
		return fitLinesToHeight(joinPanels(sliceRenderedLines(top, 0, topHeight), footer), m.height)
	}

	body := m.activeContentView()
	visibleBody := sliceRenderedLines(body, m.scrollOffset, bodyHeight)
	if strings.TrimSpace(visibleBody) == "" {
		visibleBody = mutedStyle.Render("No content in this pane.")
	}

	return fitLinesToHeight(joinPanels(top, visibleBody, footer), m.height)
}

func (m Model) managedStartupView() string {
	return joinPanels(
		m.headerView(),
		m.tabBar(),
		renderPanel(
			"Managed screen",
			[]string{
				"Measuring terminal before drawing the TUI frame.",
				"Content will scroll inside the app, not the terminal.",
			},
			"45",
		),
	)
}

func (m Model) managedTopView() string {
	var builder strings.Builder
	builder.WriteString(m.headerView())
	builder.WriteString("\n")
	builder.WriteString(m.tabBar())
	if strings.TrimSpace(m.notice) != "" {
		builder.WriteString("\n\n")
		builder.WriteString(noticeStyle.Render(m.notice))
	}
	return builder.String()
}

func (m Model) activeContentView() string {
	switch m.activeTabID() {
	case "dashboard":
		return m.dashboardView()
	case "wizard":
		return m.wizardView()
	default:
		if runner, ok := m.activeRunner(); ok {
			return m.runnerView(runner)
		}
		return renderPanel("Runner", []string{"No runner selected."}, "196")
	}
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
	result := []tab{
		{id: "dashboard", label: "Dashboard"},
		{id: "wizard", label: "Launch Wizard"},
	}
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

func (m Model) wizardTabLabel() string {
	return "◇ Launch Wizard"
}

func (m Model) chatTabLabel() string {
	runner, ok := m.selectedChatRunner()
	if !ok {
		return "○ Chat no main"
	}
	return runnerStateGlyph(runner.State) + " Chat " + runner.ID
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
	m.llamaRuntimeVariants = discoverLlamaRuntimeVariants(m.llamaRuntimeRoot)
	m.normalizeWizardSelection()
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
	if m.active != index {
		m.resetScroll()
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

func (m Model) palette() tuiPalette {
	palettes := tuiPalettes()
	for _, palette := range palettes {
		if palette.id == m.paletteID {
			return palette
		}
	}
	return palettes[0]
}

func (m Model) headerView() string {
	parts := []string{
		titleStyle.Render("◆ LiteRT sidecar"),
		"LiteRT: " + runtimeUseBadge(m.runtimeAliveCount("litert")),
		"llama.cpp: " + runtimeUseBadge(m.runtimeAliveCount("llamacpp")),
		fmt.Sprintf("Runners: %d", len(m.snapshot.Runners)),
	}
	if m.width > 0 || m.height > 0 {
		parts = append(parts, fmt.Sprintf("Viewport: %dx%d", m.width, m.height))
	}

	style := lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(lipgloss.Color(m.palette().header)).
		Padding(0, 1)
	if m.width > 2 {
		style = style.Width(m.width - 2)
	}
	return style.Render(strings.Join(parts, "  "))
}

func (m Model) runnerByID(id string) (server.RunnerSnapshot, bool) {
	for _, runner := range m.snapshot.Runners {
		if runner.ID == id {
			return runner, true
		}
	}
	return server.RunnerSnapshot{}, false
}

func (m Model) runtimeAliveCount(runtimeName string) int {
	count := 0
	for _, runner := range m.snapshot.Runners {
		if runner.Runtime != runtimeName || !runnerIsAlive(runner) {
			continue
		}
		count++
	}
	return count
}

func runtimeUseBadge(alive int) string {
	if alive > 0 {
		return coloredStatus("active", runtimeUseBadgeColor(alive))
	}
	return coloredStatus("idle", runtimeUseBadgeColor(alive))
}

func runtimeUseBadgeColor(alive int) string {
	if alive > 0 {
		return "82"
	}
	return "196"
}

func (m Model) selectedChatRunner() (server.RunnerSnapshot, bool) {
	var fallbackRunner server.RunnerSnapshot
	hasFallback := false
	for _, runner := range m.snapshot.Runners {
		if runner.Role != "main" {
			continue
		}
		if strings.EqualFold(runner.State, "running") {
			return runner, true
		}
		if !hasFallback {
			fallbackRunner = runner
			hasFallback = true
		}
	}
	return fallbackRunner, hasFallback
}

func (m *Model) setActiveTab(id string) {
	for index, item := range m.tabs() {
		if item.id == id {
			if m.active != index {
				m.resetScroll()
			}
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
	line := strings.Join(parts, " ")
	if m.width > 0 {
		return lipgloss.NewStyle().Width(m.width).Render(line)
	}
	return line
}

func (m Model) missionControlView() string {
	return renderPanelWidth("Mission control / Live state", m.missionControlLines(), "45", m.width)
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
	case "wizard":
		return "◇ Launch Wizard / RunnerController.CreateRunner"
	case "chat":
		if runner, ok := m.selectedChatRunner(); ok {
			return "● Chat / " + runner.ID + " -> /v1/chat/completions"
		}
		return "○ Chat / no main runner"
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
	return renderCommandRail(m.commandRailLines())
}

func (m Model) footerView() string {
	if !m.globalMenuOpen {
		return m.bottomActionBarView()
	}
	return joinPanels(m.globalMenuView(), m.bottomActionBarView())
}

func (m Model) globalMenuView() string {
	menu := m.globalMenuMainView()
	paletteMenu := m.paletteMenuView()
	return lipgloss.JoinHorizontal(lipgloss.Top, menu, strings.Repeat(" ", panelGridColumnGap), paletteMenu)
}

func (m Model) globalMenuMainView() string {
	return renderPanel(
		"Global menu",
		[]string{
			"Next tab        change view",
			"Previous tab    change view",
			"Palettes >      choose theme",
			"Esc Quit",
		},
		m.palette().menu,
	)
}

func (m Model) paletteMenuView() string {
	palettes := tuiPalettes()
	lines := make([]string, 0, len(palettes))
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("16")).
		Background(lipgloss.Color(m.palette().actionBG)).
		Padding(0, 1)
	for _, palette := range palettes {
		label := palette.label
		if palette.id == m.paletteID {
			label = selectedStyle.Render("● " + label)
		} else {
			label = "  " + label
		}
		lines = append(lines, label)
	}
	return renderPanel("Palettes", lines, m.palette().menu)
}

func (m Model) bottomActionBarView() string {
	palette := m.palette()
	segments := m.bottomActionSegments()

	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color(palette.footerFG)).
			Background(lipgloss.Color(palette.footerBG))
		switch segment.id {
		case "menu":
			style = style.Bold(true).Foreground(lipgloss.Color(palette.runtimeHeader))
		case "runner-start":
			style = style.Bold(true).
				Foreground(lipgloss.Color(palette.actionFG)).
				Background(lipgloss.Color(palette.actionBG))
		case "runner-stop", "runner-restart":
			style = style.Bold(true).Foreground(lipgloss.Color(palette.variantHeader))
		case "hint":
			style = style.Faint(true)
		}
		parts = append(parts, style.Render(segment.label))
	}
	line := strings.Join(parts, separatorStyle(palette).Render(" | "))
	if m.width > 0 {
		line = truncateToWidth(line, maxInt(0, m.width-2))
	}
	style := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(lipgloss.Color(palette.footerFG)).
		Background(lipgloss.Color(palette.footerBG))
	if m.width > 0 {
		style = style.Width(m.width)
	}
	return style.Render(line)
}

type bottomActionSegment struct {
	id    string
	label string
	start int
	end   int
}

func (m Model) bottomActionSegments() []bottomActionSegment {
	items := []bottomActionSegment{
		{id: "menu", label: "Menu"},
		{id: "next", label: "Tab Next"},
		{id: "previous", label: "Shift+Tab Prev"},
		{id: "quit", label: "Esc Quit"},
	}
	switch m.activeTabID() {
	case "dashboard":
		items = append(items, bottomActionSegment{id: "hint", label: "Dashboard: click model roles"})
	case "wizard":
		items = append(items, bottomActionSegment{id: "hint", label: "Wizard: click toggles | Enter Start"})
	default:
		if runner, ok := m.activeRunner(); ok {
			items = append(
				items,
				bottomActionSegment{id: "hint", label: fmt.Sprintf("Runner %s", runner.ID)},
				bottomActionSegment{id: "runner-start", label: "Start"},
				bottomActionSegment{id: "runner-stop", label: "Stop"},
				bottomActionSegment{id: "runner-restart", label: "Restart"},
			)
		}
	}

	position := 1
	for index := range items {
		items[index].start = position
		items[index].end = position + lipgloss.Width(items[index].label)
		position = items[index].end + 3
	}
	return items
}

func separatorStyle(palette tuiPalette) lipgloss.Style {
	return lipgloss.NewStyle().
		Faint(true).
		Foreground(lipgloss.Color(palette.footerFG)).
		Background(lipgloss.Color(palette.footerBG))
}

func (m Model) commandRailLines() []string {
	scrollLine := ""
	if m.managedScreen {
		scrollLine = m.scrollStatusLine()
	}
	return m.commandRailLinesWithScrollLine(scrollLine)
}

func (m Model) commandRailViewWithScrollLine(scrollLine string) string {
	return renderCommandRail(m.commandRailLinesWithScrollLine(scrollLine))
}

func (m Model) commandRailLinesWithScrollLine(scrollLine string) []string {
	lines := []string{
		fmt.Sprintf("Global: Tab/Right next | Shift+Tab/Left previous | 1-%d jump | Esc quit", len(m.tabs())),
	}
	if m.managedScreen && strings.TrimSpace(scrollLine) != "" {
		lines = append(lines, scrollLine)
	}

	switch m.activeTabID() {
	case "dashboard":
		lines = append(
			lines,
			"Dashboard: specs + topology + runnable backends",
			"API: status.get + /sidecar/v1/status",
		)
	case "wizard":
		lines = append(
			lines,
			"Launch Wizard: t Runtime | b Variant | m/e/r Role | n/p Model | Enter create",
			"API: RunnerController.CreateRunner + POST /sidecar/v1/runners",
			"API: WebSocket api.request POST /sidecar/v1/runners",
		)
	case "chat":
		lines = append(
			lines,
			"Chat: type prompt | Enter send | Backspace edit",
			"API: POST /v1/chat/completions through supervisor route authority",
		)
	case "models":
		lines = append(
			lines,
			"Models: d Download | w Launch wizard",
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
	status := panelSpec{"Status", m.dashboardStatusLines(), "45"}
	if m.dashboardModelDropdown == "" {
		return renderPanelSpec(status, m.width)
	}
	return m.panelGrid(
		status,
		panelSpec{
			roleDisplayName(m.dashboardModelDropdown) + " models",
			m.modelDropdownItemLines(m.dashboardModelDropdown),
			"205",
		},
	)
}

func (m Model) dashboardStatusLines() []string {
	return []string{
		"Runners by runtime",
		fmt.Sprintf("LiteRT      %d alive", m.runtimeAliveCount("litert")),
		fmt.Sprintf("llama.cpp   %d alive", m.runtimeAliveCount("llamacpp")),
		"",
		"Runners by role",
		fmt.Sprintf("Main        %d alive", m.roleAliveCount("main")),
		fmt.Sprintf("Embedding   %d alive", m.roleAliveCount("embedding")),
		fmt.Sprintf("Reranking   %d alive", m.roleAliveCount("reranking")),
		"",
		fmt.Sprintf(
			"Models ---- Main %d -- Embedding %d -- Reranking %d",
			m.presentModelCount("main"),
			m.presentModelCount("embedding"),
			m.presentModelCount("reranking"),
		),
	}
}

func (m Model) roleAliveCount(role string) int {
	count := 0
	for _, runner := range m.snapshot.Runners {
		if runner.Role != role || !runnerIsAlive(runner) {
			continue
		}
		count++
	}
	return count
}

func runnerIsAlive(runner server.RunnerSnapshot) bool {
	return strings.EqualFold(runner.State, "running")
}

func (m Model) presentModelCount(role string) int {
	count := 0
	for _, entry := range m.models {
		if entry.Role == role && entry.State == catalog.StatePresent {
			count++
		}
	}
	return count
}

func (m Model) modelDropdownLines(role string) []string {
	title := roleDisplayName(role) + " models"
	return append([]string{title}, m.modelDropdownItemLines(role)...)
}

func (m Model) modelDropdownItemLines(role string) []string {
	lines := []string{}
	found := false
	for _, entry := range m.models {
		if entry.Role != role || entry.State != catalog.StatePresent {
			continue
		}
		found = true
		lines = append(lines, "• "+entry.ID+"  "+entry.Filename)
	}
	if !found {
		lines = append(lines, "No local "+role+" models.")
	}
	return lines
}

func roleDisplayName(role string) string {
	switch role {
	case "embedding":
		return "Embedding"
	case "reranking":
		return "Reranking"
	default:
		return "Main"
	}
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
	return m.panelGrid(
		panelSpec{"Runner " + runner.ID, m.runnerSummaryLines(runner), "82"},
		panelSpec{"Routes / Controls", m.runnerRouteControlLines(runner), "45"},
	)
}

func (m Model) runnerSummaryLines(runner server.RunnerSnapshot) []string {
	lines := []string{
		formatKV("State", fallback(runner.State, "unknown")),
		formatKV("Runtime", fallback(runner.Runtime, "unknown")),
		formatKV("Role", fallback(runner.Role, "unknown")),
		formatKV("Backend", fallback(runner.Backend, "default")),
		formatKV("Model path", fallback(runner.ModelPath, "not configured")),
		formatKV("Model ID", fallback(runner.ModelID, "not configured")),
		formatKV("Upstream", fallback(runner.Upstream, "unavailable")),
		formatKV("PID", fallbackInt(runner.PID, "not running")),
	}
	if runner.Detail != "" {
		lines = append(lines, formatKV("Detail", runner.Detail))
	}
	return lines
}

func (m Model) runnerRouteControlLines(runner server.RunnerSnapshot) []string {
	basePath := "/sidecar/v1/runners/" + runner.ID
	lines := []string{
		formatKV("Route", runnerRoleRoute(runner.Role)),
		formatKV("Upstream", fallback(runner.Upstream, "unavailable")),
		formatKV("Models", endpointPath(runner.Upstream, "/v1/models")),
		"",
		"Actions ---- s Start -- x Stop -- r Restart",
		"POST " + basePath + "/start",
		"POST " + basePath + "/stop",
		"POST " + basePath + "/restart",
	}
	switch runner.Role {
	case "main":
		lines = append(lines, "OpenAI ---- "+runnerRoleRoute(runner.Role))
	case "embedding":
		lines = append(lines, "OpenAI ---- "+runnerRoleRoute(runner.Role))
	case "reranking":
		lines = append(lines, "OpenAI ---- "+runnerRoleRoute(runner.Role))
	}
	return lines
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
	return renderPanelSpec(m.runnerEditorSpec(runner), 0)
}

func (m Model) runnerEditorSpec(runner server.RunnerSnapshot) panelSpec {
	if m.edit == nil || m.edit.runner.ID != runner.ID {
		return panelSpec{}
	}
	newValue := m.edit.value
	if m.edit.secret {
		newValue = secretEditValue(m.edit.value)
	}
	return panelSpec{
		"Editing " + m.edit.label + " for " + runner.ID,
		[]string{
			formatKV("Current", fallback(m.edit.current, "not configured")),
			"",
			"New value",
			newValue,
			"Enter saves through PATCH /sidecar/v1/runners/{id}; Esc cancels.",
		},
		"205",
	}
}

func (m Model) wizardView() string {
	return m.panelGrid(
		panelSpec{"Launch Wizard", m.wizardChoiceLines(), "214"},
		panelSpec{"Local Models", m.wizardLocalModelLines(), "45"},
	)
}

func (m Model) wizardChoiceLines() []string {
	return []string{
		m.wizardOptionBar(
			"runtime",
			[]wizardOption{
				{label: "litert", selected: m.wizardRuntime == "litert"},
				{label: "llama.cpp", selected: m.wizardRuntime == "llamacpp"},
			},
			m.palette().runtimeBG,
			m.palette().runtimeHeader,
			m.palette().runtimeButton,
			m.palette().runtimeSelected,
		),
		"",
		m.wizardVariantToggleLine(),
		"",
		m.wizardOptionBar(
			"model role",
			[]wizardOption{
				{label: "main", selected: m.wizardRole == "main"},
				{label: "embedding", selected: m.wizardRole == "embedding"},
				{label: "reranking", selected: m.wizardRole == "reranking"},
			},
			m.palette().roleBG,
			m.palette().roleHeader,
			m.palette().roleButton,
			m.palette().roleSelected,
		),
		"",
		m.wizardStartLine(),
	}
}

func (m Model) wizardLocalModelLines() []string {
	lines := []string{}
	candidates := m.wizardCandidateModels()
	if len(candidates) == 0 {
		return []string{"no local models match this runtime, variant, and role"}
	}
	selected := clampInt(m.wizardModelSelection, 0, len(candidates)-1)
	for index, entry := range candidates {
		prefix := "  "
		if index == selected {
			prefix = "> "
		}
		line := prefix + entry.Filename
		if index == selected {
			lines = append(lines, m.fullWidthWizardLine(
				lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("16")).
					Background(lipgloss.Color(m.palette().modelSelected)),
				line,
			))
			continue
		}
		lines = append(lines, m.fullWidthWizardLine(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color(m.palette().modelBG)),
			line,
		))
	}
	return lines
}

func selectedToken(label string, selected bool) string {
	if selected {
		return "[" + label + "]"
	}
	return label
}

type wizardOption struct {
	label    string
	selected bool
}

func (m Model) wizardOptionBar(
	label string,
	options []wizardOption,
	background string,
	header string,
	button string,
	selected string,
) string {
	headerText := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(header)).
		Background(lipgloss.Color(background)).
		Padding(0, 1).
		Render(label)

	parts := []string{headerText}
	for _, option := range options {
		text := selectedToken(option.label, option.selected)
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color(button)).
			Background(lipgloss.Color(background)).
			Padding(0, 1)
		if option.selected {
			style = style.
				Bold(true).
				Foreground(lipgloss.Color("16")).
				Background(lipgloss.Color(selected))
		}
		parts = append(parts, style.Render(text))
	}

	return m.fullWidthWizardLine(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(button)).
			Background(lipgloss.Color(background)),
		strings.Join(parts, " "),
	)
}

func (m Model) wizardStartLine() string {
	button := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.palette().actionFG)).
		Background(lipgloss.Color(m.palette().actionBG)).
		Padding(0, 1).
		Render("[ START ]")
	return m.fullWidthWizardLine(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.palette().actionFG)).
			Background(lipgloss.Color(m.palette().actionBG)),
		button,
	)
}

func (m Model) fullWidthWizardLine(style lipgloss.Style, line string) string {
	width := m.wizardContentWidth()
	if width <= 0 {
		return style.Render(line)
	}
	line = truncateToWidth(line, width)
	style = style.Width(width)
	return style.Render(line)
}

func (m Model) wizardContentWidth() int {
	width := m.width
	if m.usesWidePanelGrid() {
		width = m.panelGridColumnWidth()
	}
	return panelInnerWidth(width)
}

func (m Model) wizardVariantToggleLine() string {
	if m.wizardRuntime == "llamacpp" {
		options := make([]wizardOption, 0, len(llamaTypeOptions()))
		for _, option := range llamaTypeOptions() {
			options = append(options, wizardOption{
				label:    option,
				selected: m.wizardBackend == option,
			})
		}
		return m.wizardOptionBar(
			"llama type",
			options,
			m.palette().variantBG,
			m.palette().variantHeader,
			m.palette().variantButton,
			m.palette().variantSelected,
		)
	}
	options := make([]wizardOption, 0, len(litertBackendOptions()))
	for _, option := range litertBackendOptions() {
		options = append(options, wizardOption{
			label:    option,
			selected: m.wizardBackend == option,
		})
	}
	return m.wizardOptionBar(
		"LiteRT backend",
		options,
		m.palette().variantBG,
		m.palette().variantHeader,
		m.palette().variantButton,
		m.palette().variantSelected,
	)
}

func (m Model) wizardSelectionLines() []string {
	variant := m.wizardVariantLabel()
	lines := []string{
		formatKV("Runtime", m.wizardRuntime),
		formatKV("Variant", variant),
		formatKV("Variants", strings.Join(m.wizardVariantNames(), ", ")),
		formatKV("Role", m.wizardRole),
		formatKV("Route", runnerRoleRoute(m.wizardRole)),
		"",
		"t Runtime toggle litert/llamacpp",
		"b Variant cycle after runtime selection",
		"m/e/r Role main/embedding/reranking",
		"n/p Model cycle downloaded applicable models",
		"Enter creates through RunnerController.CreateRunner",
		"POST /sidecar/v1/runners",
		"WebSocket api.request POST /sidecar/v1/runners",
	}
	if m.wizardRuntime == "llamacpp" && len(m.llamaRuntimeVariants) == 0 {
		lines = append(lines, "", "Install llama runtime folders under native/llama-runtimes first.")
	}
	return lines
}

func (m Model) wizardVariantNames() []string {
	if m.wizardRuntime == "llamacpp" {
		if len(m.llamaRuntimeVariants) == 0 {
			return []string{"none installed"}
		}
		names := make([]string, 0, len(m.llamaRuntimeVariants))
		for _, variant := range m.llamaRuntimeVariants {
			names = append(names, variant.Name)
		}
		return names
	}
	return litertBackendOptions()
}

func (m Model) wizardModelLines() []string {
	candidates := m.wizardCandidateModels()
	if len(candidates) == 0 {
		return []string{
			fmt.Sprintf(
				"No downloaded %s/%s models.",
				m.wizardRuntime,
				m.wizardRole,
			),
			"Download or place a matching model, then refresh will show it here.",
		}
	}

	selectedIndex := clampInt(m.wizardModelSelection, 0, len(candidates)-1)
	lines := make([]string, 0, len(candidates)+1)
	for index, entry := range candidates {
		marker := " "
		if index == selectedIndex {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf(
			"%s %s  %s  %s",
			marker,
			entry.ID,
			entry.Filename,
			entry.TargetPath,
		))
	}
	return lines
}

func (m Model) wizardDryRunLines() []string {
	spec, entry, err := m.wizardRunnerSpec()
	if err != nil {
		return []string{
			"Selected runner unavailable.",
			err.Error(),
		}
	}

	return []string{
		formatKV("ID", spec.ID),
		formatKV("Role", spec.Role),
		formatKV("Runtime", spec.Runtime),
		formatKV("Backend", spec.Backend),
		formatKV("Variant", m.wizardVariantLabel()),
		formatKV("Model", spec.ModelID),
		formatKV("Catalog entry", entry.ID),
		formatKV("Model path", spec.ModelPath),
		formatKV("Executable", fallback(spec.Executable, "default runtime executable")),
		formatKV("Host", spec.Host),
		formatKV("Port", strconv.Itoa(spec.Port)),
		formatKV("API route", runnerRoleRoute(spec.Role)),
		formatKV("Launch", runnerSpecLaunchMode(spec)),
		formatKV("Upstream", spec.Upstream),
		"",
		"Command preview",
		wizardCommandPreview(spec),
	}
}

func (m Model) chatView() string {
	return m.panelGrid(
		panelSpec{"Chat console / Main runner", m.chatRunnerLines(), "82"},
		panelSpec{"Composer", m.chatComposerLines(), "214"},
		panelSpec{"Transcript", m.chatTranscriptLines(), "45"},
		panelSpec{"API parity / Route authority", m.chatParityLines(), "39"},
	)
}

func (m Model) chatRunnerLines() []string {
	runner, ok := m.selectedChatRunner()
	if !ok {
		return []string{
			"Selected runner: none",
			"Create or start a main runner from Launch Wizard or Models.",
		}
	}

	return []string{
		formatKV("Selected runner", runner.ID),
		formatKV("State", fallback(runner.State, "unknown")),
		formatKV("Route", runnerRoleRoute(runner.Role)),
		formatKV("Upstream", fallback(runner.Upstream, "unavailable")),
		formatKV("Model", fallback(runner.ModelID, "not configured")),
		formatKV("Backend", fallback(runner.Backend, "default")),
		formatKV("Capabilities", capabilitiesLine(runner.Capabilities)),
	}
}

func (m Model) chatComposerLines() []string {
	prompt := m.chatDraft
	if prompt == "" {
		prompt = "(empty)"
	}
	state := "ready"
	if m.chatBusy {
		state = "waiting for response"
	}
	return []string{
		formatKV("Prompt", prompt),
		formatKV("State", state),
		"Enter sends via POST /v1/chat/completions; Backspace edits.",
		"Requests use stream=false and the selected main runner model ID.",
	}
}

func (m Model) chatTranscriptLines() []string {
	if len(m.chatMessages) == 0 {
		return []string{"No messages yet."}
	}

	lines := make([]string, 0, len(m.chatMessages))
	for _, message := range m.chatMessages {
		prefix := "Assistant"
		if message.role == "user" {
			prefix = "You"
		}
		lines = append(lines, prefix+": "+message.content)
	}
	return lines
}

func (m Model) chatParityLines() []string {
	lines := []string{
		"TUI Enter -> POST " + m.chatEndpoint,
		"/v1/chat/completions -> runner supervisor route authority",
	}
	if runner, ok := m.selectedChatRunner(); ok {
		lines = append(
			lines,
			"Selected runner: "+runner.ID,
			"Route: main -> "+runner.ID,
		)
	}
	return lines
}

func (m Model) modelsView() string {
	return m.panelGrid(
		panelSpec{"Model readiness / Required artifacts", m.modelReadinessLines(), "82"},
		panelSpec{"Runner creation / Launch Wizard", modelActionLines(), "214"},
		panelSpec{"Catalog cards / Download state", m.modelCatalogCardLines(), "45"},
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
		"Open Launch Wizard",
		"w Launch wizard with runtime, variant, role, and downloaded model filters",
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
	return m.panelGrid(
		panelSpec{"Log signal / Live cache", m.logSignalLines(), "82"},
		panelSpec{"Source activity / Streams", m.logSourceActivityLines(), "45"},
		panelSpec{"Recent log events", m.recentLogEventLines(16), "244"},
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

	return m.panelGrid(
		panelSpec{
			"Settings",
			[]string{
				"Controls",
				"s Start release   d Start debug   r Restart release   g Restart debug   x Stop runtime",
				"",
				formatKV("Runtime controller", runtimeState),
				formatKV("Runner controller", runnerState),
				formatKV("HTTP listen", "configured by -addr"),
				formatKV("Default upstream", fallback(m.runtime.Upstream, "unavailable")),
				formatKV("Runtime mode", fallback(m.runtime.Mode, "release")),
				formatKV("Log entries", fmt.Sprintf("%d cached", len(m.logEntries))),
			},
			"45",
		},
		panelSpec{"Runtime config editor", m.runtimeConfigLines(), "82"},
		m.runtimeEditorSpec(),
		panelSpec{"Shared action map", settingsActionMapLines(), "214"},
		panelSpec{"Runner API parity / Live snapshot", m.settingsRunnerParityLines(), "39"},
		panelSpec{"WebSocket API parity", settingsAPIParityLines(), "205"},
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
		"Wizard Enter -> RunnerController.CreateRunner -> POST /sidecar/v1/runners",
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
	return renderPanelSpec(m.runtimeEditorSpec(), 0)
}

func (m Model) runtimeEditorSpec() panelSpec {
	if m.runtimeEdit == nil {
		return panelSpec{}
	}
	newValue := m.runtimeEdit.value
	if m.runtimeEdit.secret {
		newValue = secretEditValue(m.runtimeEdit.value)
	}
	return panelSpec{
		"Editing " + m.runtimeEdit.label,
		[]string{
			formatKV("Current", fallback(m.runtimeEdit.current, "not configured")),
			"",
			"New value",
			newValue,
			"Enter stores this config for runtime.start/runtime.restart; Esc cancels.",
		},
		"205",
	}
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

func (m Model) wizardCreateCmd() tea.Cmd {
	return func() tea.Msg {
		if m.runnerController == nil {
			return runnerCreateMsg{
				label: m.wizardRole,
				err:   fmt.Errorf("runner controller is not configured"),
			}
		}

		spec, entry, err := m.wizardRunnerSpec()
		if err != nil {
			return runnerCreateMsg{
				label: m.wizardRole,
				err:   err,
			}
		}

		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()

		runner, err := m.runnerController.CreateRunner(ctx, spec)
		if err == nil {
			runner, err = m.runnerController.StartRunner(ctx, runner.ID)
		}
		return runnerCreateMsg{
			label:  entry.ID,
			runner: runner,
			err:    err,
		}
	}
}

func (m Model) chatCompletionCmd(prompt string, runner server.RunnerSnapshot) tea.Cmd {
	endpoint := m.chatEndpoint
	modelID := fallback(runner.ModelID, runner.ID)
	ctx := m.ctx

	return func() tea.Msg {
		response, err := postChatCompletion(ctx, endpoint, modelID, prompt)
		return chatCompletionMsg{
			prompt:   prompt,
			response: response,
			err:      err,
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

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatAPIItem `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatAPIItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatAPIItem `json:"message"`
	} `json:"choices"`
}

func postChatCompletion(
	ctx context.Context,
	endpoint string,
	modelID string,
	prompt string,
) (string, error) {
	payload := chatCompletionRequest{
		Model: modelID,
		Messages: []chatAPIItem{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Stream: false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("create chat request: %w", err)
	}
	request.Header.Set("content-type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("send chat request: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read chat response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("chat response %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", fmt.Errorf("decode chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("chat response had no choices")
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("chat response was empty")
	}
	return content, nil
}

func (m *Model) toggleWizardRuntime() {
	if m.wizardRuntime == "litert" {
		m.wizardRuntime = "llamacpp"
		m.wizardBackend = m.firstAvailableLlamaType()
	} else {
		m.wizardRuntime = "litert"
		m.wizardBackend = "cpu"
	}
	m.wizardVariantSelection = 0
	m.wizardModelSelection = 0
	m.normalizeWizardSelection()
}

func (m *Model) cycleWizardVariant() {
	if m.wizardRuntime == "llamacpp" {
		options := llamaTypeOptions()
		current := 0
		for index, option := range options {
			if option == m.wizardBackend {
				current = index
				break
			}
		}
		m.wizardBackend = options[(current+1)%len(options)]
		m.normalizeWizardSelection()
		return
	}

	options := litertBackendOptions()
	current := 0
	for index, option := range options {
		if option == m.wizardBackend {
			current = index
			break
		}
	}
	m.wizardBackend = options[(current+1)%len(options)]
	m.normalizeWizardSelection()
}

func (m *Model) setWizardRole(role string) {
	m.wizardRole = role
	m.wizardModelSelection = 0
	m.normalizeWizardSelection()
}

func (m *Model) cycleWizardModel(delta int) {
	candidates := m.wizardCandidateModels()
	if len(candidates) == 0 {
		m.wizardModelSelection = 0
		return
	}
	m.wizardModelSelection = (m.wizardModelSelection + delta) % len(candidates)
	if m.wizardModelSelection < 0 {
		m.wizardModelSelection += len(candidates)
	}
}

func (m *Model) setWizardVariantFromMouse(x int) {
	if m.wizardRuntime == "llamacpp" {
		options := llamaTypeOptions()
		index := clampInt((x-17)/8, 0, len(options)-1)
		m.wizardBackend = options[index]
		m.normalizeWizardSelection()
		return
	}
	options := litertBackendOptions()
	index := clampInt((x-22)/6, 0, len(options)-1)
	m.wizardBackend = options[index]
	m.normalizeWizardSelection()
}

func (m *Model) setWizardModelSelection(index int) {
	candidates := m.wizardCandidateModels()
	if len(candidates) == 0 {
		m.wizardModelSelection = 0
		return
	}
	m.wizardModelSelection = clampInt(index, 0, len(candidates)-1)
}

func (m *Model) normalizeWizardSelection() {
	if m.wizardRuntime != "litert" && m.wizardRuntime != "llamacpp" {
		m.wizardRuntime = "litert"
	}
	if !isWizardRole(m.wizardRole) {
		m.wizardRole = "main"
	}
	if m.wizardRuntime == "litert" && !containsString(litertBackendOptions(), m.wizardBackend) {
		m.wizardBackend = "cpu"
	}
	if m.wizardRuntime == "llamacpp" && !containsString(llamaTypeOptions(), m.wizardBackend) {
		m.wizardBackend = m.firstAvailableLlamaType()
	}
	if len(m.llamaRuntimeVariants) == 0 {
		m.wizardVariantSelection = 0
	} else {
		m.wizardVariantSelection = clampInt(m.wizardVariantSelection, 0, len(m.llamaRuntimeVariants)-1)
	}
	candidates := m.wizardCandidateModels()
	if len(candidates) == 0 {
		m.wizardModelSelection = 0
		return
	}
	m.wizardModelSelection = clampInt(m.wizardModelSelection, 0, len(candidates)-1)
}

func isWizardRole(role string) bool {
	return role == "main" || role == "embedding" || role == "reranking"
}

func (m Model) wizardVariantLabel() string {
	if m.wizardRuntime == "llamacpp" {
		if variant, ok := m.selectedLlamaRuntimeVariant(); ok {
			return variant.Name
		}
		return "no installed llama runtimes"
	}
	return m.wizardBackend
}

func (m Model) selectedLlamaRuntimeVariant() (llamaRuntimeVariant, bool) {
	if len(m.llamaRuntimeVariants) == 0 {
		return llamaRuntimeVariant{}, false
	}
	for _, variant := range m.llamaRuntimeVariants {
		if llamaRuntimeType(variant.Name) == m.wizardBackend {
			return variant, true
		}
	}
	index := clampInt(m.wizardVariantSelection, 0, len(m.llamaRuntimeVariants)-1)
	return m.llamaRuntimeVariants[index], true
}

func (m Model) wizardCandidateModels() []catalog.Entry {
	entries := make([]catalog.Entry, 0, len(m.models))
	for _, entry := range m.models {
		if entry.State != catalog.StatePresent {
			continue
		}
		if entry.Runtime != m.wizardRuntime || entry.Role != m.wizardRole {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func (m Model) wizardSelectedEntry() (catalog.Entry, error) {
	candidates := m.wizardCandidateModels()
	if len(candidates) == 0 {
		return catalog.Entry{}, fmt.Errorf(
			"no downloaded %s/%s models are available",
			m.wizardRuntime,
			m.wizardRole,
		)
	}
	index := clampInt(m.wizardModelSelection, 0, len(candidates)-1)
	return candidates[index], nil
}

func (m Model) wizardRunnerSpec() (server.RunnerSpec, catalog.Entry, error) {
	entry, err := m.wizardSelectedEntry()
	if err != nil {
		return server.RunnerSpec{}, catalog.Entry{}, err
	}

	backend := m.wizardBackend
	executable := ""
	if entry.Runtime == "llamacpp" {
		variant, ok := m.selectedLlamaRuntimeVariant()
		if !ok {
			return server.RunnerSpec{}, catalog.Entry{}, fmt.Errorf(
				"no installed llama runtime variants under native/llama-runtimes",
			)
		}
		backend = variant.Backend
		executable = variant.Executable
	}

	spec := catalogRunnerSpec(entry, runnerPreset{
		id:      m.nextRunnerID(entry.Runtime, entry.Role),
		role:    entry.Role,
		backend: backend,
		exe:     executable,
		modelID: entry.ID,
		port:    m.nextWizardPort(entry.Role),
	})
	return spec, entry, nil
}

func (m Model) nextRunnerID(runtimeName string, role string) string {
	prefix := "LR"
	if runtimeName == "llamacpp" {
		prefix = "LM"
	}
	roleCode := roleLetter(role)
	next := 1
	for _, runner := range m.snapshot.Runners {
		parts := strings.Split(runner.ID, "-")
		if len(parts) != 3 || parts[1] != roleCode {
			continue
		}
		value, err := strconv.Atoi(parts[2])
		if err == nil && value >= next {
			next = value + 1
		}
	}
	return fmt.Sprintf("%s-%s-%d", prefix, roleCode, next)
}

func roleLetter(role string) string {
	switch role {
	case "embedding":
		return "E"
	case "reranking":
		return "R"
	default:
		return "M"
	}
}

func (m Model) nextWizardPort(role string) int {
	port := defaultPortForRole(role)
	used := map[int]bool{}
	for _, runner := range m.snapshot.Runners {
		if runner.Port > 0 {
			used[runner.Port] = true
		}
	}
	for used[port] {
		port++
	}
	return port
}

func defaultPortForRole(role string) int {
	switch role {
	case "embedding":
		return 9483
	case "reranking":
		return 9484
	default:
		return 9482
	}
}

func litertBackendOptions() []string {
	return []string{"cpu", "gpu", "npu"}
}

func llamaTypeOptions() []string {
	return []string{"cpu", "gpu", "openvino", "cuda13", "cuda12", "sycl"}
}

func (m Model) firstAvailableLlamaType() string {
	for _, variant := range m.llamaRuntimeVariants {
		return llamaRuntimeType(variant.Name)
	}
	return "cpu"
}

func llamaRuntimeType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "cuda-13") || strings.Contains(lower, "cuda13") || strings.Contains(lower, "cuda-13."):
		return "cuda13"
	case strings.Contains(lower, "cuda-12") || strings.Contains(lower, "cuda12") || strings.Contains(lower, "cuda-12."):
		return "cuda12"
	case strings.Contains(lower, "openvino"):
		return "openvino"
	case strings.Contains(lower, "sycl"):
		return "sycl"
	case strings.Contains(lower, "vulkan"), strings.Contains(lower, "hip"), strings.Contains(lower, "radeon"), strings.Contains(lower, "opencl"):
		return "gpu"
	default:
		return "cpu"
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func discoverLlamaRuntimeVariants(root string) []llamaRuntimeVariant {
	root = resolveLlamaRuntimeRoot(root)
	entries, err := os.ReadDir(root)
	if err != nil {
		return []llamaRuntimeVariant{}
	}

	variants := make([]llamaRuntimeVariant, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		executable := findLlamaServerExecutable(dir)
		if executable == "" {
			continue
		}
		variants = append(variants, llamaRuntimeVariant{
			Name:       entry.Name(),
			Backend:    backendForLlamaRuntime(entry.Name()),
			Executable: executable,
		})
	}
	sort.Slice(variants, func(left int, right int) bool {
		return variants[left].Name < variants[right].Name
	})
	return variants
}

func resolveLlamaRuntimeRoot(root string) string {
	if strings.TrimSpace(root) != "" {
		return root
	}
	if envRoot := os.Getenv("LLAMA_RUNTIME_ROOT"); strings.TrimSpace(envRoot) != "" {
		return envRoot
	}
	if repoRoot := findRepoRoot(); repoRoot != "" {
		return filepath.Join(repoRoot, "native", "llama-runtimes")
	}
	return filepath.Join("native", "llama-runtimes")
}

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if stat, err := os.Stat(filepath.Join(dir, "native", "llama-runtimes")); err == nil && stat.IsDir() {
			return dir
		}
		if stat, err := os.Stat(filepath.Join(dir, "package.json")); err == nil && !stat.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func findLlamaServerExecutable(root string) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, dirEntry os.DirEntry, err error) error {
		if err != nil || found != "" || dirEntry.IsDir() {
			return nil
		}
		name := dirEntry.Name()
		if name == "llama-server" || name == "llama-server.exe" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func backendForLlamaRuntime(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "cuda"):
		return "cuda"
	case strings.Contains(lower, "vulkan"):
		return "vulkan"
	case strings.Contains(lower, "openvino"):
		return "openvino"
	case strings.Contains(lower, "sycl"):
		return "sycl"
	case strings.Contains(lower, "hip") || strings.Contains(lower, "radeon"):
		return "gpu"
	case strings.Contains(lower, "opencl"):
		return "gpu"
	case strings.Contains(lower, "macos"):
		return "metal"
	default:
		return "cpu"
	}
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
		ID:         preset.id,
		Runtime:    entry.Runtime,
		Role:       preset.role,
		Backend:    fallback(preset.backend, "cpu"),
		Executable: preset.exe,
		ModelPath:  entry.TargetPath,
		ModelID:    preset.modelID,
		Host:       "127.0.0.1",
		Port:       preset.port,
		Launch:     true,
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

const (
	widePanelGridMinWidth = 150
	panelGridColumnGap    = 2
)

type panelSpec struct {
	title string
	lines []string
	color string
}

func (m Model) panelGrid(panels ...panelSpec) string {
	width := m.width
	if !m.usesWidePanelGrid() {
		rendered := make([]string, 0, len(panels))
		for _, panel := range panels {
			rendered = append(rendered, renderPanelSpec(panel, width))
		}
		return joinPanels(rendered...)
	}

	columnWidth := m.panelGridColumnWidth()
	columns := [2][]string{}
	columnHeights := [2]int{}
	for _, panel := range panels {
		rendered := renderPanelSpec(panel, columnWidth)
		if strings.TrimSpace(rendered) == "" {
			continue
		}
		columnIndex := 0
		if columnHeights[1] < columnHeights[0] {
			columnIndex = 1
		}
		if len(columns[columnIndex]) > 0 {
			columnHeights[columnIndex] += 2
		}
		columns[columnIndex] = append(columns[columnIndex], rendered)
		columnHeights[columnIndex] += viewLineCount(rendered)
	}

	left := joinPanels(columns[0]...)
	right := joinPanels(columns[1]...)
	if strings.TrimSpace(right) == "" {
		return left
	}
	if strings.TrimSpace(left) == "" {
		return right
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", panelGridColumnGap), right)
}

func renderPanelSpec(panel panelSpec, width int) string {
	if strings.TrimSpace(panel.title) == "" && len(panel.lines) == 0 {
		return ""
	}
	return renderPanelWidth(panel.title, panel.lines, panel.color, width)
}

func renderPanel(title string, lines []string, color string) string {
	return renderPanelWidth(title, lines, color, 0)
}

func renderPanelWidth(title string, lines []string, color string, width int) string {
	body := strings.Join(lines, "\n")
	if strings.TrimSpace(body) == "" {
		body = "No data."
	}
	style := lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(lipgloss.Color(color)).
		Padding(0, 1)
	if width > style.GetHorizontalFrameSize() {
		style = style.Width(panelInnerWidth(width))
	}
	return style.Render(subtitleStyle.Render(title) + "\n" + body)
}

func panelInnerWidth(width int) int {
	if width <= 4 {
		return 0
	}
	return width - 4
}

func renderCommandRail(lines []string) string {
	return mutedStyle.Render(strings.Join(lines, "\n"))
}

func truncateToWidth(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
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

func managedBodyHeight(totalHeight int, top string, footer string) int {
	bodyHeight := totalHeight - viewLineCount(top) - viewLineCount(footer) - 4
	if bodyHeight < 0 {
		return 0
	}
	return bodyHeight
}

func (m Model) scrollPageSize() int {
	pageSize := managedBodyHeight(m.height, m.managedTopView(), m.commandRailSizingView())
	if pageSize < 1 {
		return 1
	}
	return pageSize
}

func (m Model) maxScrollOffset() int {
	bodyHeight := managedBodyHeight(m.height, m.managedTopView(), m.commandRailSizingView())
	if bodyHeight <= 0 {
		return 0
	}
	lineCount := viewLineCount(m.activeContentView())
	if lineCount <= bodyHeight {
		return 0
	}
	return lineCount - bodyHeight
}

func (m Model) scrollStatusLine() string {
	bodyHeight := managedBodyHeight(m.height, m.managedTopView(), m.commandRailSizingView())
	totalLines := viewLineCount(m.activeContentView())
	if bodyHeight <= 0 || totalLines <= bodyHeight {
		return "Scroll: content fits | Up/Down PgUp/PgDn Home/End"
	}
	startLine := clampInt(m.scrollOffset, 0, m.maxScrollOffset()) + 1
	endLine := minInt(startLine+bodyHeight-1, totalLines)
	return fmt.Sprintf(
		"Scroll: lines %d-%d/%d | Up/Down line | PgUp/PgDn page | Home/End",
		startLine,
		endLine,
		totalLines,
	)
}

func (m Model) commandRailSizingView() string {
	return m.footerView()
}

func (m *Model) resetScroll() {
	m.scrollOffset = 0
}

func (m *Model) clampScrollOffset() {
	m.scrollOffset = clampInt(m.scrollOffset, 0, m.maxScrollOffset())
}

func sliceRenderedLines(value string, offset int, height int) string {
	if height <= 0 || value == "" {
		return ""
	}
	lines := strings.Split(value, "\n")
	offset = clampInt(offset, 0, maxInt(0, len(lines)-height))
	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[offset:end], "\n")
}

func fitLinesToHeight(value string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := []string{}
	if value != "" {
		lines = strings.Split(value, "\n")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func viewLineCount(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}

func firstRenderedLine(value string) string {
	if value == "" {
		return ""
	}
	line, _, _ := strings.Cut(value, "\n")
	return line
}

func clampInt(value int, minimum int, maximum int) int {
	if maximum < minimum {
		return minimum
	}
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
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
	return coloredStatus(state, color)
}

func coloredStatus(state string, color string) string {
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

func runnerSpecLaunchMode(spec server.RunnerSpec) string {
	if !spec.Launch {
		return "external upstream"
	}
	return "managed by sidecar"
}

func wizardCommandPreview(spec server.RunnerSpec) string {
	executable := fallback(spec.Executable, "llama-server")
	args := []string{
		executable,
		"--host",
		fallback(spec.Host, "127.0.0.1"),
		"--port",
		strconv.Itoa(spec.Port),
	}
	if spec.ModelPath != "" {
		args = append(args, "--model", spec.ModelPath)
	}
	switch spec.Role {
	case "embedding":
		args = append(args, "--embedding")
	case "reranking":
		args = append(args, "--embedding", "--pooling", "rank", "--reranking")
	}
	return strings.Join(args, " ")
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
