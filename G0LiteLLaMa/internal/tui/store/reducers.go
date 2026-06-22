package store

import (
	"encoding/json"
	"strings"
)

// Action types for the UI domain.
const (
	ActionTypeSelectTab  ActionType = "ui:select-tab"
	ActionTypeWizard     ActionType = "wizard:state"
)

// SelectTabPayload is the payload for a tab selection action.
type SelectTabPayload struct {
	TabID string `json:"tab_id"`
}

// WizardStatePayload carries the full wizard configuration snapshot.
type WizardStatePayload struct {
	Runtime         string            `json:"runtime,omitempty"`
	Backend         string            `json:"backend,omitempty"`
	Role            string            `json:"role,omitempty"`
	OptionOverrides map[string]string `json:"optionOverrides,omitempty"`
}

// EffectSpec represents scheduled side-effect work returned by a reducer.
// Reducers produce EffectSpec values but never execute them.
type EffectSpec struct {
	// Placeholder — expanded when effects are wired.
}

// Reducer transforms AppState in response to an ActionEnvelope.
// It must be deterministic and side-effect-free.
type Reducer func(AppState, ActionEnvelope) (AppState, []EffectSpec)

// RootReduce routes actions to domain reducers and manages cross-domain
// coordination. It always increments the state revision for accepted actions.
func RootReduce(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	state.Revision++

	switch action.Type {
	case ActionTypeSelectTab:
		return reduceSelectTab(state, action)
	case ActionTypeNewChatSession:
		return ReduceChat(state, action)
	case ActionTypeProxyRequestStart:
		return ReduceProxy(state, action)
	case ActionTypeProxyResponseChunk:
		return ReduceProxy(state, action)
	case ActionTypeProxyResponseEnd:
		return ReduceProxy(state, action)
	case ActionTypeProxyResponseError:
		return ReduceProxy(state, action)
	case ActionTypeWizard:
		return ReduceWizard(state, action)
	default:
		return state, nil
	}
}

func reduceSelectTab(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	var p SelectTabPayload
	if err := json.Unmarshal(action.Payload, &p); err != nil {
		return state, nil
	}
	state.UI.ActiveTab = p.TabID
	return state, nil
}

// ReduceUI handles UI-domain actions.
func ReduceUI(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	switch action.Type {
	case ActionTypeSelectTab:
		return reduceSelectTab(state, action)
	default:
		return state, nil
	}
}

// ReduceViewport is a pass-through reducer for the Viewport domain.
func ReduceViewport(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceRunners is a pass-through reducer for the Runners domain.
func ReduceRunners(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceModels is a pass-through reducer for the Models domain.
func ReduceModels(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceRuntime is a pass-through reducer for the Runtime domain.
func ReduceRuntime(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceChat handles Chat-domain actions and proxy observations that create
// or update chat sessions.
func ReduceChat(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	switch action.Type {
	case ActionTypeNewChatSession:
		return reduceChatNewSession(state, action)
	case ActionTypeProxyRequestStart:
		return reduceChatFromProxyRequestStart(state, action)
	default:
		return state, nil
	}
}

func reduceChatNewSession(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	sessionID := string(action.ID)
	if sessionID == "" {
		// Generate a reasonable session ID from the action ID; RootReduce always
		// assigns an ID before the reducer runs so this should not happen.
		return state, nil
	}
	if state.Chat.Sessions == nil {
		state.Chat.Sessions = make(map[string]ChatSession)
	}
	session := ChatSession{
		ID:        sessionID,
		Source:    SourceTUI,
		CreatedAt: action.Time,
		UpdatedAt: action.Time,
	}
	state.Chat.Sessions[sessionID] = session
	state.Chat.ActiveSessionID = sessionID
	return state, nil
}

// ReduceProxy handles proxy observation actions. Chat-relevant observations
// are forwarded to ReduceChat for session management.
func ReduceProxy(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	switch action.Type {
	case ActionTypeProxyRequestStart:
		return reduceProxyRequestStart(state, action)
	case ActionTypeProxyResponseChunk:
		return reduceProxyResponseChunk(state, action)
	case ActionTypeProxyResponseEnd:
		return reduceProxyResponseEnd(state, action)
	case ActionTypeProxyResponseError:
		return reduceProxyResponseError(state, action)
	default:
		return state, nil
	}
}

func reduceProxyRequestStart(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	var p ProxyRequestStartPayload
	if err := json.Unmarshal(action.Payload, &p); err != nil {
		return state, nil
	}
	// Route chat-completions request starts to the chat reducer for
	// auto-creating API sessions.
	if isChatPath(p.Path) {
		return ReduceChat(state, action)
	}
	return state, nil
}

func reduceProxyResponseChunk(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	// Store-level chunk tracking is pass-through in this slice. The action
	// remains in the event log for debugging and TUI observation.
	return state, nil
}

func reduceProxyResponseEnd(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

func reduceProxyResponseError(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

func reduceChatFromProxyRequestStart(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	var p ProxyRequestStartPayload
	if err := json.Unmarshal(action.Payload, &p); err != nil {
		return state, nil
	}
	if !isChatPath(p.Path) {
		return state, nil
	}
	cid := action.CorrelationID
	if cid == "" {
		cid = action.ID
	}
	sessionID := string(cid)

	if state.Chat.Sessions == nil {
		state.Chat.Sessions = make(map[string]ChatSession)
	}
	if _, exists := state.Chat.Sessions[sessionID]; !exists {
		state.Chat.Sessions[sessionID] = ChatSession{
			ID:        sessionID,
			Source:    SourceOpenAI,
			CreatedAt: action.Time,
			UpdatedAt: action.Time,
		}
	}
	// Do not change ActiveSessionID — API sessions are observed but never
	// replace the active local TUI session.
	return state, nil
}

// isChatPath returns true when path corresponds to a chat/completions route.
func isChatPath(path string) bool {
	return path == "/v1/chat/completions" || path == "/v1/chat/completions/" ||
		strings.HasPrefix(path, "/v1/chat/completions/")
}

// ReduceWizard handles wizard:state actions to persist configuration across
// process restarts.
func ReduceWizard(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	switch action.Type {
	case ActionTypeWizard:
		return reduceWizardState(state, action)
	default:
		return state, nil
	}
}

func reduceWizardState(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	var p WizardStatePayload
	if err := json.Unmarshal(action.Payload, &p); err != nil {
		return state, nil
	}
	if p.Runtime != "" {
		state.Wizard.Runtime = p.Runtime
	}
	if p.Backend != "" {
		state.Wizard.Backend = p.Backend
	}
	if p.Role != "" {
		state.Wizard.Role = p.Role
	}
	if p.OptionOverrides != nil {
		state.Wizard.OptionOverrides = p.OptionOverrides
	}
	return state, nil
}

// ReduceTasks is a pass-through reducer for the Tasks domain.
func ReduceTasks(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}
