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

// ActionSource identifies where the action originated.
type ActionSource string

const (
	SourceTUI    ActionSource = "tui"
	SourceAPI    ActionSource = "api"
	SourceTask   ActionSource = "task"
	SourceOpenAI ActionSource = "openai-proxy"
	SourceSystem ActionSource = "system"
)

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

// ChatState holds chat sessions. Expanded in a later slice.
type ChatState struct{}

// WizardState holds the launch wizard state. Expanded in a later slice.
type WizardState struct{}

// TaskState holds active task tracking. Expanded in a later slice.
type TaskState struct{}

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
