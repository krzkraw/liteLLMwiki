// Package store provides the Redux-style state management foundation for the
// G0LiteLLaMa TUI. Actions dispatch through a CommandBus, reducers produce
// new AppState revisions deterministically, and effects are scheduled as work
// outside reducers.
package store

import (
	"encoding/json"
	"time"
)

// StateRevision is a monotonically increasing revision counter incremented on
// every accepted action.
type StateRevision uint64

// ActionID uniquely identifies a single action.
type ActionID string

// ActionType categorises an action for routing to the correct reducer.
type ActionType string

// ActionTypeNewChatSession creates a new TUI chat session and marks it active.
const ActionTypeNewChatSession ActionType = "chat:new-session"

// NewChatSessionPayload creates a new chat session.
type NewChatSessionPayload struct {
	// Label is an optional human-readable session label.
	Label string `json:"label,omitempty"`
}

// ActionSource identifies where the action originated.
type ActionSource string

const (
	SourceTUI    ActionSource = "tui"
	SourceAPI    ActionSource = "api"
	SourceTask   ActionSource = "task"
	SourceOpenAI ActionSource = "openai-proxy"
	SourceSystem ActionSource = "system"
)

// Proxy observation action types. Dispatched by the proxy round tripper when
// observing OpenAI-compatible /v1/* traffic through the store.
const (
	ActionTypeProxyRequestStart  ActionType = "proxy:request-start"
	ActionTypeProxyResponseChunk ActionType = "proxy:response-chunk"
	ActionTypeProxyResponseEnd   ActionType = "proxy:response-end"
	ActionTypeProxyResponseError ActionType = "proxy:response-error"
)

// ProxyRequestStartPayload is the payload for proxy:request-start actions.
type ProxyRequestStartPayload struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Role   string `json:"role"`
}

// ProxyResponseChunkPayload is the payload for proxy:response-chunk actions.
type ProxyResponseChunkPayload struct {
	CorrelationID string `json:"correlationId"`
	Data          []byte `json:"data"`
	Index         int    `json:"index"`
}

// ProxyResponseEndPayload is the payload for proxy:response-end actions.
type ProxyResponseEndPayload struct {
	CorrelationID string `json:"correlationId"`
	StatusCode    int    `json:"statusCode"`
	ContentType   string `json:"contentType,omitempty"`
}

// ProxyResponseErrorPayload is the payload for proxy:response-error actions.
type ProxyResponseErrorPayload struct {
	CorrelationID string `json:"correlationId"`
	Error         string `json:"error"`
}

// ActionEnvelope wraps every action with routing, tracing, and timing metadata.
type ActionEnvelope struct {
	ID            ActionID        `json:"id"`
	Type          ActionType      `json:"type"`
	Source        ActionSource    `json:"source"`
	CorrelationID ActionID        `json:"correlationId,omitempty"`
	ParentID      ActionID        `json:"parentId,omitempty"`
	Time          time.Time       `json:"time"`
	Payload       json.RawMessage `json:"payload"`
}

// ViewportState holds the terminal dimensions.
type ViewportState struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// RunnersState holds runner snapshots. Expanded in a later slice.
type RunnersState struct{}

// ModelsState holds model catalog state. Expanded in a later slice.
type ModelsState struct{}

// RuntimeState holds runtime status. Expanded in a later slice.
type RuntimeState struct{}

// ChatSession represents a single chat session, created by the TUI or by API
// observation.
type ChatSession struct {
	ID        string        `json:"id"`
	Source    ActionSource  `json:"source"`
	Messages  []ChatMessage `json:"messages,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// ChatMessage is a single message within a chat session.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatState holds chat sessions.
type ChatState struct {
	Sessions        map[string]ChatSession `json:"sessions,omitempty"`
	ActiveSessionID string                 `json:"activeSessionId,omitempty"`
}

// WizardState holds launch wizard selections that should survive process restart.
type WizardState struct {
	Runtime         string            `json:"runtime,omitempty"`
	Backend         string            `json:"backend,omitempty"`
	Role            string            `json:"role,omitempty"`
	OptionOverrides map[string]string `json:"optionOverrides,omitempty"`
}

// TaskState holds active task tracking.
type TaskState struct {
	Items map[string]Task `json:"items,omitempty"`
}

// UIState holds user-interface-local state.
type UIState struct {
	ActiveTab string `json:"activeTab"`
}

// AppState is the root application state. All substates are defined upfront so
// the state shape is stable across implementation slices.
type AppState struct {
	Revision StateRevision `json:"revision"`
	Viewport ViewportState `json:"viewport,omitempty"`
	Runners  RunnersState  `json:"runners,omitempty"`
	Models   ModelsState   `json:"models,omitempty"`
	Runtime  RuntimeState  `json:"runtime,omitempty"`
	Chat     ChatState     `json:"chat,omitempty"`
	Wizard   WizardState   `json:"wizard,omitempty"`
	Tasks    TaskState     `json:"tasks,omitempty"`
	UI       UIState       `json:"ui,omitempty"`
}
