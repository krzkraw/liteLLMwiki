package sqlite

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"g0litellama/internal/tui/store"
)

func TestOpenDBCreatesFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}
}

func TestOpenDBApplySchemaIdempotent(t *testing.T) {
	db := newDB(t)
	tables := []string{"actions", "events", "tasks", "snapshots", "settings"}
	for _, name := range tables {
		var n int
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name,
		).Scan(&n); err != nil {
			t.Fatalf("check table %s: %v", name, err)
		}
		if n != 1 {
			t.Errorf("table %s not found after schema apply", name)
		}
	}
}

func TestAppendActionAndFlush(t *testing.T) {
	st := newStore(t)
	defer st.Close()

	err := st.AppendAction(store.ActionEnvelope{
		ID:   "act-1",
		Type: store.ActionTypeSelectTab,
		Payload: store.MustPayload(store.SelectTabPayload{
			TabID: "wizard",
		}),
		Time: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("AppendAction: %v", err)
	}

	if err := st.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Verify the action was written.
	var id, typ, src, created string
	err = st.db.QueryRow(
		`SELECT id, type, source, created_at FROM actions WHERE id = ?`, "act-1",
	).Scan(&id, &typ, &src, &created)
	if err != nil {
		t.Fatalf("read action: %v", err)
	}
	if typ != string(store.ActionTypeSelectTab) {
		t.Errorf("expected type %q, got %q", store.ActionTypeSelectTab, typ)
	}
}

func TestFlushKeepsBuffersOnError(t *testing.T) {
	db := newDB(t)
	st := &Store{db: db}

	action := store.ActionEnvelope{
		ID:      "a1",
		Type:    store.ActionTypeSelectTab,
		Source:  store.SourceTUI,
		Payload: store.MustPayload(store.SelectTabPayload{TabID: "wizard"}),
	}
	event := store.StoredEvent{
		Revision: 1,
		ActionID: "a1",
		Type:     store.ActionTypeSelectTab,
		Payload:  action.Payload,
	}
	if err := st.AppendAction(action); err != nil {
		t.Fatalf("AppendAction: %v", err)
	}
	if err := st.AppendEvents([]store.StoredEvent{event}); err != nil {
		t.Fatalf("AppendEvents: %v", err)
	}
	if err := st.Save(store.AppState{Revision: 1}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if err := st.Flush(); err == nil {
		t.Fatal("Flush error = nil, want closed database error")
	}
	if len(st.evBuf) != 1 {
		t.Fatalf("action buffer length = %d, want 1", len(st.evBuf))
	}
	if len(st.evtBuf) != 1 {
		t.Fatalf("event buffer length = %d, want 1", len(st.evtBuf))
	}
	if st.ssBuf == nil {
		t.Fatal("snapshot buffer was dropped after failed flush")
	}
}

func TestAppendManyActionsFlushInOneTx(t *testing.T) {
	st := newStore(t)
	defer st.Close()

	for i := 0; i < 10; i++ {
		id := store.ActionID("act-" + string(rune('0'+i)))
		st.AppendAction(store.ActionEnvelope{
			ID:      id,
			Type:    store.ActionTypeSelectTab,
			Payload: store.MustPayload(store.SelectTabPayload{TabID: "dashboard"}),
		})
	}
	if err := st.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	var count int
	if err := st.db.QueryRow("SELECT COUNT(*) FROM actions").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 actions, got %d", count)
	}
}

func TestSaveAndLoadSnapshot(t *testing.T) {
	st := newStore(t)
	defer st.Close()

	state := store.AppState{
		Revision: 7,
		Viewport: store.ViewportState{Width: 120, Height: 40},
		UI:       store.UIState{ActiveTab: "chat"},
	}
	if err := st.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := st.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	loaded, rev, err := st.LoadLatest()
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if rev != 7 {
		t.Errorf("expected rev 7, got %d", rev)
	}
	if loaded.UI.ActiveTab != "chat" {
		t.Errorf("expected ActiveTab chat, got %q", loaded.UI.ActiveTab)
	}
	if loaded.Viewport.Width != 120 || loaded.Viewport.Height != 40 {
		t.Errorf("viewport mismatch: %+v", loaded.Viewport)
	}
}

func TestLoadLatestEmptyReturnsZero(t *testing.T) {
	db := newDB(t)
	st := NewWithDB(db)
	defer st.Close()

	state, rev, err := st.LoadLatest()
	if err != nil {
		t.Fatalf("LoadLatest on empty db: %v", err)
	}
	if rev != 0 {
		t.Errorf("expected rev 0 on empty, got %d", rev)
	}
	if state.Revision != 0 {
		t.Errorf("expected zero AppState, got rev %d", state.Revision)
	}
}

func TestSnapshotOverwrite(t *testing.T) {
	st := newStore(t)
	defer st.Close()

	st.Save(store.AppState{Revision: 1, UI: store.UIState{ActiveTab: "dashboard"}})
	st.Save(store.AppState{Revision: 2, UI: store.UIState{ActiveTab: "chat"}})
	if err := st.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	_, rev, err := st.LoadLatest()
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if rev != 2 {
		t.Errorf("expected latest rev 2, got %d", rev)
	}
}

func TestAppendEventsAndSince(t *testing.T) {
	st := newStore(t)
	defer st.Close()

	events := []store.StoredEvent{
		{Revision: 1, ActionID: "a1", Type: store.ActionTypeSelectTab, Payload: json.RawMessage(`{"tab_id":"dashboard"}`), CreatedAt: time.Now().UnixNano()},
		{Revision: 2, ActionID: "a2", Type: store.ActionTypeSelectTab, Payload: json.RawMessage(`{"tab_id":"wizard"}`), CreatedAt: time.Now().UnixNano()},
		{Revision: 3, ActionID: "a3", Type: store.ActionTypeSelectTab, Payload: json.RawMessage(`{"tab_id":"chat"}`), CreatedAt: time.Now().UnixNano()},
	}
	if err := st.AppendEvents(events); err != nil {
		t.Fatalf("AppendEvents: %v", err)
	}
	if err := st.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	got, err := st.Since(0)
	if err != nil {
		t.Fatalf("Since(0): %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}

	got2, err := st.Since(1)
	if err != nil {
		t.Fatalf("Since(1): %v", err)
	}
	if len(got2) != 2 {
		t.Fatalf("expected 2 events after rev 1, got %d", len(got2))
	}
	if got2[0].Revision != 2 {
		t.Errorf("expected first after rev 2, got %d", got2[0].Revision)
	}

	got3, err := st.Since(3)
	if err != nil {
		t.Fatalf("Since(3): %v", err)
	}
	if len(got3) != 0 {
		t.Fatalf("expected 0 events after rev 3, got %d", len(got3))
	}
}

func TestReplayFromSnapshot(t *testing.T) {
	// Simulates: snapshot at rev 2, then 2 more actions → close → re-open →
	// load snapshot → Since(snapshotRev) → Replay → verify final state.
	dbPath := filepath.Join(t.TempDir(), "replay.db")

	st1, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Actions 1-2: build state to rev 2, snapshot at rev 2.
	st1.AppendAction(store.ActionEnvelope{
		ID:      "[replay] act-1",
		Type:    store.ActionTypeSelectTab,
		Payload: store.MustPayload(store.SelectTabPayload{TabID: "dashboard"}),
		Time:    time.Now(),
	})
	st1.AppendAction(store.ActionEnvelope{
		ID:      "[replay] act-2",
		Type:    store.ActionTypeSelectTab,
		Payload: store.MustPayload(store.SelectTabPayload{TabID: "wizard"}),
		Time:    time.Now(),
	})
	st1.Save(store.AppState{Revision: 2, UI: store.UIState{ActiveTab: "wizard"}})

	// Events 3-4: dispatch after snapshot.
	st1.AppendEvents([]store.StoredEvent{
		{
			Revision: 3, ActionID: "[replay] act-3",
			Type:    store.ActionTypeSelectTab,
			Payload: store.MustPayload(store.SelectTabPayload{TabID: "chat"}),
		},
		{
			Revision: 4, ActionID: "[replay] act-4",
			Type:    store.ActionTypeSelectTab,
			Payload: store.MustPayload(store.SelectTabPayload{TabID: "wizard"}),
		},
	})
	if err := st1.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	st1.Close()

	// Re-open and replay.
	st2, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer st2.Close()

	snapshotState, snapshotRev, err := st2.LoadLatest()
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if snapshotRev != 2 {
		t.Fatalf("expected snapshot rev 2, got %d", snapshotRev)
	}

	events, err := st2.Since(snapshotRev)
	if err != nil {
		t.Fatalf("Since(%d): %v", snapshotRev, err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events after rev 2, got %d", len(events))
	}

	// Replay: start from snapshot state, apply each event.
	replayState := snapshotState
	for _, e := range events {
		action := store.ActionEnvelope{
			ID:      e.ActionID,
			Type:    e.Type,
			Payload: e.Payload,
		}
		var effects []store.EffectSpec
		replayState, effects = store.RootReduce(replayState, action)
		_ = effects
	}

	if replayState.UI.ActiveTab != "wizard" {
		t.Errorf("expected ActiveTab wizard after replay, got %q", replayState.UI.ActiveTab)
	}
	if replayState.Revision != 4 {
		t.Errorf("expected rev 4 after replay, got %d", replayState.Revision)
	}
}

func TestPersistTasksTable(t *testing.T) {
	db := newDB(t)
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Insert parent task.
	if _, err := db.Exec(
		`INSERT INTO tasks (id, parent_id, state_path, kind, status, progress_json, error, created_at, updated_at)
		 VALUES (?, NULL, ?, ?, ?, ?, ?, ?, ?)`,
		"task-parent", "runners.LR-M-1.start", "download", "running",
		`{"percent":45,"message":"downloading chunk 3/7"}`, "", now, now,
	); err != nil {
		t.Fatalf("insert parent task: %v", err)
	}

	// Insert child task.
	if _, err := db.Exec(
		`INSERT INTO tasks (id, parent_id, state_path, kind, status, progress_json, error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"task-child", "task-parent", "runners.LR-M-1.start.io", "io", "failed",
		`{}`, "permission denied", now, now,
	); err != nil {
		t.Fatalf("insert child task: %v", err)
	}

	// Read back and verify parent.
	var parentID, statePath, kind, status, progress, errStr string
	var parentPtr *string
	err := db.QueryRow(
		`SELECT id, parent_id, state_path, kind, status, progress_json, error
		 FROM tasks WHERE id = ?`, "task-parent",
	).Scan(&parentID, &parentPtr, &statePath, &kind, &status, &progress, &errStr)
	if err != nil {
		t.Fatalf("read parent: %v", err)
	}
	if kind != "download" || status != "running" {
		t.Errorf("parent task kind/status: %s/%s", kind, status)
	}
	if parentPtr != nil {
		t.Errorf("expected nil parent_id for root task, got %s", *parentPtr)
	}

	// Child has parent_id set.
	var childID string
	var childParent string
	var childErr string
	err = db.QueryRow(
		`SELECT id, parent_id, error FROM tasks WHERE id = ?`, "task-child",
	).Scan(&childID, &childParent, &childErr)
	if err != nil {
		t.Fatalf("read child: %v", err)
	}
	if childParent != "task-parent" {
		t.Errorf("expected child parent_id task-parent, got %q", childParent)
	}
	if childErr != "permission denied" {
		t.Errorf("expected child error 'permission denied', got %q", childErr)
	}
}

func TestPersistSettings(t *testing.T) {
	db := newDB(t)
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := db.Exec(
		`INSERT OR REPLACE INTO settings (key, value_json, updated_at)
		 VALUES (?, ?, ?)`,
		"theme", `"dark"`, now,
	); err != nil {
		t.Fatalf("insert setting: %v", err)
	}
	if _, err := db.Exec(
		`INSERT OR REPLACE INTO settings (key, value_json, updated_at)
		 VALUES (?, ?, ?)`,
		"model-default", `"gemma4-litert"`, now,
	); err != nil {
		t.Fatalf("insert setting: %v", err)
	}

	var theme string
	err := db.QueryRow(`SELECT value_json FROM settings WHERE key = ?`, "theme").Scan(&theme)
	if err != nil {
		t.Fatalf("read theme: %v", err)
	}
	if theme != `"dark"` {
		t.Errorf("expected theme \"dark\", got %s", theme)
	}
}

// --- helpers ---

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close db: %v", err)
		}
	})
	return db
}

func newStore(t *testing.T) *Store {
	t.Helper()
	db := newDB(t)
	return NewWithDB(db)
}
