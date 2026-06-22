package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCommandBusDispatchRevision(t *testing.T) {
	bus := NewCommandBus(AppState{})

	ctx := context.Background()
	initialRev := bus.State().Revision

	_, err := bus.Dispatch(ctx, ActionEnvelope{
		Type:   ActionTypeSelectTab,
		Source: SourceTUI,
		Payload: MustPayload(SelectTabPayload{TabID: "dashboard"}),
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if got := bus.State().Revision; got != initialRev+1 {
		t.Errorf("expected revision %d, got %d", initialRev+1, got)
	}

	if got := bus.State().UI.ActiveTab; got != "dashboard" {
		t.Errorf("expected ActiveTab dashboard, got %q", got)
	}
}

func TestCommandBusAtomicCommit(t *testing.T) {
	bus := NewCommandBus(AppState{})

	ctx := context.Background()

	// Dispatch two tab selections in sequence.
	_, _ = bus.Dispatch(ctx, ActionEnvelope{
		Type:    ActionTypeSelectTab,
		Source:  SourceTUI,
		Payload: MustPayload(SelectTabPayload{TabID: "wizard"}),
	})
	_, _ = bus.Dispatch(ctx, ActionEnvelope{
		Type:    ActionTypeSelectTab,
		Source:  SourceTUI,
		Payload: MustPayload(SelectTabPayload{TabID: "chat"}),
	})

	if got := bus.State().UI.ActiveTab; got != "chat" {
		t.Errorf("expected ActiveTab chat, got %q", got)
	}
	if got := bus.State().Revision; got != 2 {
		t.Errorf("expected revision 2, got %d", got)
	}

	log := bus.Log()
	if len(log) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(log))
	}
	if log[0].Revision != 1 {
		t.Errorf("expected first log revision 1, got %d", log[0].Revision)
	}
	if log[1].Revision != 2 {
		t.Errorf("expected second log revision 2, got %d", log[1].Revision)
	}
}

func TestCommandBusConcurrentSafe(t *testing.T) {
	bus := NewCommandBus(AppState{})
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			bus.Dispatch(ctx, ActionEnvelope{
				Type:    ActionTypeSelectTab,
				Source:  SourceTUI,
				Payload: MustPayload(SelectTabPayload{TabID: "dashboard"}),
			})
		}
		close(done)
	}()

	for i := 0; i < 50; i++ {
		bus.Dispatch(ctx, ActionEnvelope{
			Type:    ActionTypeSelectTab,
			Source:  SourceTUI,
			Payload: MustPayload(SelectTabPayload{TabID: "wizard"}),
		})
	}
	<-done

	if got := bus.State().Revision; got != 100 {
		t.Errorf("expected revision 100 after 100 dispatches, got %d", got)
	}
}

func TestReplaySameActionsSameState(t *testing.T) {
	actions := []ActionEnvelope{
		{
			Type:    ActionTypeSelectTab,
			Source:  SourceTUI,
			Payload: MustPayload(SelectTabPayload{TabID: "dashboard"}),
		},
		{
			ID:      "custom-id",
			Type:    ActionTypeSelectTab,
			Source:  SourceTUI,
			Time:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Payload: MustPayload(SelectTabPayload{TabID: "wizard"}),
		},
		{
			Type:    ActionTypeSelectTab,
			Source:  SourceTUI,
			Payload: MustPayload(SelectTabPayload{TabID: "chat"}),
		},
		{
			Type:    ActionTypeSelectTab,
			Source:  SourceTUI,
			Payload: MustPayload(SelectTabPayload{TabID: "wizard"}),
		},
	}

	// Replay twice — must produce identical state.
	state1, n1 := Replay(actions)
	state2, n2 := Replay(actions)

	if n1 != n2 {
		t.Errorf("replayed action count differs: %d vs %d", n1, n2)
	}
	if n1 != len(actions) {
		t.Errorf("expected %d replayed, got %d", len(actions), n1)
	}
	if state1.UI.ActiveTab != state2.UI.ActiveTab {
		t.Errorf("ActiveTab mismatch: %q vs %q", state1.UI.ActiveTab, state2.UI.ActiveTab)
	}
	if state1.Revision != state2.Revision {
		t.Errorf("Revision mismatch: %d vs %d", state1.Revision, state2.Revision)
	}
}

func TestReplayEmptyActions(t *testing.T) {
	state, n := Replay(nil)
	if n != 0 {
		t.Errorf("expected 0 replayed, got %d", n)
	}
	if state.Revision != 0 {
		t.Errorf("expected revision 0, got %d", state.Revision)
	}
	if state.UI.ActiveTab != "" {
		t.Errorf("expected empty ActiveTab, got %q", state.UI.ActiveTab)
	}
}

func TestReplayDeterministicAcrossCalls(t *testing.T) {
	// Same sequence replayed three times must yield three identical states.
	actions := []ActionEnvelope{
		{
			Type:    ActionTypeSelectTab,
			Source:  SourceTUI,
			Payload: MustPayload(SelectTabPayload{TabID: "dashboard"}),
		},
		{
			Type:    ActionTypeSelectTab,
			Source:  SourceTUI,
			Payload: MustPayload(SelectTabPayload{TabID: "chat"}),
		},
		{
			Type:    ActionTypeSelectTab,
			Source:  SourceTUI,
			Payload: MustPayload(SelectTabPayload{TabID: "wizard"}),
		},
	}

	s0, _ := Replay(actions)
	s1, _ := Replay(actions)
	s2, _ := Replay(actions)

	if s0.UI.ActiveTab != s2.UI.ActiveTab {
		t.Fatalf("non-deterministic ActiveTab: %q vs %q", s0.UI.ActiveTab, s2.UI.ActiveTab)
	}
	if s0.UI.ActiveTab != s1.UI.ActiveTab {
		t.Fatalf("non-deterministic ActiveTab: %q vs %q", s0.UI.ActiveTab, s1.UI.ActiveTab)
	}
	if s0.Revision != s1.Revision || s1.Revision != s2.Revision {
		t.Fatal("non-deterministic revision across replays")
	}
}

func TestUnknownActionTypePassesThrough(t *testing.T) {
	bus := NewCommandBus(AppState{UI: UIState{ActiveTab: "dashboard"}})
	ctx := context.Background()

	_, err := bus.Dispatch(ctx, ActionEnvelope{
		Type:    ActionType("unknown:type"),
		Source:  SourceSystem,
		Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Dispatch unknown action: %v", err)
	}

	// State unchanged for unknown types.
	if got := bus.State().UI.ActiveTab; got != "dashboard" {
		t.Errorf("expected ActiveTab to remain dashboard, got %q", got)
	}
	// Revision still increments — action was accepted.
	if got := bus.State().Revision; got != 1 {
		t.Errorf("expected revision 1, got %d", got)
	}
}

func TestSubscriptionReceivesActions(t *testing.T) {
	bus := NewCommandBus(AppState{})
	ctx := context.Background()
	sub := bus.Subscribe()

	_, err := bus.Dispatch(ctx, ActionEnvelope{
		Type:    ActionTypeSelectTab,
		Source:  SourceTUI,
		Payload: MustPayload(SelectTabPayload{TabID: "wizard"}),
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	select {
	case sa := <-sub:
		if sa.Action.Type != ActionTypeSelectTab {
			t.Errorf("expected SelectTab type, got %q", sa.Action.Type)
		}
		if sa.Revision != 1 {
			t.Errorf("expected revision 1, got %d", sa.Revision)
		}
	default:
		t.Fatal("expected action on subscription channel")
	}
}
