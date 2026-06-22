package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"g0litellama/internal/proxy"
	"g0litellama/internal/tui/store"
)

// waitForActionType polls the command bus log until an action of the given
// type appears or the timeout elapses.
func waitForActionType(t *testing.T, bus *store.CommandBus, typ store.ActionType, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		for _, a := range bus.Log() {
			if a.Action.Type == typ {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for action %q", typ)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// proxyTestHarness creates an upstream test server, a proxy pointing at it, a
// command bus, and the full server handler. The caller must close the upstream.
func proxyTestHarness(t *testing.T, upstream http.Handler) (*httptest.Server, http.Handler, *store.CommandBus) {
	t.Helper()
	upstreamSrv := httptest.NewServer(upstream)

	p, err := proxy.New(upstreamSrv.URL)
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	bus := store.NewCommandBus(store.AppState{})
	s := New(Options{Proxy: p, CommandBus: bus})
	return upstreamSrv, s.Handler(), bus
}

func TestObservingRoundTripperDispatchesActions(t *testing.T) {
	t.Parallel()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"model":"test","choices":[{"text":"hello"}]}`))
	})
	upstreamSrv, handler, bus := proxyTestHarness(t, upstream)
	defer upstreamSrv.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("proxy status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Poll until response-end arrives (async dispatch).
	deadline := time.After(3 * time.Second)
	var foundStart, foundEnd bool
	for {
		for _, a := range bus.Log() {
			if a.Action.Type == store.ActionTypeProxyRequestStart {
				foundStart = true
			}
			if a.Action.Type == store.ActionTypeProxyResponseEnd {
				foundEnd = true
			}
		}
		if foundStart && foundEnd {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for request-start and response-end; have start=%v end=%v",
				foundStart, foundEnd)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestObservingRoundTripperPreservesResponse(t *testing.T) {
	t.Parallel()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"ok"}`))
	})
	upstreamSrv, handler, _ := proxyTestHarness(t, upstream)
	defer upstreamSrv.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"result":"ok"}` {
		t.Fatalf("body = %q, want %q", rec.Body.String(), `{"result":"ok"}`)
	}
}

func TestObservingCreatesAPIChatSession(t *testing.T) {
	t.Parallel()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[]}`))
	})
	upstreamSrv, handler, bus := proxyTestHarness(t, upstream)
	defer upstreamSrv.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proxy status = %d", rec.Code)
	}

	waitForActionType(t, bus, store.ActionTypeProxyRequestStart, 3*time.Second)
	waitForActionType(t, bus, store.ActionTypeProxyResponseEnd, 3*time.Second)

	state := bus.State()
	if len(state.Chat.Sessions) != 1 {
		t.Fatalf("chat sessions = %d, want 1", len(state.Chat.Sessions))
	}
	for id, sess := range state.Chat.Sessions {
		if sess.Source != store.SourceOpenAI {
			t.Fatalf("session %q source = %q, want %q", id, sess.Source, store.SourceOpenAI)
		}
		if sess.ID != id {
			t.Fatalf("session map key = %q, want %q", id, sess.ID)
		}
	}
}

func TestObservingDoesNotChangeActiveSession(t *testing.T) {
	t.Parallel()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	upstreamSrv, handler, bus := proxyTestHarness(t, upstream)
	defer upstreamSrv.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proxy status = %d", rec.Code)
	}

	waitForActionType(t, bus, store.ActionTypeProxyResponseEnd, 3*time.Second)

	state := bus.State()
	if state.Chat.ActiveSessionID != "" {
		t.Fatalf("ActiveSessionID = %q, want empty (API sessions must not become active)", state.Chat.ActiveSessionID)
	}
	if len(state.Chat.Sessions) != 1 {
		t.Fatalf("chat sessions = %d, want 1", len(state.Chat.Sessions))
	}
	for _, sess := range state.Chat.Sessions {
		if sess.Source != store.SourceOpenAI {
			t.Fatalf("session source = %q, want %q", sess.Source, store.SourceOpenAI)
		}
	}
}

