package rest

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
	"github.com/trunk-recorder/tr-engine/internal/ingest"
	"go.uber.org/zap"
)

// Swagger response types

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error" example:"Resource not found"`
}

// Site represents a trunk-recorder recording site (wraps System with clearer field names)
// @Description A trunk-recorder recording site. Note: "system_id" is the database ID used in API calls.
type Site struct {
	SystemID   int    `json:"system_id" example:"1"`                    // Database ID, use this in API calls
	ID         int    `json:"id" example:"1"`                           // Deprecated: use system_id
	InstanceID int    `json:"instance_id" example:"1"`                  // Trunk-recorder instance
	SysNum     int    `json:"sys_num" example:"0"`                      // System number within TR config
	ShortName  string `json:"short_name" example:"butco"`               // Trunk-recorder short name
	SystemType string `json:"system_type,omitempty" example:"p25"`      // p25, smartnet, conventional
	SysID      string `json:"sysid,omitempty" example:"348"`            // P25 System ID (hex)
	WACN       string `json:"wacn,omitempty" example:"BEE00"`           // P25 WACN
	NAC        string `json:"nac,omitempty" example:"340"`              // P25 NAC (site-specific)
	RFSS       int    `json:"rfss,omitempty" example:"4"`               // P25 RFSS
	SiteID     int    `json:"site_id,omitempty" example:"1"`            // P25 Site ID
}

// SiteListResponse represents a list of recording sites
// @Description List of trunk-recorder recording sites. Each site monitors one radio system/site combination.
type SiteListResponse struct {
	Sites []Site `json:"sites"`
	Count int    `json:"count" example:"2"`
}

// TalkgroupListResponse represents a list of talkgroups
type TalkgroupListResponse struct {
	Talkgroups interface{} `json:"talkgroups"`
	Count      int         `json:"count" example:"100"`
	Limit      int         `json:"limit" example:"50"`
	Offset     int         `json:"offset" example:"0"`
}

// UnitListResponse represents a list of units
type UnitListResponse struct {
	Units  []*models.Unit `json:"units"`
	Count  int            `json:"count" example:"50"`
	Limit  int            `json:"limit" example:"50"`
	Offset int            `json:"offset" example:"0"`
}

// ActiveUnitListResponse represents a list of active units
type ActiveUnitListResponse struct {
	Units  interface{} `json:"units"`
	Count  int         `json:"count" example:"10"`
	Limit  int         `json:"limit" example:"50"`
	Offset int         `json:"offset" example:"0"`
	Window int         `json:"window" example:"5"`
}

// CallListResponse represents a list of calls
type CallListResponse struct {
	Calls  interface{} `json:"calls"`
	Count  int         `json:"count" example:"25"`
	Limit  int         `json:"limit" example:"50"`
	Offset int         `json:"offset" example:"0"`
}

// UnitEventListResponse represents a list of unit events
type UnitEventListResponse struct {
	Events []*models.UnitEvent `json:"events"`
	Count  int                 `json:"count" example:"100"`
	Limit  int                 `json:"limit" example:"50"`
	Offset int                 `json:"offset" example:"0"`
}

// CallGroupListResponse represents a list of call groups
type CallGroupListResponse struct {
	CallGroups interface{} `json:"call_groups"`
	Count      int         `json:"count" example:"50"`
	Limit      int         `json:"limit" example:"50"`
	Offset     int         `json:"offset" example:"0"`
}

// CallGroupDetailResponse represents a call group with its calls
type CallGroupDetailResponse struct {
	CallGroup interface{} `json:"call_group"`
	Calls     interface{} `json:"calls"`
}

// RatesResponse represents decode rate data
type RatesResponse struct {
	Rates interface{} `json:"rates"`
	Count int         `json:"count" example:"100"`
}

// ActivityResponse represents activity summary
type ActivityResponse struct {
	Systems        int64       `json:"systems" example:"3"`
	Talkgroups     int64       `json:"talkgroups" example:"500"`
	Units          int64       `json:"units" example:"1000"`
	Calls24h       int64       `json:"calls_24h" example:"5000"`
	SystemActivity interface{} `json:"system_activity"`
}

// RecorderListResponse represents a list of recorder states
type RecorderListResponse struct {
	Recorders []*ingest.RecorderInfo `json:"recorders"`
	Count     int                    `json:"count" example:"12"`
}

// Handler handles REST API requests
// RecorderProvider provides recorder state information
type RecorderProvider interface {
	GetRecorders() interface{}
}

type Handler struct {
	db               *database.DB
	processor        *ingest.Processor
	logger           *zap.Logger
	audioBasePath    string
	recorderProvider RecorderProvider
}

// NewHandler creates a new Handler
func NewHandler(db *database.DB, processor *ingest.Processor, logger *zap.Logger, audioBasePath string) *Handler {
	return &Handler{
		db:            db,
		processor:     processor,
		logger:        logger,
		audioBasePath: audioBasePath,
	}
}

// SetRecorderProvider sets the provider for recorder state (used in watch mode)
func (h *Handler) SetRecorderProvider(provider RecorderProvider) {
	h.recorderProvider = provider
}

// populateAudioURL sets the AudioURL field on a call if it has audio
func populateAudioURL(call *models.Call) {
	if call != nil && call.AudioPath != "" {
		call.AudioURL = "/api/v1/calls/" + strconv.FormatInt(call.ID, 10) + "/audio"
	}
}

// populateAudioURLs sets the AudioURL field on a slice of calls
func populateAudioURLs(calls []*models.Call) {
	for _, call := range calls {
		populateAudioURL(call)
	}
}

// populateRecentCallAudioURLs sets the AudioURL field on a slice of recent calls
func populateRecentCallAudioURLs(calls []*database.RecentCallInfo) {
	for _, call := range calls {
		if call != nil && call.AudioPath != "" {
			call.AudioURL = "/api/v1/calls/" + strconv.FormatInt(call.ID, 10) + "/audio"
		}
	}
}

// resolveCall looks up a call by tr_call_id string, falling back to numeric DB id
func (h *Handler) resolveCall(c *gin.Context) (*models.Call, error) {
	idParam := c.Param("id")
	if idParam == "" {
		return nil, nil
	}

	// Try as tr_call_id first
	call, err := h.db.GetCallByTRCallID(c.Request.Context(), idParam)
	if err != nil {
		return nil, err
	}
	if call != nil {
		return call, nil
	}

	// Fall back to numeric DB id
	if numID, parseErr := strconv.ParseInt(idParam, 10, 64); parseErr == nil {
		call, err = h.db.GetCallByID(c.Request.Context(), numID)
		if err != nil {
			return nil, err
		}
		return call, nil
	}

	return nil, nil
}

// Common query parameter parsing
func (h *Handler) parsePagination(c *gin.Context) (limit, offset int) {
	limit = 50
	offset = 0

	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 1000 {
		limit = l
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = o
	}

	return
}

func (h *Handler) parseTimeRange(c *gin.Context) (startTime, endTime *time.Time) {
	if s := c.Query("start_time"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			startTime = &t
		}
	}
	if e := c.Query("end_time"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			endTime = &t
		}
	}
	return
}

// ListSystems godoc
// @Summary      List recording sites
// @Description  Returns all trunk-recorder recording sites. Each site monitors one radio system/site combination. For P25 networks with multiple sites, use /p25-systems to see them grouped by sysid+wacn.
// @Tags         systems
// @Produce      json
// @Success      200  {object}  rest.SiteListResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /systems [get]
func (h *Handler) ListSystems(c *gin.Context) {
	systems, err := h.db.ListSystems(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to list systems", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list systems"})
		return
	}

	// Convert to Site response type with clearer field names
	sites := make([]Site, len(systems))
	for i, sys := range systems {
		sites[i] = Site{
			SystemID:   sys.ID,
			ID:         sys.ID, // deprecated alias
			InstanceID: sys.InstanceID,
			SysNum:     sys.SysNum,
			ShortName:  sys.ShortName,
			SystemType: sys.SystemType,
			SysID:      sys.SysID,
			WACN:       sys.WACN,
			NAC:        sys.NAC,
			RFSS:       sys.RFSS,
			SiteID:     sys.SiteID,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"sites": sites,
		"count": len(sites),
	})
}

