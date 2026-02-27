package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/hlog"
	"github.com/snarg/tr-engine/internal/database"
	"github.com/snarg/tr-engine/internal/trconfig"
)

type UnitsHandler struct {
	db       *database.DB
	csvPaths map[int]string // system_id → unit CSV file path for writeback
}

func NewUnitsHandler(db *database.DB, csvPaths map[int]string) *UnitsHandler {
	return &UnitsHandler{db: db, csvPaths: csvPaths}
}

var unitSortFields = map[string]string{
	"alpha_tag":       "u.alpha_tag",
	"unit_id":         "u.unit_id",
	"last_seen":       "u.last_seen",
	"last_event_time": "u.last_event_time",
}

// ListUnits returns radio units with optional filters.
func (h *UnitsHandler) ListUnits(w http.ResponseWriter, r *http.Request) {
	p, err := ParsePagination(r)
	if err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, err.Error())
		return
	}
	sort := ParseSort(r, "unit_id", unitSortFields)

	filter := database.UnitFilter{
		Limit:  p.Limit,
		Offset: p.Offset,
		Sort:   sort.SQLOrderBy(unitSortFields),
	}

	if v, ok := QueryString(r, "sysid"); ok {
		filter.Sysid = &v
	}
	if v, ok := QueryString(r, "search"); ok {
		filter.Search = &v
	}
	if v, ok := QueryInt(r, "active_within"); ok {
		filter.ActiveWithin = &v
	}
	filter.Talkgroups = QueryIntList(r, "talkgroup")

	units, total, err := h.db.ListUnits(r.Context(), filter)
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

// GetUnit returns a single unit by composite or plain ID.
func (h *UnitsHandler) GetUnit(w http.ResponseWriter, r *http.Request) {
	cid, err := ParseCompositeID(r, "id")
	if err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, err.Error())
		return
	}

	if cid.IsPlain {
		matches, err := h.db.FindUnitSystems(r.Context(), cid.EntityID)
		if err != nil || len(matches) == 0 {
			WriteError(w, http.StatusNotFound, "unit not found")
			return
		}
		if len(matches) > 1 {
			WriteAmbiguous(w, cid.EntityID, matches)
			return
		}
		cid.SystemID = matches[0].SystemID
	}

	unit, err := h.db.GetUnitByComposite(r.Context(), cid.SystemID, cid.EntityID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "unit not found")
		return
	}
	WriteJSON(w, http.StatusOK, unit)
}

// UpdateUnit patches unit metadata.
func (h *UnitsHandler) UpdateUnit(w http.ResponseWriter, r *http.Request) {
	cid, err := ParseCompositeID(r, "id")
	if err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, err.Error())
		return
	}

	if cid.IsPlain {
		matches, err := h.db.FindUnitSystems(r.Context(), cid.EntityID)
		if err != nil || len(matches) == 0 {
			WriteError(w, http.StatusNotFound, "unit not found")
			return
		}
		if len(matches) > 1 {
			WriteAmbiguous(w, cid.EntityID, matches)
			return
		}
		cid.SystemID = matches[0].SystemID
	}

	var patch struct {
		AlphaTag       *string `json:"alpha_tag"`
		AlphaTagSource *string `json:"alpha_tag_source"`
	}
	if err := DecodeJSON(r, &patch); err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidBody, "invalid request body")
		return
	}

	if err := h.db.UpdateUnitFields(r.Context(), cid.SystemID, cid.EntityID,
		patch.AlphaTag, patch.AlphaTagSource); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to update unit")
		return
	}

	unit, err := h.db.GetUnitByComposite(r.Context(), cid.SystemID, cid.EntityID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "unit not found")
		return
	}

	// Best-effort writeback to TR's unit tags CSV
	if patch.AlphaTag != nil {
		if csvPath, ok := h.csvPaths[cid.SystemID]; ok {
			if csvErr := trconfig.UpdateUnitCSV(csvPath, cid.EntityID, *patch.AlphaTag); csvErr != nil {
				log := hlog.FromRequest(r)
				log.Warn().Err(csvErr).Str("csv_path", csvPath).Int("unit_id", cid.EntityID).
					Msg("failed to write back unit CSV")
			}
		}
	}

	WriteJSON(w, http.StatusOK, unit)
}

// ListUnitCalls returns calls that include transmissions from a specific unit.
func (h *UnitsHandler) ListUnitCalls(w http.ResponseWriter, r *http.Request) {
	cid, err := ParseCompositeID(r, "id")
	if err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, err.Error())
		return
	}

	if cid.IsPlain {
		matches, err := h.db.FindUnitSystems(r.Context(), cid.EntityID)
		if err != nil || len(matches) == 0 {
			WriteError(w, http.StatusNotFound, "unit not found")
			return
		}
		if len(matches) > 1 {
			WriteAmbiguous(w, cid.EntityID, matches)
			return
		}
		cid.SystemID = matches[0].SystemID
	}

	p, err := ParsePagination(r)
	if err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, err.Error())
		return
	}
	filter := database.CallFilter{
		Limit:     p.Limit,
		Offset:    p.Offset,
		SystemIDs: []int{cid.SystemID},
		UnitIDs:   []int{cid.EntityID},
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

// ListUnitEvents returns events for a specific unit.
func (h *UnitsHandler) ListUnitEvents(w http.ResponseWriter, r *http.Request) {
	cid, err := ParseCompositeID(r, "id")
	if err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, err.Error())
		return
	}

	if cid.IsPlain {
		matches, err := h.db.FindUnitSystems(r.Context(), cid.EntityID)
		if err != nil || len(matches) == 0 {
			WriteError(w, http.StatusNotFound, "unit not found")
			return
		}
		if len(matches) > 1 {
			WriteAmbiguous(w, cid.EntityID, matches)
			return
		}
		cid.SystemID = matches[0].SystemID
	}

	p, err := ParsePagination(r)
	if err != nil {
		WriteErrorWithCode(w, http.StatusBadRequest, ErrInvalidParameter, err.Error())
		return
	}
	filter := database.UnitEventFilter{
		SystemID: cid.SystemID,
		UnitID:   cid.EntityID,
		Limit:    p.Limit,
		Offset:   p.Offset,
	}
	if v, ok := QueryString(r, "type"); ok {
		filter.EventType = &v
	}
	if v, ok := QueryInt(r, "talkgroup"); ok {
		filter.Tgid = &v
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

	events, total, err := h.db.ListUnitEvents(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
	})
}

// Routes registers unit routes on the given router.
func (h *UnitsHandler) Routes(r chi.Router) {
	r.Get("/units", h.ListUnits)
	r.Get("/units/{id}", h.GetUnit)
	r.Patch("/units/{id}", h.UpdateUnit)
	r.Get("/units/{id}/calls", h.ListUnitCalls)
	r.Get("/units/{id}/events", h.ListUnitEvents)
}
