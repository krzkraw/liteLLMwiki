package store

import "encoding/json"

// StoredEvent pairs an action with the revision at which it was committed,
// for replay and persistence.
type StoredEvent struct {
	Revision  StateRevision  `json:"revision"`
	ActionID  ActionID       `json:"actionId"`
	Type      ActionType     `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt int64          `json:"createdAt"`
}

// EventLog records actions for persistence and enables replay from a given
// revision. Implementations may buffer writes for performance.
type EventLog interface {
	AppendAction(ActionEnvelope) error
	AppendEvents([]StoredEvent) error
	Since(StateRevision) ([]StoredEvent, error)
}

// SnapshotStore persists full AppState snapshots indexed by revision.
type SnapshotStore interface {
	LoadLatest() (AppState, StateRevision, error)
	Save(AppState) error
}
