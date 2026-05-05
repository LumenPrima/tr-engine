package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/database"
)

type StorageHandler struct {
	db  *database.DB
	log zerolog.Logger
}

func NewStorageHandler(db *database.DB, log zerolog.Logger) *StorageHandler {
	return &StorageHandler{db: db, log: log}
}

func (h *StorageHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.GetStorageStats(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get storage stats: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, stats)
}

type purgeRequest struct {
	OlderThan string `json:"older_than"`
}

type purgeTableSpec struct {
	table      string
	timeColumn string
	usePartitionDrop bool
}

var purgeAllowlist = map[string]purgeTableSpec{
	"mqtt_raw_messages":      {"mqtt_raw_messages", "", true},
	"console_messages":       {"console_messages", "log_time", false},
	"trunking_messages":      {"trunking_messages", "time", false},
	"plugin_statuses":        {"plugin_statuses", "time", false},
	"call_active_checkpoints": {"call_active_checkpoints", "snapshot_time", false},
}

func (h *StorageHandler) PurgeTable(w http.ResponseWriter, r *http.Request) {
	table := chi.URLParam(r, "table")
	if table == "" {
		WriteError(w, http.StatusBadRequest, "table parameter required")
		return
	}

	spec, ok := purgeAllowlist[table]
	if !ok {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, "unknown table")
		return
	}

	var req purgeRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidBody, "invalid request body")
		return
	}
	if req.OlderThan == "" {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, "older_than is required (e.g. '48h')")
		return
	}
	if _, err := time.ParseDuration(req.OlderThan); err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, "expected duration like '48h' or '7d'")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	start := time.Now()
	response := map[string]any{
		"table": spec.table,
		"warning": "This action is irreversible.",
	}

	olderThan, _ := time.ParseDuration(req.OlderThan)
	response["older_than"] = req.OlderThan

	if spec.usePartitionDrop {
		dropped, err := h.db.DropOldWeeklyPartitions(ctx, spec.table, olderThan)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "purge failed: "+err.Error())
			return
		}
		timedOut := ctx.Err() != nil
		response["partitions_dropped"] = len(dropped)
		response["timed_out"] = timedOut
		response["duration_ms"] = time.Since(start).Milliseconds()
		if len(dropped) > 0 {
			response["partitions_dropped_names"] = dropped
		}

		h.log.Info().
			Str("table", spec.table).
			Str("older_than", req.OlderThan).
			Int("partitions_dropped", len(dropped)).
			Bool("timed_out", timedOut).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Msg("manual purge completed")
	} else {
		n, err := h.db.PurgeOlderThan(ctx, spec.table, spec.timeColumn, olderThan)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "purge failed: "+err.Error())
			return
		}
		timedOut := ctx.Err() != nil
		response["rows_deleted"] = n
		response["timed_out"] = timedOut
		response["duration_ms"] = time.Since(start).Milliseconds()

		h.log.Info().
			Str("table", spec.table).
			Str("older_than", req.OlderThan).
			Int64("rows_deleted", n).
			Bool("timed_out", timedOut).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Msg("manual purge completed")
	}

	status := http.StatusOK
	if ctx.Err() != nil {
		status = http.StatusGatewayTimeout
	}
	WriteJSON(w, status, response)
}

func (h *StorageHandler) Routes(r chi.Router) {
	r.Get("/admin/storage/stats", h.GetStats)
	r.Post("/admin/storage/purge/{table}", h.PurgeTable)
}
