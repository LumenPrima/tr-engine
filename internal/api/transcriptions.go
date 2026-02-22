package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/database"
)

type TranscriptionsHandler struct {
	db   *database.DB
	live LiveDataSource
}

func NewTranscriptionsHandler(db *database.DB, live LiveDataSource) *TranscriptionsHandler {
	return &TranscriptionsHandler{db: db, live: live}
}

func (h *TranscriptionsHandler) Routes(r chi.Router) {
	r.Get("/calls/{id}/transcription", h.GetCallTranscription)
	r.Get("/calls/{id}/transcriptions", h.ListCallTranscriptions)
	r.Put("/calls/{id}/transcription", h.SubmitCorrection)
	r.Post("/calls/{id}/transcribe", h.TranscribeCall)
	r.Post("/calls/{id}/transcription/verify", h.VerifyTranscription)
	r.Post("/calls/{id}/transcription/reject", h.RejectTranscription)
	r.Post("/calls/{id}/transcription/exclude", h.ExcludeFromDataset)
	r.Get("/transcriptions/search", h.SearchTranscriptions)
	r.Get("/transcriptions/queue", h.GetQueueStats)
}

// GetCallTranscription returns the primary transcription for a call.
func (h *TranscriptionsHandler) GetCallTranscription(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	t, err := h.db.GetPrimaryTranscription(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "no transcription found")
		return
	}
	WriteJSON(w, http.StatusOK, t)
}

// ListCallTranscriptions returns all transcription variants for a call.
func (h *TranscriptionsHandler) ListCallTranscriptions(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	transcriptions, err := h.db.ListTranscriptionsByCall(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list transcriptions")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"transcriptions": transcriptions,
		"total":          len(transcriptions),
	})
}

// SubmitCorrection accepts a human correction for a call's transcription.
func (h *TranscriptionsHandler) SubmitCorrection(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	var body struct {
		Text     string          `json:"text"`
		Source   string          `json:"source"`   // default "human"
		Provider string          `json:"provider"` // default ""
		Language string          `json:"language"` // default ""
		Words    json.RawMessage `json:"words"`    // optional pre-built segments
	}
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Text == "" {
		WriteError(w, http.StatusBadRequest, "text is required")
		return
	}

	source := body.Source
	if source == "" {
		source = "human"
	}

	// Look up the call to get start_time for partitioned insert
	call, err := h.db.GetCallForTranscription(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "call not found")
		return
	}

	row := &database.TranscriptionRow{
		CallID:        call.CallID,
		CallStartTime: call.StartTime,
		Text:          body.Text,
		Source:        source,
		IsPrimary:     true,
		Provider:      body.Provider,
		Language:      body.Language,
		Words:         body.Words,
	}

	txID, err := h.db.InsertTranscription(r.Context(), row)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to save correction")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"id":      txID,
		"call_id": call.CallID,
		"source":  source,
	})
}

// TranscribeCall enqueues a call for (re-)transcription.
func (h *TranscriptionsHandler) TranscribeCall(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	if h.live == nil {
		WriteError(w, http.StatusServiceUnavailable, "transcription not available")
		return
	}

	if !h.live.EnqueueTranscription(id) {
		WriteError(w, http.StatusServiceUnavailable, "transcription queue full or not configured")
		return
	}
	WriteJSON(w, http.StatusAccepted, map[string]any{
		"call_id": id,
		"status":  "queued",
	})
}

// VerifyTranscription marks a transcription as verified.
func (h *TranscriptionsHandler) VerifyTranscription(w http.ResponseWriter, r *http.Request) {
	h.setTranscriptionStatus(w, r, "verified")
}

// RejectTranscription marks a transcription as rejected.
func (h *TranscriptionsHandler) RejectTranscription(w http.ResponseWriter, r *http.Request) {
	h.setTranscriptionStatus(w, r, "reviewed")
}

// ExcludeFromDataset marks a transcription as excluded from training datasets.
func (h *TranscriptionsHandler) ExcludeFromDataset(w http.ResponseWriter, r *http.Request) {
	h.setTranscriptionStatus(w, r, "excluded")
}

func (h *TranscriptionsHandler) setTranscriptionStatus(w http.ResponseWriter, r *http.Request, status string) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	call, err := h.db.GetCallForTranscription(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "call not found")
		return
	}

	if err := h.db.UpdateCallTranscriptionStatus(r.Context(), call.CallID, call.StartTime, status); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"call_id": call.CallID,
		"status":  status,
	})
}

// SearchTranscriptions performs full-text search across transcriptions.
func (h *TranscriptionsHandler) SearchTranscriptions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		WriteError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	filter := database.TranscriptionSearchFilter{
		SystemIDs: QueryIntListAliased(r, "system_id", "systems"),
		SiteIDs:   QueryIntListAliased(r, "site_id", "sites"),
		Tgids:     QueryIntListAliased(r, "tgid", "tgids"),
		Limit:     p.Limit,
		Offset:    p.Offset,
	}
	if t, ok := QueryTime(r, "start_time"); ok {
		filter.StartTime = &t
	}
	if t, ok := QueryTime(r, "end_time"); ok {
		filter.EndTime = &t
	}

	hits, total, err := h.db.SearchTranscriptions(r.Context(), q, filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "search failed")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"results": hits,
		"total":   total,
		"limit":   p.Limit,
		"offset":  p.Offset,
	})
}

// GetQueueStats returns transcription queue statistics.
func (h *TranscriptionsHandler) GetQueueStats(w http.ResponseWriter, r *http.Request) {
	if h.live == nil {
		WriteJSON(w, http.StatusOK, map[string]any{
			"status": "not_configured",
		})
		return
	}

	stats := h.live.TranscriptionQueueStats()
	if stats == nil {
		WriteJSON(w, http.StatusOK, map[string]any{
			"status": "not_configured",
		})
		return
	}

	// Encode via json round-trip to use struct tags
	raw, _ := json.Marshal(stats)
	var result map[string]any
	json.Unmarshal(raw, &result)
	result["status"] = "ok"
	WriteJSON(w, http.StatusOK, result)
}
