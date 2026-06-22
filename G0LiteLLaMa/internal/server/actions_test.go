package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"g0litellama/internal/tui/store"
)

func newActionTestServer(t *testing.T) http.Handler {
	t.Helper()

	bus := store.NewCommandBus(store.AppState{})
	s := New(Options{CommandBus: bus})
	return s.Handler()
}

func TestActionDispatchValid(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	payload := store.MustPayload(store.SelectTabPayload{TabID: "dashboard"})
	env := store.ActionEnvelope{
		Type:    store.ActionTypeSelectTab,
		Source:  store.SourceAPI,
		Payload: payload,
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/g0litellama/v1/actions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		ActionID string `json:"actionId"`
		Revision uint64 `json:"revision"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ActionID == "" {
		t.Fatal("actionId is empty")
	}
	if resp.Revision == 0 {
		t.Fatal("revision is 0")
	}
}

func TestActionDispatchNoSourceDefaultsToAPI(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	payload := store.MustPayload(store.SelectTabPayload{TabID: "dashboard"})
	env := store.ActionEnvelope{
		Type:    store.ActionTypeSelectTab,
		Payload: payload,
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/g0litellama/v1/actions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestActionDispatchMethodNotAllowed(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/actions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestActionDispatchBadContentType(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/g0litellama/v1/actions", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
}

func TestActionDispatchInvalidBody(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/g0litellama/v1/actions", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetStateReturnsFullState(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/state", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var state store.AppState
	if err := json.NewDecoder(rec.Body).Decode(&state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.Revision != 0 {
		t.Fatalf("revision = %d, want 0", state.Revision)
	}
}

func TestGetStateAfterAction(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	payload := store.MustPayload(store.SelectTabPayload{TabID: "chat"})
	env := store.ActionEnvelope{
		Type:    store.ActionTypeSelectTab,
		Source:  store.SourceAPI,
		Payload: payload,
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/g0litellama/v1/actions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dispatch: status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/g0litellama/v1/state", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var state store.AppState
	if err := json.NewDecoder(rec.Body).Decode(&state); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if state.UI.ActiveTab != "chat" {
		t.Fatalf("activeTab = %q, want %q", state.UI.ActiveTab, "chat")
	}
}

func TestGetStateRunners(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/state/runners", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetStateModels(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/state/models", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetChatSession(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/state/chat/sessions/abc123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/tasks/abc123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetTaskMissingID(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/tasks/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Without an id segment the route won't match; expect 404 from default
	// mux behaviour.
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 404 or 400", rec.Code)
	}
}

func TestGetSettings(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/settings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestGetEventsNoAfter(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/events", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var events []store.StoredEvent
	if err := json.NewDecoder(rec.Body).Decode(&events); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestGetEventsInvalidAfter(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/events?after=abc", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestActionDispatchIncrementsRevision(t *testing.T) {
	t.Parallel()
	handler := newActionTestServer(t)

	// Dispatch two actions and verify revision increments.
	for i := 0; i < 2; i++ {
		payload := store.MustPayload(store.SelectTabPayload{TabID: "dashboard"})
		env := store.ActionEnvelope{
			Type:    store.ActionTypeSelectTab,
			Source:  store.SourceAPI,
			Payload: payload,
		}
		body, _ := json.Marshal(env)
		req := httptest.NewRequest(http.MethodPost, "/g0litellama/v1/actions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("dispatch %d: status=%d", i, rec.Code)
		}
	}

	// Verify state revision reflects both actions.
	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/state", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var state store.AppState
	if err := json.NewDecoder(rec.Body).Decode(&state); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if state.Revision != 2 {
		t.Fatalf("revision = %d, want 2", state.Revision)
	}
}

func TestActionAPIErrorResponse(t *testing.T) {
	t.Parallel()

	// Verify structured error response format.
	handler := newActionTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/g0litellama/v1/actions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", rec.Header().Get("Content-Type"))
	}

	var errResp ActionAPIError
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errResp.Message == "" {
		t.Fatal("error message is empty")
	}
	if errResp.Code == "" {
		t.Fatal("error code is empty")
	}
}
