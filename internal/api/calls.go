package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/audio"
	"github.com/snarg/tr-engine/internal/database"
)

type CallsHandler struct {
	db         *database.DB
	audioDir   string
	trAudioDir string
	live       LiveDataSource
}

func NewCallsHandler(db *database.DB, audioDir, trAudioDir string, live LiveDataSource) *CallsHandler {
	return &CallsHandler{db: db, audioDir: audioDir, trAudioDir: trAudioDir, live: live}
}

// enrichAudioURLs sets audio_url on calls that have a call_filename but no
// audio_file_path, when TR_AUDIO_DIR mode is active.
func (h *CallsHandler) enrichAudioURLs(calls []database.CallAPI) {
	if h.trAudioDir == "" {
		return
	}
	for i := range calls {
		if calls[i].AudioURL == nil && calls[i].CallFilename != "" {
			url := fmt.Sprintf("/api/v1/calls/%d/audio", calls[i].CallID)
			calls[i].AudioURL = &url
		}
	}
}

var callSortFields = map[string]string{
	"start_time": "c.start_time",
	"stop_time":  "c.stop_time",
	"duration":   "c.duration",
	"tgid":       "c.tgid",
	"freq":       "c.freq",
}

// ListCalls returns calls with comprehensive filters.
func (h *CallsHandler) ListCalls(w http.ResponseWriter, r *http.Request) {
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	sort := ParseSort(r, "-start_time", callSortFields)

	filter := database.CallFilter{
		Limit:  p.Limit,
		Offset: p.Offset,
		Sort:   sort.SQLOrderBy(callSortFields),
	}

	filter.Sysids = QueryStringListAliased(r, "sysid", "sysids")
	filter.SystemIDs = QueryIntListAliased(r, "system_id", "systems")
	filter.SiteIDs = QueryIntListAliased(r, "site_id", "sites")
	filter.Tgids = QueryIntListAliased(r, "tgid", "tgids")
	filter.UnitIDs = QueryIntListAliased(r, "unit_id", "units", "unit_ids")
	if v, ok := QueryBool(r, "emergency"); ok {
		filter.Emergency = &v
	}
	if v, ok := QueryBool(r, "encrypted"); ok {
		filter.Encrypted = &v
	}
	if v, ok := QueryBool(r, "deduplicate"); ok {
		filter.Deduplicate = v
	}
	if t, ok := QueryTime(r, "start_time"); ok {
		filter.StartTime = &t
	}
	if t, ok := QueryTime(r, "end_time"); ok {
		filter.EndTime = &t
	}
	if msg := ValidateTimeRange(filter.StartTime, filter.EndTime); msg != "" {
		WriteError(w, http.StatusBadRequest, msg)
		return
	}

	calls, total, err := h.db.ListCalls(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list calls")
		return
	}
	h.enrichAudioURLs(calls)
	WriteJSON(w, http.StatusOK, map[string]any{
		"calls":  calls,
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
	})
}

// ListActiveCalls returns currently active calls from the in-memory MQTT tracker.
func (h *CallsHandler) ListActiveCalls(w http.ResponseWriter, r *http.Request) {
	if h.live == nil {
		WriteJSON(w, http.StatusOK, map[string]any{
			"calls": []any{},
			"total": 0,
		})
		return
	}

	calls := h.live.ActiveCalls()

	// Apply filters
	sysid, hasSysid := QueryString(r, "sysid")
	tgid, hasTgid := QueryInt(r, "tgid")
	emergency, hasEmergency := QueryBool(r, "emergency")
	encrypted, hasEncrypted := QueryBool(r, "encrypted")

	if hasSysid || hasTgid || hasEmergency || hasEncrypted {
		filtered := make([]ActiveCallData, 0, len(calls))
		for _, c := range calls {
			if hasSysid && c.Sysid != sysid {
				continue
			}
			if hasTgid && c.Tgid != tgid {
				continue
			}
			if hasEmergency && c.Emergency != emergency {
				continue
			}
			if hasEncrypted && c.Encrypted != encrypted {
				continue
			}
			filtered = append(filtered, c)
		}
		calls = filtered
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"calls": calls,
		"total": len(calls),
	})
}

// GetCall returns a single call by ID.
func (h *CallsHandler) GetCall(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	call, err := h.db.GetCallByID(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "call not found")
		return
	}
	if h.trAudioDir != "" && call.AudioURL == nil && call.CallFilename != "" {
		url := fmt.Sprintf("/api/v1/calls/%d/audio", call.CallID)
		call.AudioURL = &url
	}
	WriteJSON(w, http.StatusOK, call)
}

// GetCallAudio streams the audio file for a call.
func (h *CallsHandler) GetCallAudio(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	audioPath, callFilename, err := h.db.GetCallAudioPath(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "audio not found")
		return
	}

	fullPath := h.resolveAudioFile(audioPath, callFilename)
	if fullPath == "" {
		WriteError(w, http.StatusNotFound, "audio file not found on disk")
		return
	}

	// Set content type based on extension
	ext := strings.ToLower(filepath.Ext(fullPath))
	contentTypes := map[string]string{
		".m4a": "audio/mp4",
		".mp3": "audio/mpeg",
		".wav": "audio/wav",
		".ogg": "audio/ogg",
	}
	if ct, ok := contentTypes[ext]; ok {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%d%s"`, id, ext))
	http.ServeFile(w, r, fullPath)
}

// resolveAudioFile finds the audio file on disk.
func (h *CallsHandler) resolveAudioFile(audioPath, callFilename string) string {
	return audio.ResolveFile(h.audioDir, h.trAudioDir, audioPath, callFilename)
}

// GetCallFrequencies returns frequency entries for a call.
func (h *CallsHandler) GetCallFrequencies(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	freqs, err := h.db.GetCallFrequencies(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "call not found")
		return
	}

	total := len(freqs)
	p, _ := ParsePagination(r)
	if p.Offset > 0 || p.Limit < total {
		if p.Offset >= total {
			freqs = []database.CallFrequencyAPI{}
		} else {
			end := p.Offset + p.Limit
			if end > total {
				end = total
			}
			freqs = freqs[p.Offset:end]
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"frequencies": freqs,
		"total":       total,
		"limit":       p.Limit,
		"offset":      p.Offset,
	})
}

// GetCallTransmissions returns transmission entries for a call.
func (h *CallsHandler) GetCallTransmissions(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	txs, err := h.db.GetCallTransmissions(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "call not found")
		return
	}

	total := len(txs)
	p, _ := ParsePagination(r)
	if p.Offset > 0 || p.Limit < total {
		if p.Offset >= total {
			txs = []database.CallTransmissionAPI{}
		} else {
			end := p.Offset + p.Limit
			if end > total {
				end = total
			}
			txs = txs[p.Offset:end]
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"transmissions": txs,
		"total":         total,
		"limit":         p.Limit,
		"offset":        p.Offset,
	})
}

// Routes registers call routes on the given router.
func (h *CallsHandler) Routes(r chi.Router) {
	r.Get("/calls", h.ListCalls)
	r.Get("/calls/active", h.ListActiveCalls)
	r.Get("/calls/{id}", h.GetCall)
	r.Get("/calls/{id}/audio", h.GetCallAudio)
	r.Get("/calls/{id}/frequencies", h.GetCallFrequencies)
	r.Get("/calls/{id}/transmissions", h.GetCallTransmissions)
}
