package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/snarg/tr-engine/internal/database"
	"github.com/snarg/tr-engine/internal/mqttclient"
)

type HealthResponse struct {
	Status         string                      `json:"status"`
	Version        string                      `json:"version"`
	UptimeSeconds  int64                       `json:"uptime_seconds"`
	Checks         map[string]string           `json:"checks"`
	TrunkRecorders []TRInstanceStatusData       `json:"trunk_recorders,omitempty"`
}

type HealthHandler struct {
	db        *database.DB
	mqtt      *mqttclient.Client
	live      LiveDataSource
	version   string
	startTime time.Time
}

func NewHealthHandler(db *database.DB, mqtt *mqttclient.Client, live LiveDataSource, version string, startTime time.Time) *HealthHandler {
	return &HealthHandler{
		db:        db,
		mqtt:      mqtt,
		live:      live,
		version:   version,
		startTime: startTime,
	}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	status := "healthy"
	httpStatus := http.StatusOK

	// Database check
	if err := h.db.HealthCheck(r.Context()); err != nil {
		checks["database"] = "error"
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	} else {
		checks["database"] = "ok"
	}

	// MQTT check
	if h.mqtt != nil {
		if h.mqtt.IsConnected() {
			checks["mqtt"] = "ok"
		} else {
			checks["mqtt"] = "disconnected"
			if status == "healthy" {
				status = "degraded"
			}
		}
	} else {
		checks["mqtt"] = "not_configured"
	}

	// File watcher check
	if h.live != nil {
		if ws := h.live.WatcherStatus(); ws != nil {
			checks["file_watcher"] = ws.Status
		}
	}

	// TR instance status
	var trInstances []TRInstanceStatusData
	if h.live != nil {
		trInstances = h.live.TRInstanceStatus()
	}

	resp := HealthResponse{
		Status:         status,
		Version:        h.version,
		UptimeSeconds:  int64(time.Since(h.startTime).Seconds()),
		Checks:         checks,
		TrunkRecorders: trInstances,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(resp)
}
