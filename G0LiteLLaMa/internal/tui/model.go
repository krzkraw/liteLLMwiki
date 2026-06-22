package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"g0litellama/internal/catalog"
	"g0litellama/internal/runtimeconfig"
	"g0litellama/internal/server"
	"g0litellama/internal/tui/shapes"
	"g0litellama/internal/tui/store"
	"g0litellama/internal/tui/store/sqlite"
)

const refreshInterval = time.Second
const defaultChatEndpoint = "http://127.0.0.1:9379/v1/chat/completions"

const (
	dashboardModelMainX   = 23
	dashboardModelEmbedX  = 33
	dashboardModelRerankX = 49

	wizardRuntimeLlamaX   = 24
	wizardRoleMainX       = 18
	wizardRoleEmbeddingX  = 25
	wizardRoleRerankingX  = 37
	wizardStartX          = 6
	setupBackendFirstLine = 3
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
	BackendConfigPath string
	DBPath            string // SQLite database path; empty means use default
	CommandBus        *store.CommandBus // if nil, created from DBPath
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

type chatStreamStartMsg struct {
	chunks <-chan chatStreamChunkMsg
}

type chatStreamChunkMsg struct {
	content string
	err     error
	done    bool
	chunks  <-chan chatStreamChunkMsg
}

type runnerUpdateMsg struct {
	field  string
	value  string
	runner server.RunnerSnapshot
	err    error
}

type dashboardRouteMsg struct {
	role   string
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

type optionModal struct {
	option  wizardCLIOption
	input   textinput.Model
	popover Popover
}

type wizardCommandEdit struct {
	input textinput.Model
}

type buttonHit struct {
	action  string
	row     int
	start   int
	end     int
	payload string
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

type setupBackendRow struct {
	Runtime      string
	RuntimeLabel string
	Backend      string
}

type Model struct {
	runtimeController server.RuntimeController
	runnerController  server.RunnerController
	logs              *server.LogBroadcaster
	catalog           *catalog.Catalog
	ctx               context.Context
	chatEndpoint      string

	active               int
	width                int
	height               int
	snapshot             server.RunnerSnapshotResponse
	runtime              server.RuntimeStatus
	logEntries           []server.LogEntry
	models               []catalog.Entry
	notice               string
	edit                 *runnerEdit
	runtimeEdit          *runtimeEdit
	runtimeDraft         server.RuntimeControlConfig
	chatDraft            string
	chatPendingDraft     string
	chatMessages         []chatMessage
	chatBusy             bool
	chatStatus           string
	chatTokensPerSecond  float64
	chatStreamStarted    time.Time
	chatAssistantIndex   int
	chatCancel           context.CancelFunc
	chatSettingsDropdown string
	chatPopupRow         int
	chatPopupColumn      int
	chatCustomField      string
	chatCustomValue      string
	chatCommandPopup     bool
	chatInputFocused     bool
	chatTargetRole       string
	chatRunnerID         string
	chatTargetDropdown   bool
	chatSystemField      TextAreaField
	chatSystemEditing    bool
	chatThinking         bool
	chatSettingsOpen     bool
	chatTemperature      string
	chatTopP             string
	chatMaxTokens        string
	chatStream           bool
	chatScrollBox ScrollBox
	managedScreen        bool
	store                *store.CommandBus
	persistCloser        io.Closer
	scrollOffset         int

	wizardRuntime           string
	wizardBackend           string
	wizardRole              string
	wizardVariantSelection  int
	wizardModelSelection    int
	wizardOptionPage        int
	wizardOptionOverrides   map[string]string
	wizardCommandExtras     []wizardCommandExtra
	wizardRemovedDefaults   map[string]bool
	wizardCommandEdit       *wizardCommandEdit
	optionModal             *optionModal
	llamaRuntimeRoot        string
	llamaRuntimeVariants    []llamaRuntimeVariant
	backendConfigPath       string
	backendStatus           runtimeconfig.Status
	setupSelection          int
	dashboardModelDropdown  string
	dashboardRunnerDropdown string
	globalMenuOpen          bool
	globalPaletteMenuOpen   bool
	paletteID               string
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
	id          string
	label       string
	backdrop    string
	tabBG       string
	tabActiveBG string
	header      string
	footerBG    string
	footerFG    string
	menu        string

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
	cliRowBG      string
	actionBG      string
	actionFG      string
}

func tuiPalettes() []tuiPalette {
	return []tuiPalette{
		{
			id:              "neon",
			label:           "Neon",
			backdrop:        "234",
			tabBG:           "236",
			tabActiveBG:     "39",
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
			roleBG:          "94",
			roleHeader:      "229",
			roleButton:      "222",
			roleSelected:    "214",
			modelBG:         "22",
			modelSelected:   "82",
			cliRowBG:        "22",
			actionBG:        "28",
			actionFG:        "16",
		},
		{
			id:              "amber",
			label:           "Amber",
			backdrop:        "236",
			tabBG:           "94",
			tabActiveBG:     "214",
			header:          "214",
			footerBG:        "94",
			footerFG:        "230",
			menu:            "214",
			runtimeBG:       "94",
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
			cliRowBG:        "22",
			actionBG:        "34",
			actionFG:        "16",
		},
		{
			id:              "ocean",
			label:           "Ocean",
			backdrop:        "17",
			tabBG:           "24",
			tabActiveBG:     "81",
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
			cliRowBG:        "23",
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
	bus := options.CommandBus
	var closer io.Closer
	if bus == nil {
		bus, closer = newCommandBusWithSQLite(options)
	}
	model := Model{
		runtimeController:     options.RuntimeController,
		runnerController:      options.RunnerController,
		logs:                  options.Logs,
		catalog:               options.Catalog,
		ctx:                   ctx,
		chatEndpoint:          chatEndpoint,
		chatTargetRole:        "main",
		chatStatus:            "idle",
		chatAssistantIndex:    -1,
		chatTemperature:       "default",
		chatTopP:              "default",
		chatMaxTokens:         "default",
		chatStream:            true,
		active:                0,
		store:                 bus,
		persistCloser:         closer,
		managedScreen:         options.ManagedScreen,
		wizardRuntime:         strOrDefault(bus.State().Wizard.Runtime, "litert"),
		wizardBackend:         strOrDefault(bus.State().Wizard.Backend, "cpu"),
		wizardRole:            strOrDefault(bus.State().Wizard.Role, "main"),
		wizardOptionOverrides: mapOrDefault(bus.State().Wizard.OptionOverrides),
		wizardRemovedDefaults: map[string]bool{},
		llamaRuntimeRoot:      options.LlamaRuntimeRoot,
		backendConfigPath:     resolveBackendConfigPath(options.BackendConfigPath),
		paletteID:             "neon",
	}
	model.refresh()
	model.normalizeWizardSelection()
	return model
}

// newCommandBusWithSQLite creates a CommandBus with optional SQLite persistence.
// If SQLite cannot be opened (e.g. in tests or CI without a writable home dir)
// the bus operates without a persistence backend. The second return value is a
// closer for the SQLite store; it is nil when persistence is not active.
func newCommandBusWithSQLite(options ModelOptions) (*store.CommandBus, io.Closer) {
	dbPath := options.DBPath
	if dbPath == "" {
		var err error
		dbPath, err = sqlite.DBPath()
		if err != nil {
			log.Printf("persistence: resolve db path: %v — continuing without persistence", err)
			return store.NewCommandBus(store.AppState{}), nil
		}
	}

	s, err := sqlite.New(dbPath)
	if err != nil {
		log.Printf("persistence: open db %s: %v — continuing without persistence", dbPath, err)
		return store.NewCommandBus(store.AppState{}), nil
	}

	// Load persisted state from SQLite (latest snapshot + event replay).
	initialState, err := sqlite.ReplayFromStore(s)
	if err != nil {
		log.Printf("persistence: replay: %v — starting fresh", err)
		initialState = store.AppState{}
	}

	bus := store.NewCommandBus(initialState,
		store.WithEventLog(s),
		store.WithSnapshotStore(s),
	)
	return bus, s
}

func Run(
	ctx context.Context,
	runtimeController server.RuntimeController,
	runnerController server.RunnerController,
	logs *server.LogBroadcaster,
	modelCatalog *catalog.Catalog,
	opts ...RunOption,
) error {
	ro := runOptions{}
	for _, o := range opts {
		o(&ro)
	}
	program := tea.NewProgram(
		NewModel(ModelOptions{
			RuntimeController: runtimeController,
			RunnerController:  runnerController,
			Logs:              logs,
			Catalog:           modelCatalog,
			Context:           ctx,
			ManagedScreen:     true,
			CommandBus:        ro.commandBus,
		}),
		tea.WithContext(ctx),
	)
	finalModel, err := program.Run()
	if m, ok := finalModel.(Model); ok && m.persistCloser != nil {
		m.persistCloser.Close()
	}
	return err
}

// RunOption configures optional Run behaviour.
type RunOption func(*runOptions)

type runOptions struct {
	commandBus *store.CommandBus
}

// WithCommandBus passes a shared CommandBus to the TUI. When set, the TUI
// skips its own CommandBus creation and does not own the close lifecycle.
func WithCommandBus(bus *store.CommandBus) RunOption {
	return func(o *runOptions) {
		o.commandBus = bus
	}
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
	case tea.KeyPressMsg:
		if m.optionModal != nil {
			return m.updateOptionModalKey(msg)
		}
		if m.wizardCommandEdit != nil {
			return m.updateWizardCommandEditKey(msg)
		}
		if m.runtimeEdit != nil {
			return m.updateRuntimeEditKey(msg)
		}
		if m.edit != nil {
			return m.updateEditKey(msg)
		}
		return m.updateKey(msg)
	case tea.MouseClickMsg:
		if m.optionModal != nil {
			return m.updateOptionModalMouse(msg)
		}
		if m.wizardCommandEdit != nil {
			return m.updateWizardCommandEditMouse(msg)
		}
		return m.updateMouse(msg)
	case tea.MouseWheelMsg:
		return m.updateMouseWheel(msg)
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
		m.chatStatus = "idle"
		if msg.err != nil {
			m.chatStatus = "error"
			if m.chatAssistantIndex < 0 {
				m.chatDraft = fallback(m.chatPendingDraft, msg.prompt)
			}
			m.chatPendingDraft = ""
			m.notice = fmt.Sprintf("chat prompt failed: %v", msg.err)
			break
		}
		m.chatPendingDraft = ""
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "assistant",
			content: msg.response,
		})
		m.notice = "sent chat prompt through /v1/chat/completions"
	case chatStreamStartMsg:
		return m, waitForChatStreamChunk(msg.chunks)
	case chatStreamChunkMsg:
		return m.applyChatStreamChunk(msg)
	case runnerUpdateMsg:
		m.refresh()
		m.setActiveTab("runner:" + msg.runner.ID)
		m.notice = m.runnerUpdateNotice(msg)
	case dashboardRouteMsg:
		m.refresh()
		m.setActiveTab("dashboard")
		m.dashboardRunnerDropdown = ""
		if msg.err != nil {
			m.notice = fmt.Sprintf("%s route update failed: %v", msg.role, msg.err)
			break
		}
		m.notice = fmt.Sprintf("%s route -> %s", msg.role, msg.runner.ID)
	}

	return m, nil
}

func (m Model) updateMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.activeTabID() == "chat" {
		m.chatScrollBox.ViewLines = m.chatScrollViewLines()
		m.chatScrollBox.SetLines(m.chatMessageLines())
		switch msg.Button {
		case tea.MouseWheelUp:
			m.chatScrollBox.ScrollUp(1)
		case tea.MouseWheelDown:
			m.chatScrollBox.ScrollDown(1)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}

	if hit, ok := m.buttonHitAt(msg.X, msg.Y); ok {
		if next, cmd, handled := m.handleButtonHit(hit); handled {
			return next, cmd
		}
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
	case "setup":
		return m.updateSetupMouse(msg.X, msg.Y), nil
	case "chat":
		return m.updateChatMouse(msg.X, msg.Y)
	default:
		if runner, ok := m.activeRunner(); ok {
			return m.updateRunnerBodyMouse(runner, msg.X, msg.Y)
		}
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
		m.globalPaletteMenuOpen = false
		return m, nil, true
	case "next":
		m.cycleTab(1)
		return m, nil, true
	case "previous":
		m.cycleTab(-1)
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
	case "runner-edit-command":
		if runner, ok := m.activeRunner(); ok {
			m.edit = newCommandLineRunnerEdit(runner)
			return m, nil, true
		}
	case "runner-close":
		if runner, ok := m.activeRunner(); ok {
			return m, m.runnerActionCmd("close", runner.ID), true
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

func (m Model) buttonHitAt(x int, y int) (buttonHit, bool) {
	for _, hit := range m.buttonHitRegistry() {
		if y == hit.row && x >= hit.start && x < hit.end {
			return hit, true
		}
	}
	return buttonHit{}, false
}

func (m Model) buttonHitRegistry() []buttonHit {
	view := m.viewContent()
	var hits []buttonHit
	addTextHit := func(text string, action string, payload string) {
		if hit, ok := renderedTextHit(view, text, action, payload); ok {
			hits = append(hits, hit)
		}
	}
	addTextHitOnRow := func(row int, text string, action string, payload string) {
		if hit, ok := renderedTextHitOnRow(view, row, text, action, payload); ok {
			hits = append(hits, hit)
		}
	}
	addTokenHitOnRow := func(row int, text string, action string, payload string) {
		if hit, ok := renderedTokenHitOnRow(view, row, text, action, payload); ok {
			hits = append(hits, hit)
		}
	}
	addTokenHitOnVisibleLine := func(lineNeedle string, text string, action string, payload string) {
		row := lineNumberContainingText(view, lineNeedle)
		addTokenHitOnRow(row, text, action, payload)
	}
	addTextHitOnVisibleLine := func(lineNeedle string, text string, action string, payload string) {
		row := lineNumberContainingText(view, lineNeedle)
		addTextHitOnRow(row, text, action, payload)
	}
	addPopupRowHit := func(text string, action string, payload string) {
		lines := strings.Split(view, "\n")
		for row := maxInt(0, m.chatPopupRow); row < len(lines); row++ {
			plain := ansi.Strip(lines[row])
			if !strings.Contains(plain, text) {
				continue
			}
			hits = append(hits, buttonHit{
				action:  action,
				row:     row,
				start:   0,
				end:     maxInt(ansi.StringWidth(plain), m.width),
				payload: payload,
			})
			return
		}
	}
	addWholeRowHit := func(row int, start int, action string, payload string) {
		if row < 0 {
			return
		}
		lines := strings.Split(view, "\n")
		if row >= len(lines) {
			return
		}
		hits = append(hits, buttonHit{
			action:  action,
			row:     row,
			start:   start,
			end:     maxInt(ansi.StringWidth(ansi.Strip(lines[row])), m.width),
			payload: payload,
		})
	}

	if footerRow := lastLineNumberContainingText(view, "Menu"); footerRow >= 0 {
		for _, segment := range m.bottomActionSegments() {
			payload := segment.label
			if strings.HasPrefix(segment.id, "runner-") {
				payload = m.activeRunnerID()
			}
			hits = append(hits, buttonHit{
				action:  segment.id,
				row:     footerRow,
				start:   segment.start,
				end:     segment.end,
				payload: payload,
			})
		}
	}
	if m.globalMenuOpen {
		addTextHit("Next tab", "global-next", "")
		addTextHit("Previous tab", "global-previous", "")
		addTextHit("Palette themes", "global-palette", "")
		addTextHit("Esc Quit", "global-quit", "")
		if m.globalPaletteMenuOpen {
			for _, palette := range tuiPalettes() {
				addTextHit(palette.label, "global-palette-select", palette.id)
			}
		}
	}

	if m.optionModal != nil {
		for _, sample := range wizardOptionSamples(m.optionModal.option) {
			addTextHit("["+sample+"]", "modal-sample", sample)
		}
		addTextHit("[ Save ]", "modal-save", "")
		addTextHit("[ Reset ]", "modal-clear", "")
		addTextHit("[ X ]", "modal-close", "")
		return hits
	}
	if m.wizardCommandEdit != nil {
		addTextHit("[ Save ]", "wizard-command-save", "")
		addTextHit("[ X ]", "wizard-command-close", "")
		return hits
	}

	if runner, ok := m.activeRunner(); ok {
		actionRow := lineNumberContainingText(view, "Actions")
		addTextHitOnRow(actionRow, "Start", "runner-start", runner.ID)
		addTextHitOnRow(actionRow, "Stop", "runner-stop", runner.ID)
		addTextHitOnRow(actionRow, "Restart", "runner-restart", runner.ID)
		addTextHitOnRow(actionRow, "Edit Cmd", "runner-edit-command", runner.ID)
		addTextHitOnRow(actionRow, "Close", "runner-close", runner.ID)
	}

	if m.activeTabID() == "wizard" {
		for _, runtimeName := range []string{"litert", "llama.cpp"} {
			addTokenHitOnVisibleLine("runtime", selectedToken(runtimeName, (runtimeName == "litert" && m.wizardRuntime == "litert") || (runtimeName == "llama.cpp" && m.wizardRuntime == "llamacpp")), "wizard-runtime", runtimeName)
		}
		backendOptions := m.litertBackendOptions()
		backendLine := "LiteRT backend"
		if m.wizardRuntime == "llamacpp" {
			backendOptions = m.llamaTypeOptions()
			backendLine = "llama type"
		}
		for _, backend := range backendOptions {
			addTokenHitOnVisibleLine(backendLine, selectedToken(backend, backend == m.wizardBackend), "wizard-backend", backend)
		}
		for _, role := range []string{"main", "embedding", "reranking"} {
			addTokenHitOnVisibleLine("model role", selectedToken(role, role == m.wizardRole), "wizard-role", role)
		}
		for index, entry := range m.wizardCandidateModels() {
			addTextHit(entry.Filename, "wizard-model", strconv.Itoa(index))
		}
		for _, option := range m.visibleWizardCLIOptions() {
			if hit, ok := renderedTextHit(view, wizardOptionButtonText(option), "wizard-option", option.ID); ok {
				addWholeRowHit(hit.row, hit.start, "wizard-option", option.ID)
			}
		}
		addTextHit("[ Prev ]", "wizard-options-prev", "")
		addTextHit("[ Next ]", "wizard-options-next", "")
		addTextHit("Command Preview", "wizard-command-edit", "")
		addTextHitOnVisibleLine("[ START ]", "[ START ]", "wizard-start", "")
	}
	if m.activeTabID() == "dashboard" && m.dashboardRunnerDropdown != "" {
		for index, runner := range m.runnersForRole(m.dashboardRunnerDropdown) {
			row := lineNumberContainingText(view, fmt.Sprintf("%d %s", index+1, runner.ID))
			addTextHitOnRow(row, runner.ID, "dashboard-route-runner", m.dashboardRunnerDropdown+":"+runner.ID)
		}
	}
	if m.activeTabID() == "chat" {
		for _, field := range []string{"Thinking", "Target", "System", "Temperature", "Top P", "Max Tokens", "Stream"} {
			addTextHit(field+":", "chat-setting", strings.ToLower(strings.ReplaceAll(field, " ", "-")))
		}
		addTextHit("Send", "chat-send", "")
		addTextHit("Ready. Input your prompt", "chat-input", "")
		addTextHit("Wait...", "chat-input", "")
		for _, command := range chatCommandNames() {
			addTextHit(command, "chat-command", command)
		}
		for _, option := range m.chatDropdownOptions() {
			addPopupRowHit(option.label, "chat-dropdown-option", option.value)
		}
	}

	return hits
}

func (m Model) handleButtonHit(hit buttonHit) (Model, tea.Cmd, bool) {
	switch hit.action {
	case "menu", "next", "previous", "quit", "runner-start", "runner-stop", "runner-restart", "runner-edit-command", "runner-close", "chat-clear", "chat-new":
		return m.handleBottomBarOrRunnerHit(hit)
	case "global-next":
		m.cycleTab(1)
		m.globalMenuOpen = false
		m.globalPaletteMenuOpen = false
		return m, nil, true
	case "global-previous":
		m.cycleTab(-1)
		m.globalMenuOpen = false
		m.globalPaletteMenuOpen = false
		return m, nil, true
	case "global-palette":
		m.globalPaletteMenuOpen = true
		return m, nil, true
	case "global-palette-select":
		m.paletteID = hit.payload
		m.globalMenuOpen = false
		m.globalPaletteMenuOpen = false
		return m, nil, true
	case "global-quit":
		m.globalPaletteMenuOpen = false
		return m, tea.Quit, true
	case "wizard-runtime":
		if hit.payload == "llama.cpp" {
			m.wizardRuntime = "llamacpp"
			m.wizardBackend = m.firstAvailableLlamaType()
		} else {
			m.wizardRuntime = "litert"
			m.wizardBackend = "cpu"
		}
		m.wizardModelSelection = 0
		m.wizardOptionPage = 0
		m.normalizeWizardSelection()
		m.dispatchWizardState()
		return m, nil, true
	case "wizard-backend":
		m.wizardBackend = hit.payload
		m.wizardOptionPage = 0
		m.normalizeWizardSelection()
		m.dispatchWizardState()
		return m, nil, true
	case "wizard-role":
		m.setWizardRole(hit.payload)
		return m, nil, true
	case "wizard-model":
		index, _ := strconv.Atoi(hit.payload)
		m.setWizardModelSelection(index)
		return m, nil, true
	case "wizard-option":
		if option, ok := m.wizardOptionByID(hit.payload); ok {
			m.openOptionModalAt(option, hit.row+1, hit.start)
			return m, nil, true
		}
	case "wizard-options-prev":
		m.pageWizardOptions(-1)
		return m, nil, true
	case "wizard-options-next":
		m.pageWizardOptions(1)
		return m, nil, true
	case "wizard-command-edit":
		m.openWizardCommandEdit()
		return m, nil, true
	case "wizard-command-save":
		m.saveWizardCommandEdit()
		return m, nil, true
	case "wizard-command-close":
		m.wizardCommandEdit = nil
		return m, nil, true
	case "wizard-start":
		return m, m.wizardCreateCmd(), true
	case "dashboard-route-runner":
		role, id, ok := strings.Cut(hit.payload, ":")
		if !ok {
			return m, nil, true
		}
		m.dashboardRunnerDropdown = ""
		if runner, ok := m.runnerByID(id); ok {
			return m, m.dashboardRouteCmd(role, runner), true
		}
		return m, nil, true
	case "chat-target":
		if isWizardRole(hit.payload) {
			m.chatTargetRole = hit.payload
			m.chatTargetDropdown = false
		}
		return m, nil, true
	case "chat-system":
		m.chatSystemEditing = true
		m.chatSettingsOpen = false
		m.chatTargetDropdown = false
		return m, nil, true
	case "chat-thinking":
		m.chatThinking = !m.chatThinking
		return m, nil, true
	case "chat-settings":
		m.chatSettingsOpen = !m.chatSettingsOpen
		m.chatTargetDropdown = false
		return m, nil, true
	case "chat-setting":
		m.openChatSetting(hit.payload, hit.row, hit.start)
		return m, nil, true
	case "chat-dropdown-option":
		m.applyChatDropdownOption(hit.payload)
		return m, nil, true
	case "chat-command":
		next, cmd := m.runChatCommand(hit.payload)
		return next, cmd, true
	case "chat-input":
		m.chatInputFocused = true
		return m, nil, true
	case "chat-send":
		next, cmd := m.submitChatPrompt()
		return next, cmd, true
	case "modal-save":
		m.saveOptionModal()
		return m, nil, true
	case "modal-sample":
		m.saveOptionModalValue(hit.payload)
		return m, nil, true
	case "modal-clear":
		if m.optionModal != nil {
			delete(m.wizardOptionOverrides, m.optionModal.option.ID)
			m.optionModal = nil
		}
		return m, nil, true
	case "modal-close":
		m.optionModal = nil
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) handleBottomBarOrRunnerHit(hit buttonHit) (Model, tea.Cmd, bool) {
	switch hit.action {
	case "menu":
		m.globalMenuOpen = !m.globalMenuOpen
		m.globalPaletteMenuOpen = false
		return m, nil, true
	case "next":
		m.cycleTab(1)
		return m, nil, true
	case "previous":
		m.cycleTab(-1)
		return m, nil, true
	case "quit":
		return m, tea.Quit, true
	case "runner-start":
		if runner, ok := m.runnerByID(fallback(hit.payload, m.activeRunnerID())); ok {
			return m, m.runnerActionCmd("start", runner.ID), true
		}
	case "runner-stop":
		if runner, ok := m.runnerByID(fallback(hit.payload, m.activeRunnerID())); ok {
			return m, m.runnerActionCmd("stop", runner.ID), true
		}
	case "runner-restart":
		if runner, ok := m.runnerByID(fallback(hit.payload, m.activeRunnerID())); ok {
			return m, m.runnerActionCmd("restart", runner.ID), true
		}
	case "runner-edit-command":
		if runner, ok := m.runnerByID(fallback(hit.payload, m.activeRunnerID())); ok {
			m.edit = newCommandLineRunnerEdit(runner)
			return m, nil, true
		}
	case "runner-close":
		if runner, ok := m.runnerByID(fallback(hit.payload, m.activeRunnerID())); ok {
			return m, m.runnerActionCmd("close", runner.ID), true
		}
	case "chat-clear":
		next, cmd := m.runChatCommand("/clear")
		return next, cmd, true
	case "chat-new":
		next, cmd := m.runChatCommand("/new")
		return next, cmd, true
	}
	return m, nil, false
}

func (m Model) handleGlobalMenuClick(x int, y int) (Model, tea.Cmd, bool) {
	if !m.globalMenuOpen || m.height <= 0 {
		return m, nil, false
	}
	menuTop := m.globalMenuTopRow()
	if y < menuTop || y >= m.height-1 {
		m.globalMenuOpen = false
		m.globalPaletteMenuOpen = false
		return m, nil, true
	}
	menuWidth := lipgloss.Width(firstRenderedLine(m.globalMenuMainView()))
	paletteX := menuWidth + panelGridColumnGap
	if m.globalPaletteMenuOpen && x >= paletteX {
		index := y - menuTop - 2
		palettes := tuiPalettes()
		if index >= 0 && index < len(palettes) {
			m.paletteID = palettes[index].id
			m.globalMenuOpen = false
			m.globalPaletteMenuOpen = false
			return m, nil, true
		}
		m.globalMenuOpen = false
		m.globalPaletteMenuOpen = false
		return m, nil, true
	}
	if x >= menuWidth {
		m.globalMenuOpen = false
		m.globalPaletteMenuOpen = false
		return m, nil, true
	}

	switch y - menuTop {
	case 2:
		m.cycleTab(1)
		m.globalMenuOpen = false
		m.globalPaletteMenuOpen = false
		return m, nil, true
	case 3:
		m.cycleTab(-1)
		m.globalMenuOpen = false
		m.globalPaletteMenuOpen = false
		return m, nil, true
	case 4:
		m.globalPaletteMenuOpen = true
		return m, nil, true
	case 5:
		m.globalPaletteMenuOpen = false
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
	tabs := m.tabs()
	for index := range tabs {
		if x >= position && x < position+tabBoxWidth {
			m.setActiveTab(tabs[index].id)
			return true
		}
		position += tabBoxWidth + 1
	}
	return false
}

func (m Model) updateDashboardMouse(x int, y int) Model {
	for _, role := range []string{"main", "embedding", "reranking"} {
		if y == lineNumberContainingText(m.viewContent(), m.dashboardRunnerSlotPrefix(role)) &&
			len(m.runnersForRole(role)) > 1 {
			m.dashboardRunnerDropdown = role
			m.dashboardModelDropdown = ""
			return m
		}
	}
	if y != lineNumberContainingText(m.viewContent(), "Models ----") {
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

func (m Model) updateDashboardKey(value string) (Model, tea.Cmd, bool) {
	if m.dashboardRunnerDropdown != "" {
		index, err := strconv.Atoi(value)
		if err != nil {
			return m, nil, false
		}
		runners := m.runnersForRole(m.dashboardRunnerDropdown)
		if index < 1 || index > len(runners) {
			return m, nil, true
		}
		runner := runners[index-1]
		role := m.dashboardRunnerDropdown
		m.dashboardRunnerDropdown = ""
		return m, m.dashboardRouteCmd(role, runner), true
	}

	roleByKey := map[string]string{
		"m": "main",
		"e": "embedding",
		"r": "reranking",
	}
	role, ok := roleByKey[strings.ToLower(value)]
	if !ok || len(m.runnersForRole(role)) <= 1 {
		return m, nil, false
	}
	m.dashboardRunnerDropdown = role
	m.dashboardModelDropdown = ""
	return m, nil, true
}

func (m Model) updateWizardMouse(x int, y int) (tea.Model, tea.Cmd) {
	if option, ok := m.wizardOptionFromMouse(x, y); ok {
		m.openOptionModal(option)
		return m, nil
	}
	if index, ok := m.wizardModelIndexFromMouse(x, y); ok {
		m.setWizardModelSelection(index)
		return m, nil
	}

	choiceOffset := y - panelContentRow(m.contentTopRow(), 0)
	switch {
	case choiceOffset >= 0 && choiceOffset <= 2:
		if x >= wizardRuntimeLlamaX {
			m.wizardRuntime = "llamacpp"
			m.wizardBackend = m.firstAvailableLlamaType()
		} else {
			m.wizardRuntime = "litert"
			m.wizardBackend = "cpu"
		}
		m.wizardModelSelection = 0
		m.normalizeWizardSelection()
	case choiceOffset >= 3 && choiceOffset <= 5:
		m.setWizardVariantFromMouse(x)
	case choiceOffset >= 6 && choiceOffset <= 8:
		switch {
		case x >= wizardRoleRerankingX:
			m.setWizardRole("reranking")
		case x >= wizardRoleEmbeddingX:
			m.setWizardRole("embedding")
		case x >= wizardRoleMainX:
			m.setWizardRole("main")
		}
	case choiceOffset == 9:
		if x >= wizardStartX && x < wizardStartX+12 {
			return m, m.wizardCreateCmd()
		}
	}
	m.dispatchWizardState()
	return m, nil
}

func (m Model) updateRunnerBodyMouse(runner server.RunnerSnapshot, x int, y int) (tea.Model, tea.Cmd) {
	view := m.viewContent()
	switch {
	case renderedPointHitsText(view, x, y, "Start"):
		return m, m.runnerActionCmd("start", runner.ID)
	case renderedPointHitsText(view, x, y, "Stop"):
		return m, m.runnerActionCmd("stop", runner.ID)
	case renderedPointHitsText(view, x, y, "Restart"):
		return m, m.runnerActionCmd("restart", runner.ID)
	case renderedPointHitsText(view, x, y, "Edit Cmd"):
		m.edit = newCommandLineRunnerEdit(runner)
		return m, nil
	case renderedPointHitsText(view, x, y, "Close"):
		return m, m.runnerActionCmd("close", runner.ID)
	}
	return m, nil
}

func (m Model) updateSetupMouse(_ int, y int) Model {
	index, ok := m.setupBackendIndexFromMouse(y)
	if !ok {
		return m
	}
	m.setupSelection = index
	return m.toggleSelectedSetupBackend()
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

func (m Model) setupBackendIndexFromMouse(y int) (int, bool) {
	rows := m.setupBackendRows()
	row := y - panelContentRow(m.contentTopRow(), setupBackendFirstLine)
	if row < 0 || row >= len(rows) {
		return 0, false
	}
	return row, true
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

func (m Model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.activeTabID() == "chat" {
		return m.updateChatKey(msg)
	}
	if m.activeTabID() == "setup" {
		if next, cmd, ok := m.updateSetupKey(msg); ok {
			return next, cmd
		}
	}
	if m.managedScreen {
		if next, ok := m.updateScrollKey(msg); ok {
			return next, nil
		}
	}

	key := msg.Key()
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.globalMenuOpen {
		switch key.Code {
		case tea.KeyEsc, tea.KeyF1:
			m.globalMenuOpen = false
			m.globalPaletteMenuOpen = false
			return m, nil
		default:
			return m, nil
		}
	}
	switch key.Code {
	case tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyF1:
		m.globalMenuOpen = !m.globalMenuOpen
		m.globalPaletteMenuOpen = false
		return m, nil
	case tea.KeyRight:
		m.cycleTab(1)
		return m, nil
	case tea.KeyTab:
		if key.Mod.Contains(tea.ModShift) {
			m.cycleTab(-1)
		} else {
			m.cycleTab(1)
		}
		return m, nil
	case tea.KeyLeft:
		m.cycleTab(-1)
		return m, nil
	case tea.KeyBackspace, tea.KeyEnter:
		if m.activeTabID() == "wizard" && key.Code == tea.KeyEnter {
			return m, m.wizardCreateCmd()
		}
	}

	if msg.String() == "ctrl+h" {
		return m, nil
	}
	value := key.Text
	if value == "" && len([]rune(msg.String())) == 1 {
		value = msg.String()
	}
	if value == "" {
		return m, nil
	}
	if m.activeTabID() == "dashboard" {
		if next, cmd, ok := m.updateDashboardKey(value); ok {
			return next, cmd
		}
	}
	if m.selectRuneTab(value) {
		return m, nil
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
		case "k":
			if m.openApplicableWizardOption("cache-type-k") {
				return m, nil
			}
		case "c":
			m.openWizardCommandEdit()
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
		if value == "C" {
			m.edit = newCommandLineRunnerEdit(runner)
			return m, nil
		}
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
		case "c":
			return m, m.runnerActionCmd("close", runner.ID)
		}
	}

	return m, nil
}

func (m Model) updateSetupKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch msg.Key().Code {
	case tea.KeyUp:
		m.setupSelection--
		m.clampSetupSelection()
		return m, nil, true
	case tea.KeyDown:
		m.setupSelection++
		m.clampSetupSelection()
		return m, nil, true
	case tea.KeyEnter, tea.KeySpace:
		return m.toggleSelectedSetupBackend(), nil, true
	default:
		if msg.Key().Text == " " {
			return m.toggleSelectedSetupBackend(), nil, true
		}
	}
	return m, nil, false
}

func (m Model) updateScrollKey(msg tea.KeyPressMsg) (Model, bool) {
	switch msg.Key().Code {
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

func (m Model) updateEditKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	edit := *m.edit
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	key := msg.Key()
	switch key.Code {
	case tea.KeyEsc:
		m.edit = nil
		return m, nil
	case tea.KeyBackspace:
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
	default:
		if msg.String() == "ctrl+h" {
			if len(m.edit.value) > 0 {
				m.edit.value = m.edit.value[:len(m.edit.value)-1]
			}
			return m, nil
		}
		input := key.Text
		if input == "" || edit.numeric && !isDigits(input) {
			return m, nil
		}
		m.edit.value += input
		return m, nil
	}
}

func (m Model) updateRuntimeEditKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	edit := *m.runtimeEdit
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	key := msg.Key()
	switch key.Code {
	case tea.KeyEsc:
		m.runtimeEdit = nil
		return m, nil
	case tea.KeyBackspace:
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
	default:
		if msg.String() == "ctrl+h" {
			if len(m.runtimeEdit.value) > 0 {
				m.runtimeEdit.value = m.runtimeEdit.value[:len(m.runtimeEdit.value)-1]
			}
			return m, nil
		}
		input := key.Text
		if input == "" || edit.numeric && !isDigits(input) {
			return m, nil
		}
		m.runtimeEdit.value += input
		return m, nil
	}
}

func (m Model) updateOptionModalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch msg.Key().Code {
	case tea.KeyEsc:
		m.optionModal = nil
		return m, nil
	case tea.KeyEnter:
		m.saveOptionModal()
		return m, nil
	}
	input, cmd := m.optionModal.input.Update(msg)
	m.optionModal.input = input
	return m, cmd
}

func (m Model) updateOptionModalMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	p := shapes.Point{Row: msg.Y, Col: msg.X}
	if m.optionModal.popover.Contains(p) {
		// Close button hit.
		if m.optionModal.popover.CloseHitTarget().Rect.Contains(p) {
			m.optionModal = nil
			return m, nil
		}
		// Check for save/reset/sample button hits (text-search for now).
		if hit, ok := m.buttonHitAt(msg.X, msg.Y); ok {
			next, cmd, _ := m.handleButtonHit(hit)
			return next, cmd
		}
		return m, nil
	}
	// Outside click closes.
	m.optionModal = nil
	return m, nil
}

func (m Model) updateWizardCommandEditKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch msg.Key().Code {
	case tea.KeyEsc:
		m.wizardCommandEdit = nil
		return m, nil
	case tea.KeyEnter:
		m.saveWizardCommandEdit()
		return m, nil
	}
	input, cmd := m.wizardCommandEdit.input.Update(msg)
	m.wizardCommandEdit.input = input
	return m, cmd
}

func (m Model) updateWizardCommandEditMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	if hit, ok := m.buttonHitAt(msg.X, msg.Y); ok {
		next, cmd, _ := m.handleButtonHit(hit)
		return next, cmd
	}
	return m, nil
}

func (m Model) updateChatMouse(x int, y int) (tea.Model, tea.Cmd) {
	// Check scrollbar click first.
	if m.handleChatScrollbarClick(x, y) {
		return m, nil
	}
	if hit, ok := m.buttonHitAt(x, y); ok {
		next, cmd, _ := m.handleButtonHit(hit)
		return next, cmd
	}
	m.chatSettingsDropdown = ""
	m.chatTargetDropdown = false
	m.chatCommandPopup = false
	m.chatCustomField = ""
	m.chatCustomValue = ""
	return m, nil
}

// handleChatScrollbarClick computes the scrollbar position and scrolls the
// chat ScrollBox to the row corresponding to the click position.
func (m *Model) handleChatScrollbarClick(x, y int) bool {
	if m.activeTabID() != "chat" {
		return false
	}
	// Scrollbar is the rightmost 2 columns of the box inner content area.
	// In the full view (padded to m.width), scrollbar occupies:
	//   col = m.width - 6  and  col = m.width - 5
	scrollStart := m.width - 6
	scrollEnd := m.width - 5
	if x < scrollStart || x > scrollEnd {
		return false
	}

	m.chatScrollBox.ViewLines = m.chatScrollViewLines()
	m.chatScrollBox.SetLines(m.chatMessageLines())

	// Compute the starting row of the messages box in the full view.
	var startRow int
	if m.managedScreen {
		startRow = viewLineCount(m.managedTopView()) + 1
	} else {
		startRow = viewLineCount(m.headerView()) + viewLineCount(m.tabBar()) + 2
	}

	boxRow := y - startRow
	if boxRow < 0 || boxRow >= m.chatScrollBox.ViewLines {
		return false
	}

	// Arrow clicks: top row ▲ scrolls up 1, bottom row ▼ scrolls down 1.
	if boxRow == 0 {
		m.chatScrollBox.ScrollUp(1)
		return true
	}
	if boxRow == m.chatScrollBox.ViewLines-1 {
		m.chatScrollBox.ScrollDown(1)
		return true
	}

	// Track click: jump to exact proportional position.
	total := len(m.chatScrollBox.Lines)
	if total <= m.chatScrollBox.ViewLines {
		return true
	}
	maxOff := total - m.chatScrollBox.ViewLines
	// Track area excludes the first and last row (arrows).
	trackRows := m.chatScrollBox.ViewLines - 2
	trackRow := boxRow - 1
	if trackRows > 0 {
		m.chatScrollBox.Pinned = false
		m.chatScrollBox.Offset = trackRow * maxOff / trackRows
		m.chatScrollBox.ClampOffset()
	}
	return true
}

func (m *Model) saveOptionModal() {
	if m.optionModal == nil {
		return
	}
	m.saveOptionModalValue(m.optionModal.input.Value())
}

func (m *Model) saveOptionModalValue(value string) {
	if m.optionModal == nil {
		return
	}
	value = strings.TrimSpace(value)
	if m.wizardOptionOverrides == nil {
		m.wizardOptionOverrides = map[string]string{}
	}
	if m.optionModal.option.Kind == "bool" && value == "on" {
		value = ""
	}
	if m.optionModal.option.Kind == "bool" && value == "off" {
		delete(m.wizardOptionOverrides, m.optionModal.option.ID)
		m.optionModal = nil
		m.dispatchWizardState()
		return
	}
	m.wizardOptionOverrides[m.optionModal.option.ID] = value
	m.optionModal = nil
	m.dispatchWizardState()
}

func (m *Model) saveWizardCommandEdit() {
	if m.wizardCommandEdit == nil {
		return
	}
	m.applyWizardCommandLine(m.wizardCommandEdit.input.Value())
	m.wizardCommandEdit = nil
}

func (m Model) updateChatKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.chatCustomField != "" {
		switch msg.Key().Code {
		case tea.KeyEsc:
			m.chatCustomField = ""
			m.chatCustomValue = ""
			m.chatSettingsDropdown = ""
			return m, nil
		case tea.KeyEnter:
			if m.applyChatCustomValue() {
				m.chatCustomField = ""
				m.chatCustomValue = ""
				m.chatSettingsDropdown = ""
			}
			return m, nil
		case tea.KeyBackspace:
			if len(m.chatCustomValue) > 0 {
				m.chatCustomValue = m.chatCustomValue[:len(m.chatCustomValue)-1]
			}
			return m, nil
		}
		if msg.String() == "ctrl+h" {
			if len(m.chatCustomValue) > 0 {
				m.chatCustomValue = m.chatCustomValue[:len(m.chatCustomValue)-1]
			}
			return m, nil
		}
		if msg.Key().Text != "" && isChatNumericInput(msg.Key().Text) {
			m.chatCustomValue += msg.Key().Text
		}
		return m, nil
	}
	if m.chatSystemEditing {
		switch msg.Key().Code {
		case tea.KeyEsc:
			m.chatSystemEditing = false
			m.chatSystemField.Blur()
			return m, nil
		case tea.KeyEnter:
			if msg.Key().Mod&tea.ModShift == 0 {
				m.chatSystemEditing = false
				m.chatSystemField.Blur()
				m.chatSettingsDropdown = ""
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.chatSystemField, cmd = m.chatSystemField.Update(msg)
		return m, cmd
	}
	switch msg.Key().Code {
	case tea.KeyEsc:
		if m.chatCommandPopup || m.chatTargetDropdown || m.chatSettingsOpen || m.chatSettingsDropdown != "" {
			m.chatCommandPopup = false
			m.chatTargetDropdown = false
			m.chatSettingsOpen = false
			m.chatSettingsDropdown = ""
			return m, nil
		}
		return m, tea.Quit
	case tea.KeyPgUp:
		pageSize := m.chatScrollViewLines()
		m.chatScrollBox.SetLines(m.chatMessageLines())
		m.chatScrollBox.ScrollUp(pageSize)
		return m, nil
	case tea.KeyUp:
		m.chatScrollBox.SetLines(m.chatMessageLines())
		m.chatScrollBox.ScrollUp(1)
		return m, nil
	case tea.KeyPgDown:
		pageSize := m.chatScrollViewLines()
		m.chatScrollBox.SetLines(m.chatMessageLines())
		m.chatScrollBox.ScrollDown(pageSize)
		return m, nil
	case tea.KeyDown:
		m.chatScrollBox.SetLines(m.chatMessageLines())
		m.chatScrollBox.ScrollDown(1)
		return m, nil
	}
	if msg.String() == "ctrl+h" {
		if len(m.chatDraft) > 0 {
			m.chatDraft = m.chatDraft[:len(m.chatDraft)-1]
		}
		return m, nil
	}
	switch msg.Key().Code {
	case tea.KeyBackspace:
		if len(m.chatDraft) > 0 {
			m.chatDraft = m.chatDraft[:len(m.chatDraft)-1]
		}
		m.chatCommandPopup = strings.HasPrefix(m.chatDraft, "/")
		return m, nil
	case tea.KeyEnter:
		if msg.Key().Mod&tea.ModShift != 0 {
			m.chatDraft += "\n"
			return m, nil
		}
		if m.chatCommandPopup {
			if command := m.selectedChatCommand(); command != "" {
				return m.runChatCommand(command)
			}
			return m, nil
		}
		return m.submitChatPrompt()
	default:
		if msg.Key().Text == "" {
			return m, nil
		}
		if m.chatSettingsDropdown != "" {
			return m, nil
		}
		m.chatDraft += msg.Key().Text
		m.chatCommandPopup = strings.HasPrefix(m.chatDraft, "/")
		return m, nil
	}
}

func (m *Model) cycleChatTarget(delta int) {
	roles := []string{"main", "embedding", "reranking"}
	current := 0
	for index, role := range roles {
		if role == m.chatTarget() {
			current = index
			break
		}
	}
	next := (current + delta + len(roles)) % len(roles)
	m.chatTargetRole = roles[next]
	m.chatTargetDropdown = false
}

func (m *Model) selectChatTargetByKey(value string) bool {
	targets := map[string]string{
		"1": "main",
		"m": "main",
		"2": "embedding",
		"e": "embedding",
		"3": "reranking",
		"r": "reranking",
	}
	role, ok := targets[strings.ToLower(value)]
	if !ok {
		return false
	}
	m.chatTargetRole = role
	m.chatTargetDropdown = false
	return true
}

func (m Model) View() tea.View {
	view := tea.NewView(m.viewContent())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m Model) viewContent() string {
	var content string
	if m.managedScreen {
		if m.height <= 0 {
			content = m.managedStartupView()
			return m.screenFrame(content)
		}
		content = m.managedScreenView()
		return m.screenFrame(content)
	}
	content = m.fullView()
	return m.screenFrame(content)
}

func (m Model) screenFrame(content string) string {
	style := lipgloss.NewStyle().Background(lipgloss.Color(m.palette().backdrop))
	if m.width > 0 {
		style = style.Width(m.width)
	}
	return style.Render(content)
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
	if m.wizardCommandEdit != nil {
		builder.WriteString("\n")
		builder.WriteString(renderPanelSpec(m.wizardCommandEditSpec(), m.width))
	}
	return m.applyWizardOptionOverlay(pinFooterToBottom(builder.String(), m.footerView(), m.height))
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

	body := m.scrollableBodyView()
	visibleBody := sliceRenderedLines(body, m.scrollOffset, bodyHeight)
	if strings.TrimSpace(visibleBody) == "" {
		visibleBody = mutedStyle.Render("No content in this pane.")
	}
	visibleBody = fitLinesToHeight(visibleBody, bodyHeight)

	return m.applyWizardOptionOverlay(fitLinesToHeight(joinPanels(top, visibleBody, footer), m.height))
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
	case "setup":
		return m.setupView()
	case "chat":
		return m.chatView()
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

func newCommandLineRunnerEdit(runner server.RunnerSnapshot) *runnerEdit {
	current := runnerCommandLine(runner.Command)
	edit := newRunnerEdit(runner, "commandLine", "Command", current, false)
	edit.value = current
	return edit
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
	if m.store != nil {
		s := m.store.State()
		if s.UI.ActiveTab != "" {
			for _, t := range tabs {
				if t.id == s.UI.ActiveTab {
					return s.UI.ActiveTab
				}
			}
			// Stored tab ID no longer exists in current tabs (e.g. runner was
			// closed). Fall through to the local fallback.
		}
	}
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
		{id: "setup", label: "Setup"},
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
	if len(m.snapshot.Runners) > 0 {
		result = append(result, tab{id: "chat", label: m.chatTabLabel()})
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
		return "○ Chat no " + m.chatTarget()
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
	m.backendStatus = loadBackendStatus(m.backendConfigPath)
	m.llamaRuntimeVariants = m.filterLlamaRuntimeVariants(
		discoverLlamaRuntimeVariants(m.llamaRuntimeRoot),
	)
	m.clampSetupSelection()
	m.normalizeWizardSelection()
	if m.store != nil {
		s := m.store.State()
		if s.UI.ActiveTab != "" && len(m.tabs()) > 0 {
			tabIndex := -1
			for i, t := range m.tabs() {
				if t.id == s.UI.ActiveTab {
					tabIndex = i
					break
				}
			}
			if tabIndex >= 0 {
				m.active = tabIndex
			} else {
				// Stored tab no longer exists; reset to first tab.
				_, _ = m.store.Dispatch(m.ctx, store.ActionEnvelope{
					Type:    store.ActionTypeSelectTab,
					Source:  store.SourceSystem,
					Payload: store.MustPayload(store.SelectTabPayload{TabID: m.tabs()[0].id}),
				})
				m.active = 0
			}
		}
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
	if m.active != index {
		m.resetScroll()
	}
	m.setActiveTab(m.tabs()[index].id)
	return true
}

func (m Model) activeRunner() (server.RunnerSnapshot, bool) {
	id := strings.TrimPrefix(m.activeTabID(), "runner:")
	if id == m.activeTabID() {
		return server.RunnerSnapshot{}, false
	}
	return m.runnerByID(id)
}

func (m Model) activeRunnerID() string {
	if runner, ok := m.activeRunner(); ok {
		return runner.ID
	}
	return ""
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
	background := panelBackgroundForAccent(m.palette().header)
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Background(lipgloss.Color(background))
	separator := statusStyle.Render("  ")
	parts := []string{
		titleStyle.Background(lipgloss.Color(background)).Render("◆ G0LiteLLaMa"),
		statusStyle.Render("LiteRT: ") + runtimeUseBadge(m.runtimeAliveCount("litert"), background),
		statusStyle.Render("llama.cpp: ") + runtimeUseBadge(m.runtimeAliveCount("llamacpp"), background),
		statusStyle.Render(fmt.Sprintf("Runners: %d", len(m.snapshot.Runners))),
	}
	if m.width > 0 || m.height > 0 {
		parts = append(parts, statusStyle.Render(fmt.Sprintf("Viewport: %dx%d", m.width, m.height)))
	}

	style := lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(lipgloss.Color(m.palette().header)).
		Background(lipgloss.Color(background)).
		Padding(0, 1)
	if m.width > 2 {
		style = style.Width(m.width)
	}
	return style.Render(strings.Join(parts, separator))
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

func runtimeUseBadge(alive int, background string) string {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(runtimeUseBadgeColor(alive))).
		Background(lipgloss.Color(background))
	if alive > 0 {
		return style.Render("active")
	}
	return style.Render("idle")
}

func runtimeUseBadgeColor(alive int) string {
	if alive > 0 {
		return "82"
	}
	return "196"
}

func (m Model) selectedChatRunner() (server.RunnerSnapshot, bool) {
	if strings.TrimSpace(m.chatRunnerID) != "" {
		if runner, ok := m.runnerByID(m.chatRunnerID); ok && runner.Role == m.chatTarget() {
			return runner, true
		}
	}
	return m.defaultChatRunnerForRole(m.chatTarget())
}

func (m Model) defaultChatRunnerForRole(role string) (server.RunnerSnapshot, bool) {
	var fallbackRunner server.RunnerSnapshot
	hasFallback := false
	for _, runner := range m.snapshot.Runners {
		if runner.Role != role {
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

func (m Model) selectedRunnerForRole(role string) (server.RunnerSnapshot, bool) {
	if id := m.snapshot.Routes[role]; id != "" {
		if runner, ok := m.runnerByID(id); ok && runner.Role == role {
			return runner, true
		}
	}
	var fallbackRunner server.RunnerSnapshot
	hasFallback := false
	for _, runner := range m.snapshot.Runners {
		if runner.Role != role {
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

func (m Model) chatTarget() string {
	if isWizardRole(m.chatTargetRole) {
		return m.chatTargetRole
	}
	return "main"
}

func (m *Model) cycleTab(delta int) {
	tabs := m.tabs()
	next := (m.active + delta) % len(tabs)
	if next < 0 {
		next += len(tabs)
	}
	m.setActiveTab(tabs[next].id)
}

func (m *Model) setActiveTab(id string) {
	if m.store != nil {
		_, _ = m.store.Dispatch(m.ctx, store.ActionEnvelope{
			Type:    store.ActionTypeSelectTab,
			Source:  store.SourceTUI,
			Payload: store.MustPayload(store.SelectTabPayload{TabID: id}),
		})
	}
	for index, item := range m.tabs() {
		if item.id == id {
			if m.active != index {
				m.resetScroll()
			}
			m.active = index
			m.pinChatRunnerIfUnset()
			return
		}
	}
}

func (m *Model) pinChatRunnerIfUnset() {
	if m.activeTabID() != "chat" || strings.TrimSpace(m.chatRunnerID) != "" {
		return
	}
	if runner, ok := m.defaultChatRunnerForRole(m.chatTarget()); ok {
		m.chatRunnerID = runner.ID
	}
}

func (m Model) tabBar() string {
	tabs := m.tabs()
	parts := make([]string, 0, len(tabs))
	activeID := m.activeTabID()
	for index, item := range tabs {
		label := fmt.Sprintf("%d %s", index+1, item.label)
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color(m.palette().tabBG))
		if item.id == activeID {
			style = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("16")).
				Background(lipgloss.Color(m.palette().tabActiveBG))
		}
		parts = append(parts, style.Render(centeredText(label, tabBoxWidth)))
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
		return "◆ Dashboard / status.get + /g0litellama/v1/status"
	case "wizard":
		return "◇ Launch Wizard / RunnerController.CreateRunner"
	case "setup":
		return "◇ Setup / G0LiteLLaMa/runtime-config/backends.json"
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
	if !m.globalPaletteMenuOpen {
		return menu
	}
	paletteMenu := m.paletteMenuView()
	return lipgloss.JoinHorizontal(lipgloss.Top, menu, strings.Repeat(" ", panelGridColumnGap), paletteMenu)
}

func (m Model) globalMenuMainView() string {
	return renderMenuPanel(
		"Global menu",
		[]string{
			"Next tab",
			"Previous tab",
			"Palette themes >",
			"Esc Quit",
		},
		m.palette(),
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
	return renderMenuPanel("Palette choices", lines, m.palette())
}

func (m Model) bottomActionBarView() string {
	palette := m.palette()
	segments := m.bottomActionSegments()

	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		style := lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color(palette.footerFG)).
			Background(lipgloss.Color(palette.tabBG))
		switch segment.id {
		case "menu":
			style = style.Bold(true).
				Foreground(lipgloss.Color("16")).
				Background(lipgloss.Color(palette.tabActiveBG))
		case "runner-start":
			style = style.Bold(true).
				Foreground(lipgloss.Color(palette.actionFG)).
				Background(lipgloss.Color(palette.actionBG))
		case "runner-stop", "runner-restart", "runner-edit-command", "runner-close", "chat-clear", "chat-new":
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
		items = append(items, bottomActionSegment{id: "hint", label: "Dashboard: m/e/r routes | click model roles"})
	case "wizard":
		items = append(items, bottomActionSegment{id: "hint", label: "Wizard: click toggles | k Cache K | c Command | Enter Start"})
	case "setup":
		items = append(items, bottomActionSegment{id: "hint", label: "Setup: Up/Down select | Enter toggle"})
	case "chat":
		items = append(
			items,
			bottomActionSegment{id: "chat-clear", label: "Clear"},
			bottomActionSegment{id: "chat-new", label: "New"},
		)
	default:
		if runner, ok := m.activeRunner(); ok {
			items = append(
				items,
				bottomActionSegment{id: "hint", label: fmt.Sprintf("Runner %s", runner.ID)},
				bottomActionSegment{id: "runner-start", label: "Start"},
				bottomActionSegment{id: "runner-stop", label: "Stop"},
				bottomActionSegment{id: "runner-restart", label: "Restart"},
				bottomActionSegment{id: "runner-edit-command", label: "Edit Cmd"},
				bottomActionSegment{id: "runner-close", label: "X Close"},
			)
		}
	}

	position := 1
	for index := range items {
		items[index].start = position
		items[index].end = position + lipgloss.Width(items[index].label) + 2
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
			"API: status.get + /g0litellama/v1/status",
		)
	case "wizard":
		lines = append(
			lines,
			"Launch Wizard: t Runtime | b Variant | m/e/r Role | n/p Model | c Command | Enter create",
			"API: RunnerController.CreateRunner + POST /g0litellama/v1/runners",
			"API: WebSocket api.request POST /g0litellama/v1/runners",
		)
	case "setup":
		lines = append(
			lines,
			"Setup: Up/Down backend | Enter/Space toggle enabled state",
			"Config: "+m.backendConfigPath,
			"Launch Wizard reads the updated runtime backend status immediately",
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
			"API: Catalog.Download + POST /g0litellama/v1/models/download",
			"API: RunnerController.CreateRunner + POST /g0litellama/v1/runners",
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
			"API: api.request GET/PATCH/POST /g0litellama/v1/*",
		)
	default:
		runner, ok := m.activeRunner()
		if !ok {
			lines = append(lines, "Runner: no active runner")
			return lines
		}
		basePath := "/g0litellama/v1/runners/" + runner.ID
		lines = append(
			lines,
			fmt.Sprintf("Runner %s: s Start | x Stop | r Restart | C Edit Cmd | c Close", runner.ID),
			"Edit: b/p/h/i/m/e/u/f/C/l/v/t/o",
			"API: RunnerController + "+basePath,
			"Route: "+runnerRoleRoute(runner.Role)+" -> "+fallback(runner.Upstream, "unavailable"),
		)
	}

	return lines
}

func (m Model) dashboardView() string {
	status := panelSpec{"Status", m.dashboardStatusLines(), "45"}
	if m.dashboardRunnerDropdown != "" {
		return m.panelGrid(
			status,
			panelSpec{
				roleDisplayName(m.dashboardRunnerDropdown) + " runners",
				m.runnerDropdownItemLines(m.dashboardRunnerDropdown),
				"205",
			},
		)
	}
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
		"Runners by role",
		fmt.Sprintf("Main        %d alive", m.roleAliveCount("main")),
		fmt.Sprintf("Embedding   %d alive", m.roleAliveCount("embedding")),
		fmt.Sprintf("Reranking   %d alive", m.roleAliveCount("reranking")),
		"Runner slots",
		m.dashboardRunnerSlotLine("main"),
		m.dashboardRunnerSlotLine("embedding"),
		m.dashboardRunnerSlotLine("reranking"),
		fmt.Sprintf(
			"Models ---- Main %d -- Embedding %d -- Reranking %d",
			m.presentModelCount("main"),
			m.presentModelCount("embedding"),
			m.presentModelCount("reranking"),
		),
	}
}

func (m Model) dashboardRunnerSlotLine(role string) string {
	runnerID := m.snapshot.Routes[role]
	runner, ok := m.runnerByID(runnerID)
	value := "none"
	if ok {
		value = runner.ID + " " + runner.State
	}
	if len(m.runnersForRole(role)) > 1 {
		value += "  [choose]"
	}
	return m.dashboardRunnerSlotPrefix(role) + value
}

func (m Model) dashboardRunnerSlotPrefix(role string) string {
	return fmt.Sprintf("%-12s", role)
}

func (m Model) runnersForRole(role string) []server.RunnerSnapshot {
	runners := []server.RunnerSnapshot{}
	for _, runner := range m.snapshot.Runners {
		if runner.Role == role {
			runners = append(runners, runner)
		}
	}
	return runners
}

func (m Model) runnerDropdownItemLines(role string) []string {
	runners := m.runnersForRole(role)
	if len(runners) == 0 {
		return []string{"No " + role + " runners."}
	}
	lines := make([]string, 0, len(runners))
	active := m.snapshot.Routes[role]
	for index, runner := range runners {
		marker := " "
		if runner.ID == active {
			marker = "●"
		}
		lines = append(lines, fmt.Sprintf("%s %d %s  %s  %s", marker, index+1, runner.ID, runner.State, runner.Runtime))
	}
	return lines
}

func (m Model) setupView() string {
	return renderPanelSpec(
		panelSpec{"Backend Setup", m.setupBackendLines(), "82"},
		m.width,
	)
}

func (m Model) setupBackendLines() []string {
	rows := m.setupBackendRows()
	selected := clampInt(m.setupSelection, 0, len(rows)-1)
	lines := []string{
		"Up/Down selects a backend; Enter/Space toggles enabled state.",
		"Click a backend row to toggle it.",
		"Disabled backends are hidden from Launch Wizard choices.",
		"Runtime     Backend   State",
	}
	for index, row := range rows {
		marker := " "
		if index == selected {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf(
			"%s %-10s %-9s %s",
			marker,
			row.RuntimeLabel,
			row.Backend,
			m.setupBackendState(row),
		))
	}
	lines = append(lines, "", "Config: "+m.backendConfigPath)
	return lines
}

func (m Model) setupBackendState(row setupBackendRow) string {
	if m.backendStatus.Visible(row.Runtime, row.Backend) {
		return "enabled"
	}
	return "disabled"
}

func (m Model) toggleSelectedSetupBackend() Model {
	row, ok := m.selectedSetupBackend()
	if !ok {
		return m
	}
	nextWorking := !m.backendStatus.Visible(row.Runtime, row.Backend)
	if err := runtimeconfig.SetBackendWorking(
		m.backendConfigPath,
		row.Runtime,
		row.Backend,
		nextWorking,
	); err != nil {
		m.notice = fmt.Sprintf("update %s/%s backend failed: %v", row.Runtime, row.Backend, err)
		return m
	}

	m.refresh()
	state := "disabled"
	if nextWorking {
		state = "enabled"
	}
	m.notice = fmt.Sprintf("%s %s/%s backend", state, row.Runtime, row.Backend)
	return m
}

func (m Model) selectedSetupBackend() (setupBackendRow, bool) {
	rows := m.setupBackendRows()
	if len(rows) == 0 {
		return setupBackendRow{}, false
	}
	index := clampInt(m.setupSelection, 0, len(rows)-1)
	return rows[index], true
}

func (m *Model) clampSetupSelection() {
	rows := m.setupBackendRows()
	if len(rows) == 0 {
		m.setupSelection = 0
		return
	}
	m.setupSelection = clampInt(m.setupSelection, 0, len(rows)-1)
}

func (m Model) setupBackendRows() []setupBackendRow {
	llamaOptions := m.availableLlamaTypeOptions()
	rows := make([]setupBackendRow, 0, len(litertBackendOptions())+len(llamaOptions))
	for _, backend := range litertBackendOptions() {
		rows = append(rows, setupBackendRow{
			Runtime:      "litert",
			RuntimeLabel: "LiteRT",
			Backend:      backend,
		})
	}
	for _, backend := range llamaOptions {
		rows = append(rows, setupBackendRow{
			Runtime:      "llamacpp",
			RuntimeLabel: "llama.cpp",
			Backend:      backend,
		})
	}
	return rows
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
			return fmt.Sprintf("open runner tab %d %s or Models for downloads", index+4, runner.ID)
		}
	}
	for index, runner := range m.snapshot.Runners {
		if strings.TrimSpace(runner.ID) != "" {
			return fmt.Sprintf("open runner tab %d %s and press s to start", index+4, runner.ID)
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
		formatKV("G0LiteLLaMa API", "127.0.0.1:9379 /g0litellama/v1/*"),
		formatKV("Control WS", "ws://127.0.0.1:9379/g0litellama/v1/ws"),
		formatKV("runtime upstream", fallback(m.runtime.Upstream, "unavailable")),
		formatKV("Executable", fallback(m.runtime.Executable, "not configured")),
		"api.request => G0LiteLLaMa controllers => runner supervisor",
	}
}

func (m Model) topologyGraphLines() []string {
	lines := []string{
		"◉ API client",
		"│  api.request / ws://127.0.0.1:9379/g0litellama/v1/ws",
		"▼",
		"◆ G0LiteLLaMa API",
		"│  127.0.0.1:9379 /g0litellama/v1/*",
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
	panels := []panelSpec{
		panelSpec{"Runner " + runner.ID, m.runnerSummaryLines(runner), "82"},
		panelSpec{"Routes / Controls", m.runnerRouteControlLines(runner), "45"},
	}
	if editor := m.runnerEditorSpec(runner); editor.title != "" {
		panels = append(panels, editor)
	}
	return joinPanels(
		m.panelGrid(panels...),
		renderPanelSpec(m.runnerTerminalSpec(runner), m.width),
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
		formatKV("Command", fallback(runnerCommandLine(runner.Command), "not configured")),
	}
	if runner.Detail != "" {
		lines = append(lines, formatKV("Detail", runner.Detail))
	}
	return lines
}

func (m Model) runnerRouteControlLines(runner server.RunnerSnapshot) []string {
	basePath := "/g0litellama/v1/runners/" + runner.ID
	lines := []string{
		formatKV("Route", runnerRoleRoute(runner.Role)),
		formatKV("Upstream", fallback(runner.Upstream, "unavailable")),
		formatKV("Models", endpointPath(runner.Upstream, "/v1/models")),
		"",
		"Actions ---- s Start -- x Stop -- r Restart -- C Edit Cmd -- c Close",
		"POST " + basePath + "/start",
		"POST " + basePath + "/stop",
		"POST " + basePath + "/restart",
		"POST " + basePath + "/close",
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
	basePath := "/g0litellama/v1/runners/" + runner.ID
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
		"RunnerController.StartRunner / StopRunner / RestartRunner / CloseRunner / UpdateRunner",
		"WebSocket api.request parity:",
		"POST " + basePath + "/start",
		"POST " + basePath + "/stop",
		"POST " + basePath + "/restart",
		"POST " + basePath + "/close",
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
			"C",
			"Command",
			fallback(runnerCommandLine(runner.Command), "not configured"),
			"commandLine",
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
		"PATCH /g0litellama/v1/runners/" + runner.ID,
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
	basePath := "/g0litellama/v1/runners/" + runner.ID
	return []string{
		"Controls",
		"Actions",
		"s Start   x Stop   r Restart   C Edit Cmd   c Close",
		"POST " + basePath + "/start",
		"POST " + basePath + "/stop",
		"POST " + basePath + "/restart",
		"POST " + basePath + "/close",
		"",
		"Edit settings",
		"Settings editor",
		"b Backend CPU/GPU   l Launch managed/external   v Verbose",
		"t Runtime   o Role",
		"p Port   h Host   i Model ID   C Command",
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

func (m Model) runnerTerminalSpec(runner server.RunnerSnapshot) panelSpec {
	return panelSpec{
		title: "Terminal / Logs",
		lines: m.runnerTerminalLines(runner),
		color: "244",
	}
}

func (m Model) runnerTerminalLines(runner server.RunnerSnapshot) []string {
	lines := []string{
		formatKV("Command", fallback(runnerCommandLine(runner.Command), "not configured")),
		"",
	}
	lines = append(lines, m.runnerLogLines(runner.ID, runnerTerminalLogLimit)...)
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
			"Enter saves through PATCH /g0litellama/v1/runners/{id}; Esc cancels.",
		},
		"205",
	}
}

func (m Model) wizardView() string {
	return m.panelGrid(
		panelSpec{"Launch Wizard", m.wizardChoiceLines(), "214"},
		panelSpec{"Local Models", m.wizardLocalModelLines(), "45"},
		panelSpec{"CLI Options", m.wizardCLIOptionLines(), "82"},
		panelSpec{"Command Preview", m.wizardCommandPreviewLines(), "244"},
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
		m.wizardVariantToggleLine(),
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

func (m Model) wizardCLIOptionLines() []string {
	options := m.visibleWizardCLIOptions()
	if len(options) == 0 {
		return []string{"no options for selected runtime/backend/role"}
	}
	lines := make([]string, 0, len(options)+1)
	for _, option := range options {
		lines = append(lines, m.wizardCLIOptionLine(option))
	}
	if totalPages := m.wizardCLIOptionPageCount(); totalPages > 1 {
		lines = append(lines, m.fullWidthWizardLine(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("238")),
			fmt.Sprintf("Page %d/%d  [ Prev ]  [ Next ]", m.currentWizardOptionPage()+1, totalPages),
		))
	}
	return lines
}

func (m Model) wizardCLIOptionLine(option wizardCLIOption) string {
	background := m.palette().cliRowBG

	command := wizardOptionButtonTextFromLabel(wizardOptionDisplayName(option))
	value := wizardCLIOptionValueText(m, option)
	description := singleLineText(option.Description)
	if value == "" {
		value = option.DefaultText
	}

	width := m.wizardContentWidth()
	commandWidth := m.wizardCLICommandColumnWidth()
	if width <= 0 {
		return wizardCLIOptionANSI(m.palette().modelSelected, background, true) +
			padRight(command, commandWidth) +
			wizardCLIOptionANSI(m.palette().variantHeader, background, false) +
			padRight(value, wizardCLIValueWidth) +
			wizardCLIOptionANSI("252", background, false) +
			description +
			"\x1b[m"
	}

	descriptionWidth := maxInt(0, width-commandWidth-wizardCLIValueWidth)
	description = padRight(truncateToWidth(description, descriptionWidth), descriptionWidth)
	return wizardCLIOptionANSI(m.palette().modelSelected, background, true) +
		padRight(command, commandWidth) +
		wizardCLIOptionANSI(m.palette().variantHeader, background, false) +
		padRight(value, wizardCLIValueWidth) +
		wizardCLIOptionANSI("252", background, false) +
		description +
		"\x1b[m"
}

func (m Model) wizardCLICommandColumnWidth() int {
	width := wizardCLICommandWidth
	for _, option := range m.visibleWizardCLIOptions() {
		width = maxInt(width, lipgloss.Width(wizardOptionButtonText(option))+1)
	}
	return width
}

func wizardCLIOptionValueText(m Model, option wizardCLIOption) string {
	value, ok := m.wizardOptionOverrides[option.ID]
	if !ok {
		return ""
	}
	if value == "" {
		return "on"
	}
	return value
}

func wizardCLIOptionANSI(fg string, bg string, bold bool) string {
	if bold {
		return fmt.Sprintf("\x1b[1;38;5;%s;48;5;%sm", fg, bg)
	}
	return fmt.Sprintf("\x1b[22;38;5;%s;48;5;%sm", fg, bg)
}

func (m Model) wizardCommandPreviewLines() []string {
	spec, _, err := m.wizardRunnerSpec()
	if err != nil {
		return []string{err.Error()}
	}
	return []string{wizardCommandPreview(spec)}
}

func (m Model) wizardCommandEditSpec() panelSpec {
	if m.wizardCommandEdit == nil {
		return panelSpec{}
	}
	return panelSpec{
		title: "Edit Command Preview",
		lines: []string{
			"Input",
			m.wizardCommandEdit.input.View(),
			"",
			"[ Save ]  [ X ]",
		},
		color: "205",
	}
}

func (m Model) optionModalSpec() panelSpec {
	if m.optionModal == nil {
		return panelSpec{}
	}
	option := m.optionModal.option
	lines := []string{
		option.Long,
		option.Description,
		formatKV("Default", fallback(option.DefaultText, "none")),
	}
	if option.Short != "" {
		lines = append([]string{formatKV("Shortcut", option.Short)}, lines...)
	}
	if len(option.Enums) > 0 {
		lines = append(lines, formatKV("Enum", strings.Join(option.Enums, ", ")))
	}
	lines = append(lines,
		"",
		"Input",
		m.optionModal.input.View(),
		"",
		"[ Save ]  [ Reset ]  [ X ]",
	)
	return panelSpec{
		title: "CLI Option " + wizardOptionDisplayName(option),
		lines: lines,
		color: "205",
	}
}

func (m Model) optionModalView() string {
	if m.optionModal == nil {
		return ""
	}
	return m.optionModal.popover.Render()
}

func (m Model) buildOptionModalBody(option wizardCLIOption, input textinput.Model, width int) string {
	lines := []string{
		option.Long,
		truncateToWidth(option.Description, width),
		formatKV("Default", fallback(option.DefaultText, "none")),
	}
	if option.Short != "" {
		lines = append([]string{formatKV("Shortcut", option.Short)}, lines...)
	}
	if samples := wizardOptionSamples(option); len(samples) > 0 {
		buttons := make([]string, 0, len(samples))
		for _, sample := range samples {
			buttons = append(buttons, "["+sample+"]")
		}
		lines = append(lines, wrapWords("Samples:", buttons, width)...)
	}
	lines = append(lines,
		"",
		"Input",
		input.View(),
		"",
		"[ Save ]  [ Reset ]  [ X ]",
	)
	return strings.Join(lines, "\n")
}

func (m Model) applyWizardOptionOverlay(view string) string {
	if m.optionModal == nil {
		return view
	}
	return m.optionModal.popover.Apply(view)
}

func (m Model) wizardOptionFromMouse(x int, y int) (wizardCLIOption, bool) {
	view := m.viewContent()
	for _, option := range m.visibleWizardCLIOptions() {
		row := lineNumberContainingText(view, wizardOptionButtonText(option))
		if row == y {
			return option, true
		}
	}
	return wizardCLIOption{}, false
}

func (m *Model) openOptionModal(option wizardCLIOption) {
	m.openOptionModalAt(option, m.contentTopRow()+1, 0)
}

func (m *Model) openOptionModalAt(option wizardCLIOption, row int, column int) {
	input := textinput.New()
	input.SetValue(m.wizardOptionOverrides[option.ID])
	input.Focus()

	width := clampInt(m.width-column-2, 46, 74)
	body := m.buildOptionModalBody(option, input, width)
	bodyLines := strings.Split(body, "\n")

	popover := Popover{
		Title:  "CLI Option " + wizardOptionDisplayName(option),
		Body:   body,
		Width:  width,
		Height: len(bodyLines),
	}
	anchor := shapes.Rect{Row: row, Col: column, Rows: 1, Cols: width}
	viewport := shapes.Rect{Row: 0, Col: 0, Rows: m.height, Cols: m.width}
	popover.Layout(anchor, viewport)

	m.optionModal = &optionModal{option: option, input: input, popover: popover}
}

func (m *Model) openWizardCommandEdit() {
	input := textinput.New()
	input.SetValue(strings.Join(m.currentWizardCommandArgs(), " "))
	input.Focus()
	m.wizardCommandEdit = &wizardCommandEdit{input: input}
	if m.managedScreen {
		m.scrollOffset = m.maxScrollOffset()
	}
}

func (m *Model) openApplicableWizardOption(id string) bool {
	for _, option := range m.allWizardCLIOptions() {
		if option.ID == id {
			m.openOptionModal(option)
			return true
		}
	}
	return false
}

func optionLabel(option wizardCLIOption) string {
	if option.Short != "" {
		return option.Short
	}
	return option.Long
}

func wizardOptionDisplayName(option wizardCLIOption) string {
	return strings.TrimLeft(optionLabel(option), "-")
}

func wizardOptionButtonText(option wizardCLIOption) string {
	return wizardOptionButtonTextFromLabel(wizardOptionDisplayName(option))
}

func wizardOptionButtonTextFromLabel(label string) string {
	return "[" + strings.TrimLeft(label, "-") + "]"
}

func (m Model) currentWizardOptionPage() int {
	return clampInt(m.wizardOptionPage, 0, maxInt(0, m.wizardCLIOptionPageCount()-1))
}

func (m Model) wizardCLIOptionPageCount() int {
	total := len(m.allWizardCLIOptions())
	if total == 0 {
		return 1
	}
	return (total + wizardCLIOptionPageSize - 1) / wizardCLIOptionPageSize
}

func (m Model) visibleWizardCLIOptions() []wizardCLIOption {
	options := m.allWizardCLIOptions()
	if len(options) <= wizardCLIOptionPageSize {
		return options
	}
	start := m.currentWizardOptionPage() * wizardCLIOptionPageSize
	end := minInt(start+wizardCLIOptionPageSize, len(options))
	return options[start:end]
}

func (m *Model) pageWizardOptions(delta int) {
	pageCount := m.wizardCLIOptionPageCount()
	if pageCount <= 1 {
		m.wizardOptionPage = 0
		return
	}
	m.wizardOptionPage = (m.currentWizardOptionPage() + delta + pageCount) % pageCount
}

func (m Model) allWizardCLIOptions() []wizardCLIOption {
	extras := make([]wizardCLIOption, 0, len(m.wizardCommandExtras))
	for _, extra := range m.wizardCommandExtras {
		extras = append(extras, extra.Option)
	}
	return append(extras, applicableWizardOptions(m.wizardRuntime, m.wizardBackend, m.wizardRole)...)
}

func (m Model) wizardOptionByID(id string) (wizardCLIOption, bool) {
	for _, option := range m.allWizardCLIOptions() {
		if option.ID == id {
			return option, true
		}
	}
	return wizardCLIOption{}, false
}

func (m Model) currentWizardCommandArgs() []string {
	spec, _, err := m.wizardRunnerSpec()
	if err != nil {
		return nil
	}
	return spec.Command
}

func (m *Model) applyWizardCommandLine(line string) {
	args := strings.Fields(line)
	overrides := map[string]string{}
	extras := []wizardCommandExtra{}
	extraSeen := map[string]bool{}
	options := applicableWizardOptions(m.wizardRuntime, m.wizardBackend, m.wizardRole)

	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		if arg == "--n-gpu-layers" {
			if index+1 < len(args) {
				if args[index+1] != "999" {
					overrides["gpu-layers"] = args[index+1]
				}
				index++
			}
			continue
		}
		if option, ok := wizardOptionForFlag(options, arg); ok {
			if option.Kind == "bool" {
				overrides[option.ID] = ""
				continue
			}
			if index+1 < len(args) {
				overrides[option.ID] = args[index+1]
				index++
			}
			continue
		}
		if wizardFixedCommandFlagHasValue(arg) {
			index++
			continue
		}
		if wizardFixedCommandFlag(arg) {
			continue
		}
		option := wizardCustomOption(arg)
		if option.ID == "" || extraSeen[option.ID] {
			continue
		}
		extraSeen[option.ID] = true
		if index+1 < len(args) && !strings.HasPrefix(args[index+1], "-") {
			overrides[option.ID] = args[index+1]
			index++
		} else {
			overrides[option.ID] = ""
		}
		extras = append(extras, wizardCommandExtra{Option: option})
	}

	m.wizardOptionOverrides = overrides
	m.wizardCommandExtras = extras
	m.wizardRemovedDefaults = m.removedWizardDefaults(args)
	m.wizardOptionPage = 0
}

func wizardOptionForFlag(options []wizardCLIOption, flag string) (wizardCLIOption, bool) {
	for _, option := range options {
		if flag == option.Short || flag == option.Long {
			return option, true
		}
	}
	return wizardCLIOption{}, false
}

func wizardFixedCommandFlagHasValue(flag string) bool {
	switch flag {
	case "-m", "--model", "--alias", "--host", "--port":
		return true
	default:
		return false
	}
}

func wizardFixedCommandFlag(flag string) bool {
	return flag == "serve"
}

func wizardCustomOption(flag string) wizardCLIOption {
	id := strings.TrimLeft(flag, "-")
	if id == "" {
		return wizardCLIOption{}
	}
	option := wizardCLIOption{
		ID:          id,
		Kind:        "string",
		DefaultText: "from command preview",
		Description: "Added from command preview.",
	}
	if strings.HasPrefix(flag, "--") {
		option.Long = flag
	} else {
		option.Short = flag
	}
	return option
}

func (m Model) removedWizardDefaults(args []string) map[string]bool {
	removed := map[string]bool{}
	if usesWizardGPUBackend(m.wizardBackend) && !containsString(args, "--n-gpu-layers") {
		removed["gpu-layers"] = true
	}
	if m.wizardRole == "embedding" && !containsString(args, "--embedding") {
		removed["embedding"] = true
	}
	if m.wizardRole == "reranking" {
		if !containsString(args, "--embedding") {
			removed["embedding"] = true
		}
		if !containsString(args, "--pooling") {
			removed["pooling"] = true
		}
		if !containsString(args, "--reranking") {
			removed["reranking"] = true
		}
	}
	return removed
}

func appendWizardOptionOverrides(
	args []string,
	runtimeName string,
	backend string,
	role string,
	overrides map[string]string,
) []string {
	if len(overrides) == 0 {
		return args
	}
	for _, option := range applicableWizardOptions(runtimeName, backend, role) {
		value, ok := overrides[option.ID]
		if !ok {
			continue
		}
		flag := optionLabel(option)
		args = removeFlag(args, flag, option.Long, option.Kind != "bool")
		if option.Kind == "bool" && strings.TrimSpace(value) == "" {
			args = append(args, flag)
			continue
		}
		args = append(args, flag, strings.TrimSpace(value))
	}
	return args
}

func copyStringMap(values map[string]string) map[string]string {
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func removeFlag(args []string, shortFlag string, longFlag string, hasValue bool) []string {
	out := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == shortFlag || arg == longFlag {
			if hasValue && index+1 < len(args) {
				index++
			}
			continue
		}
		out = append(out, arg)
	}
	return out
}

func usesWizardGPUBackend(backend string) bool {
	switch backend {
	case "gpu", "metal", "cuda13", "cuda12", "sycl":
		return true
	default:
		return false
	}
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

type wizardCLIOption struct {
	ID          string
	Runtime     string
	Backends    []string
	Roles       []string
	Short       string
	Long        string
	Kind        string
	DefaultText string
	Enums       []string
	Description string
}

type wizardCommandExtra struct {
	Option wizardCLIOption
}

func wizardOptionCatalog() []wizardCLIOption {
	// Catalog is static by design; source list lives in docs/superpowers/specs/2026-06-20-G0LiteLLaMa-tui-cli-options-design.md.
	kvEnums := []string{"f32", "f16", "bf16", "q8_0", "q4_0", "q4_1", "iq4_nl", "q5_0", "q5_1"}
	gpuBackends := []string{"gpu", "metal", "cuda13", "cuda12", "sycl"}
	return []wizardCLIOption{
		{ID: "host", Runtime: "litert", Long: "--host", Kind: "string", DefaultText: "0.0.0.0", Description: "Server bind host for litert-lm serve."},
		{ID: "port", Runtime: "litert", Long: "--port", Kind: "int", DefaultText: "9379", Description: "Server bind port for litert-lm serve."},
		{ID: "verbose", Runtime: "litert", Long: "--verbose", Kind: "bool", DefaultText: "off", Description: "Enable verbose LiteRT-LM server logging."},
		{ID: "ctx-size", Runtime: "llamacpp", Short: "-c", Long: "--ctx-size", Kind: "int", DefaultText: "model default", Description: "Context size. Lower values reduce KV cache memory."},
		{ID: "cache-type-k", Runtime: "llamacpp", Short: "-ctk", Long: "--cache-type-k", Kind: "enum", DefaultText: "f16", Enums: kvEnums, Description: "KV cache K quantization type."},
		{ID: "cache-type-v", Runtime: "llamacpp", Short: "-ctv", Long: "--cache-type-v", Kind: "enum", DefaultText: "f16", Enums: kvEnums, Description: "KV cache V quantization type."},
		{ID: "flash-attn", Runtime: "llamacpp", Short: "-fa", Long: "--flash-attn", Kind: "enum", DefaultText: "auto", Enums: []string{"on", "off", "auto"}, Description: "Flash Attention mode."},
		{ID: "gpu-layers", Runtime: "llamacpp", Backends: gpuBackends, Short: "-ngl", Long: "--gpu-layers", Kind: "string", DefaultText: "auto", Description: "Layers to offload. Use integer, auto, or all."},
		{ID: "fit", Runtime: "llamacpp", Short: "-fit", Long: "--fit", Kind: "enum", DefaultText: "on", Enums: []string{"on", "off"}, Description: "Memory fitting mode."},
		{ID: "fit-target", Runtime: "llamacpp", Short: "-fitt", Long: "--fit-target", Kind: "int", DefaultText: "1024", Description: "Per-device MiB margin for memory fitting."},
		{ID: "fit-ctx", Runtime: "llamacpp", Short: "-fitc", Long: "--fit-ctx", Kind: "int", DefaultText: "4096", Description: "Minimum context size fit may use."},
		{ID: "batch-size", Runtime: "llamacpp", Short: "-b", Long: "--batch-size", Kind: "int", DefaultText: "2048", Description: "Logical batch size."},
		{ID: "ubatch-size", Runtime: "llamacpp", Short: "-ub", Long: "--ubatch-size", Kind: "int", DefaultText: "512", Description: "Physical batch size."},
		{ID: "parallel", Runtime: "llamacpp", Short: "-np", Long: "--parallel", Kind: "int", DefaultText: "-1", Description: "Server slots."},
		{ID: "kv-offload", Runtime: "llamacpp", Backends: gpuBackends, Short: "-kvo", Long: "--kv-offload", Kind: "bool", DefaultText: "on", Description: "KV cache offload."},
		{ID: "no-kv-offload", Runtime: "llamacpp", Backends: gpuBackends, Long: "--no-kv-offload", Kind: "bool", DefaultText: "off", Description: "Keep KV cache off device when GPU memory is tight."},
		{ID: "no-mmap", Runtime: "llamacpp", Long: "--no-mmap", Kind: "bool", DefaultText: "off", Description: "Disable memory-mapped model loading."},
		{ID: "mlock", Runtime: "llamacpp", Long: "--mlock", Kind: "bool", DefaultText: "off", Description: "Keep model in RAM instead of swap or compression."},
		{ID: "cache-ram", Runtime: "llamacpp", Short: "-cram", Long: "--cache-ram", Kind: "int", DefaultText: "8192", Description: "Prompt cache RAM MiB."},
		{ID: "cache-prompt", Runtime: "llamacpp", Long: "--cache-prompt", Kind: "bool", DefaultText: "on", Description: "Prompt caching."},
		{ID: "cache-reuse", Runtime: "llamacpp", Long: "--cache-reuse", Kind: "int", DefaultText: "0", Description: "KV shifting reuse chunk size."},
		{ID: "device", Runtime: "llamacpp", Backends: gpuBackends, Short: "-dev", Long: "--device", Kind: "string", DefaultText: "all", Description: "Restrict offload devices."},
		{ID: "list-devices", Runtime: "llamacpp", Backends: gpuBackends, Long: "--list-devices", Kind: "bool", DefaultText: "off", Description: "Print visible devices and exit."},
		{ID: "main-gpu", Runtime: "llamacpp", Backends: gpuBackends, Short: "-mg", Long: "--main-gpu", Kind: "int", DefaultText: "0", Description: "Primary GPU index."},
		{ID: "split-mode", Runtime: "llamacpp", Backends: gpuBackends, Short: "-sm", Long: "--split-mode", Kind: "enum", DefaultText: "layer", Enums: []string{"none", "layer", "row", "tensor"}, Description: "Multi-GPU split mode."},
		{ID: "tensor-split", Runtime: "llamacpp", Backends: gpuBackends, Short: "-ts", Long: "--tensor-split", Kind: "string", DefaultText: "none", Description: "Comma-separated split proportions."},
		{ID: "no-host", Runtime: "llamacpp", Backends: gpuBackends, Long: "--no-host", Kind: "bool", DefaultText: "off", Description: "Bypass host buffer."},
		{ID: "op-offload", Runtime: "llamacpp", Backends: gpuBackends, Long: "--op-offload", Kind: "bool", DefaultText: "on", Description: "Host operation offload."},
		{ID: "no-op-offload", Runtime: "llamacpp", Backends: gpuBackends, Long: "--no-op-offload", Kind: "bool", DefaultText: "off", Description: "Disable host operation offload."},
		{ID: "cpu-moe", Runtime: "llamacpp", Backends: gpuBackends, Short: "-cmoe", Long: "--cpu-moe", Kind: "bool", DefaultText: "off", Description: "Keep all MoE weights on CPU."},
		{ID: "n-cpu-moe", Runtime: "llamacpp", Backends: gpuBackends, Short: "-ncmoe", Long: "--n-cpu-moe", Kind: "int", DefaultText: "0", Description: "Keep first N MoE layers on CPU."},
		{ID: "spec-type", Runtime: "llamacpp", Long: "--spec-type", Kind: "enum", DefaultText: "none", Enums: []string{"none", "draft-simple", "draft-mtp", "ngram-cache", "ngram-simple", "ngram-map-k", "ngram-map-k4v", "ngram-mod"}, Description: "Speculative decoding type."},
		{ID: "spec-default", Runtime: "llamacpp", Long: "--spec-default", Kind: "bool", DefaultText: "off", Description: "Use default speculative decoding config."},
		{ID: "spec-draft-n-max", Runtime: "llamacpp", Long: "--spec-draft-n-max", Kind: "int", DefaultText: "3", Description: "Max draft tokens."},
		{ID: "spec-draft-n-min", Runtime: "llamacpp", Long: "--spec-draft-n-min", Kind: "int", DefaultText: "0", Description: "Minimum draft tokens."},
		{ID: "model-draft", Runtime: "llamacpp", Short: "-md", Long: "--model-draft", Kind: "string", DefaultText: "none", Description: "Local draft model path."},
		{ID: "spec-draft-hf", Runtime: "llamacpp", Long: "--spec-draft-hf", Kind: "string", DefaultText: "none", Description: "Hugging Face draft model repo."},
		{ID: "gpu-layers-draft", Runtime: "llamacpp", Backends: gpuBackends, Short: "-ngld", Long: "--gpu-layers-draft", Kind: "string", DefaultText: "auto", Description: "Draft model GPU layers."},
		{ID: "device-draft", Runtime: "llamacpp", Backends: gpuBackends, Short: "-devd", Long: "--device-draft", Kind: "string", DefaultText: "all", Description: "Draft model offload devices."},
		{ID: "cache-type-k-draft", Runtime: "llamacpp", Short: "-ctkd", Long: "--cache-type-k-draft", Kind: "enum", DefaultText: "f16", Enums: kvEnums, Description: "Draft K cache type."},
		{ID: "cache-type-v-draft", Runtime: "llamacpp", Short: "-ctvd", Long: "--cache-type-v-draft", Kind: "enum", DefaultText: "f16", Enums: kvEnums, Description: "Draft V cache type."},
		{ID: "spec-ngram-mod-n-match", Runtime: "llamacpp", Long: "--spec-ngram-mod-n-match", Kind: "int", DefaultText: "24", Description: "ngram-mod lookup length."},
		{ID: "spec-ngram-mod-n-min", Runtime: "llamacpp", Long: "--spec-ngram-mod-n-min", Kind: "int", DefaultText: "48", Description: "ngram-mod minimum tokens."},
		{ID: "spec-ngram-mod-n-max", Runtime: "llamacpp", Long: "--spec-ngram-mod-n-max", Kind: "int", DefaultText: "64", Description: "ngram-mod maximum tokens."},
		{ID: "embedding", Runtime: "llamacpp", Roles: []string{"embedding", "reranking"}, Long: "--embedding", Kind: "bool", DefaultText: "role default", Description: "Enable embedding mode."},
		{ID: "pooling", Runtime: "llamacpp", Roles: []string{"embedding", "reranking"}, Long: "--pooling", Kind: "enum", DefaultText: "role default", Enums: []string{"none", "mean", "cls", "last", "rank"}, Description: "Embedding pooling mode."},
		{ID: "reranking", Runtime: "llamacpp", Roles: []string{"reranking"}, Long: "--reranking", Kind: "bool", DefaultText: "role default", Description: "Enable reranking endpoint."},
		{ID: "embd-normalize", Runtime: "llamacpp", Long: "--embd-normalize", Kind: "int", DefaultText: "2", Description: "Embedding normalization mode."},
		{ID: "reasoning", Runtime: "llamacpp", Short: "-rea", Long: "--reasoning", Kind: "enum", DefaultText: "auto", Enums: []string{"on", "off", "auto"}, Description: "Reasoning mode."},
		{ID: "reasoning-budget", Runtime: "llamacpp", Long: "--reasoning-budget", Kind: "int", DefaultText: "-1", Description: "Thinking token budget."},
		{ID: "timeout", Runtime: "llamacpp", Short: "-to", Long: "--timeout", Kind: "int", DefaultText: "3600", Description: "Server read/write timeout seconds."},
	}
}

func wizardOptionIDsFor(runtimeName string, backend string, role string) []string {
	options := applicableWizardOptions(runtimeName, backend, role)
	ids := make([]string, 0, len(options))
	for _, option := range options {
		ids = append(ids, option.ID)
	}
	return ids
}

func wizardOptionByID(id string) (wizardCLIOption, bool) {
	for _, option := range wizardOptionCatalog() {
		if option.ID == id {
			return option, true
		}
	}
	return wizardCLIOption{}, false
}

func wizardOptionSamples(option wizardCLIOption) []string {
	if len(option.Enums) > 0 {
		return option.Enums
	}
	switch option.Kind {
	case "bool":
		return []string{"on", "off"}
	case "int":
		if strings.TrimSpace(option.DefaultText) != "" && option.DefaultText != "model default" && option.DefaultText != "role default" {
			return []string{option.DefaultText}
		}
	}
	return nil
}

func applicableWizardOptions(runtimeName string, backend string, role string) []wizardCLIOption {
	var options []wizardCLIOption
	for _, option := range wizardOptionCatalog() {
		if option.Runtime != runtimeName {
			continue
		}
		if len(option.Backends) > 0 && !containsString(option.Backends, backend) {
			continue
		}
		if len(option.Roles) > 0 && !containsString(option.Roles, role) {
			continue
		}
		if runtimeName == "llamacpp" && backend == "sycl" {
			option.Description = strings.TrimSpace(option.Description) + " SYCL depends on Intel GPU drivers and oneAPI runtime; shared memory pressure matters, and upstream generally recommends FP16 SYCL builds."
		}
		options = append(options, option)
	}
	return options
}

func (m Model) wizardOptionBar(
	label string,
	options []wizardOption,
	background string,
	header string,
	button string,
	selected string,
) string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(header)).
		Background(lipgloss.Color(background))

	topParts := []string{headerStyle.Render(strings.Repeat(" ", wizardOptionLabelWidth))}
	middleParts := []string{headerStyle.Render(centeredText(label, wizardOptionLabelWidth))}
	bottomParts := []string{headerStyle.Render(strings.Repeat(" ", wizardOptionLabelWidth))}
	for _, option := range options {
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color(button)).
			Background(lipgloss.Color(background))
		if option.selected {
			style = style.
				Bold(true).
				Foreground(lipgloss.Color("16")).
				Background(lipgloss.Color(selected))
		}
		topParts = append(topParts, style.Render(strings.Repeat(" ", wizardOptionBoxWidth)))
		middleParts = append(middleParts, style.Render(centeredText(selectedToken(option.label, option.selected), wizardOptionBoxWidth)))
		bottomParts = append(bottomParts, style.Render(strings.Repeat(" ", wizardOptionBoxWidth)))
	}

	rowStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(button)).
		Background(lipgloss.Color(background))
	return strings.Join([]string{
		m.fullWidthWizardLine(rowStyle, strings.Join(topParts, " ")),
		m.fullWidthWizardLine(rowStyle, strings.Join(middleParts, " ")),
		m.fullWidthWizardLine(rowStyle, strings.Join(bottomParts, " ")),
	}, "\n")
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
	return panelBodyWidth(width)
}

func (m Model) wizardVariantToggleLine() string {
	if m.wizardRuntime == "llamacpp" {
		backendOptions := m.llamaTypeOptions()
		options := make([]wizardOption, 0, len(backendOptions))
		for _, option := range backendOptions {
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
	backendOptions := m.litertBackendOptions()
	options := make([]wizardOption, 0, len(backendOptions))
	for _, option := range backendOptions {
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
		"POST /g0litellama/v1/runners",
		"WebSocket api.request POST /g0litellama/v1/runners",
	}
	if m.wizardRuntime == "llamacpp" && len(m.llamaRuntimeVariants) == 0 {
		lines = append(lines, "", "Install llama runtime folders under G0LiteLLaMa/llama-runtimes first.")
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
	return m.litertBackendOptions()
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
	status := m.chatStatusLine()
	settings := renderPanelSpec(panelSpec{"Prompt settings", m.chatSettingsLines(), "205"}, m.width)
	input := m.chatInputBoxView()
	top := joinChatPanels(status, settings)
	messageRows := m.chatTranscriptWindowSize(top, input)
	messages := m.chatMessagesBoxView(messageRows)
	panels := []string{status, settings, messages, input}
	if m.chatSettingsDropdown != "" || m.chatCommandPopup || m.chatSystemEditing {
		base := joinChatPanels(panels...)
		row := maxInt(0, m.chatPopupRow-m.contentTopRow())
		return overlayBlock(base, m.chatPopupView(), row, maxInt(0, m.chatPopupColumn))
	}
	return joinChatPanels(panels...)
}

func (m Model) chatStatusLine() string {
	route := runnerRoleRoute(m.chatTarget())
	modelID := "not configured"
	if runner, ok := m.selectedChatRunner(); ok {
		modelID = fallback(runner.ModelID, runner.ID)
	}
	return fmt.Sprintf("%s | %s | %s | %s", route, modelID, m.chatVisibleStatus(), m.chatThroughputText())
}

func (m Model) chatTranscriptWindowSize(top string, composer string) int {
	if m.height <= 0 {
		return 8
	}
	available := m.chatBodyTargetHeight()
	rows := available - viewLineCount(top) - viewLineCount(composer) - 2
	return maxInt(1, rows)
}

func (m Model) chatBodyTargetHeight() int {
	if m.managedScreen {
		return managedBodyHeight(m.height, m.managedTopView(), m.footerView())
	}
	prefix := viewLineCount(m.headerView()) + viewLineCount(m.tabBar()) + 3
	if strings.TrimSpace(m.notice) != "" {
		prefix += viewLineCount(noticeStyle.Render(m.notice)) + 2
	}
	return maxInt(0, m.height-prefix-viewLineCount(m.footerView()))
}

func (m Model) chatRunnerLines() []string {
	runner, ok := m.selectedChatRunner()
	if !ok {
		return []string{
			"Target " + strings.Join(m.chatTargetTokens(), " "),
			"Selected runner: none",
			"Create or start a " + m.chatTarget() + " runner from Launch Wizard.",
			formatKV("Thinking", boolOnOff(m.chatThinking)) + "    Settings [?]",
		}
	}

	return []string{
		"Target " + strings.Join(m.chatTargetTokens(), " "),
		formatKV("Selected runner", runner.ID),
		formatKV("Route", runnerRoleRoute(runner.Role)),
		formatKV("Model", fallback(runner.ModelID, "not configured")),
		formatKV("Thinking", boolOnOff(m.chatThinking)) + "    Settings [?]",
	}
}

func (m Model) chatTargetTokens() []string {
	roles := []string{"main", "embedding", "reranking"}
	tokens := make([]string, 0, len(roles))
	for _, role := range roles {
		tokens = append(tokens, selectedToken(role, role == m.chatTarget()))
	}
	return tokens
}

func (m Model) chatTargetLines() []string {
	lines := make([]string, 0, 3)
	for index, role := range []string{"main", "embedding", "reranking"} {
		marker := " "
		if role == m.chatTarget() {
			marker = "●"
		}
		lines = append(lines, fmt.Sprintf("%s %d %s", marker, index+1, role))
	}
	return lines
}

func (m Model) chatComposerTitle() string {
	return "Input"
}

func (m Model) chatComposerLines() []string {
	value := m.chatDraft
	mode := "normal"
	if m.chatSystemEditing {
		value = m.chatSystemField.Value()
		mode = "system prompt"
	}
	state := "ready"
	if m.chatBusy {
		state = "waiting for response"
	}
	if value == "" {
		value = "(empty)"
	}
	return []string{
		formatKV("Mode", mode),
		formatKV("Text", value),
		formatKV("State", state),
		"@ toggles system/input; Enter sends or saves system.",
	}
}

func (m Model) chatMessageLines() []string {
	if len(m.chatMessages) == 0 {
		return nil
	}

	fullWidth := panelBodyWidth(m.width) - 1 // content before scrollbar
	lines := make([]string, 0, len(m.chatMessages))
	for _, message := range m.chatMessages {
		for _, line := range strings.Split(message.content, "\n") {
			var rendered string
			switch message.role {
			case "user":
				rendered = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("53")).Padding(0, 1).Render(line)
				// Right-align user messages: prepend styled spaces.
				cw := lipgloss.Width(rendered)
				if cw < fullWidth {
					pad := lipgloss.NewStyle().Background(lipgloss.Color("53")).Render(strings.Repeat(" ", fullWidth-cw))
					rendered = pad + rendered
				}
			case "assistant":
				rendered = lipgloss.NewStyle().Foreground(lipgloss.Color("219")).Background(lipgloss.Color("24")).Padding(0, 1).Render(line)
				// Fill remaining width with styled spaces matching bg.
				cw := lipgloss.Width(rendered)
				if cw < fullWidth {
					pad := lipgloss.NewStyle().Background(lipgloss.Color("24")).Render(strings.Repeat(" ", fullWidth-cw))
					rendered += pad
				}
			case "error":
				rendered = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("160")).Padding(0, 1).Render(line)
				cw := lipgloss.Width(rendered)
				if cw < fullWidth {
					pad := lipgloss.NewStyle().Background(lipgloss.Color("160")).Render(strings.Repeat(" ", fullWidth-cw))
					rendered += pad
				}
			default:
				rendered = lipgloss.NewStyle().Foreground(lipgloss.Color("219")).Render(line)
			}
			lines = append(lines, rendered)
		}
	}
	return lines
}

func (m Model) chatParityLines() []string {
	lines := []string{
		"TUI Enter -> POST " + m.chatEndpoint,
		"/v1/chat/completions -> main route",
		"/v1/embeddings -> embedding route",
		"/v1/rerank -> reranking route",
	}
	if runner, ok := m.selectedChatRunner(); ok {
		lines = append(
			lines,
			"Selected runner: "+runner.ID,
			"Route: "+m.chatTarget()+" -> "+runner.ID,
		)
	}
	return lines
}

func (m Model) chatSettingsLines() []string {
	systemPrompt := strings.TrimSpace(m.chatSystemField.Value())
	if systemPrompt == "" {
		systemPrompt = "empty"
	} else if len(systemPrompt) > 80 {
		systemPrompt = systemPrompt[:77] + "..."
	}
	target := m.chatTarget()
	return []string{
		strings.Join([]string{
			"Thinking: " + boolOnOff(m.chatThinking),
			"Target: " + target,
			"System: " + systemPrompt,
			"Temperature: " + m.chatTemperature,
			"Top P: " + m.chatTopP,
			"Max Tokens: " + m.chatMaxTokens,
			"Stream: " + boolOnOff(m.chatStream),
		}, "  "),
	}
}

func (m Model) chatMessagesBoxView(rows int) string {
	box := m.chatScrollBox
	box.ViewLines = maxInt(1, rows)
	box.SetLines(m.chatMessageLines())
	if len(box.Lines) == 0 {
		empty := []string{mutedStyle.Render("No messages yet.")}
		for len(empty) < rows {
			empty = append(empty, "")
		}
		return renderBox(empty, "45", m.width)
	}
	// Build a box where the scrollbar sits flush against the right border.
	// Line: │ <content> \x1b[48;5;24m<scrollbar2>\x1b[0m│
	// (left border + 1 space + content + 2-char scrollbar + right border)
	boxW := panelInnerWidth(m.width) // total box width incl borders
	contentW := panelBodyWidth(m.width) - 1 // text before scrollbar = m.width - 9
	scrollContent := box.View(contentW)
	lines := strings.Split(scrollContent, "\n")

	borderStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("45")).Width(boxW)

	// Top border: ╭──────────╮ (top + left + right)
	topBorder := borderStyle.Border(lipgloss.NormalBorder(), true, true, false, true).Render("")
	// Bottom border: ╰──────────╯ (left + bottom + right)
	bottomBorder := borderStyle.Border(lipgloss.NormalBorder(), false, true, true, true).Render("")

	var body strings.Builder
	for _, line := range lines {
		body.WriteString("│ ")
		body.WriteString(line)
		body.WriteString("│\n")
	}

	return topBorder + "\n" + strings.TrimRight(body.String(), "\n") + "\n" + bottomBorder
}

func (m Model) chatInputBoxView() string {
	placeholder := "Ready. Input your prompt"
	if m.chatBusy {
		placeholder = "Wait..."
	}
	value := m.chatDraft
	if strings.TrimSpace(value) == "" {
		value = mutedStyle.Render(placeholder)
	}
	lines := strings.Split(value, "\n")
	lines = append(lines, m.chatSendLabel())
	return renderBox(lines, "214", m.width)
}

func (m Model) chatSendLabel() string {
	if m.chatBusy {
		return mutedStyle.Render("Send")
	}
	return "Send"
}

func (m Model) chatPopupTitle() string {
	if m.chatCommandPopup {
		return "Commands"
	}
	if m.chatSystemEditing {
		return "System"
	}
	return "Menu"
}

func (m Model) chatPopupLines() []string {
	if m.chatCustomField != "" {
		value := m.chatCustomValue
		if value == "" {
			value = mutedStyle.Render("type value")
		}
		return []string{value, "Enter applies. Esc cancels."}
	}
	if m.chatCommandPopup {
		prefix := strings.TrimPrefix(m.chatDraft, "/")
		var lines []string
		for _, command := range chatCommandNames() {
			if strings.HasPrefix(strings.TrimPrefix(command, "/"), prefix) {
				lines = append(lines, command)
			}
		}
		if len(lines) == 0 {
			return []string{"No commands."}
		}
		return lines
	}
	if m.chatSystemEditing {
		value := strings.TrimSpace(m.chatSystemField.Value())
		if value == "" {
			return []string{mutedStyle.Render("empty"), "Enter saves. Shift+Enter newline."}
		}
		lines := strings.Split(value, "\n")
		return append(lines, "Enter saves. Shift+Enter newline.")
	}
	options := m.chatDropdownOptions()
	lines := make([]string, 0, len(options))
	for _, option := range options {
		lines = append(lines, option.label)
	}
	return lines
}

func (m Model) chatPopupView() string {
	return renderMenuPanel(m.chatPopupTitle(), m.chatPopupLines(), m.palette())
}

type chatDropdownOption struct {
	label string
	value string
}

func (m Model) chatDropdownOptions() []chatDropdownOption {
	switch m.chatSettingsDropdown {
	case "thinking":
		return []chatDropdownOption{{"on", "thinking:on"}, {"off", "thinking:off"}}
	case "target":
		return []chatDropdownOption{{"main", "target:main"}, {"embedding", "target:embedding"}, {"reranking", "target:reranking"}}
	case "runner":
		runners := m.runnersForRole(m.chatTarget())
		options := make([]chatDropdownOption, 0, len(runners))
		for _, runner := range runners {
			options = append(options, chatDropdownOption{runner.ID, "runner:" + runner.ID})
		}
		return options
	case "temp":
		return []chatDropdownOption{{"default", "temp:default"}, {"0.2", "temp:0.2"}, {"0.7", "temp:0.7"}, {"1.0", "temp:1.0"}, {"custom...", "temp:custom"}}
	case "top-p":
		return []chatDropdownOption{{"default", "top-p:default"}, {"0.8", "top-p:0.8"}, {"0.9", "top-p:0.9"}, {"1.0", "top-p:1.0"}, {"custom...", "top-p:custom"}}
	case "max":
		return []chatDropdownOption{{"default", "max:default"}, {"256", "max:256"}, {"1024", "max:1024"}, {"4096", "max:4096"}, {"custom...", "max:custom"}}
	case "stream":
		return []chatDropdownOption{{"on", "stream:on"}, {"off", "stream:off"}}
	default:
		return nil
	}
}

func (m *Model) openChatSetting(field string, row int, column int) {
	switch field {
	case "temperature":
		field = "temp"
	case "max-tokens":
		field = "max"
	}
	m.chatCommandPopup = false
	m.chatTargetDropdown = false
	m.chatSettingsOpen = false
	m.chatCustomField = ""
	m.chatCustomValue = ""
	m.chatPopupRow = row + 1
	m.chatPopupColumn = column
	if field == "system" {
		m.chatSystemEditing = true
		m.chatSettingsDropdown = ""
		popupWidth := maxInt(40, m.width-m.chatPopupColumn-4)
		existing := m.chatSystemField.Value()
		m.chatSystemField = NewTextAreaField(popupWidth, 8)
		m.chatSystemField.SetValue(existing)
		m.chatSystemField.Focus()
		return
	}
	m.chatSystemEditing = false
	m.chatSettingsDropdown = field
}

func (m *Model) applyChatDropdownOption(payload string) {
	field, value, ok := strings.Cut(payload, ":")
	if !ok {
		return
	}
	switch field {
	case "thinking":
		m.chatThinking = value == "on"
	case "target":
		if isWizardRole(value) {
			m.chatTargetRole = value
			m.chatRunnerID = ""
			if runner, ok := m.defaultChatRunnerForRole(value); ok {
				m.chatRunnerID = runner.ID
			}
		}
	case "runner":
		if runner, ok := m.runnerByID(value); ok && runner.Role == m.chatTarget() {
			m.chatRunnerID = value
		}
	case "temp":
		if value == "custom" {
			m.chatCustomField = "temp"
			m.chatCustomValue = ""
			return
		}
		m.chatTemperature = value
	case "top-p":
		if value == "custom" {
			m.chatCustomField = "top-p"
			m.chatCustomValue = ""
			return
		}
		m.chatTopP = value
	case "max":
		if value == "custom" {
			m.chatCustomField = "max"
			m.chatCustomValue = ""
			return
		}
		m.chatMaxTokens = value
	case "stream":
		m.chatStream = value == "on"
	}
	m.chatSettingsDropdown = ""
}

func (m *Model) applyChatCustomValue() bool {
	value := strings.TrimSpace(m.chatCustomValue)
	if value == "" {
		return false
	}
	switch m.chatCustomField {
	case "temp":
		m.chatTemperature = value
	case "top-p":
		m.chatTopP = value
	case "max":
		m.chatMaxTokens = value
	default:
		return false
	}
	return true
}

func isChatNumericInput(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && char != '.' {
			return false
		}
	}
	return true
}

func (m Model) chatVisibleStatus() string {
	if strings.TrimSpace(m.chatStatus) == "" {
		return "idle"
	}
	return m.chatStatus
}

func (m Model) chatThroughputText() string {
	if m.chatTokensPerSecond <= 0 || m.chatVisibleStatus() == "idle" {
		return "0 tok/s"
	}
	return fmt.Sprintf("%.0f tok/s", m.chatTokensPerSecond)
}

func chatCommandNames() []string {
	return []string{"/clear", "/stop", "/new", "/settings"}
}

func (m Model) selectedChatCommand() string {
	prefix := strings.TrimPrefix(m.chatDraft, "/")
	for _, command := range chatCommandNames() {
		if strings.HasPrefix(strings.TrimPrefix(command, "/"), prefix) {
			return command
		}
	}
	return ""
}

func (m Model) runChatCommand(command string) (Model, tea.Cmd) {
	switch command {
	case "/clear":
		m.chatMessages = nil
	case "/stop":
		if m.chatCancel != nil {
			m.chatCancel()
			m.chatCancel = nil
		}
		m.chatBusy = false
		m.chatStatus = "idle"
	case "/new":
		m.chatDraft = ""
		m.chatMessages = nil
		if m.store != nil {
			m.store.Dispatch(m.ctx, store.ActionEnvelope{
				Type:   store.ActionTypeNewChatSession,
				Source: store.SourceTUI,
			})
		}
	case "/settings":
		m.chatSettingsDropdown = "thinking"
	}
	m.chatCommandPopup = false
	return m, nil
}

func (m Model) submitChatPrompt() (Model, tea.Cmd) {
	prompt := strings.TrimSpace(m.chatDraft)
	if prompt == "" || m.chatBusy {
		return m, nil
	}
	runner, ok := m.selectedChatRunner()
	if !ok {
		m.chatStatus = "error"
		m.notice = "chat prompt failed: no main runner configured"
		return m, nil
	}
	m.chatRunnerID = runner.ID
	m.chatDraft = ""
	m.chatPendingDraft = prompt
	m.chatCommandPopup = false
	m.chatBusy = true
	m.chatStatus = "processing"
	m.chatTokensPerSecond = 0
	m.chatStreamStarted = time.Now()
	m.chatAssistantIndex = -1
	m.chatMessages = append(m.chatMessages, chatMessage{
		role:    "user",
		content: prompt,
	})
	ctx, cancel := context.WithCancel(m.ctx)
	m.chatCancel = cancel
	return m, m.chatCompletionCmd(ctx, prompt, runner)
}

// chatScrollViewLines computes the number of visible rows available for the
// chat transcript ScrollBox based on the current model state and window size.
func (m Model) chatScrollViewLines() int {
	if m.height <= 0 {
		return 8
	}
	available := m.chatBodyTargetHeight()
	// Top = status line (1) + settings box (4 rows with border)
	// Input = input box (4 rows with border)
	// Subtract 2 for gap padding
	return maxInt(1, available-1-4-4-2)
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
		nextAction = "d Download " + entry.ID + " via POST /g0litellama/v1/models/download"
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
		"POST /g0litellama/v1/runners",
		"",
		"Download models",
		"d Download next missing required model",
		"Catalog.Download",
		"POST /g0litellama/v1/models/download",
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
	)
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
		case "close":
			_, err = m.runnerController.CloseRunner(ctx, id)
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

func (m Model) chatCompletionCmd(ctx context.Context, prompt string, runner server.RunnerSnapshot) tea.Cmd {
	endpoint := m.chatEndpoint
	modelID := fallback(runner.ModelID, runner.ID)
	systemPrompt := strings.TrimSpace(m.chatSystemField.Value())
	stream := m.chatStream

	return func() tea.Msg {
		if stream {
			chunks := make(chan chatStreamChunkMsg, 16)
			go streamChatCompletion(ctx, endpoint, modelID, systemPrompt, prompt, chunks)
			return chatStreamStartMsg{chunks: chunks}
		}
		response, err := postChatCompletion(ctx, endpoint, modelID, systemPrompt, prompt)
		return chatCompletionMsg{
			prompt:   prompt,
			response: response,
			err:      err,
		}
	}
}

func waitForChatStreamChunk(chunks <-chan chatStreamChunkMsg) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-chunks
		if !ok {
			return chatStreamChunkMsg{done: true}
		}
		chunk.chunks = chunks
		return chunk
	}
}

func (m Model) applyChatStreamChunk(msg chatStreamChunkMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.chatBusy = false
		m.chatStatus = "error"
		m.chatCancel = nil
		if m.chatAssistantIndex >= 0 && m.chatAssistantIndex < len(m.chatMessages) {
			m.chatMessages[m.chatAssistantIndex].content += "\n[error: " + msg.err.Error() + "]"
		} else {
			m.chatDraft = m.chatPendingDraft
			m.chatMessages = append(m.chatMessages, chatMessage{role: "error", content: msg.err.Error()})
		}
		m.chatPendingDraft = ""
		m.notice = fmt.Sprintf("chat prompt failed: %v", msg.err)
		return m, nil
	}
	if msg.done {
		m.chatBusy = false
		m.chatStatus = "idle"
		m.chatTokensPerSecond = 0
		m.chatAssistantIndex = -1
		m.chatCancel = nil
		m.chatPendingDraft = ""
		m.notice = "sent chat prompt through /v1/chat/completions"
		return m, nil
	}
	if msg.content == "" {
		if msg.chunks == nil {
			return m, nil
		}
		return m, waitForChatStreamChunk(msg.chunks)
	}
	m.chatStatus = "responding"
	if m.chatAssistantIndex < 0 || m.chatAssistantIndex >= len(m.chatMessages) {
		m.chatMessages = append(m.chatMessages, chatMessage{role: "assistant"})
		m.chatAssistantIndex = len(m.chatMessages) - 1
	}
	m.chatMessages[m.chatAssistantIndex].content += msg.content
	elapsed := time.Since(m.chatStreamStarted).Seconds()
	if elapsed > 0 {
		m.chatTokensPerSecond = float64(len(strings.Fields(m.chatMessages[m.chatAssistantIndex].content))) / elapsed
	}
	// Update ScrollBox with new content. Pinned mode auto-follows.
	m.chatScrollBox.SetLines(m.chatMessageLines())
	if msg.chunks == nil {
		return m, nil
	}
	return m, waitForChatStreamChunk(msg.chunks)
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

func (m Model) dashboardRouteCmd(role string, runner server.RunnerSnapshot) tea.Cmd {
	return func() tea.Msg {
		if m.runnerController == nil {
			return dashboardRouteMsg{role: role, runner: runner, err: fmt.Errorf("runner controller is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()

		updated, err := m.runnerController.RouteRunner(ctx, role, runner.ID)
		if updated.ID == "" {
			updated = runner
		}
		return dashboardRouteMsg{role: role, runner: updated, err: err}
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
	case "close":
		return "closed " + message.id
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
	case "commandLine":
		return server.RunnerPatch{CommandLine: &value}, value, nil
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

type chatStreamResponse struct {
	Choices []struct {
		Delta chatAPIItem `json:"delta"`
	} `json:"choices"`
}

func streamChatCompletion(
	ctx context.Context,
	endpoint string,
	modelID string,
	systemPrompt string,
	prompt string,
	chunks chan<- chatStreamChunkMsg,
) {
	defer close(chunks)
	messages := []chatAPIItem{}
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, chatAPIItem{
			Role:    "system",
			Content: strings.TrimSpace(systemPrompt),
		})
	}
	messages = append(messages, chatAPIItem{
		Role:    "user",
		Content: prompt,
	})
	payload := chatCompletionRequest{
		Model:    modelID,
		Messages: messages,
		Stream:   true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		chunks <- chatStreamChunkMsg{err: fmt.Errorf("encode chat request: %w", err)}
		return
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		chunks <- chatStreamChunkMsg{err: fmt.Errorf("create chat request: %w", err)}
		return
	}
	request.Header.Set("content-type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		chunks <- chatStreamChunkMsg{err: fmt.Errorf("send chat request: %w", err)}
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			chunks <- chatStreamChunkMsg{err: fmt.Errorf("read chat response: %w", readErr)}
			return
		}
		chunks <- chatStreamChunkMsg{err: fmt.Errorf("chat response %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))}
		return
	}

	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			chunks <- chatStreamChunkMsg{done: true}
			return
		}
		var decoded chatStreamResponse
		if err := json.Unmarshal([]byte(data), &decoded); err != nil {
			chunks <- chatStreamChunkMsg{err: fmt.Errorf("decode chat stream: %w", err)}
			return
		}
		for _, choice := range decoded.Choices {
			if choice.Delta.Content != "" {
				chunks <- chatStreamChunkMsg{content: choice.Delta.Content}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		chunks <- chatStreamChunkMsg{err: fmt.Errorf("read chat stream: %w", err)}
		return
	}
	chunks <- chatStreamChunkMsg{done: true}
}

func postChatCompletion(
	ctx context.Context,
	endpoint string,
	modelID string,
	systemPrompt string,
	prompt string,
) (string, error) {
	messages := []chatAPIItem{}
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, chatAPIItem{
			Role:    "system",
			Content: strings.TrimSpace(systemPrompt),
		})
	}
	messages = append(messages, chatAPIItem{
		Role:    "user",
		Content: prompt,
	})
	payload := chatCompletionRequest{
		Model:    modelID,
		Messages: messages,
		Stream:   false,
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
	m.wizardOptionPage = 0
	m.normalizeWizardSelection()
	m.dispatchWizardState()
}

func (m *Model) cycleWizardVariant() {
	if m.wizardRuntime == "llamacpp" {
		options := m.llamaTypeOptions()
		if len(options) == 0 {
			return
		}
		current := 0
		for index, option := range options {
			if option == m.wizardBackend {
				current = index
				break
			}
		}
		m.wizardBackend = options[(current+1)%len(options)]
		m.wizardOptionPage = 0
		m.normalizeWizardSelection()
		m.dispatchWizardState()
		return
	}

	options := m.litertBackendOptions()
	if len(options) == 0 {
		return
	}
	current := 0
	for index, option := range options {
		if option == m.wizardBackend {
			current = index
			break
		}
	}
	m.wizardBackend = options[(current+1)%len(options)]
	m.wizardOptionPage = 0
	m.normalizeWizardSelection()
	m.dispatchWizardState()
}

func (m *Model) dispatchWizardState() {
	if m.store == nil {
		return
	}
	ctx := context.Background()
	m.store.Dispatch(ctx, store.ActionEnvelope{
		Type:    store.ActionTypeWizard,
		Source:  store.SourceTUI,
		Payload: store.MustPayload(store.WizardStatePayload{
			Runtime:         m.wizardRuntime,
			Backend:         m.wizardBackend,
			Role:            m.wizardRole,
			OptionOverrides: m.wizardOptionOverrides,
		}),
	})
}

func (m *Model) setWizardRole(role string) {
	m.wizardRole = role
	m.wizardModelSelection = 0
	m.wizardOptionPage = 0
	m.normalizeWizardSelection()
	m.dispatchWizardState()
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
		options := m.llamaTypeOptions()
		if len(options) == 0 {
			return
		}
		index := clampInt((x-17)/8, 0, len(options)-1)
		m.wizardBackend = options[index]
		m.wizardOptionPage = 0
		m.normalizeWizardSelection()
		m.dispatchWizardState()
		return
	}
	options := m.litertBackendOptions()
	if len(options) == 0 {
		return
	}
	index := clampInt((x-22)/6, 0, len(options)-1)
	m.wizardBackend = options[index]
	m.wizardOptionPage = 0
	m.normalizeWizardSelection()
	m.dispatchWizardState()
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
	if m.wizardRuntime == "litert" && !containsString(m.litertBackendOptions(), m.wizardBackend) {
		m.wizardBackend = firstString(m.litertBackendOptions(), "cpu")
	}
	if m.wizardRuntime == "llamacpp" && !containsString(m.llamaTypeOptions(), m.wizardBackend) {
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
		backend := variant.Backend
		if backend == "" {
			backend = llamaRuntimeType(variant.Name)
		}
		if backend == m.wizardBackend {
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
				"no installed llama runtime variants under G0LiteLLaMa/llama-runtimes",
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
	m.applyWizardOptionSpecOverrides(&spec)
	m.deconflictWizardPort(&spec)
	spec.Command = m.wizardCommandArgv(spec)
	return spec, entry, nil
}

func (m Model) applyWizardOptionSpecOverrides(spec *server.RunnerSpec) {
	if value, ok := m.wizardOptionOverrides["host"]; ok && strings.TrimSpace(value) != "" {
		spec.Host = strings.TrimSpace(value)
	}
	if value, ok := m.wizardOptionOverrides["port"]; ok && strings.TrimSpace(value) != "" {
		if port, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && port > 0 {
			spec.Port = port
		}
	}
	if _, ok := m.wizardOptionOverrides["verbose"]; ok {
		spec.Verbose = true
	}
	spec.Upstream = fmt.Sprintf("http://%s:%d", fallback(spec.Host, "127.0.0.1"), spec.Port)
}

func (m Model) deconflictWizardPort(spec *server.RunnerSpec) {
	spec.Port = m.nextAvailableWizardPort(spec.Port)
	spec.Upstream = fmt.Sprintf("http://%s:%d", fallback(spec.Host, "127.0.0.1"), spec.Port)
}

func (m Model) nextAvailableWizardPort(port int) int {
	if port <= 0 {
		port = defaultPortForRole(m.wizardRole)
	}
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

func (m Model) wizardCommandArgv(spec server.RunnerSpec) []string {
	var args []string
	switch spec.Runtime {
	case "litert":
		args = []string{
			fallback(spec.Executable, "litert-lm"),
			"serve",
			"--host",
			fallback(spec.Host, "127.0.0.1"),
			"--port",
			strconv.Itoa(spec.Port),
		}
	case "llamacpp":
		args = []string{
			fallback(spec.Executable, "llama-server"),
			"-m",
			spec.ModelPath,
			"--alias",
			fallback(spec.ModelID, spec.ID),
			"--host",
			fallback(spec.Host, "127.0.0.1"),
			"--port",
			strconv.Itoa(spec.Port),
		}
		if usesWizardGPUBackend(spec.Backend) && !m.hasWizardOverride("gpu-layers") && !m.hasWizardRemovedDefault("gpu-layers") {
			args = append(args, "--n-gpu-layers", "999")
		}
		switch spec.Role {
		case "embedding":
			if !m.hasWizardOverride("embedding") && !m.hasWizardRemovedDefault("embedding") {
				args = append(args, "--embedding")
			}
		case "reranking":
			if !m.hasWizardOverride("embedding") && !m.hasWizardRemovedDefault("embedding") {
				args = append(args, "--embedding")
			}
			if !m.hasWizardOverride("pooling") && !m.hasWizardRemovedDefault("pooling") {
				args = append(args, "--pooling", "rank")
			}
			if !m.hasWizardOverride("reranking") && !m.hasWizardRemovedDefault("reranking") {
				args = append(args, "--reranking")
			}
		}
	default:
		return nil
	}
	overrides := m.wizardOptionOverrides
	if _, ok := overrides["port"]; ok {
		overrides = copyStringMap(overrides)
		overrides["port"] = strconv.Itoa(spec.Port)
	}
	args = appendWizardOptionOverrides(args, spec.Runtime, spec.Backend, spec.Role, overrides)
	return m.appendWizardCommandExtras(args)
}

func (m Model) hasWizardOverride(id string) bool {
	_, ok := m.wizardOptionOverrides[id]
	return ok
}

func (m Model) hasWizardRemovedDefault(id string) bool {
	return m.wizardRemovedDefaults != nil && m.wizardRemovedDefaults[id]
}

func (m Model) appendWizardCommandExtras(args []string) []string {
	for _, extra := range m.wizardCommandExtras {
		value, ok := m.wizardOptionOverrides[extra.Option.ID]
		if !ok {
			continue
		}
		flag := optionLabel(extra.Option)
		if strings.TrimSpace(value) == "" {
			args = append(args, flag)
		} else {
			args = append(args, flag, strings.TrimSpace(value))
		}
	}
	return args
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

func (m Model) litertBackendOptions() []string {
	return m.visibleBackendOptions("litert", litertBackendOptions())
}

func llamaTypeOptions() []string {
	return []string{"cpu", "gpu", "metal", "openvino", "cuda13", "cuda12", "sycl"}
}

func (m Model) llamaTypeOptions() []string {
	return m.visibleBackendOptions("llamacpp", m.availableLlamaTypeOptions())
}

func (m Model) availableLlamaTypeOptions() []string {
	available := map[string]bool{}
	for _, variant := range m.llamaRuntimeVariants {
		backend := variant.Backend
		if backend == "" {
			backend = llamaRuntimeType(variant.Name)
		}
		available[backend] = true
	}

	options := []string{}
	for _, option := range llamaTypeOptions() {
		if available[option] {
			options = append(options, option)
		}
	}
	return options
}

func (m Model) visibleBackendOptions(runtimeName string, options []string) []string {
	visible := make([]string, 0, len(options))
	for _, option := range options {
		if m.backendStatus.Visible(runtimeName, option) {
			visible = append(visible, option)
		}
	}
	return visible
}

func (m Model) filterLlamaRuntimeVariants(variants []llamaRuntimeVariant) []llamaRuntimeVariant {
	filtered := make([]llamaRuntimeVariant, 0, len(variants))
	for _, variant := range variants {
		backend := variant.Backend
		if backend == "" {
			backend = llamaRuntimeType(variant.Name)
		}
		if m.backendStatus.Visible("llamacpp", backend) {
			filtered = append(filtered, variant)
		}
	}
	return filtered
}

func (m Model) firstAvailableLlamaType() string {
	for _, backend := range m.availableLlamaTypeOptions() {
		if m.backendStatus.Visible("llamacpp", backend) {
			return backend
		}
	}
	return firstString(m.llamaTypeOptions(), "cpu")
}

func llamaRuntimeType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "cuda-13") || strings.Contains(lower, "cuda13") || strings.Contains(lower, "cuda-13."):
		return "cuda13"
	case strings.Contains(lower, "cuda-12") || strings.Contains(lower, "cuda12") || strings.Contains(lower, "cuda-12."):
		return "cuda12"
	case strings.Contains(lower, "macos"):
		return "metal"
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

func firstString(values []string, fallbackValue string) string {
	if len(values) == 0 {
		return fallbackValue
	}
	return values[0]
}

func loadBackendStatus(path string) runtimeconfig.Status {
	status, err := runtimeconfig.Load(path)
	if err != nil {
		return runtimeconfig.Status{}
	}
	return status
}

func resolveBackendConfigPath(path string) string {
	if strings.TrimSpace(path) != "" {
		return path
	}
	if envPath := os.Getenv("RUNTIME_BACKEND_CONFIG"); strings.TrimSpace(envPath) != "" {
		return envPath
	}
	if repoRoot := findRepoRoot(); repoRoot != "" {
		return runtimeconfig.DefaultPath(repoRoot)
	}
	return filepath.FromSlash("G0LiteLLaMa/runtime-config/backends.json")
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
			Backend:    llamaRuntimeType(entry.Name()),
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
		return filepath.Join(repoRoot, "G0LiteLLaMa", "llama-runtimes")
	}
	return filepath.Join("G0LiteLLaMa", "llama-runtimes")
}

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if stat, err := os.Stat(filepath.Join(dir, "G0LiteLLaMa", "llama-runtimes")); err == nil && stat.IsDir() {
			return dir
		}
		if stat, err := os.Stat(filepath.Join(dir, "G0LiteLLaMa", "go.mod")); err == nil && !stat.IsDir() {
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
	widePanelGridMinWidth   = 150
	panelGridColumnGap      = 2
	runnerTerminalLogLimit  = 10
	wizardCLIOptionPageSize = 8
	tabBoxWidth             = 18
	wizardOptionLabelWidth  = 13
	wizardOptionBoxWidth    = 12
	wizardCLICommandWidth   = 10
	wizardCLIValueWidth     = 15
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

func renderBox(lines []string, color string, width int) string {
	body := strings.Join(lines, "\n")
	if strings.TrimSpace(body) == "" {
		body = " "
	}
	background := panelBackgroundForAccent(color)
	style := lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(lipgloss.Color(color)).
		Background(lipgloss.Color(background)).
		Padding(0, 1)
	if width > style.GetHorizontalFrameSize() {
		style = style.Width(panelInnerWidth(width))
	}
	return style.Render(body)
}

func renderMenuPanel(title string, lines []string, palette tuiPalette) string {
	return renderMenuPanelWidth(title, lines, palette, 0)
}

func renderMenuPanelWidth(title string, lines []string, palette tuiPalette, width int) string {
	body := strings.Join(lines, "\n")
	if strings.TrimSpace(body) == "" {
		body = "No data."
	}
	style := lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(lipgloss.Color(palette.menu)).
		Background(lipgloss.Color(palette.footerBG)).
		Foreground(lipgloss.Color(palette.footerFG)).
		Padding(0, 1)
	if width > style.GetHorizontalFrameSize() {
		style = style.Width(panelInnerWidth(width))
	}
	return style.Render(subtitleStyle.Render(title) + "\n" + body)
}

func renderPanelWidth(title string, lines []string, color string, width int) string {
	body := strings.Join(lines, "\n")
	if strings.TrimSpace(body) == "" {
		body = "No data."
	}
	background := panelBackgroundForAccent(color)
	style := lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(lipgloss.Color(color)).
		Background(lipgloss.Color(background)).
		Padding(0, 1)
	if width > style.GetHorizontalFrameSize() {
		style = style.Width(panelInnerWidth(width))
	}
	return style.Render(subtitleStyle.Background(lipgloss.Color(background)).Render(title) + "\n" + body)
}

func panelInnerWidth(width int) int {
	if width <= 4 {
		return 0
	}
	return width - 4
}

func panelBodyWidth(width int) int {
	return panelInnerWidth(panelInnerWidth(width))
}

func panelBackgroundForAccent(color string) string {
	switch color {
	case "39", "45", "81", "82":
		return "24"
	case "214", "222", "229":
		return "58"
	case "196", "205", "213", "219":
		return "53"
	case "244":
		return "236"
	default:
		return "23"
	}
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

func centeredText(value string, width int) string {
	textWidth := lipgloss.Width(value)
	if width <= textWidth {
		return value
	}
	left := (width - textWidth) / 2
	right := width - textWidth - left
	return strings.Repeat(" ", left) + value + strings.Repeat(" ", right)
}

func padRight(value string, width int) string {
	textWidth := lipgloss.Width(value)
	if width <= textWidth {
		return truncateToWidth(value, width)
	}
	return value + strings.Repeat(" ", width-textWidth)
}

func singleLineText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func wrapWords(prefix string, words []string, width int) []string {
	if len(words) == 0 {
		return nil
	}
	if width <= 0 {
		return []string{prefix + " " + strings.Join(words, " ")}
	}
	lines := []string{}
	current := prefix
	for _, word := range words {
		next := current + " " + word
		if lipgloss.Width(next) > width && current != prefix {
			lines = append(lines, current)
			current = strings.Repeat(" ", lipgloss.Width(prefix)) + " " + word
			continue
		}
		current = next
	}
	return append(lines, current)
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

func joinChatPanels(panels ...string) string {
	visible := make([]string, 0, len(panels))
	for _, panel := range panels {
		if strings.TrimSpace(panel) == "" {
			continue
		}
		visible = append(visible, panel)
	}
	return strings.Join(visible, "\n")
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
	lineCount := viewLineCount(m.scrollableBodyView())
	if lineCount <= bodyHeight {
		return 0
	}
	return lineCount - bodyHeight
}

func (m Model) scrollStatusLine() string {
	bodyHeight := managedBodyHeight(m.height, m.managedTopView(), m.commandRailSizingView())
	totalLines := viewLineCount(m.scrollableBodyView())
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

func (m Model) scrollableBodyView() string {
	body := m.activeContentView()
	if m.wizardCommandEdit != nil {
		body = joinPanels(body, renderPanelSpec(m.wizardCommandEditSpec(), m.width))
	}
	return body
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

func pinFooterToBottom(body string, footer string, height int) string {
	if height <= 0 {
		return joinPanels(body, footer)
	}
	footerLines := strings.Split(footer, "\n")
	if len(footerLines) > height {
		return strings.Join(footerLines[len(footerLines)-height:], "\n")
	}
	bodyHeight := height - len(footerLines)
	bodyLines := []string{}
	if body != "" {
		bodyLines = strings.Split(body, "\n")
	}
	if len(bodyLines) > bodyHeight {
		return joinPanels(body, footer)
	}
	for len(bodyLines) < bodyHeight {
		bodyLines = append(bodyLines, "")
	}
	return strings.Join(append(bodyLines, footerLines...), "\n")
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

func strOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func mapOrDefault(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func boolOnOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
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
		return "use x/r for process control or edit b/p/h/i/m/e/u/f/C/l/v/t/o"
	case "starting":
		return "wait for health, then inspect logs if it stalls"
	case "created", "stopped", "exited":
		return "press s to start or edit b/p/h/i/m/e/u/f/C/l/v/t/o"
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
	return "managed by G0LiteLLaMa"
}

func runnerSpecLaunchMode(spec server.RunnerSpec) string {
	if !spec.Launch {
		return "external upstream"
	}
	return "managed by G0LiteLLaMa"
}

func runnerCommandLine(command []string) string {
	if len(command) == 0 {
		return ""
	}
	parts := make([]string, 0, len(command))
	for _, arg := range command {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		parts = append(parts, shellQuoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func renderedPointHitsText(view string, x int, y int, text string) bool {
	hit, ok := renderedTextHit(view, text, "", "")
	return ok && y == hit.row && x >= hit.start && x < hit.end
}

func renderedTextHit(view string, text string, action string, payload string) (buttonHit, bool) {
	for row, line := range strings.Split(view, "\n") {
		if hit, ok := textHitInLine(line, row, text, action, payload); ok {
			return hit, true
		}
	}
	return buttonHit{}, false
}

func renderedTextHitOnRow(view string, row int, text string, action string, payload string) (buttonHit, bool) {
	lines := strings.Split(view, "\n")
	if row < 0 || row >= len(lines) {
		return buttonHit{}, false
	}
	return textHitInLine(lines[row], row, text, action, payload)
}

func textHitInLine(line string, row int, text string, action string, payload string) (buttonHit, bool) {
	plain := ansi.Strip(line)
	start := strings.Index(plain, text)
	if start < 0 {
		return buttonHit{}, false
	}
	cellStart := ansi.StringWidth(plain[:start])
	cellEnd := cellStart + ansi.StringWidth(text)
	return buttonHit{
		action:  action,
		row:     row,
		start:   cellStart,
		end:     cellEnd,
		payload: payload,
	}, true
}

func renderedPointHitsToken(view string, x int, y int, text string) bool {
	hit, ok := renderedTokenHit(view, text, "", "")
	return ok && y == hit.row && x >= hit.start && x < hit.end
}

func renderedTokenHit(view string, text string, action string, payload string) (buttonHit, bool) {
	lines := strings.Split(view, "\n")
	for row, line := range lines {
		if hit, ok := tokenHitInLine(line, row, text, action, payload); ok {
			return hit, true
		}
	}
	return buttonHit{}, false
}

func renderedTokenHitOnRow(view string, row int, text string, action string, payload string) (buttonHit, bool) {
	lines := strings.Split(view, "\n")
	if row < 0 || row >= len(lines) {
		return buttonHit{}, false
	}
	return tokenHitInLine(lines[row], row, text, action, payload)
}

func tokenHitInLine(line string, row int, text string, action string, payload string) (buttonHit, bool) {
	plain := ansi.Strip(line)
	searchFrom := 0
	for {
		start := strings.Index(plain[searchFrom:], text)
		if start < 0 {
			return buttonHit{}, false
		}
		start += searchFrom
		end := start + len(text)
		beforeOK := start == 0 || plain[start-1] == ' '
		afterOK := end >= len(plain) || plain[end] == ' ' || plain[end] == '='
		if beforeOK && afterOK {
			cellStart := ansi.StringWidth(plain[:start])
			cellEnd := cellStart + ansi.StringWidth(text)
			return buttonHit{
				action:  action,
				row:     row,
				start:   cellStart,
				end:     cellEnd,
				payload: payload,
			}, true
		}
		searchFrom = end
	}
}

func lineNumberContainingText(view string, text string) int {
	for row, line := range strings.Split(view, "\n") {
		if strings.Contains(ansi.Strip(line), text) {
			return row
		}
	}
	return -1
}

func lastLineNumberContainingText(view string, text string) int {
	lines := strings.Split(view, "\n")
	for row := len(lines) - 1; row >= 0; row-- {
		if strings.Contains(ansi.Strip(lines[row]), text) {
			return row
		}
	}
	return -1
}

func shellQuoteArg(arg string) string {
	if !strings.ContainsAny(arg, " \t\n\r'\"\\") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func wizardCommandPreview(spec server.RunnerSpec) string {
	return runnerCommandLine(spec.Command)
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