// GetSystem godoc
// @Summary      Get a system
// @Description  Returns a single system by ID
// @Tags         systems
// @Produce      json
// @Param        id   path      int  true  "System ID"
// @Success      200  {object}  models.System
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /systems/{id} [get]
func (h *Handler) GetSystem(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid system ID"})
		return
	}

	system, err := h.db.GetSystemByID(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get system", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get system"})
		return
	}
	if system == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "System not found"})
		return
	}

	c.JSON(http.StatusOK, system)
}

// P25Site represents a trunk-recorder site within a P25 system
type P25Site struct {
	ShortName string `json:"short_name"`
	NAC       string `json:"nac"`
	RFSS      int    `json:"rfss"`
	SiteID    int    `json:"site_id"`
	SystemID  int    `json:"system_id"` // database ID for API calls
}

// P25System represents a logical P25 system (grouped by sysid+wacn)
type P25System struct {
	SysID string    `json:"sysid"`
	WACN  string    `json:"wacn"`
	Sites []P25Site `json:"sites"`
}

// ListP25Systems godoc
// @Summary      List P25 systems
// @Description  Returns P25 systems grouped by sysid+wacn. Each P25 system may have multiple sites (trunk-recorder instances monitoring different NACs on the same network).
// @Tags         systems
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /p25-systems [get]
func (h *Handler) ListP25Systems(c *gin.Context) {
	systems, err := h.db.ListSystems(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to list systems", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list systems"})
		return
	}

	// Group by sysid+wacn
	p25Map := make(map[string]*P25System)
	for _, sys := range systems {
		if sys.SysID == "" {
			continue // Skip non-P25 systems
		}
		key := sys.SysID + ":" + sys.WACN
		if p25Map[key] == nil {
			p25Map[key] = &P25System{
				SysID: sys.SysID,
				WACN:  sys.WACN,
				Sites: []P25Site{},
			}
		}
		p25Map[key].Sites = append(p25Map[key].Sites, P25Site{
			ShortName: sys.ShortName,
			NAC:       sys.NAC,
			RFSS:      sys.RFSS,
			SiteID:    sys.SiteID,
			SystemID:  sys.ID,
		})
	}

	// Convert to slice
	p25Systems := make([]P25System, 0, len(p25Map))
	for _, sys := range p25Map {
		p25Systems = append(p25Systems, *sys)
	}

	c.JSON(http.StatusOK, gin.H{
		"p25_systems": p25Systems,
		"count":       len(p25Systems),
	})
}

// ListSystemTalkgroups godoc
// @Summary      List system talkgroups
// @Description  Returns all talkgroups for a specific system
// @Tags         systems
// @Produce      json
// @Param        id      path   int  true   "System ID"
// @Param        limit   query  int  false  "Results per page"  default(50)
// @Param        offset  query  int  false  "Page offset"       default(0)
// @Success      200  {object}  rest.TalkgroupListResponse
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /systems/{id}/talkgroups [get]
func (h *Handler) ListSystemTalkgroups(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid system ID"})
		return
	}

	limit, offset := h.parsePagination(c)

	talkgroups, err := h.db.ListTalkgroupsBySystem(c.Request.Context(), id, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list talkgroups", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list talkgroups"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"talkgroups": talkgroups,
		"count":      len(talkgroups),
		"limit":      limit,
		"offset":     offset,
	})
}

// ListTalkgroups godoc
// @Summary      List all talkgroups
// @Description  Returns all talkgroups, optionally filtered by SYSID
// @Tags         talkgroups
// @Produce      json
// @Param        sysid   query  string  false  "Filter by SYSID (P25 system identifier)"
// @Param        limit   query  int     false  "Results per page"  default(50)
// @Param        offset  query  int     false  "Page offset"       default(0)
// @Success      200  {object}  rest.TalkgroupListResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /talkgroups [get]
func (h *Handler) ListTalkgroups(c *gin.Context) {
	limit, offset := h.parsePagination(c)

	// Get SYSID filter (new: filter by P25 SYSID string)
	sysidFilter := c.Query("sysid")

	// Get search filter if provided
	search := c.Query("search")

	// Get sort options
	sortField := c.DefaultQuery("sort", "alpha_tag")
	sortDir := c.DefaultQuery("sort_dir", "asc")

	// Validate sort field (whitelist to prevent SQL injection)
	// For text columns, add NULLS LAST to put nulls at end
	validSortFields := map[string]string{
		"alpha_tag":  "alpha_tag",
		"tgid":       "tgid",
		"last_seen":  "last_seen",
		"first_seen": "first_seen",
		"group":      "tg_group",
	}
	nullsLastFields := map[string]bool{"alpha_tag": true, "group": true}

	orderBy, ok := validSortFields[sortField]
	if !ok {
		orderBy = "alpha_tag"
		sortField = "alpha_tag"
	}

	// Validate sort direction
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "asc"
	}
	orderClause := orderBy + " " + sortDir
	if nullsLastFields[sortField] {
		orderClause += " NULLS LAST"
	}

	var talkgroups interface{}
	var err error

	// Build query
	var conditions []string
	var args []interface{}
	argIdx := 1

	if sysidFilter != "" {
		conditions = append(conditions, "sysid = $"+strconv.Itoa(argIdx))
		args = append(args, sysidFilter)
		argIdx++
	}

	if search != "" {
		searchPattern := "%" + search + "%"
		searchCond := "(LOWER(alpha_tag) LIKE LOWER($" + strconv.Itoa(argIdx) + ") OR CAST(tgid AS TEXT) LIKE $" + strconv.Itoa(argIdx+1) + " OR LOWER(tg_group) LIKE LOWER($" + strconv.Itoa(argIdx+2) + ") OR LOWER(tag) LIKE LOWER($" + strconv.Itoa(argIdx+3) + "))"
		conditions = append(conditions, searchCond)
		args = append(args, searchPattern, searchPattern, searchPattern, searchPattern)
		argIdx += 4
	}

	query := `SELECT sysid, tgid, alpha_tag, description, tg_group, tag, priority, mode, first_seen, last_seen FROM talkgroups`
	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for i := 1; i < len(conditions); i++ {
			query += " AND " + conditions[i]
		}
	}
	query += " ORDER BY " + orderClause + " LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.db.Pool().Query(c.Request.Context(), query, args...)
	if err == nil {
		defer rows.Close()
		var results []map[string]interface{}
		for rows.Next() {
			var tgid, priority int
			var sysid string
			var alphaTag, description, group, tag, mode *string
			var firstSeen, lastSeen time.Time
			if scanErr := rows.Scan(&sysid, &tgid, &alphaTag, &description, &group, &tag, &priority, &mode, &firstSeen, &lastSeen); scanErr != nil {
				continue
			}
			results = append(results, map[string]interface{}{
				"sysid":       sysid,
				"tgid":        tgid,
				"alpha_tag":   alphaTag,
				"description": description,
				"group":       group,
				"tag":         tag,
				"priority":    priority,
				"mode":        mode,
				"first_seen":  firstSeen,
				"last_seen":   lastSeen,
			})
		}
		talkgroups = results
	}

	if err != nil {
		h.logger.Error("Failed to list talkgroups", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list talkgroups"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"talkgroups": talkgroups,
		"limit":      limit,
		"offset":     offset,
	})
}

