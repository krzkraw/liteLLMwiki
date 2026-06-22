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
	ID            ActionID
	Type          ActionType
	Source        ActionSource
	CorrelationID ActionID
	ParentID      ActionID
	Time          time.Time
	Payload       json.RawMessage
}

// ViewportState holds the terminal dimensions.
type ViewportState struct {
	Width  int
	Height int
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
	ActiveTab string
}

// AppState is the root application state. All substates are defined upfront so
// the state shape is stable across implementation slices.
type AppState struct {
	Revision StateRevision
	Viewport ViewportState
	Runners  RunnersState
	Models   ModelsState
	Runtime  RuntimeState
	Chat     ChatState
	Wizard   WizardState
	Tasks    TaskState
	UI       UIState
}
