package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type RecordersHandler struct {
	live LiveDataSource
}

func NewRecordersHandler(live LiveDataSource) *RecordersHandler {
	return &RecordersHandler{live: live}
}

// ListRecorders returns all known recorder states from in-memory cache.
func (h *RecordersHandler) ListRecorders(w http.ResponseWriter, r *http.Request) {
	if h.live == nil {
		WriteJSON(w, http.StatusOK, map[string]any{
			"recorders": []any{},
			"total":     0,
		})
		return
	}

	recorders := h.live.LatestRecorders()
	WriteJSON(w, http.StatusOK, map[string]any{
		"recorders": recorders,
		"total":     len(recorders),
	})
}

// Routes registers recorder routes on the given router.
func (h *RecordersHandler) Routes(r chi.Router) {
	r.Get("/recorders", h.ListRecorders)
}