// GetTalkgroupEncryptionStats godoc
// @Summary      Get encryption stats per talkgroup
// @Description  Returns counts of encrypted and clear calls per talkgroup from recent activity
// @Tags         talkgroups
// @Produce      json
// @Param        hours  query  int  false  "Hours of history to include (default 24)"
// @Success      200    {object}  map[string]interface{}
// @Failure      500    {object}  rest.ErrorResponse
// @Router       /talkgroups/encryption-stats [get]
func (h *Handler) GetTalkgroupEncryptionStats(c *gin.Context) {
	hours := 24
	if h := c.Query("hours"); h != "" {
		if v, err := strconv.Atoi(h); err == nil && v > 0 && v <= 168 {
			hours = v
		}
	}

	rows, err := h.db.Pool().Query(c.Request.Context(), `
		SELECT c.tgid,
			COUNT(*) FILTER (WHERE c.encrypted = true) as encrypted_count,
			COUNT(*) FILTER (WHERE c.encrypted = false) as clear_count
		FROM calls c
		WHERE c.start_time > NOW() - INTERVAL '1 hour' * $1
			AND c.tgid IS NOT NULL
		GROUP BY c.tgid
	`, hours)
	if err != nil {
		h.logger.Error("Failed to get encryption stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get encryption stats"})
		return
	}
	defer rows.Close()

	stats := make(map[int]map[string]int)
	for rows.Next() {
		var tgid, encryptedCount, clearCount int
		if err := rows.Scan(&tgid, &encryptedCount, &clearCount); err != nil {
			continue
		}
		stats[tgid] = map[string]int{
			"encrypted": encryptedCount,
			"clear":     clearCount,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"stats": stats,
		"hours": hours,
	})
}

// GetTalkgroup godoc
// @Summary      Get a talkgroup
// @Description  Returns a single talkgroup. Accepts sysid:tgid format (e.g., "348:9178") or plain tgid (returns 409 if ambiguous)
// @Tags         talkgroups
// @Produce      json
// @Param        id   path      string  true  "Talkgroup ID (sysid:tgid or plain tgid)"
// @Success      200  {object}  models.Talkgroup
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      409  {object}  rest.ErrorResponse  "Ambiguous tgid exists in multiple systems"
// @Router       /talkgroups/{id} [get]
func (h *Handler) GetTalkgroup(c *gin.Context) {
	idParam := c.Param("id")

	var tg struct {
		SYSID       string    `json:"sysid"`
		TGID        int       `json:"tgid"`
		AlphaTag    *string   `json:"alpha_tag"`
		Description *string   `json:"description"`
		Group       *string   `json:"group"`
		Tag         *string   `json:"tag"`
		Priority    int       `json:"priority"`
		Mode        *string   `json:"mode"`
		FirstSeen   time.Time `json:"first_seen"`
		LastSeen    time.Time `json:"last_seen"`
	}

	ctx := c.Request.Context()
	var err error

	// Check if it's a sysid:tgid format
	if parts := strings.Split(idParam, ":"); len(parts) == 2 {
		sysid := parts[0]
		tgid, parseErr := strconv.Atoi(parts[1])
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid talkgroup ID format"})
			return
		}
		err = h.db.Pool().QueryRow(ctx, `
			SELECT sysid, tgid, alpha_tag, description, tg_group, tag, priority, mode, first_seen, last_seen
			FROM talkgroups WHERE sysid = $1 AND tgid = $2
		`, sysid, tgid).Scan(&tg.SYSID, &tg.TGID, &tg.AlphaTag, &tg.Description, &tg.Group, &tg.Tag, &tg.Priority, &tg.Mode, &tg.FirstSeen, &tg.LastSeen)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Talkgroup not found"})
			return
		}
		c.JSON(http.StatusOK, tg)
		return
	}

	// Plain numeric - lookup by tgid
	tgid, parseErr := strconv.Atoi(idParam)
	if parseErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid talkgroup ID"})
		return
	}

	// Check if this tgid exists in multiple systems
	rows, err := h.db.Pool().Query(ctx, `
		SELECT sysid, alpha_tag FROM talkgroups WHERE tgid = $1
	`, tgid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var systems []map[string]string
	for rows.Next() {
		var sysid string
		var alphaTag *string
		if err := rows.Scan(&sysid, &alphaTag); err != nil {
			continue
		}
		tag := ""
		if alphaTag != nil {
			tag = *alphaTag
		}
		systems = append(systems, map[string]string{"sysid": sysid, "alpha_tag": tag})
	}

	if len(systems) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Talkgroup not found"})
		return
	}

	if len(systems) > 1 {
		// Ambiguous - exists in multiple systems
		c.JSON(http.StatusConflict, gin.H{
			"error":      "ambiguous_identifier",
			"message":    "tgid " + idParam + " exists in multiple systems",
			"systems":    systems,
			"resolution": "Use explicit format: {sysid}:" + idParam,
		})
		return
	}

	// Unique - get full talkgroup details
	err = h.db.Pool().QueryRow(ctx, `
		SELECT sysid, tgid, alpha_tag, description, tg_group, tag, priority, mode, first_seen, last_seen
		FROM talkgroups WHERE tgid = $1
	`, tgid).Scan(&tg.SYSID, &tg.TGID, &tg.AlphaTag, &tg.Description, &tg.Group, &tg.Tag, &tg.Priority, &tg.Mode, &tg.FirstSeen, &tg.LastSeen)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Talkgroup not found"})
		return
	}

	c.JSON(http.StatusOK, tg)
}

// ListTalkgroupCalls godoc
// @Summary      List talkgroup calls
// @Description  Returns calls for a specific talkgroup. Accepts sysid:tgid format (e.g., "348:9178") or plain tgid.
// @Tags         talkgroups
// @Produce      json
// @Param        id          path   string  true   "Talkgroup ID (sysid:tgid or plain tgid)"
// @Param        start_time  query  string  false  "Start time filter (RFC3339)"
// @Param        end_time    query  string  false  "End time filter (RFC3339)"
// @Param        limit       query  int     false  "Results per page"  default(50)
// @Param        offset      query  int     false  "Page offset"       default(0)
// @Success      200  {object}  rest.CallListResponse
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /talkgroups/{id}/calls [get]
func (h *Handler) ListTalkgroupCalls(c *gin.Context) {
	idParam := c.Param("id")

	var sysid *string
	var tgid int
	var err error

	// Check if it's a sysid:tgid format
	if parts := strings.Split(idParam, ":"); len(parts) == 2 {
		sysid = &parts[0]
		tgid, err = strconv.Atoi(parts[1])
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid talkgroup ID format"})
			return
		}
	} else {
		// Plain numeric tgid
		tgid, err = strconv.Atoi(idParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid talkgroup ID"})
			return
		}
	}

	limit, offset := h.parsePagination(c)
	startTime, endTime := h.parseTimeRange(c)

	calls, err := h.db.ListCalls(c.Request.Context(), nil, sysid, &tgid, startTime, endTime, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list calls", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list calls"})
		return
	}

	populateAudioURLs(calls)
	c.JSON(http.StatusOK, gin.H{
		"calls":  calls,
		"count":  len(calls),
		"limit":  limit,
		"offset": offset,
	})
}

