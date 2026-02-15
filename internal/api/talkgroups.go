package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/database"
)

type TalkgroupsHandler struct {
	db *database.DB
}

func NewTalkgroupsHandler(db *database.DB) *TalkgroupsHandler {
	return &TalkgroupsHandler{db: db}
}

var talkgroupSortFields = map[string]string{
	"alpha_tag":  "t.alpha_tag",
	"tgid":       "t.tgid",
	"group":      `t."group"`,
	"last_seen":  "t.last_seen",
	"call_count": "COALESCE(ts.call_count, 0)",
	"calls_1h":   "COALESCE(ts.calls_1h, 0)",
	"calls_24h":  "COALESCE(ts.calls_24h, 0)",
}

// ListTalkgroups returns talkgroups with embedded stats.
func (h *TalkgroupsHandler) ListTalkgroups(w http.ResponseWriter, r *http.Request) {
	p := ParsePagination(r)
	sort := ParseSort(r, "alpha_tag", talkgroupSortFields)

	filter := database.TalkgroupFilter{
		Limit:  p.Limit,
		Offset: p.Offset,
		Sort:   sort.SQLOrderBy(talkgroupSortFields),
	}

	if v, ok := QueryInt(r, "system_id"); ok {
		filter.SystemID = &v
	}
	if v, ok := QueryString(r, "sysid"); ok {
		filter.Sysid = &v
	}
	if v, ok := QueryString(r, "group"); ok {
		filter.Group = &v
	}
	if v, ok := QueryString(r, "search"); ok {
		filter.Search = &v
	}

	talkgroups, total, err := h.db.ListTalkgroups(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list talkgroups")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"talkgroups": talkgroups,
		"total":      total,
		"limit":      p.Limit,
		"offset":     p.Offset,
	})
}

// GetTalkgroup returns a single talkgroup by composite or plain ID.
func (h *TalkgroupsHandler) GetTalkgroup(w http.ResponseWriter, r *http.Request) {
	cid, err := ParseCompositeID(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if cid.IsPlain {
		matches, err := h.db.FindTalkgroupSystems(r.Context(), cid.EntityID)
		if err != nil || len(matches) == 0 {
			WriteError(w, http.StatusNotFound, "talkgroup not found")
			return
		}
		if len(matches) > 1 {
			WriteAmbiguous(w, cid.EntityID, matches)
			return
		}
		cid.SystemID = matches[0].SystemID
	}

	tg, err := h.db.GetTalkgroupByComposite(r.Context(), cid.SystemID, cid.EntityID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "talkgroup not found")
		return
	}
	WriteJSON(w, http.StatusOK, tg)
}

// UpdateTalkgroup patches talkgroup metadata.
func (h *TalkgroupsHandler) UpdateTalkgroup(w http.ResponseWriter, r *http.Request) {
	cid, err := ParseCompositeID(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if cid.IsPlain {
		matches, err := h.db.FindTalkgroupSystems(r.Context(), cid.EntityID)
		if err != nil || len(matches) == 0 {
			WriteError(w, http.StatusNotFound, "talkgroup not found")
			return
		}
		if len(matches) > 1 {
			WriteAmbiguous(w, cid.EntityID, matches)
			return
		}
		cid.SystemID = matches[0].SystemID
	}

	var patch struct {
		AlphaTag    *string `json:"alpha_tag"`
		Description *string `json:"description"`
		Group       *string `json:"group"`
		Tag         *string `json:"tag"`
		Priority    *int    `json:"priority"`
	}
	if err := DecodeJSON(r, &patch); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.db.UpdateTalkgroupFields(r.Context(), cid.SystemID, cid.EntityID,
		patch.AlphaTag, patch.Description, patch.Group, patch.Tag, patch.Priority); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to update talkgroup")
		return
	}

	tg, err := h.db.GetTalkgroupByComposite(r.Context(), cid.SystemID, cid.EntityID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "talkgroup not found")
		return
	}
	WriteJSON(w, http.StatusOK, tg)
}

// ListTalkgroupCalls returns calls for a specific talkgroup.
func (h *TalkgroupsHandler) ListTalkgroupCalls(w http.ResponseWriter, r *http.Request) {
	cid, err := ParseCompositeID(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if cid.IsPlain {
		matches, err := h.db.FindTalkgroupSystems(r.Context(), cid.EntityID)
		if err != nil || len(matches) == 0 {
			WriteError(w, http.StatusNotFound, "talkgroup not found")
			return
		}
		if len(matches) > 1 {
			WriteAmbiguous(w, cid.EntityID, matches)
			return
		}
		cid.SystemID = matches[0].SystemID
	}

	p := ParsePagination(r)
	filter := database.CallFilter{
		Limit:    p.Limit,
		Offset:   p.Offset,
		SystemID: &cid.SystemID,
		Tgid:     &cid.EntityID,
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
	WriteJSON(w, http.StatusOK, map[string]any{
		"calls":  calls,
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
	})
}

// ListTalkgroupUnits returns units affiliated with a talkgroup.
func (h *TalkgroupsHandler) ListTalkgroupUnits(w http.ResponseWriter, r *http.Request) {
	cid, err := ParseCompositeID(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if cid.IsPlain {
		matches, err := h.db.FindTalkgroupSystems(r.Context(), cid.EntityID)
		if err != nil || len(matches) == 0 {
			WriteError(w, http.StatusNotFound, "talkgroup not found")
			return
		}
		if len(matches) > 1 {
			WriteAmbiguous(w, cid.EntityID, matches)
			return
		}
		cid.SystemID = matches[0].SystemID
	}

	p := ParsePagination(r)
	window := 60
	if v, ok := QueryInt(r, "window"); ok && v >= 1 && v <= 1440 {
		window = v
	}

	units, total, err := h.db.ListTalkgroupUnits(r.Context(), cid.SystemID, cid.EntityID, window, p.Limit, p.Offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list units")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"units":  units,
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
	})
}

// GetEncryptionStats returns encryption stats per talkgroup.
func (h *TalkgroupsHandler) GetEncryptionStats(w http.ResponseWriter, r *http.Request) {
	hours := 24
	if v, ok := QueryInt(r, "hours"); ok && v >= 1 {
		hours = v
	}
	sysid, _ := QueryString(r, "sysid")

	stats, err := h.db.GetEncryptionStats(r.Context(), hours, sysid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get encryption stats")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"stats": stats,
		"total": len(stats),
		"hours": hours,
	})
}

// Routes registers talkgroup routes on the given router.
func (h *TalkgroupsHandler) Routes(r chi.Router) {
	r.Get("/talkgroups", h.ListTalkgroups)
	r.Get("/talkgroups/encryption-stats", h.GetEncryptionStats)
	r.Get("/talkgroups/{id}", h.GetTalkgroup)
	r.Patch("/talkgroups/{id}", h.UpdateTalkgroup)
	r.Get("/talkgroups/{id}/calls", h.ListTalkgroupCalls)
	r.Get("/talkgroups/{id}/units", h.ListTalkgroupUnits)
}
