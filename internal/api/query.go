package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/hlog"
	"github.com/snarg/tr-engine/internal/database"
)

type QueryHandler struct {
	db *database.DB
}

func NewQueryHandler(db *database.DB) *QueryHandler {
	return &QueryHandler{db: db}
}

type queryRequest struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params"`
	Limit  int    `json:"limit"`
}

func (h *QueryHandler) ExecuteQuery(w http.ResponseWriter, r *http.Request) {
	log := hlog.FromRequest(r)

	var req queryRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sql := strings.TrimSpace(req.SQL)
	if sql == "" {
		WriteError(w, http.StatusBadRequest, "sql field is required")
		return
	}

	if strings.Contains(sql, ";") {
		log.Warn().Str("sql", sql).Msg("query rejected: semicolons forbidden")
		WriteError(w, http.StatusBadRequest, "multiple statements not allowed (semicolons are forbidden)")
		return
	}

	maxRows := req.Limit
	if maxRows <= 0 {
		maxRows = 1000
	}
	if maxRows > 50000 {
		WriteError(w, http.StatusBadRequest, "limit must be <= 50000")
		return
	}

	if req.Params == nil {
		req.Params = []any{}
	}

	log.Info().Str("sql", sql).Int("limit", maxRows).Msg("executing query")

	result, err := h.db.ExecuteReadOnlyQuery(r.Context(), sql, req.Params, maxRows)
	if err != nil {
		log.Warn().Err(err).Str("sql", sql).Msg("query failed")
		WriteErrorDetail(w, http.StatusBadRequest, "query failed", err.Error())
		return
	}

	log.Info().Str("sql", sql).Int("row_count", result.RowCount).Msg("query completed")
	WriteJSON(w, http.StatusOK, result)
}

func (h *QueryHandler) Routes(r chi.Router) {
	r.Post("/query", h.ExecuteQuery)
}
