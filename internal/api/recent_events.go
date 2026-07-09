package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/database"
)

type recentEventQuerier interface {
	GetRecentEvents(ctx context.Context, filter database.RecentEventFilter) ([]database.RecentEventAPI, int, error)
}

type RecentEventsHandler struct {
	db      *database.DB
	querier recentEventQuerier
}

func NewRecentEventsHandler(db *database.DB) *RecentEventsHandler {
	return &RecentEventsHandler{db: db, querier: db}
}

func (h *RecentEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, err := ParsePagination(r)
	if err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, err.Error())
		return
	}

	filter := database.RecentEventFilter{
		SystemIDs: QueryIntListAliased(r, "system_id", "systems"),
		SiteIDs:   QueryIntListAliased(r, "site_id", "sites"),
		Tgids:     QueryIntListAliased(r, "tgid", "tgids"),
		UnitIDs:   QueryIntListAliased(r, "unit_id", "units", "unit_ids"),
		Limit:     p.Limit,
		Offset:    p.Offset,
	}

	if v, ok := QueryString(r, "types"); ok {
		types := strings.Split(v, ",")
		for _, eventType := range types {
			eventType = strings.TrimSpace(eventType)
			if eventType != "" {
				filter.Types = append(filter.Types, eventType)
			}
		}
	}
	if t, ok := QueryTime(r, "start_time"); ok {
		filter.StartTime = &t
	}
	if t, ok := QueryTime(r, "end_time"); ok {
		filter.EndTime = &t
	}
	if msg := ValidateTimeRange(filter.StartTime, filter.EndTime); msg != "" {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidTimeRange, msg)
		return
	}
	if exceeds24Hours(filter.StartTime, filter.EndTime) {
		WriteError(w, http.StatusBadRequest, "time range cannot exceed 24 hours")
		return
	}

	querier := h.querier
	if querier == nil {
		querier = h.db
	}

	events, total, err := querier.GetRecentEvents(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list recent events")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
	})
}

func (h *RecentEventsHandler) Routes(r chi.Router) {
	r.Get("/recent-events", h.ServeHTTP)
}

func exceeds24Hours(startTime, endTime *time.Time) bool {
	if startTime == nil {
		return false
	}
	effectiveEnd := time.Now().UTC()
	if endTime != nil {
		effectiveEnd = endTime.UTC()
	}
	return effectiveEnd.Sub(startTime.UTC()) > 24*time.Hour
}