func TestObservingNonChatPathDoesNotCreateSession(t *testing.T) {
	t.Parallel()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[[0.1,0.2,0.3]]`))
	})
	upstreamSrv, handler, bus := proxyTestHarness(t, upstream)
	defer upstreamSrv.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"input":"test"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proxy status = %d", rec.Code)
	}

	waitForActionType(t, bus, store.ActionTypeProxyResponseEnd, 3*time.Second)

	state := bus.State()
	if len(state.Chat.Sessions) != 0 {
		t.Fatalf("chat sessions = %d, want 0 (embeddings should not create a session)", len(state.Chat.Sessions))
	}
}

func TestObservingStreamingDispatchesChunks(t *testing.T) {
	t.Parallel()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flush")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for i := 0; i < 3; i++ {
			w.Write([]byte("data: {\"chunk\":" + string(rune('0'+i)) + "}\n\n"))
			flusher.Flush()
		}
	})
	upstreamSrv, handler, bus := proxyTestHarness(t, upstream)
	defer upstreamSrv.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test","stream":true}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proxy status = %d", rec.Code)
	}

	deadline := time.After(3 * time.Second)
	var hasEnd bool
	for {
		log := bus.Log()
		for _, a := range log {
			if a.Action.Type == store.ActionTypeProxyResponseEnd {
				hasEnd = true
				break
			}
		}
		if hasEnd {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for response-end action; log has %d entries", len(log))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	chunks := 0
	for _, a := range bus.Log() {
		if a.Action.Type == store.ActionTypeProxyResponseChunk {
			chunks++
		}
	}
	if chunks < 1 {
		t.Fatalf("expected at least 1 response-chunk action, got %d", chunks)
	}
}

func TestObservingActionCorrelationIDs(t *testing.T) {
	t.Parallel()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})
	upstreamSrv, handler, bus := proxyTestHarness(t, upstream)
	defer upstreamSrv.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proxy status = %d", rec.Code)
	}

	deadline := time.After(3 * time.Second)
	var cid store.ActionID
	for {
		log := bus.Log()
		var foundStart, foundEnd bool
		for _, a := range log {
			if a.Action.Type == store.ActionTypeProxyRequestStart {
				foundStart = true
				cid = a.Action.CorrelationID
			}
			if a.Action.Type == store.ActionTypeProxyResponseEnd {
				foundEnd = true
			}
		}
		if foundStart && foundEnd {
			for _, a := range log {
				if a.Action.CorrelationID != cid {
					t.Fatalf("action %q correlation ID = %q, want %q", a.Action.Type, a.Action.CorrelationID, cid)
				}
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for start+end; log has %d entries", len(log))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if cid == "" {
		t.Fatal("request-start has empty correlation ID")
	}
}

func TestRoleForProxyPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want string
	}{
		{"/v1/chat/completions", "main"},
		{"/v1/completions", "main"},
		{"/v1/embeddings", "embedding"},
		{"/v1/embeddings/foo", "embedding"},
		{"/v1/rerank", "reranking"},
		{"/v1/rerank/bar", "reranking"},
		{"/v1/models", "main"},
	}
	for _, tt := range tests {
		got := roleForProxyPath(tt.path)
		if got != tt.want {
			t.Errorf("roleForProxyPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestObservingWithNoCommandBus(t *testing.T) {
	t.Parallel()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"pass":"through"}`))
	})
	upstreamSrv := httptest.NewServer(upstream)
	defer upstreamSrv.Close()

	p, err := proxy.New(upstreamSrv.URL)
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	s := New(Options{Proxy: p})
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"pass":"through"}` {
		t.Fatalf("body = %q, want %q", rec.Body.String(), `{"pass":"through"}`)
	}
}

func TestObservingWithProxyNoTarget(t *testing.T) {
	t.Parallel()
	p, err := proxy.New("http://127.0.0.1:19999")
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	bus := store.NewCommandBus(store.AppState{})
	s := New(Options{Proxy: p, CommandBus: bus})
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}

	deadline := time.After(3 * time.Second)
	var foundStart, foundErr bool
	for {
		for _, a := range bus.Log() {
			if a.Action.Type == store.ActionTypeProxyRequestStart {
				foundStart = true
			}
			if a.Action.Type == store.ActionTypeProxyResponseError {
				foundErr = true
			}
		}
		if foundStart && foundErr {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for start+error; have start=%v error=%v",
				foundStart, foundErr)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestReduceProxyRequestStartChatCreatesSession(t *testing.T) {
	t.Parallel()
	state := store.AppState{}

	payload := store.MustPayload(store.ProxyRequestStartPayload{
		Method: "POST",
		Path:   "/v1/chat/completions",
		Role:   "main",
	})
	action := store.ActionEnvelope{
		Type:          store.ActionTypeProxyRequestStart,
		Source:        store.SourceOpenAI,
		CorrelationID: "test-cid-1",
		Payload:       payload,
	}

	newState, _ := store.RootReduce(state, action)

	if len(newState.Chat.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(newState.Chat.Sessions))
	}
	sess, ok := newState.Chat.Sessions["test-cid-1"]
	if !ok {
		t.Fatal("session not found by correlation ID")
	}
	if sess.Source != store.SourceOpenAI {
		t.Fatalf("session source = %q, want %q", sess.Source, store.SourceOpenAI)
	}
	if newState.Chat.ActiveSessionID != "" {
		t.Fatalf("ActiveSessionID = %q, want empty", newState.Chat.ActiveSessionID)
	}
}

func TestReduceProxyRequestStartNonChatNoSession(t *testing.T) {
	t.Parallel()
	state := store.AppState{}

	payload := store.MustPayload(store.ProxyRequestStartPayload{
		Method: "POST",
		Path:   "/v1/embeddings",
		Role:   "embedding",
	})
	action := store.ActionEnvelope{
		Type:          store.ActionTypeProxyRequestStart,
		Source:        store.SourceOpenAI,
		CorrelationID: "test-cid-2",
		Payload:       payload,
	}

	newState, _ := store.RootReduce(state, action)

	if len(newState.Chat.Sessions) != 0 {
		t.Fatalf("sessions = %d, want 0", len(newState.Chat.Sessions))
	}
}

func TestReduceProxyRequestStartBadPayloadNoCrash(t *testing.T) {
	t.Parallel()
	state := store.AppState{}

	action := store.ActionEnvelope{
		Type:          store.ActionTypeProxyRequestStart,
		Source:        store.SourceOpenAI,
		CorrelationID: "test-cid-3",
		Payload:       json.RawMessage(`{invalid`),
	}

	newState, effects := store.RootReduce(state, action)
	if newState.Revision != 1 {
		t.Fatalf("revision = %d, want 1", newState.Revision)
	}
	if effects != nil && len(effects) > 0 {
		t.Fatalf("unexpected effects: %v", effects)
	}
}
