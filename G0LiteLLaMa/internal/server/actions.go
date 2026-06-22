package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"g0litellama/internal/tui/store"
)

func generateID() store.ActionID {
	b := make([]byte, 16)
	rand.Read(b)
	return store.ActionID(hex.EncodeToString(b))
}

func (s *Server) handleActionDispatch(w http.ResponseWriter, r *http.Request) {
	if s.commandBus == nil {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusServiceUnavailable, Message: "command bus not available", Code: ErrCodeInternal})
		return
	}
	if r.Method != http.MethodPost {
		WriteActionAPIError(w, ErrActionMethodNotAllowed)
		return
	}
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		WriteActionAPIError(w, ErrActionUnsupportedMedia)
		return
	}

	var env store.ActionEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		WriteActionAPIError(w, fmt.Errorf("decode action envelope: %w", &ActionAPIError{Status: http.StatusBadRequest, Message: err.Error(), Code: ErrCodeBadRequest}))
		return
	}
	if env.Source == "" {
		env.Source = store.SourceAPI
	}
	if env.ID == "" {
		env.ID = generateID()
	}

	newState, err := s.commandBus.Dispatch(r.Context(), env)
	if err != nil {
		WriteActionAPIError(w, fmt.Errorf("dispatch action: %w", &ActionAPIError{Status: http.StatusInternalServerError, Message: err.Error(), Code: ErrCodeInternal}))
		return
	}

	resp := struct {
		ActionID store.ActionID      `json:"actionId"`
		Revision store.StateRevision `json:"revision"`
	}{
		ActionID: env.ID,
		Revision: newState.Revision,
	}
	WriteActionJSON(w, resp)
}

func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	if s.commandBus == nil {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusServiceUnavailable, Message: "command bus not available", Code: ErrCodeInternal})
		return
	}
	WriteActionJSON(w, s.commandBus.State())
}

func (s *Server) handleGetChatSession(w http.ResponseWriter, r *http.Request) {
	if s.commandBus == nil {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusServiceUnavailable, Message: "command bus not available", Code: ErrCodeInternal})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusBadRequest, Message: "chat session id is required", Code: ErrCodeBadRequest})
		return
	}
	state := s.commandBus.State()
	session, ok := state.Chat.Sessions[id]
	if !ok {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusNotFound, Message: fmt.Sprintf("chat session %s not found", id), Code: ErrCodeNotFound})
		return
	}
	WriteActionJSON(w, session)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	if s.commandBus == nil {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusServiceUnavailable, Message: "command bus not available", Code: ErrCodeInternal})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusBadRequest, Message: "task id is required", Code: ErrCodeBadRequest})
		return
	}
	state := s.commandBus.State()
	task, ok := state.Tasks.Items[id]
	if !ok {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusNotFound, Message: fmt.Sprintf("task %s not found", id), Code: ErrCodeNotFound})
		return
	}
	WriteActionJSON(w, task)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.commandBus == nil {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusServiceUnavailable, Message: "command bus not available", Code: ErrCodeInternal})
		return
	}
	// Settings are schema-only; return empty for now.
	WriteActionJSON(w, map[string]any{})
}

func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	if s.commandBus == nil {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusServiceUnavailable, Message: "command bus not available", Code: ErrCodeInternal})
		return
	}

	afterStr := r.URL.Query().Get("after")
	var after store.StateRevision
	if afterStr != "" {
		v, err := strconv.ParseUint(afterStr, 10, 64)
		if err != nil {
			WriteActionAPIError(w, &ActionAPIError{Status: http.StatusBadRequest, Message: fmt.Sprintf("invalid after revision: %s", afterStr), Code: ErrCodeBadRequest})
			return
		}
		after = store.StateRevision(v)
	}

	events, err := s.commandBus.EventsSince(after)
	if err != nil {
		WriteActionAPIError(w, fmt.Errorf("read events: %w", &ActionAPIError{Status: http.StatusInternalServerError, Message: err.Error(), Code: ErrCodeInternal}))
		return
	}
	WriteActionJSON(w, events)
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	if s.commandBus == nil {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusServiceUnavailable, Message: "command bus not available", Code: ErrCodeInternal})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusInternalServerError, Message: "streaming not supported", Code: ErrCodeInternal})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.commandBus.Subscribe()
	defer func() {
		s.commandBus.Unsubscribe(ch)
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case sa, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(sa)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleGetStateRunners(w http.ResponseWriter, r *http.Request) {
	if s.commandBus == nil {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusServiceUnavailable, Message: "command bus not available", Code: ErrCodeInternal})
		return
	}
	state := s.commandBus.State()
	WriteActionJSON(w, state.Runners)
}

func (s *Server) handleGetStateModels(w http.ResponseWriter, r *http.Request) {
	if s.commandBus == nil {
		WriteActionAPIError(w, &ActionAPIError{Status: http.StatusServiceUnavailable, Message: "command bus not available", Code: ErrCodeInternal})
		return
	}
	state := s.commandBus.State()
	WriteActionJSON(w, state.Models)
}

// parseActionError extracts an ActionAPIError from an error chain, returning
// the original wrapped ActionAPIError. Used by tests and middleware.
func parseActionError(err error) *ActionAPIError {
	var apiErr *ActionAPIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}
