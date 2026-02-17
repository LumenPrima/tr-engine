package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/database"
)

type UnitEventsHandler struct {
	db *database.DB
}

func NewUnitEventsHandler(db *database.DB) *UnitEventsHandler {
	return &UnitEventsHandler{db: db}
}

var unitEventSortFields = map[string]string{
	"time":       "ue.time",
	"unit_rid":   "ue.unit_rid",
	"tgid":       "ue.tgid",
	"event_type": "ue.event_type",
}

// ListUnitEventsGlobal returns unit events across a system with comprehensive filters.
func (h *UnitEventsHandler) ListUnitEventsGlobal(w http.ResponseWriter, r *http.Request) {
	// Require system_id or sysid
	systemIDs := QueryIntList(r, "system_id")
	sysids := QueryStringList(r, "sysid")
	if len(systemIDs) == 0 && len(sysids) == 0 {
		WriteError(w, http.StatusBadRequest, "system_id or sysid is required")
		return
	}

	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	sort := ParseSort(r, "-time", unitEventSortFields)

	filter := database.GlobalUnitEventFilter{
		SystemIDs: systemIDs,
		Sysids:    sysids,
		Limit:     p.Limit,
		Offset:    p.Offset,
		Sort:      sort.SQLOrderBy(unitEventSortFields),
	}

	filter.UnitIDs = QueryIntList(r, "unit_id")
	if v, ok := QueryString(r, "type"); ok {
		types := strings.Split(v, ",")
		for i := range types {
			types[i] = strings.TrimSpace(types[i])
		}
		filter.EventTypes = types
	}
	filter.Tgids = QueryIntList(r, "tgid")
	if v, ok := QueryBool(r, "emergency"); ok {
		filter.Emergency = &v
	}
	if t, ok := QueryTime(r, "start_time"); ok {
		filter.StartTime = &t
	}
	if t, ok := QueryTime(r, "end_time"); ok {
		filter.EndTime = &t
	}

	// Enforce max 24h range
	if filter.StartTime != nil && filter.EndTime != nil {
		if filter.EndTime.Sub(*filter.StartTime) > 24*time.Hour {
			WriteError(w, http.StatusBadRequest, "time range cannot exceed 24 hours")
			return
		}
	}

	events, total, err := h.db.ListUnitEventsGlobal(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list unit events")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
	})
}

func (h *UnitEventsHandler) Routes(r chi.Router) {
	r.Get("/unit-events", h.ListUnitEventsGlobal)
}
