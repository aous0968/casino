package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorBody is the standard shape of every error response in the project.
type ErrorBody struct {
	Error   string            `json:"error"`
	Message string            `json:"message,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json failed", "err", err)
	}
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorBody{Error: code, Message: message})
}

func WriteValidationError(w http.ResponseWriter, fields map[string]string) {
	WriteJSON(w, http.StatusBadRequest, ErrorBody{
		Error:   "validation_failed",
		Message: "one or more fields are invalid",
		Fields:  fields,
	})
}