// ListUnits godoc
// @Summary      List all units
// @Description  Returns all radio units, optionally filtered by SYSID
// @Tags         units
// @Produce      json
// @Param        sysid   query  string  false  "Filter by SYSID (P25 system identifier)"
// @Param        limit   query  int     false  "Results per page"  default(50)
// @Param        offset  query  int     false  "Page offset"       default(0)
// @Success      200  {object}  rest.UnitListResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /units [get]
func (h *Handler) ListUnits(c *gin.Context) {
	limit, offset := h.parsePagination(c)

	// Get SYSID filter (new: filter by P25 SYSID string)
	sysidFilter := c.Query("sysid")

	// Get search filter if provided
	search := c.Query("search")

	// Get sort options
	sortField := c.DefaultQuery("sort", "alpha_tag")
	sortDir := c.DefaultQuery("sort_dir", "asc")

	// Validate sort field (whitelist to prevent SQL injection)
	validSortFields := map[string]string{
		"alpha_tag":  "alpha_tag",
		"unit_id":    "unit_id",
		"last_seen":  "last_seen",
		"first_seen": "first_seen",
	}
	nullsLastFields := map[string]bool{"alpha_tag": true}

	orderBy, ok := validSortFields[sortField]
	if !ok {
		orderBy = "alpha_tag"
		sortField = "alpha_tag"
	}

	// Validate sort direction
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "asc"
	}
	orderClause := orderBy + " " + sortDir
	if nullsLastFields[sortField] {
		orderClause += " NULLS LAST"
	}

	var units interface{}
	var err error

	// Build query
	var conditions []string
	var args []interface{}
	argIdx := 1

	if sysidFilter != "" {
		conditions = append(conditions, "sysid = $"+strconv.Itoa(argIdx))
		args = append(args, sysidFilter)
		argIdx++
	}

	if search != "" {
		searchPattern := "%" + search + "%"
		searchCond := "(LOWER(alpha_tag) LIKE LOWER($" + strconv.Itoa(argIdx) + ") OR CAST(unit_id AS TEXT) LIKE $" + strconv.Itoa(argIdx+1) + ")"
		conditions = append(conditions, searchCond)
		args = append(args, searchPattern, searchPattern)
		argIdx += 2
	}

	query := `SELECT sysid, unit_id, alpha_tag, alpha_tag_source, first_seen, last_seen FROM units`
	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for i := 1; i < len(conditions); i++ {
			query += " AND " + conditions[i]
		}
	}
	query += " ORDER BY " + orderClause + " LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.db.Pool().Query(c.Request.Context(), query, args...)
	if err == nil {
		defer rows.Close()
		var results []map[string]interface{}
		for rows.Next() {
			var sysid string
			var unitID int64
			var alphaTag, alphaTagSource *string
			var firstSeen, lastSeen time.Time
			if scanErr := rows.Scan(&sysid, &unitID, &alphaTag, &alphaTagSource, &firstSeen, &lastSeen); scanErr != nil {
				continue
			}
			results = append(results, map[string]interface{}{
				"sysid":            sysid,
				"unit_id":          unitID,
				"alpha_tag":        alphaTag,
				"alpha_tag_source": alphaTagSource,
				"first_seen":       firstSeen,
				"last_seen":        lastSeen,
			})
		}
		units = results
	}

	if err != nil {
		h.logger.Error("Failed to list units", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list units"})
		return
	}

	// Get count for response
	count := 0
	if unitSlice, ok := units.([]map[string]interface{}); ok {
		count = len(unitSlice)
	} else if unitSlice, ok := units.([]*models.Unit); ok {
		count = len(unitSlice)
	}

	c.JSON(http.StatusOK, gin.H{
		"units":  units,
		"count":  count,
		"limit":  limit,
		"offset": offset,
	})
}

// GetUnit godoc
// @Summary      Get a unit
// @Description  Returns a single unit. Accepts sysid:unit_id format (e.g., "348:1234567") or plain unit_id (returns 409 if ambiguous across systems)
// @Tags         units
// @Produce      json
// @Param        id   path      string  true  "Unit ID (sysid:unit_id or plain unit_id)"
// @Success      200  {object}  models.Unit
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      409  {object}  rest.ErrorResponse  "Ambiguous unit_id exists in multiple systems"
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /units/{id} [get]
func (h *Handler) GetUnit(c *gin.Context) {
	idParam := c.Param("id")
	ctx := c.Request.Context()

	var unit *models.Unit
	var err error

	// Check if it's a sysid:unit_id format
	if parts := strings.Split(idParam, ":"); len(parts) == 2 {
		sysid := parts[0]
		unitID, parseErr := strconv.ParseInt(parts[1], 10, 64)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid unit ID format"})
			return
		}
		unit, err = h.db.GetUnit(ctx, sysid, unitID)
		if err != nil {
			h.logger.Error("Failed to get unit", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get unit"})
			return
		}
		if unit == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Unit not found"})
			return
		}
		c.JSON(http.StatusOK, unit)
		return
	}

	// Plain numeric - lookup by unit_id
	unitRID, parseErr := strconv.ParseInt(idParam, 10, 64)
	if parseErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid unit ID"})
		return
	}

	// Check if this unit_id exists in multiple systems
	rows, err := h.db.Pool().Query(ctx, `
		SELECT sysid, alpha_tag FROM units WHERE unit_id = $1
	`, unitRID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var systems []map[string]string
	for rows.Next() {
		var sysid string
		var alphaTag *string
		if err := rows.Scan(&sysid, &alphaTag); err != nil {
			continue
		}
		tag := ""
		if alphaTag != nil {
			tag = *alphaTag
		}
		systems = append(systems, map[string]string{"sysid": sysid, "alpha_tag": tag})
	}

	if len(systems) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Unit not found"})
		return
	}

	if len(systems) > 1 {
		// Ambiguous - exists in multiple systems
		c.JSON(http.StatusConflict, gin.H{
			"error":      "ambiguous_identifier",
			"message":    "unit_id " + idParam + " exists in multiple systems",
			"systems":    systems,
			"resolution": "Use explicit format: {sysid}:" + idParam,
		})
		return
	}

	// Unique - get full unit details
	unit = &models.Unit{}
	err = h.db.Pool().QueryRow(ctx, `
		SELECT sysid, unit_id, alpha_tag, alpha_tag_source, first_seen, last_seen
		FROM units WHERE unit_id = $1
	`, unitRID).Scan(&unit.SYSID, &unit.UnitID, &unit.AlphaTag, &unit.AlphaTagSource, &unit.FirstSeen, &unit.LastSeen)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Unit not found"})
		return
	}

	c.JSON(http.StatusOK, unit)
}

// ListUnitEvents godoc
// @Summary      List unit events
// @Description  Returns events (affiliations, registrations, etc.) for a specific unit by radio ID
// @Tags         units
// @Produce      json
// @Param        id          path   int     true   "Unit radio ID (RID)"
// @Param        type        query  string  false  "Filter by event type (on, off, join, call, etc.)"
// @Param        talkgroup   query  int     false  "Filter by talkgroup ID (TGID)"
// @Param        start_time  query  string  false  "Start time filter (RFC3339)"
// @Param        end_time    query  string  false  "End time filter (RFC3339)"
// @Param        limit       query  int     false  "Results per page"  default(50)
// @Param        offset      query  int     false  "Page offset"       default(0)
// @Success      200  {object}  rest.UnitEventListResponse
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /units/{id}/events [get]
func (h *Handler) ListUnitEvents(c *gin.Context) {
	idParam := c.Param("id")

	var sysid *string
	var unitRID int64
	var err error

	// Check if it's a sysid:unit_id format
	if parts := strings.Split(idParam, ":"); len(parts) == 2 {
		sysid = &parts[0]
		unitRID, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid unit ID format"})
			return
		}
	} else {
		// Plain numeric unit_id
		unitRID, err = strconv.ParseInt(idParam, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid unit ID"})
			return
		}
	}

	limit, offset := h.parsePagination(c)
	startTime, endTime := h.parseTimeRange(c)

	var eventType *string
	if t := c.Query("type"); t != "" {
		eventType = &t
	}

	var tgid *int
	if tg := c.Query("talkgroup"); tg != "" {
		if v, err := strconv.Atoi(tg); err == nil {
			tgid = &v
		}
	}

	events, err := h.db.ListUnitEvents(c.Request.Context(), &unitRID, sysid, nil, eventType, tgid, startTime, endTime, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list unit events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list unit events"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"count":  len(events),
		"limit":  limit,
		"offset": offset,
	})
}

