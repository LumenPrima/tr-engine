package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/hlog"
)

type EventsHandler struct {
	live LiveDataSource
}

func NewEventsHandler(live LiveDataSource) *EventsHandler {
	return &EventsHandler{live: live}
}

// StreamEvents opens an SSE connection and pushes filtered events.
func (h *EventsHandler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	if h.live == nil {
		WriteError(w, http.StatusServiceUnavailable, "event streaming not available")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Parse filter parameters
	filter := EventFilter{
		Systems: QueryIntList(r, "systems"),
		Sites:   QueryIntList(r, "sites"),
		Tgids:   QueryIntList(r, "tgids"),
		Units:   QueryIntList(r, "units"),
	}
	if v, ok := QueryString(r, "types"); ok {
		filter.Types = strings.Split(v, ",")
	}
	if v, ok := QueryBool(r, "emergency_only"); ok {
		filter.EmergencyOnly = v
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Replay missed events if Last-Event-ID is provided
	lastEventID := r.Header.Get("Last-Event-ID")
	if lastEventID != "" {
		events := h.live.ReplaySince(lastEventID, filter)
		for _, e := range events {
			fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", e.ID, e.Type, e.Data)
		}
		flusher.Flush()
	}

	// Subscribe to new events
	ch, cancel := h.live.Subscribe(filter)
	defer cancel()

	// Keepalive ticker
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	log := hlog.FromRequest(r)
	log.Info().Msg("SSE client connected")

	for {
		select {
		case <-r.Context().Done():
			log.Info().Msg("SSE client disconnected")
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.Type, event.Data)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// Routes registers event routes on the given router.
func (h *EventsHandler) Routes(r chi.Router) {
	r.Get("/events/stream", h.StreamEvents)
}
