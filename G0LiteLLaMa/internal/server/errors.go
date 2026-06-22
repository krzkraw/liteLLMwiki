package server

import (
	"encoding/json"
	"errors"
	"net/http"
)

// ActionAPIError is a structured error response for the action API.
type ActionAPIError struct {
	Status  int    `json:"-"`
	Message string `json:"error"`
	Code    string `json:"code,omitempty"`
}

func (e *ActionAPIError) Error() string {
	return e.Message
}

// ActionAPIErrorCode constants identify error categories.
const (
	ErrCodeBadRequest   = "bad_request"
	ErrCodeNotFound     = "not_found"
	ErrCodeConflict     = "conflict"
	ErrCodeInternal     = "internal_error"
	ErrCodeUnsupported  = "unsupported"
)

var (
	ErrActionMethodNotAllowed = &ActionAPIError{Status: http.StatusMethodNotAllowed, Message: "method not allowed", Code: ErrCodeUnsupported}
	ErrActionNotFound         = &ActionAPIError{Status: http.StatusNotFound, Message: "not found", Code: ErrCodeNotFound}
	ErrActionDecode           = &ActionAPIError{Status: http.StatusBadRequest, Message: "decode request body", Code: ErrCodeBadRequest}
	ErrActionUnsupportedMedia = &ActionAPIError{Status: http.StatusUnsupportedMediaType, Message: "content-type must be application/json", Code: ErrCodeBadRequest}
)

// WriteActionAPIError writes a structured error response to w.
func WriteActionAPIError(w http.ResponseWriter, err error) {
	var apiErr *ActionAPIError
	if errors.As(err, &apiErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(apiErr.Status)
		json.NewEncoder(w).Encode(apiErr)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(&ActionAPIError{
		Status:  http.StatusInternalServerError,
		Message: err.Error(),
		Code:    ErrCodeInternal,
	})
}

// WriteActionJSON writes a JSON 200 response, falling back to error.
func WriteActionJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(v)
}