// ListUnitCalls godoc
// @Summary      List unit calls
// @Description  Returns calls that include transmissions from a specific unit. Accepts sysid:unit_id format (e.g., "348:902001") or plain unit_id.
// @Tags         units
// @Produce      json
// @Param        id          path   string  true   "Unit ID (sysid:unit_id or plain unit_id)"
// @Param        start_time  query  string  false  "Start time filter (RFC3339)"
// @Param        end_time    query  string  false  "End time filter (RFC3339)"
// @Param        limit       query  int     false  "Results per page"  default(50)
// @Param        offset      query  int     false  "Page offset"       default(0)
// @Success      200  {object}  rest.CallListResponse
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /units/{id}/calls [get]
func (h *Handler) ListUnitCalls(c *gin.Context) {
	idParam := c.Param("id")

	var sysid *string
	var unitID int64
	var err error

	// Check if it's a sysid:unit_id format
	if parts := strings.Split(idParam, ":"); len(parts) == 2 {
		sysid = &parts[0]
		unitID, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid unit ID format"})
			return
		}
	} else {
		// Plain numeric unit_id
		unitID, err = strconv.ParseInt(idParam, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid unit ID"})
			return
		}
	}

	limit, offset := h.parsePagination(c)
	startTime, endTime := h.parseTimeRange(c)

	// Query calls that have transmissions from this unit
	query := `
		SELECT DISTINCT c.id, c.call_group_id, c.instance_id, c.system_id, c.tg_sysid, c.tgid,
			c.tr_call_id, c.call_num, c.start_time, c.stop_time, c.duration,
			c.encrypted, c.emergency, c.freq, c.audio_path,
			tg.alpha_tag
		FROM calls c
		JOIN transmissions t ON t.call_id = c.id
		LEFT JOIN talkgroups tg ON tg.sysid = c.tg_sysid AND tg.tgid = c.tgid
		WHERE t.unit_rid = $1
	`
	args := []any{unitID}
	argNum := 2

	if sysid != nil {
		query += " AND t.unit_sysid = $" + strconv.Itoa(argNum)
		args = append(args, *sysid)
		argNum++
	}

	if startTime != nil {
		query += " AND c.start_time >= $" + strconv.Itoa(argNum)
		args = append(args, *startTime)
		argNum++
	}
	if endTime != nil {
		query += " AND c.start_time <= $" + strconv.Itoa(argNum)
		args = append(args, *endTime)
		argNum++
	}

	query += " ORDER BY c.start_time DESC LIMIT $" + strconv.Itoa(argNum)
	args = append(args, limit)
	argNum++
	query += " OFFSET $" + strconv.Itoa(argNum)
	args = append(args, offset)

	rows, err := h.db.Pool().Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to list unit calls", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list unit calls"})
		return
	}
	defer rows.Close()

	var calls []map[string]interface{}
	var callIDs []int64
	callIdx := make(map[int64]int)
	for rows.Next() {
		var callID, instanceID, systemID int64
		var callGroupID *int64
		var tgSysid *string
		var tgid *int
		var trCallID, audioPath *string
		var callNum int64
		var startTime time.Time
		var stopTime *time.Time
		var duration float32
		var encrypted, emergency bool
		var freq int64
		var tgAlphaTag *string

		if err := rows.Scan(&callID, &callGroupID, &instanceID, &systemID, &tgSysid, &tgid,
			&trCallID, &callNum, &startTime, &stopTime, &duration,
			&encrypted, &emergency, &freq, &audioPath,
			&tgAlphaTag); err != nil {
			continue
		}

		call := map[string]interface{}{
			"id":            callID,
			"call_group_id": callGroupID,
			"instance_id":   instanceID,
			"system_id":     systemID,
			"tr_call_id":    trCallID,
			"call_num":      callNum,
			"start_time":    startTime,
			"stop_time":     stopTime,
			"duration":      duration,
			"encrypted":     encrypted,
			"emergency":     emergency,
			"freq":          freq,
			"audio_path":    audioPath,
		}
		// Add audio_url if audio exists
		if audioPath != nil && *audioPath != "" {
			call["audio_url"] = "/api/v1/calls/" + strconv.FormatInt(callID, 10) + "/audio"
		}
		if tgSysid != nil {
			call["tg_sysid"] = *tgSysid
		}
		if tgid != nil {
			call["tgid"] = *tgid
		}
		if tgAlphaTag != nil {
			call["tg_alpha_tag"] = *tgAlphaTag
		}
		calls = append(calls, call)
		callIDs = append(callIDs, callID)
		callIdx[callID] = len(calls) - 1
	}

	// Fetch units for all calls
	if len(callIDs) > 0 {
		unitQuery := `
			SELECT DISTINCT t.call_id, t.unit_rid, COALESCE(u.alpha_tag, '') as alpha_tag
			FROM transmissions t
			LEFT JOIN units u ON u.sysid = t.unit_sysid AND u.unit_id = t.unit_rid
			WHERE t.call_id = ANY($1)
			ORDER BY t.call_id, t.unit_rid
		`
		unitRows, unitErr := h.db.Pool().Query(c.Request.Context(), unitQuery, callIDs)
		if unitErr == nil {
			defer unitRows.Close()
			for unitRows.Next() {
				var cID, unitRID int64
				var alphaTag string
				if err := unitRows.Scan(&cID, &unitRID, &alphaTag); err != nil {
					continue
				}
				if idx, ok := callIdx[cID]; ok {
					units, _ := calls[idx]["units"].([]map[string]interface{})
					u := map[string]interface{}{"unit_rid": unitRID}
					if alphaTag != "" {
						u["alpha_tag"] = alphaTag
					}
					calls[idx]["units"] = append(units, u)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"calls":  calls,
		"count":  len(calls),
		"limit":  limit,
		"offset": offset,
	})
}

// ListCalls godoc
// @Summary      List all calls
// @Description  Returns call recordings with optional filters
// @Tags         calls
// @Produce      json
// @Param        system      query  int     false  "Filter by system ID (database ID)"
// @Param        sysid       query  string  false  "Filter by P25 SYSID (e.g., '348')"
// @Param        talkgroup   query  int     false  "Filter by talkgroup ID"
// @Param        start_time  query  string  false  "Start time filter (RFC3339)"
// @Param        end_time    query  string  false  "End time filter (RFC3339)"
// @Param        limit       query  int     false  "Results per page"  default(50)
// @Param        offset      query  int     false  "Page offset"       default(0)
// @Success      200  {object}  rest.CallListResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /calls [get]
func (h *Handler) ListCalls(c *gin.Context) {
	limit, offset := h.parsePagination(c)
	startTime, endTime := h.parseTimeRange(c)

	var systemID, talkgroupID *int
	var sysid *string
	if s := c.Query("system"); s != "" {
		if id, err := strconv.Atoi(s); err == nil {
			systemID = &id
		}
	}
	if s := c.Query("sysid"); s != "" {
		sysid = &s
	}
	if t := c.Query("talkgroup"); t != "" {
		if id, err := strconv.Atoi(t); err == nil {
			talkgroupID = &id
		}
	}

	calls, err := h.db.ListCalls(c.Request.Context(), systemID, sysid, talkgroupID, startTime, endTime, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list calls", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list calls"})
		return
	}

	populateAudioURLs(calls)
	c.JSON(http.StatusOK, gin.H{
		"calls":  calls,
		"count":  len(calls),
		"limit":  limit,
		"offset": offset,
	})
}

// GetCall godoc
// @Summary      Get a call
// @Description  Returns a single call by its trunk-recorder call ID
// @Tags         calls
// @Produce      json
// @Param        id   path      string  true  "Trunk-recorder call ID"
// @Success      200  {object}  models.Call
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /calls/{id} [get]
func (h *Handler) GetCall(c *gin.Context) {
	call, err := h.resolveCall(c)
	if err != nil {
		h.logger.Error("Failed to get call", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get call"})
		return
	}
	if call == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Call not found"})
		return
	}

	populateAudioURL(call)
	c.JSON(http.StatusOK, call)
}

