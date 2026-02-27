package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/database"
	"github.com/snarg/tr-engine/internal/mqttclient"
)

type HealthResponse struct {
	Status         string                `json:"status"`
	Version        string                `json:"version"`
	UptimeSeconds  int64                 `json:"uptime_seconds"`
	Checks         map[string]string     `json:"checks"`
	TrunkRecorders []TRInstanceStatusData `json:"trunk_recorders,omitempty"`
	UpdateAvailable *bool                `json:"update_available,omitempty"`
	LatestVersion   string               `json:"latest_version,omitempty"`
	ReleaseURL      string               `json:"release_url,omitempty"`
}

type updateStatus struct {
	Available     bool
	LatestVersion string
	ReleaseURL    string
}

type HealthHandler struct {
	db        *database.DB
	mqtt      *mqttclient.Client
	live      LiveDataSource
	version   string
	startTime time.Time

	// Update checker state
	updateCheckURL string
	ingestModes    string
	isDocker       bool
	log            zerolog.Logger
	mu             sync.RWMutex
	update         *updateStatus
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

// ConfigureUpdateChecker sets up the update checker parameters. Call before StartUpdateChecker.
func (h *HealthHandler) ConfigureUpdateChecker(url, ingestModes string, isDocker bool, log zerolog.Logger) {
	h.updateCheckURL = url
	h.ingestModes = ingestModes
	h.isDocker = isDocker
	h.log = log
}

// StartUpdateChecker begins periodic update checks in the background.
// Does nothing if no update check URL is configured.
func (h *HealthHandler) StartUpdateChecker(ctx context.Context) {
	if h.updateCheckURL == "" {
		return
	}

	// Extract just the version prefix (e.g. "v0.8.7.6" from "v0.8.7.6 (commit=..., built=...)")
	ver := h.version
	if idx := strings.Index(ver, " "); idx > 0 {
		ver = ver[:idx]
	}

	checkURL := fmt.Sprintf("%s?product=tr-engine&v=%s&os=%s&arch=%s&go=%s&ingest=%s&docker=%t",
		h.updateCheckURL, ver, runtime.GOOS, runtime.GOARCH, runtime.Version(),
		h.ingestModes, h.isDocker)

	h.log.Debug().Str("url", checkURL).Msg("update checker configured")

	// Check immediately, then every hour
	go func() {
		h.checkForUpdate(checkURL, true)

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.checkForUpdate(checkURL, false)
			}
		}
	}()
}

func (h *HealthHandler) checkForUpdate(url string, firstCheck bool) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		h.log.Debug().Err(err).Msg("update check failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.log.Debug().Int("status", resp.StatusCode).Msg("update check returned non-200")
		return
	}

	var result struct {
		UpdateAvailable bool   `json:"update_available"`
		Latest          string `json:"latest"`
		URL             string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		h.log.Debug().Err(err).Msg("update check response parse failed")
		return
	}

	h.mu.Lock()
	h.update = &updateStatus{
		Available:     result.UpdateAvailable,
		LatestVersion: result.Latest,
		ReleaseURL:    result.URL,
	}
	h.mu.Unlock()

	if firstCheck && result.UpdateAvailable {
		h.log.Warn().
			Str("current", h.version).
			Str("latest", result.Latest).
			Str("release_url", result.URL).
			Msg("a newer version of tr-engine is available")
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

	// Transcription check
	if h.live != nil {
		if ts := h.live.TranscriptionStatus(); ts != nil {
			checks["transcription"] = ts.Status
		} else {
			checks["transcription"] = "not_configured"
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

	// Add update status if available
	h.mu.RLock()
	if h.update != nil {
		resp.UpdateAvailable = &h.update.Available
		resp.LatestVersion = h.update.LatestVersion
		resp.ReleaseURL = h.update.ReleaseURL
	}
	h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(resp)
}
