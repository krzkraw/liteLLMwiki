package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// CommandBus dispatches actions through the root reducer, appends them to the
// event log, and increments the state revision atomically.
type CommandBus struct {
	mu    sync.RWMutex
	state AppState
	log   []StoredAction
	subs  []chan StoredAction
}

// StoredAction is an action envelope paired with the revision at which it was
// committed.
type StoredAction struct {
	Action  ActionEnvelope
	Revision StateRevision
}

// NewCommandBus returns a CommandBus initialised with the given state.
func NewCommandBus(initial AppState) *CommandBus {
	return &CommandBus{
		state: initial,
	}
}

// Dispatch applies the action through the root reducer, commits the result
// atomically, and returns the new state. Dispatch is safe for concurrent use.
func (b *CommandBus) Dispatch(_ context.Context, action ActionEnvelope) (AppState, error) {
	if action.ID == "" {
		id, err := newActionID()
		if err != nil {
			return b.state, fmt.Errorf("generate action id: %w", err)
		}
		action.ID = id
	}
	if action.Time.IsZero() {
		action.Time = time.Now()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	newState, effects := RootReduce(b.state, action)
	_ = effects // effects are scheduled but not executed in this slice

	b.state = newState
	b.log = append(b.log, StoredAction{Action: action, Revision: b.state.Revision})

	for _, sub := range b.subs {
		select {
		case sub <- StoredAction{Action: action, Revision: b.state.Revision}:
		default:
		}
	}

	return b.state, nil
}

// State returns a copy of the current application state.
func (b *CommandBus) State() AppState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// Log returns a copy of every committed action in order.
func (b *CommandBus) Log() []StoredAction {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]StoredAction, len(b.log))
	copy(out, b.log)
	return out
}

// Subscribe returns a channel that receives every committed action. The
// channel has a small buffer; slow readers may miss actions.
func (b *CommandBus) Subscribe() <-chan StoredAction {
	ch := make(chan StoredAction, 64)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

// Replay rebuilds AppState by running a set of actions through RootReduce
// starting from a zero state. It returns the final state and the number of
// actions that were applied.
func Replay(actions []ActionEnvelope) (AppState, int) {
	state := AppState{}
	for _, a := range actions {
		var effects []EffectSpec
		state, effects = RootReduce(state, a)
		_ = effects
	}
	return state, len(actions)
}

// MustPayload is a helper that marshals a value into json.RawMessage, panicking
// on error. Use in tests and setup code.
func MustPayload(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func newActionID() (ActionID, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return ActionID(hex.EncodeToString(b)), nil
}