// GetCallAudio godoc
// @Summary      Get call audio
// @Description  Streams the audio file for a call
// @Tags         calls
// @Produce      audio/mpeg
// @Produce      audio/mp4
// @Produce      audio/wav
// @Produce      audio/ogg
// @Param        id   path      string  true  "Trunk-recorder call ID"
// @Success      200  {file}    binary
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /calls/{id}/audio [get]
func (h *Handler) GetCallAudio(c *gin.Context) {
	call, err := h.resolveCall(c)
	if err != nil {
		h.logger.Error("Failed to get call", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get call"})
		return
	}
	if call == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Call not found"})
		return
	}

	if call.AudioPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No audio available"})
		return
	}

	// Join base path with relative audio path from database
	audioPath := filepath.Join(h.audioBasePath, call.AudioPath)

	file, err := os.Open(audioPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Audio file not found"})
		return
	}
	defer file.Close()

	stat, _ := file.Stat()

	// Determine content type based on extension
	contentType := "audio/mpeg"
	ext := filepath.Ext(audioPath)
	switch ext {
	case ".m4a":
		contentType = "audio/mp4"
	case ".wav":
		contentType = "audio/wav"
	case ".ogg":
		contentType = "audio/ogg"
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Length", strconv.FormatInt(stat.Size(), 10))
	c.Header("Accept-Ranges", "bytes")

	io.Copy(c.Writer, file)
}

// GetCallTransmissions godoc
// @Summary      Get call transmissions
// @Description  Returns the list of unit transmissions (srcList) within a call, ordered by position in the audio
// @Tags         calls
// @Produce      json
// @Param        id   path      string  true  "Trunk-recorder call ID"
// @Success      200  {object}  map[string]interface{}  "Response with transmissions array and count"
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /calls/{id}/transmissions [get]
func (h *Handler) GetCallTransmissions(c *gin.Context) {
	call, err := h.resolveCall(c)
	if err != nil {
		h.logger.Error("Failed to get call", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get call"})
		return
	}
	if call == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Call not found"})
		return
	}

	txs, err := h.db.GetTransmissionsByCallID(c.Request.Context(), call.ID)
	if err != nil {
		h.logger.Error("Failed to get transmissions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get transmissions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transmissions": txs,
		"count":         len(txs),
	})
}

// GetCallFrequencies godoc
// @Summary      Get call frequencies
// @Description  Returns the list of frequency entries (freqList) within a call, ordered by position in the audio
// @Tags         calls
// @Produce      json
// @Param        id   path      string  true  "Trunk-recorder call ID"
// @Success      200  {object}  map[string]interface{}  "Response with frequencies array and count"
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /calls/{id}/frequencies [get]
func (h *Handler) GetCallFrequencies(c *gin.Context) {
	call, err := h.resolveCall(c)
	if err != nil {
		h.logger.Error("Failed to get call", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get call"})
		return
	}
	if call == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Call not found"})
		return
	}

	freqs, err := h.db.GetFrequenciesByCallID(c.Request.Context(), call.ID)
	if err != nil {
		h.logger.Error("Failed to get frequencies", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get frequencies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"frequencies": freqs,
		"count":       len(freqs),
	})
}

// ListCallGroups godoc
// @Summary      List call groups
// @Description  Returns deduplicated call groups (calls from multiple recorders grouped together)
// @Tags         call-groups
// @Produce      json
// @Param        start_time  query  string  false  "Start time filter (RFC3339)"
// @Param        end_time    query  string  false  "End time filter (RFC3339)"
// @Param        limit       query  int     false  "Results per page"  default(50)
// @Param        offset      query  int     false  "Page offset"       default(0)
// @Success      200  {object}  rest.CallGroupListResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /call-groups [get]
func (h *Handler) ListCallGroups(c *gin.Context) {
	limit, offset := h.parsePagination(c)
	startTime, endTime := h.parseTimeRange(c)

	query := `
		SELECT id, system_id, tg_sysid, tgid, start_time, end_time, primary_call_id, call_count, encrypted, emergency
		FROM call_groups WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if startTime != nil {
		query += " AND start_time >= $" + strconv.Itoa(argNum)
		args = append(args, *startTime)
		argNum++
	}
	if endTime != nil {
		query += " AND start_time <= $" + strconv.Itoa(argNum)
		args = append(args, *endTime)
		argNum++
	}

	query += " ORDER BY start_time DESC LIMIT $" + strconv.Itoa(argNum)
	args = append(args, limit)
	argNum++
	query += " OFFSET $" + strconv.Itoa(argNum)
	args = append(args, offset)

	rows, err := h.db.Pool().Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to list call groups", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list call groups"})
		return
	}
	defer rows.Close()

	var groups []map[string]interface{}
	for rows.Next() {
		var id int64
		var systemID int
		var tgSysid *string
		var tgid int
		var startTime time.Time
		var endTime *time.Time
		var primaryCallID *int64
		var callCount int
		var encrypted, emergency bool

		if err := rows.Scan(&id, &systemID, &tgSysid, &tgid, &startTime, &endTime, &primaryCallID, &callCount, &encrypted, &emergency); err != nil {
			continue
		}

		groups = append(groups, map[string]interface{}{
			"id":              id,
			"system_id":       systemID,
			"tg_sysid":        tgSysid,
			"tgid":            tgid,
			"start_time":      startTime,
			"end_time":        endTime,
			"primary_call_id": primaryCallID,
			"call_count":      callCount,
			"encrypted":       encrypted,
			"emergency":       emergency,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"call_groups": groups,
		"count":       len(groups),
		"limit":       limit,
		"offset":      offset,
	})
}

// GetCallGroup godoc
// @Summary      Get a call group
// @Description  Returns a call group with all its individual call recordings
// @Tags         call-groups
// @Produce      json
// @Param        id   path      int  true  "Call group ID"
// @Success      200  {object}  rest.CallGroupDetailResponse
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /call-groups/{id} [get]
func (h *Handler) GetCallGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid call group ID"})
		return
	}

	// Get the call group
	var group struct {
		ID            int64      `json:"id"`
		SystemID      int        `json:"system_id"`
		TgSysid       *string    `json:"tg_sysid"`
		TGID          int        `json:"tgid"`
		StartTime     time.Time  `json:"start_time"`
		EndTime       *time.Time `json:"end_time"`
		PrimaryCallID *int64     `json:"primary_call_id"`
		CallCount     int        `json:"call_count"`
		Encrypted     bool       `json:"encrypted"`
		Emergency     bool       `json:"emergency"`
	}

	err = h.db.Pool().QueryRow(c.Request.Context(), `
		SELECT id, system_id, tg_sysid, tgid, start_time, end_time, primary_call_id, call_count, encrypted, emergency
		FROM call_groups WHERE id = $1
	`, id).Scan(&group.ID, &group.SystemID, &group.TgSysid, &group.TGID, &group.StartTime, &group.EndTime, &group.PrimaryCallID, &group.CallCount, &group.Encrypted, &group.Emergency)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Call group not found"})
		return
	}

	// Get all calls in this group
	calls, err := h.db.Pool().Query(c.Request.Context(), `
		SELECT id, instance_id, tr_call_id, start_time, stop_time, duration, error_count, spike_count, signal_db, audio_path
		FROM calls WHERE call_group_id = $1 ORDER BY start_time
	`, id)
	if err != nil {
		h.logger.Error("Failed to get group calls", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get group calls"})
		return
	}
	defer calls.Close()

	var callList []map[string]interface{}
	for calls.Next() {
		var callID int64
		var instanceID int
		var trCallID *string
		var startTime time.Time
		var stopTime *time.Time
		var duration float32
		var errorCount, spikeCount int
		var signalDB float32
		var audioPath *string

		if err := calls.Scan(&callID, &instanceID, &trCallID, &startTime, &stopTime, &duration, &errorCount, &spikeCount, &signalDB, &audioPath); err != nil {
			continue
		}

		callList = append(callList, map[string]interface{}{
			"id":          callID,
			"instance_id": instanceID,
			"tr_call_id":  trCallID,
			"start_time":  startTime,
			"stop_time":   stopTime,
			"duration":    duration,
			"error_count": errorCount,
			"spike_count": spikeCount,
			"signal_db":   signalDB,
			"audio_path":  audioPath,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"call_group": group,
		"calls":      callList,
	})
}

// GetRates godoc
// @Summary      Get decode rates
// @Description  Returns system decode rate measurements over time
// @Tags         stats
// @Produce      json
// @Param        start_time  query  string  false  "Start time filter (RFC3339)"
// @Param        end_time    query  string  false  "End time filter (RFC3339)"
// @Success      200  {object}  rest.RatesResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /stats/rates [get]
func (h *Handler) GetRates(c *gin.Context) {
	startTime, endTime := h.parseTimeRange(c)

	query := `
		SELECT sr.system_id, s.short_name, sr.time, sr.decode_rate, sr.control_channel
		FROM system_rates sr
		JOIN systems s ON s.id = sr.system_id
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if startTime != nil {
		query += " AND sr.time >= $" + strconv.Itoa(argNum)
		args = append(args, *startTime)
		argNum++
	}
	if endTime != nil {
		query += " AND sr.time <= $" + strconv.Itoa(argNum)
		args = append(args, *endTime)
		argNum++
	}

	query += " ORDER BY sr.time DESC LIMIT 1000"

	rows, err := h.db.Pool().Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to get rates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get rates"})
		return
	}
	defer rows.Close()

	var rates []map[string]interface{}
	for rows.Next() {
		var systemID int
		var shortName string
		var rateTime time.Time
		var decodeRate float32
		var controlChannel int64

		if err := rows.Scan(&systemID, &shortName, &rateTime, &decodeRate, &controlChannel); err != nil {
			continue
		}

		rates = append(rates, map[string]interface{}{
			"system_id":       systemID,
			"short_name":      shortName,
			"time":            rateTime,
			"decode_rate":     decodeRate,
			"control_channel": controlChannel,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"rates": rates,
		"count": len(rates),
	})
}

