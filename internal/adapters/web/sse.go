package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"picoclip/internal/core/domain"
)

func (s *Server) handleSSEActivity(w http.ResponseWriter, r *http.Request) {
	streamEvents(w, r, s.bus, func(ev domain.Event) bool { return true })
}

func (s *Server) handleSSERunLogs(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	streamEvents(w, r, s.bus, func(ev domain.Event) bool {
		if ev.RunID != runID {
			return false
		}
		switch ev.Type {
		case domain.EventRunOutput, domain.EventRunCompleted, domain.EventRunFailed, domain.EventRunCanceled:
			return true
		default:
			return false
		}
	})
}

func streamEvents(w http.ResponseWriter, r *http.Request, bus interface {
	Subscribe(context.Context) (<-chan domain.Event, error)
}, allow func(domain.Event) bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, err := bus.Subscribe(ctx)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "retry: 2000\n\n")
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !allow(ev) {
				continue
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}
