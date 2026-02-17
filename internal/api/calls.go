package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
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

	filter.Sysids = QueryStringList(r, "sysid")
	filter.SystemIDs = QueryIntList(r, "system_id")
	filter.SiteIDs = QueryIntList(r, "site_id")
	filter.Tgids = QueryIntList(r, "tgid")
	filter.UnitIDs = QueryIntList(r, "unit_id")
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
// Priority: 1) AUDIO_DIR/audioPath, 2) TR_AUDIO_DIR + call_filename, 3) call_filename as absolute path.
func (h *CallsHandler) resolveAudioFile(audioPath, callFilename string) string {
	// 1) tr-engine managed audio file
	if audioPath != "" {
		full := filepath.Join(h.audioDir, audioPath)
		if _, err := os.Stat(full); err == nil {
			return full
		}
	}

	if callFilename == "" {
		return ""
	}

	// 2) TR_AUDIO_DIR configured — resolve call_filename relative to it
	if h.trAudioDir != "" {
		// call_filename is TR's absolute path (e.g. /app/tr_audio/warco/2026/2/17/file.m4a)
		// Try it directly under TR_AUDIO_DIR by extracting the basename
		// and also try the full relative structure
		full := filepath.Join(h.trAudioDir, filepath.Base(callFilename))
		if _, err := os.Stat(full); err == nil {
			return full
		}

		// Try matching: find the short_name directory in call_filename
		// and use everything from there as a relative path under TR_AUDIO_DIR.
		// e.g. /app/tr_audio/warco/2026/2/17/file.m4a → warco/2026/2/17/file.m4a
		parts := strings.Split(filepath.ToSlash(callFilename), "/")
		for i := range parts {
			if i == 0 {
				continue
			}
			candidate := filepath.Join(h.trAudioDir, filepath.Join(parts[i:]...))
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	// 3) Try call_filename as an absolute path (same machine, same filesystem)
	if filepath.IsAbs(callFilename) {
		if _, err := os.Stat(callFilename); err == nil {
			return callFilename
		}
	}

	return ""
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
	WriteJSON(w, http.StatusOK, map[string]any{
		"frequencies": freqs,
		"total":       len(freqs),
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
	WriteJSON(w, http.StatusOK, map[string]any{
		"transmissions": txs,
		"total":         len(txs),
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
