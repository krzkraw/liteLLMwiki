package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"g0litellama/internal/tui/store"
)

// Store implements store.EventLog and store.SnapshotStore backed by SQLite.
// Writes are buffered and flushed to SQLite at most every flushInterval. Use
// Flush to force a synchronous write (useful in tests).
type Store struct {
	db *sql.DB

	evMu  sync.Mutex
	evBuf []store.ActionEnvelope

	evtMu  sync.Mutex
	evtBuf []store.StoredEvent

	ssMu  sync.Mutex
	ssBuf *store.AppState

	done chan struct{}
	wg   sync.WaitGroup
}

const flushInterval = 2 * time.Second

// New opens the database at dbPath, applies the schema, and returns a Store
// that starts a background flush goroutine. Call Close to stop the goroutine
// and close the database.
func New(dbPath string) (*Store, error) {
	db, err := OpenDB(dbPath)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, done: make(chan struct{})}
	s.wg.Add(1)
	go s.flushLoop()
	return s, nil
}

// NewWithDB returns a Store that uses an already-opened database. The caller
// is responsible for closing db when the Store is done.
func NewWithDB(db *sql.DB) *Store {
	s := &Store{db: db, done: make(chan struct{})}
	s.wg.Add(1)
	go s.flushLoop()
	return s
}

// Close stops the flush goroutine (final flush) and closes the database.
func (s *Store) Close() error {
	close(s.done)
	s.wg.Wait()
	return s.db.Close()
}

// Flush synchronously writes all buffered actions, events, and the latest
// pending snapshot to SQLite.
func (s *Store) Flush() error {
	s.evMu.Lock()
	evBuf := s.evBuf
	s.evBuf = nil
	s.evMu.Unlock()

	s.evtMu.Lock()
	evtBuf := s.evtBuf
	s.evtBuf = nil
	s.evtMu.Unlock()

	s.ssMu.Lock()
	ssBuf := s.ssBuf
	s.ssBuf = nil
	s.ssMu.Unlock()

	if err := s.flush(evBuf, evtBuf, ssBuf); err != nil {
		s.requeue(evBuf, evtBuf, ssBuf)
		return err
	}
	return nil
}

func (s *Store) flushLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	drain := func() {
		s.evMu.Lock()
		evBuf := s.evBuf
		s.evBuf = nil
		s.evMu.Unlock()

		s.evtMu.Lock()
		evtBuf := s.evtBuf
		s.evtBuf = nil
		s.evtMu.Unlock()

		s.ssMu.Lock()
		ssBuf := s.ssBuf
		s.ssBuf = nil
		s.ssMu.Unlock()

		if len(evBuf) > 0 || len(evtBuf) > 0 || ssBuf != nil {
			if err := s.flush(evBuf, evtBuf, ssBuf); err != nil {
				s.requeue(evBuf, evtBuf, ssBuf)
			}
		}
	}

	for {
		select {
		case <-s.done:
			drain()
			return
		case <-ticker.C:
			drain()
		}
	}
}

