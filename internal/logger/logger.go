package logger

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
)

// Log is the package-level structured logger.
var Log *slog.Logger

func init() {
	Log = slog.New(slog.NewJSONHandler(os.Stdout, nil))
}

// AppError represents an HTTP-level application error.
type AppError struct {
	Status  int    `json:"-"`
	Message string `json:"detail"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	return e.Message
}

// NotFound returns a 404 AppError.
func NotFound(msg string) *AppError {
	return &AppError{Status: 404, Message: msg}
}

// BadRequest returns a 400 AppError.
func BadRequest(msg string) *AppError {
	return &AppError{Status: 400, Message: msg}
}

// InternalError returns a 500 AppError.
func InternalError(msg string) *AppError {
	return &AppError{Status: 500, Message: msg}
}

// WriteJSON writes v as a JSON response with the given HTTP status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// WriteError converts an error to a JSON response in FastAPI-compatible format: {"detail": "message"}.
// If the error is an *AppError, its Status is used; otherwise 500 is returned.
func WriteError(w http.ResponseWriter, err error) {
	var status int
	var msg string

	if appErr, ok := err.(*AppError); ok {
		status = appErr.Status
		msg = appErr.Message
	} else {
		status = http.StatusInternalServerError
		msg = err.Error()
	}

	WriteJSON(w, status, map[string]string{"detail": msg})
}
