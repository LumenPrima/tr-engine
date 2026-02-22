package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/database"
)

type StatsHandler struct {
	db *database.DB
}

func NewStatsHandler(db *database.DB) *StatsHandler {
	return &StatsHandler{db: db}
}

// GetStats returns overall system statistics.
func (h *StatsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.GetStats(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}
	WriteJSON(w, http.StatusOK, stats)
}

// GetDecodeRates returns decode rate measurements over time.
func (h *StatsHandler) GetDecodeRates(w http.ResponseWriter, r *http.Request) {
	filter := database.DecodeRateFilter{}
	if t, ok := QueryTime(r, "start_time"); ok {
		filter.StartTime = &t
	}
	if t, ok := QueryTime(r, "end_time"); ok {
		filter.EndTime = &t
	}

	rates, err := h.db.GetDecodeRates(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get decode rates")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"rates": rates,
		"total": len(rates),
	})
}

// ListTrunkingMessages returns paginated trunking messages.
func (h *StatsHandler) ListTrunkingMessages(w http.ResponseWriter, r *http.Request) {
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	filter := database.TrunkingMessageFilter{
		Limit:  p.Limit,
		Offset: p.Offset,
	}
	filter.SystemIDs = QueryIntList(r, "system_id")
	if v, ok := QueryString(r, "opcode"); ok {
		filter.Opcode = &v
	}
	if v, ok := QueryString(r, "opcode_type"); ok {
		filter.OpcodeType = &v
	}
	if t, ok := QueryTime(r, "start_time"); ok {
		filter.StartTime = &t
	}
	if t, ok := QueryTime(r, "end_time"); ok {
		filter.EndTime = &t
	}

	messages, total, err := h.db.ListTrunkingMessages(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list trunking messages")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"messages": messages,
		"total":    total,
	})
}

// ListConsoleMessages returns paginated console log messages.
func (h *StatsHandler) ListConsoleMessages(w http.ResponseWriter, r *http.Request) {
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	filter := database.ConsoleMessageFilter{
		Limit:  p.Limit,
		Offset: p.Offset,
	}
	if v, ok := QueryString(r, "instance_id"); ok {
		filter.InstanceID = &v
	}
	if v, ok := QueryString(r, "severity"); ok {
		filter.Severity = &v
	}
	if t, ok := QueryTime(r, "start_time"); ok {
		filter.StartTime = &t
	}
	if t, ok := QueryTime(r, "end_time"); ok {
		filter.EndTime = &t
	}

	messages, total, err := h.db.ListConsoleMessages(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list console messages")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"messages": messages,
		"total":    total,
	})
}

// GetTalkgroupActivity returns call counts grouped by talkgroup for a time range.
func (h *StatsHandler) GetTalkgroupActivity(w http.ResponseWriter, r *http.Request) {
	filter := database.TalkgroupActivityFilter{}
	filter.SystemIDs = QueryIntListAliased(r, "system_id", "systems")
	filter.SiteIDs = QueryIntListAliased(r, "site_id", "sites")
	filter.Tgids = QueryIntListAliased(r, "tgid", "tgids")

	if t, ok := QueryTime(r, "after"); ok {
		filter.After = &t
	}
	if t, ok := QueryTime(r, "before"); ok {
		filter.Before = &t
	}
	if v, ok := QueryInt(r, "limit"); ok {
		filter.Limit = v
	}
	if v, ok := QueryInt(r, "offset"); ok {
		filter.Offset = v
	}
	if v, ok := QueryString(r, "sort"); ok {
		filter.SortField = v
	}

	activity, total, err := h.db.GetTalkgroupActivity(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get talkgroup activity")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"activity": activity,
		"total":    total,
	})
}

// Routes registers stats routes on the given router.
func (h *StatsHandler) Routes(r chi.Router) {
	r.Get("/stats", h.GetStats)
	r.Get("/stats/rates", h.GetDecodeRates)
	r.Get("/stats/talkgroup-activity", h.GetTalkgroupActivity)
	r.Get("/trunking-messages", h.ListTrunkingMessages)
	r.Get("/console-messages", h.ListConsoleMessages)
}