// GetActivity godoc
// @Summary      Get activity summary
// @Description  Returns summary statistics and recent activity across all systems
// @Tags         stats
// @Produce      json
// @Success      200  {object}  rest.ActivityResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /stats/activity [get]
func (h *Handler) GetActivity(c *gin.Context) {
	ctx := c.Request.Context()

	// Get counts
	var systemCount, talkgroupCount, unitCount, callCount int64

	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM systems").Scan(&systemCount)
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM talkgroups").Scan(&talkgroupCount)
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM units").Scan(&unitCount)
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM calls WHERE start_time > NOW() - INTERVAL '24 hours'").Scan(&callCount)

	// Get recent activity by system
	rows, err := h.db.Pool().Query(ctx, `
		SELECT s.short_name, COUNT(*) as call_count
		FROM calls c
		JOIN systems s ON s.id = c.system_id
		WHERE c.start_time > NOW() - INTERVAL '24 hours'
		GROUP BY s.short_name
		ORDER BY call_count DESC
		LIMIT 10
	`)
	if err != nil {
		h.logger.Error("Failed to get activity", zap.Error(err))
	}

	var systemActivity []map[string]interface{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var shortName string
			var count int64
			if err := rows.Scan(&shortName, &count); err != nil {
				continue
			}
			systemActivity = append(systemActivity, map[string]interface{}{
				"system":     shortName,
				"call_count": count,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"systems":         systemCount,
		"talkgroups":      talkgroupCount,
		"units":           unitCount,
		"calls_24h":       callCount,
		"system_activity": systemActivity,
	})
}

// GetStats godoc
// @Summary      Get system statistics
// @Description  Returns overall system statistics including counts and call metrics
// @Tags         stats
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /stats [get]
func (h *Handler) GetStats(c *gin.Context) {
	ctx := c.Request.Context()

	var totalSystems, totalTalkgroups, totalUnits, totalCalls int64
	var callsLastHour, callsLast24h int64
	var audioFiles int64
	var audioBytes int64

	// Get total counts
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM systems").Scan(&totalSystems)
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM talkgroups").Scan(&totalTalkgroups)
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM units").Scan(&totalUnits)
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM calls").Scan(&totalCalls)

	// Get active calls from in-memory tracker (real-time from MQTT)
	var activeCalls int
	if h.processor != nil {
		activeCalls = h.processor.GetActiveCallCount()
	}

	// Get calls in last hour
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM calls WHERE start_time > NOW() - INTERVAL '1 hour'").Scan(&callsLastHour)

	// Get calls in last 24 hours
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM calls WHERE start_time > NOW() - INTERVAL '24 hours'").Scan(&callsLast24h)

	// Get audio file stats (from calls table where audio exists)
	h.db.Pool().QueryRow(ctx, "SELECT COUNT(*), COALESCE(SUM(audio_size), 0) FROM calls WHERE audio_path IS NOT NULL").Scan(&audioFiles, &audioBytes)

	c.JSON(http.StatusOK, gin.H{
		"total_systems":    totalSystems,
		"total_talkgroups": totalTalkgroups,
		"total_units":      totalUnits,
		"total_calls":      totalCalls,
		"active_calls":     activeCalls,
		"calls_last_hour":  callsLastHour,
		"calls_last_24h":   callsLast24h,
		"audio_files":      audioFiles,
		"audio_bytes":      audioBytes,
	})
}

// GetActiveCallsRealtime godoc
// @Summary      Get real-time active calls
// @Description  Returns currently active calls from in-memory tracker (real-time from MQTT)
// @Tags         calls
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /calls/active/realtime [get]
func (h *Handler) GetActiveCallsRealtime(c *gin.Context) {
	if h.processor == nil {
		// No processor in watch mode - return empty
		c.JSON(http.StatusOK, gin.H{
			"calls": []interface{}{},
			"count": 0,
		})
		return
	}
	calls := h.processor.GetActiveCalls()
	c.JSON(http.StatusOK, gin.H{
		"calls": calls,
		"count": len(calls),
	})
}

// GetRecentCalls godoc
// @Summary      Get recently ended calls
// @Description  Returns recently completed calls from database with full unit list and audio status
// @Tags         calls
// @Produce      json
// @Param        limit  query  int  false  "Number of calls to return"  default(50)
// @Success      200  {object}  map[string]interface{}
// @Router       /calls/recent [get]
func (h *Handler) GetRecentCalls(c *gin.Context) {
	limit := 50
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}

	calls, err := h.db.ListRecentCalls(c.Request.Context(), limit)
	if err != nil {
		h.logger.Error("Failed to list recent calls", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list recent calls"})
		return
	}

	populateRecentCallAudioURLs(calls)
	c.JSON(http.StatusOK, gin.H{
		"calls": calls,
		"count": len(calls),
	})
}

// ListActiveCalls godoc
// @Summary      List active calls
// @Description  Returns currently active calls (calls without a stop_time)
// @Tags         calls
// @Produce      json
// @Param        system     query  string  false  "System ID or short_name"
// @Param        sys_name   query  string  false  "System short name"
// @Param        talkgroup  query  int     false  "Filter by talkgroup ID"
// @Param        emergency  query  bool    false  "Filter by emergency status"
// @Param        encrypted  query  bool    false  "Filter by encryption status"
// @Param        limit      query  int     false  "Results per page"  default(50)
// @Param        offset     query  int     false  "Page offset"       default(0)
// @Success      200  {object}  rest.CallListResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /calls/active [get]
func (h *Handler) ListActiveCalls(c *gin.Context) {
	limit, offset := h.parsePagination(c)

	filters := database.ActiveCallFilters{}

	// Parse system filter (by ID or short_name)
	if s := c.Query("system"); s != "" {
		if id, err := strconv.Atoi(s); err == nil {
			filters.SystemID = &id
		} else {
			// Treat as short_name
			filters.ShortName = &s
		}
	}
	if s := c.Query("sys_name"); s != "" {
		filters.ShortName = &s
	}

	if t := c.Query("talkgroup"); t != "" {
		if id, err := strconv.Atoi(t); err == nil {
			filters.TGID = &id
		}
	}

	if e := c.Query("emergency"); e != "" {
		val := e == "true" || e == "1"
		filters.Emergency = &val
	}

	if e := c.Query("encrypted"); e != "" {
		val := e == "true" || e == "1"
		filters.Encrypted = &val
	}

	calls, err := h.db.ListActiveCalls(c.Request.Context(), filters, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list active calls", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list active calls"})
		return
	}

	populateAudioURLs(calls)
	c.JSON(http.StatusOK, gin.H{
		"calls":  calls,
		"count":  len(calls),
		"limit":  limit,
		"offset": offset,
	})
}

