package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/database"
)

type CallGroupsHandler struct {
	db *database.DB
}

func NewCallGroupsHandler(db *database.DB) *CallGroupsHandler {
	return &CallGroupsHandler{db: db}
}

// ListCallGroups returns deduplicated call groups.
func (h *CallGroupsHandler) ListCallGroups(w http.ResponseWriter, r *http.Request) {
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	filter := database.CallGroupFilter{
		Limit:  p.Limit,
		Offset: p.Offset,
	}

	if v, ok := QueryString(r, "sysid"); ok {
		filter.Sysid = &v
	}
	if v, ok := QueryInt(r, "tgid"); ok {
		filter.Tgid = &v
	}
	if t, ok := QueryTime(r, "start_time"); ok {
		filter.StartTime = &t
	}
	if t, ok := QueryTime(r, "end_time"); ok {
		filter.EndTime = &t
	}

	groups, total, err := h.db.ListCallGroups(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list call groups")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"call_groups": groups,
		"total":       total,
		"limit":       p.Limit,
		"offset":      p.Offset,
	})
}

// GetCallGroup returns a call group with all its individual recordings.
func (h *CallGroupsHandler) GetCallGroup(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call group ID")
		return
	}

	group, calls, err := h.db.GetCallGroupByID(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "call group not found")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"call_group": group,
		"calls":      calls,
	})
}

// Routes registers call group routes on the given router.
func (h *CallGroupsHandler) Routes(r chi.Router) {
	r.Get("/call-groups", h.ListCallGroups)
	r.Get("/call-groups/{id}", h.GetCallGroup)
}
