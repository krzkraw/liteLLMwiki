package tui

import (
	"os"
	"path/filepath"
	"testing"

	"g0litellama/internal/tui/store"
	"g0litellama/internal/tui/store/sqlite"
)

func TestNewModelWithSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "g0litellama.db")

	model := NewModel(ModelOptions{DBPath: dbPath})
	defer model.persistCloser.Close()

	// Dispatch an action through the wired CommandBus.
	_, err := model.store.Dispatch(nil, store.ActionEnvelope{
		Type:   store.ActionTypeSelectTab,
		Source: store.SourceTUI,
		Payload: store.MustPayload(store.SelectTabPayload{
			TabID: "dashboard",
		}),
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Force-flush the SQLite store.
	if s, ok := model.persistCloser.(*sqlite.Store); ok {
		if err := s.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
	} else {
		t.Fatal("persistCloser is not a *sqlite.Store")
	}

	// Verify the database file exists and has data.
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}

	// Re-open and verify the action was persisted.
	st2, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer st2.Close()

	events, err := st2.Since(0)
	if err != nil {
		t.Fatalf("Since(0): %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one persisted event, got none")
	}
}

func TestNewModelWithoutSQLiteGraceful(t *testing.T) {
	model := NewModel(ModelOptions{})
	if model.store == nil {
		t.Fatal("store should not be nil even without persistence")
	}
	if model.persistCloser != nil {
		t.Fatal("NewModel without DBPath should not open default user persistence")
	}
}