// ListActiveUnits godoc
// @Summary      List active units
// @Description  Returns units that have had recent activity within the specified time window
// @Tags         units
// @Produce      json
// @Param        window     query  int     false  "Activity window in minutes (1-60)"  default(5)
// @Param        sysid      query  string  false  "Filter by SYSID (P25 system identifier)"
// @Param        system     query  string  false  "System ID or short_name (deprecated, use sysid)"
// @Param        sys_name   query  string  false  "System short name"
// @Param        talkgroup  query  int     false  "Filter by talkgroup ID"
// @Param        limit      query  int     false  "Results per page"  default(50)
// @Param        offset     query  int     false  "Page offset"       default(0)
// @Success      200  {object}  rest.ActiveUnitListResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /units/active [get]
func (h *Handler) ListActiveUnits(c *gin.Context) {
	limit, offset := h.parsePagination(c)

	filters := database.ActiveUnitFilters{
		WindowMins: 5, // Default 5 minutes
	}

	// Parse window parameter
	if w := c.Query("window"); w != "" {
		if mins, err := strconv.Atoi(w); err == nil && mins > 0 && mins <= 60 {
			filters.WindowMins = mins
		}
	}

	// Parse SYSID filter (preferred for units)
	if s := c.Query("sysid"); s != "" {
		filters.SYSID = &s
	}

	// Parse system filter (by ID or short_name) - legacy support
	if s := c.Query("system"); s != "" {
		if id, err := strconv.Atoi(s); err == nil {
			filters.SystemID = &id
		} else {
			// Treat as short_name
			filters.ShortName = &s
		}
	}
	if s := c.Query("sys_name"); s != "" {
		filters.ShortName = &s
	}

	if t := c.Query("talkgroup"); t != "" {
		if id, err := strconv.Atoi(t); err == nil {
			filters.TGID = &id
		}
	}

	// Parse sort options
	if s := c.Query("sort"); s != "" {
		filters.SortField = s
	}
	if d := c.Query("sort_dir"); d != "" {
		filters.SortDir = d
	}

	units, err := h.db.ListActiveUnits(c.Request.Context(), filters, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list active units", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list active units"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"units":  units,
		"count":  len(units),
		"limit":  limit,
		"offset": offset,
		"window": filters.WindowMins,
	})
}

// ListRecorders godoc
// @Summary      List recorder states
// @Description  Returns all known recorder states from in-memory cache (populated by MQTT recorder messages)
// @Tags         recorders
// @Produce      json
// @Success      200  {object}  rest.RecorderListResponse
// @Router       /recorders [get]
func (h *Handler) ListRecorders(c *gin.Context) {
	// Try processor first (MQTT mode)
	if h.processor != nil {
		recorders := h.processor.GetRecorders()
		c.JSON(http.StatusOK, gin.H{
			"recorders": recorders,
			"count":     len(recorders),
		})
		return
	}

	// Fall back to recorder provider (watch mode)
	if h.recorderProvider != nil {
		recorders := h.recorderProvider.GetRecorders()
		c.JSON(http.StatusOK, gin.H{
			"recorders": recorders,
		})
		return
	}

	// No recorder source available
	c.JSON(http.StatusOK, gin.H{
		"recorders": []interface{}{},
		"count":     0,
	})
}

// ============================================================================
// Transcription endpoints
// ============================================================================

// GetCallTranscription godoc
// @Summary      Get call transcription
// @Description  Returns the transcription for a specific call
// @Tags         calls
// @Produce      json
// @Param        id   path      string  true  "Call ID (tr_call_id or numeric ID)"
// @Success      200  {object}  models.Transcription
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /calls/{id}/transcription [get]
func (h *Handler) GetCallTranscription(c *gin.Context) {
	call, err := h.resolveCall(c)
	if err != nil {
		h.logger.Error("Failed to get call", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get call"})
		return
	}
	if call == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Call not found"})
		return
	}

	transcription, err := h.db.GetTranscriptionByCallID(c.Request.Context(), call.ID)
	if err != nil {
		h.logger.Error("Failed to get transcription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get transcription"})
		return
	}
	if transcription == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No transcription available for this call"})
		return
	}

	// Include call duration for word timeline rendering
	transcription.CallDuration = call.Duration

	c.JSON(http.StatusOK, transcription)
}

// TranscribeCallRequest represents a request to queue a call for transcription
type TranscribeCallRequest struct {
	Priority int `json:"priority"`
}

// QueueCallTranscription godoc
// @Summary      Queue call for transcription
// @Description  Queues a call for transcription (or re-transcription)
// @Tags         calls
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Call ID (tr_call_id or numeric ID)"
// @Param        body body      rest.TranscribeCallRequest  false  "Request body"
// @Success      202  {object}  map[string]interface{}
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      404  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /calls/{id}/transcribe [post]
func (h *Handler) QueueCallTranscription(c *gin.Context) {
	call, err := h.resolveCall(c)
	if err != nil {
		h.logger.Error("Failed to get call", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get call"})
		return
	}
	if call == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Call not found"})
		return
	}

	if call.AudioPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Call has no audio"})
		return
	}

	var req TranscribeCallRequest
	c.ShouldBindJSON(&req) // Ignore errors, use defaults

	priority := req.Priority
	if priority < 0 {
		priority = 0
	}
	if priority > 100 {
		priority = 100
	}

	if err := h.db.QueueTranscription(c.Request.Context(), call.ID, priority); err != nil {
		h.logger.Error("Failed to queue transcription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue transcription"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":  "Call queued for transcription",
		"call_id":  call.ID,
		"priority": priority,
	})
}

// GetRecentTranscriptions godoc
// @Summary      Get recent transcriptions
// @Description  Returns recently created transcriptions with call context
// @Tags         transcriptions
// @Produce      json
// @Param        limit   query  int     false  "Max results"  default(20)
// @Param        offset  query  int     false  "Page offset"  default(0)
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /transcriptions/recent [get]
func (h *Handler) GetRecentTranscriptions(c *gin.Context) {
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	offset := 0
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	transcriptions, err := h.db.ListRecentTranscriptions(c.Request.Context(), limit, offset)
	if err != nil {
		h.logger.Error("Failed to get recent transcriptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get recent transcriptions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transcriptions": transcriptions,
		"count":          len(transcriptions),
	})
}

// SearchTranscriptions godoc
// @Summary      Search transcriptions
// @Description  Full-text search across all transcriptions
// @Tags         transcriptions
// @Produce      json
// @Param        q       query  string  true   "Search query"
// @Param        limit   query  int     false  "Results per page"  default(50)
// @Param        offset  query  int     false  "Page offset"       default(0)
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  rest.ErrorResponse
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /transcriptions/search [get]
func (h *Handler) SearchTranscriptions(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Search query 'q' is required"})
		return
	}

	limit, offset := h.parsePagination(c)

	transcriptions, err := h.db.SearchTranscriptions(c.Request.Context(), query, limit, offset)
	if err != nil {
		h.logger.Error("Failed to search transcriptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search transcriptions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transcriptions": transcriptions,
		"count":          len(transcriptions),
		"query":          query,
		"limit":          limit,
		"offset":         offset,
	})
}

// GetTranscriptionStatus godoc
// @Summary      Get transcription queue status
// @Description  Returns statistics about the transcription queue
// @Tags         transcriptions
// @Produce      json
// @Success      200  {object}  database.TranscriptionQueueStats
// @Failure      500  {object}  rest.ErrorResponse
// @Router       /transcription/status [get]
func (h *Handler) GetTranscriptionStatus(c *gin.Context) {
	stats, err := h.db.GetTranscriptionQueueStats(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get transcription status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get transcription status"})
		return
	}

	c.JSON(http.StatusOK, stats)
}
