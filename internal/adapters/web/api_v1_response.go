package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"picoclip/internal/core/domain"
)

type apiEnvelope struct {
	Data       any            `json:"data,omitempty"`
	Meta       map[string]any `json:"meta,omitempty"`
	Pagination map[string]any `json:"pagination,omitempty"`
	Error      *apiError      `json:"error,omitempty"`
}

type apiError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (s *Server) apiData(w http.ResponseWriter, data any) {
	s.jsonResponse(w, apiEnvelope{Data: data})
}

func (s *Server) apiList(w http.ResponseWriter, data any, meta map[string]any) {
	s.jsonResponse(w, apiEnvelope{Data: data, Meta: meta})
}

func (s *Server) apiError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	if errors.Is(err, domain.ErrInvalidInput) {
		status = http.StatusBadRequest
		code = "invalid_input"
	} else if errors.Is(err, domain.ErrNotFound) {
		status = http.StatusNotFound
		code = "not_found"
	} else if errors.Is(err, domain.ErrDriverUnavailable) || errors.Is(err, domain.ErrRuntimeUnavailable) {
		status = http.StatusConflict
		code = "driver_unavailable"
	} else if errors.Is(err, domain.ErrNoPendingTasks) {
		status = http.StatusNotFound
		code = "no_pending_tasks"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiEnvelope{Error: &apiError{Code: code, Message: err.Error()}})
}