func (s *Store) flush(evBuf []store.ActionEnvelope, evtBuf []store.StoredEvent, ssBuf *store.AppState) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if len(evBuf) > 0 {
		stmt, err := tx.Prepare(`INSERT OR IGNORE INTO actions
			(id, type, source, correlation_id, parent_id, payload_json, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare actions insert: %w", err)
		}
		defer stmt.Close()

		for _, a := range evBuf {
			var payload *string
			if a.Payload != nil {
				s := string(a.Payload)
				payload = &s
			}
			createdAt := a.Time.Format(time.RFC3339Nano)
			if _, err := stmt.Exec(string(a.ID), string(a.Type), string(a.Source),
				nullString(string(a.CorrelationID)), nullString(string(a.ParentID)),
				payload, createdAt); err != nil {
				return fmt.Errorf("insert action %s: %w", a.ID, err)
			}
		}
	}

	if len(evtBuf) > 0 {
		stmt, err := tx.Prepare(`INSERT OR IGNORE INTO events
			(revision, action_id, type, payload_json, created_at)
			VALUES (?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare events insert: %w", err)
		}
		defer stmt.Close()

		for _, e := range evtBuf {
			var payload *string
			if e.Payload != nil {
				s := string(e.Payload)
				payload = &s
			}
			createdAt := time.Unix(0, e.CreatedAt).Format(time.RFC3339Nano)
			if _, err := stmt.Exec(uint64(e.Revision), string(e.ActionID),
				string(e.Type), payload, createdAt); err != nil {
				return fmt.Errorf("insert event rev %d: %w", e.Revision, err)
			}
		}
	}

	if ssBuf != nil {
		data, err := json.Marshal(ssBuf)
		if err != nil {
			return fmt.Errorf("marshal snapshot: %w", err)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.Exec(`INSERT OR REPLACE INTO snapshots
			(revision, state_json, created_at) VALUES (?, ?, ?)`,
			ssBuf.Revision, string(data), now); err != nil {
			return fmt.Errorf("insert snapshot: %w", err)
		}
	}

	return tx.Commit()
}

// --- EventLog interface ---

// AppendAction buffers an action for the next SQLite flush.
func (s *Store) AppendAction(a store.ActionEnvelope) error {
	s.evMu.Lock()
	s.evBuf = append(s.evBuf, a)
	s.evMu.Unlock()
	return nil
}

// AppendEvents buffers events for the next SQLite flush.
func (s *Store) AppendEvents(events []store.StoredEvent) error {
	s.evtMu.Lock()
	s.evtBuf = append(s.evtBuf, events...)
	s.evtMu.Unlock()
	return nil
}

// Since reads every event at revision > after from the events table.
func (s *Store) Since(after store.StateRevision) ([]store.StoredEvent, error) {
	rows, err := s.db.Query(
		`SELECT revision, action_id, type, COALESCE(payload_json, ''), created_at
		 FROM events WHERE revision > ? ORDER BY revision`, uint64(after))
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []store.StoredEvent
	for rows.Next() {
		var e store.StoredEvent
		var rev uint64
		var actionID, typ, payloadStr, createdAt string
		if err := rows.Scan(&rev, &actionID, &typ, &payloadStr, &createdAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.Revision = store.StateRevision(rev)
		e.ActionID = store.ActionID(actionID)
		e.Type = store.ActionType(typ)
		if payloadStr != "" {
			e.Payload = json.RawMessage(payloadStr)
		}
		t, err := time.Parse(time.RFC3339Nano, createdAt)
		if err == nil {
			e.CreatedAt = t.UnixNano()
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// --- SnapshotStore interface ---

// Save buffers the current AppState for the next SQLite flush. Only the most
// recent buffered state is kept.
func (s *Store) Save(state store.AppState) error {
	s.ssMu.Lock()
	s.ssBuf = &state
	s.ssMu.Unlock()
	return nil
}

// LoadLatest reads the most recent snapshot from SQLite.
func (s *Store) LoadLatest() (store.AppState, store.StateRevision, error) {
	var stateJSON string
	var rev uint64

	err := s.db.QueryRow(
		`SELECT revision, state_json FROM snapshots
		 ORDER BY revision DESC LIMIT 1`).Scan(&rev, &stateJSON)
	if err == sql.ErrNoRows {
		return store.AppState{}, 0, nil
	}
	if err != nil {
		return store.AppState{}, 0, fmt.Errorf("query snapshot: %w", err)
	}

	var state store.AppState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return store.AppState{}, 0, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return state, store.StateRevision(rev), nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) requeue(evBuf []store.ActionEnvelope, evtBuf []store.StoredEvent, ssBuf *store.AppState) {
	if len(evBuf) > 0 {
		s.evMu.Lock()
		s.evBuf = append(evBuf, s.evBuf...)
		s.evMu.Unlock()
	}
	if len(evtBuf) > 0 {
		s.evtMu.Lock()
		s.evtBuf = append(evtBuf, s.evtBuf...)
		s.evtMu.Unlock()
	}
	if ssBuf != nil {
		s.ssMu.Lock()
		if s.ssBuf == nil {
			s.ssBuf = ssBuf
		}
		s.ssMu.Unlock()
	}
}

// LoadAndReplay opens the database at dbPath, loads the latest snapshot,
// replays all subsequent events through store.RootReduce, and returns the
// resulting AppState. Returns a zero AppState when the DB is empty or cannot
// be read.
func LoadAndReplay(dbPath string) (store.AppState, error) {
	s, err := New(dbPath)
	if err != nil {
		return store.AppState{}, err
	}
	defer s.Close()
	return ReplayFromStore(s)
}

// ReplayFromStore loads the latest snapshot from an already-open Store,
// replays all subsequent events through store.RootReduce, and returns the
// resulting AppState.
func ReplayFromStore(s *Store) (store.AppState, error) {
	snapshotState, snapshotRev, err := s.LoadLatest()
	if err != nil {
		snapshotState = store.AppState{}
		snapshotRev = 0
	}

	events, err := s.Since(snapshotRev)
	if err != nil {
		return store.AppState{}, fmt.Errorf("load events after rev %d: %w", snapshotRev, err)
	}

	replayState := snapshotState
	for _, e := range events {
		action := store.ActionEnvelope{
			ID:      e.ActionID,
			Type:    e.Type,
			Payload: e.Payload,
		}
		replayState, _ = store.RootReduce(replayState, action)
	}
	return replayState, nil
}
