package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/database"
)

const (
	defaultDashboardSummaryHours    = 24
	maxDashboardSummaryHours        = 720
	defaultDashboardSummaryTopLimit = 10
	maxDashboardSummaryTopLimit     = 100
)

type DashboardSummaryHandler struct {
	db        *database.DB
	live      LiveDataSource
	version   string
	startTime time.Time
}

type dashboardSummaryResponse struct {
	GeneratedAt   time.Time                       `json:"generated_at"`
	Health        dashboardSummaryHealth          `json:"health"`
	Stats         dashboardSummaryStats           `json:"stats"`
	ActiveCalls   []ActiveCallData                `json:"active_calls"`
	TopTalkgroups dashboardSummaryTopTalkgroups   `json:"top_talkgroups"`
}

type dashboardSummaryHealth struct {
	Database      string `json:"database"`
	Mqtt          string `json:"mqtt"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type dashboardSummaryStats struct {
	Systems            int     `json:"systems"`
	Sites              int     `json:"sites"`
	Talkgroups         int     `json:"talkgroups"`
	Units              int     `json:"units"`
	TotalCalls         int     `json:"total_calls"`
	Calls24h           int     `json:"calls_24h"`
	Calls1h            int     `json:"calls_1h"`
	TotalDurationHours float64 `json:"total_duration_hours"`
}

type dashboardSummaryTopTalkgroups struct {
	Activity []database.TalkgroupActivity `json:"activity"`
	Total    int                          `json:"total"`
	Hours    int                          `json:"hours"`
	Limit    int                          `json:"limit"`
}

func NewDashboardSummaryHandler(db *database.DB, live LiveDataSource, version string, startTime time.Time) *DashboardSummaryHandler {
	return &DashboardSummaryHandler{
		db:        db,
		live:      live,
		version:   version,
		startTime: startTime,
	}
}

func (h *DashboardSummaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hours, err := parseDashboardSummaryIntParam(r, "hours", defaultDashboardSummaryHours, 1, maxDashboardSummaryHours)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	topLimit, err := parseDashboardSummaryIntParam(r, "top_limit", defaultDashboardSummaryTopLimit, 1, maxDashboardSummaryTopLimit)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.db.Pool.Ping(r.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get dashboard summary")
		return
	}

	stats, err := aggregateStats(r.Context(), h.db, hours)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get dashboard summary")
		return
	}

	topTalkgroups, total, err := h.getTopTalkgroups(r.Context(), hours, topLimit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get dashboard summary")
		return
	}

	activeCalls := []ActiveCallData{}
	if h.live != nil {
		activeCalls = h.live.ActiveCalls()
		if activeCalls == nil {
			activeCalls = []ActiveCallData{}
		}
	}

	resp := dashboardSummaryResponse{
		GeneratedAt: time.Now().UTC(),
		Health: dashboardSummaryHealth{
			Database:      "ok",
			Mqtt:          dashboardSummaryMQTTStatus(h.live),
			Version:       h.version,
			UptimeSeconds: int64(time.Since(h.startTime).Seconds()),
		},
		Stats:       stats,
		ActiveCalls: activeCalls,
		TopTalkgroups: dashboardSummaryTopTalkgroups{
			Activity: topTalkgroups,
			Total:    total,
			Hours:    hours,
			Limit:    topLimit,
		},
	}

	WriteJSON(w, http.StatusOK, resp)
}

func aggregateStats(ctx context.Context, db *database.DB, hours int) (dashboardSummaryStats, error) {
	_ = hours

	stats, err := db.GetStats(ctx)
	if err != nil {
		return dashboardSummaryStats{}, err
	}

	sites, err := db.LoadAllSitesAPI(ctx)
	if err != nil {
		return dashboardSummaryStats{}, err
	}

	return dashboardSummaryStats{
		Systems:            stats.Systems,
		Sites:              len(sites),
		Talkgroups:         stats.Talkgroups,
		Units:              stats.Units,
		TotalCalls:         stats.TotalCalls,
		Calls24h:           stats.Calls24h,
		Calls1h:            stats.Calls1h,
		TotalDurationHours: stats.TotalDurationHours,
	}, nil
}

func (h *DashboardSummaryHandler) getTopTalkgroups(ctx context.Context, hours, topLimit int) ([]database.TalkgroupActivity, int, error) {
	before := time.Now().UTC()
	after := before.Add(-time.Duration(hours) * time.Hour)

	activity, total, err := h.db.GetTalkgroupActivity(ctx, database.TalkgroupActivityFilter{
		After:  &after,
		Before: &before,
		Limit:  topLimit,
		Offset: 0,
	})
	if err != nil {
		return nil, 0, err
	}
	return activity, total, nil
}

func parseDashboardSummaryIntParam(r *http.Request, name string, defaultValue, minValue, maxValue int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return defaultValue, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, &dashboardSummaryParamError{name: name, msg: "must be an integer"}
	}
	if n < minValue || n > maxValue {
		return 0, &dashboardSummaryParamError{name: name, msg: "must be between " + strconv.Itoa(minValue) + " and " + strconv.Itoa(maxValue)}
	}
	return n, nil
}

type dashboardSummaryParamError struct {
	name string
	msg  string
}

func (e *dashboardSummaryParamError) Error() string {
	return e.name + " " + e.msg
}

func (h *DashboardSummaryHandler) Routes(r chi.Router) {
	r.Get("/dashboard/summary", h.ServeHTTP)
}

func dashboardSummaryMQTTStatus(live LiveDataSource) string {
	if live == nil {
		return "not_configured"
	}

	statuses := live.TRInstanceStatus()
	if len(statuses) > 0 {
		for _, status := range statuses {
			if status.Status == "connected" {
				return "ok"
			}
		}
		return "disconnected"
	}

	if live.IngestMetrics() != nil {
		return "ok"
	}

	return "not_configured"
}
