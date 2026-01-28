package dedup

import (
	"context"
	"time"

	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
	"github.com/trunk-recorder/tr-engine/internal/metrics"
	"go.uber.org/zap"
)

// Engine handles call deduplication
type Engine struct {
	db        *database.DB
	config    config.DeduplicationConfig
	logger    *zap.Logger
	threshold float64
}

// NewEngine creates a new deduplication engine
func NewEngine(db *database.DB, cfg config.DeduplicationConfig, logger *zap.Logger) *Engine {
	threshold := cfg.Threshold
	if threshold == 0 {
		threshold = 0.7
	}

	return &Engine{
		db:        db,
		config:    cfg,
		logger:    logger,
		threshold: threshold,
	}
}

// IsEnabled returns whether deduplication is enabled
func (e *Engine) IsEnabled() bool {
	return e.config.Enabled
}

// ProcessCall processes a call for deduplication
func (e *Engine) ProcessCall(ctx context.Context, call *models.Call, tgid int, shortName string) (*models.CallGroup, error) {
	if !e.config.Enabled {
		return nil, nil
	}

	// Track dedup processing
	metrics.DedupCallsProcessed.WithLabelValues(shortName).Inc()

	// Check if this is a P25 system (has WACN+SysID) for cross-site dedup
	sameLogicalSystem, err := e.db.IsP25System(ctx, call.SystemID)
	if err != nil {
		e.logger.Warn("Failed to check P25 system status", zap.Error(err))
		sameLogicalSystem = false
	}

	// Find potential duplicate call groups
	candidates, err := e.findCandidates(ctx, call.SystemID, tgid, call.StartTime)
	if err != nil {
		return nil, err
	}

	// Score each candidate
	var bestMatch *models.CallGroup
	var bestScore float64

	for _, candidate := range candidates {
		score := e.scoreMatch(call, candidate, sameLogicalSystem)
		// Track score distribution
		metrics.DedupScoreDistribution.Observe(score)
		if score > bestScore && score >= e.threshold {
			bestMatch = candidate
			bestScore = score
		}
	}

	// Link to existing group or create new one
	if bestMatch != nil {
		metrics.DedupGroupsLinked.WithLabelValues(shortName).Inc()
		return e.linkToGroup(ctx, call, bestMatch)
	}
	metrics.DedupGroupsCreated.WithLabelValues(shortName).Inc()
	return e.createGroup(ctx, call, tgid)
}

// findCandidates finds potential duplicate call groups
func (e *Engine) findCandidates(ctx context.Context, systemID, tgid int, startTime time.Time) ([]*models.CallGroup, error) {
	return e.db.FindCallGroupCandidates(ctx, systemID, tgid, startTime, e.config.TimeWindowSeconds)
}

// scoreMatch scores how well a call matches a call group
// sameLogicalSystem indicates if the candidate came from a same P25 system (WACN+SysID match)
func (e *Engine) scoreMatch(call *models.Call, group *models.CallGroup, sameLogicalSystem bool) float64 {
	score := 0.0

	// Time overlap (0-40 points)
	overlap := e.calculateOverlap(call, group)
	score += overlap * 40

	// Same system (30 points) - includes same logical P25 system (same WACN+SysID, different NAC)
	if call.SystemID == group.SystemID || sameLogicalSystem {
		score += 30
	}

	// Duration similarity (0-20 points)
	var groupDuration float32
	if group.EndTime != nil {
		groupDuration = float32(group.EndTime.Sub(group.StartTime).Seconds())
	}
	durationDiff := abs(call.Duration - groupDuration)
	if durationDiff < 1.0 {
		score += 20
	} else if durationDiff < 3.0 {
		score += 10
	}

	// Emergency/encrypted match (0-10 points)
	if call.Emergency == group.Emergency && call.Encrypted == group.Encrypted {
		score += 10
	}

	return score / 100.0 // Normalize to 0-1
}

// calculateOverlap calculates the time overlap between a call and a group
func (e *Engine) calculateOverlap(call *models.Call, group *models.CallGroup) float64 {
	callStart := call.StartTime.Unix()
	callEnd := callStart
	if call.StopTime != nil {
		callEnd = call.StopTime.Unix()
	} else {
		callEnd = callStart + int64(call.Duration)
	}

	groupStart := group.StartTime.Unix()
	groupEnd := groupStart
	if group.EndTime != nil {
		groupEnd = group.EndTime.Unix()
	}

	// Calculate overlap
	overlapStart := max(callStart, groupStart)
	overlapEnd := min(callEnd, groupEnd)

	if overlapEnd <= overlapStart {
		return 0
	}

	// Overlap as fraction of call duration
	callDuration := float64(callEnd - callStart)
	if callDuration == 0 {
		return 0
	}

	overlap := float64(overlapEnd - overlapStart)
	return overlap / callDuration
}

// linkToGroup links a call to an existing call group
func (e *Engine) linkToGroup(ctx context.Context, call *models.Call, group *models.CallGroup) (*models.CallGroup, error) {
	// Update call with group ID
	call.CallGroupID = &group.ID

	// Update group statistics
	group.CallCount++

	// Update end time if this call ends later
	if call.StopTime != nil && (group.EndTime == nil || call.StopTime.After(*group.EndTime)) {
		group.EndTime = call.StopTime
	}

	// Update emergency/encrypted flags
	if call.Emergency {
		group.Emergency = true
	}
	if call.Encrypted {
		group.Encrypted = true
	}

	// Re-evaluate primary call (best quality)
	primaryCall, err := e.selectPrimaryCall(ctx, group)
	if err != nil {
		e.logger.Error("Failed to select primary call", zap.Error(err))
	} else if primaryCall != nil {
		group.PrimaryCallID = &primaryCall.ID
	}

	if err := e.db.UpdateCallGroup(ctx, group); err != nil {
		return nil, err
	}

	e.logger.Debug("Linked call to group",
		zap.Int64("call_id", call.ID),
		zap.Int64("group_id", group.ID),
		zap.Int("call_count", group.CallCount),
	)

	return group, nil
}

// createGroup creates a new call group for a call
func (e *Engine) createGroup(ctx context.Context, call *models.Call, tgid int) (*models.CallGroup, error) {
	group := &models.CallGroup{
		SystemID:      call.SystemID,
		TalkgroupID:   call.TalkgroupID,
		TGID:          tgid,
		StartTime:     call.StartTime,
		EndTime:       call.StopTime,
		PrimaryCallID: &call.ID,
		CallCount:     1,
		Encrypted:     call.Encrypted,
		Emergency:     call.Emergency,
	}

	if err := e.db.InsertCallGroup(ctx, group); err != nil {
		return nil, err
	}

	e.logger.Debug("Created call group",
		zap.Int64("group_id", group.ID),
		zap.Int("tgid", tgid),
	)

	return group, nil
}

// selectPrimaryCall selects the best quality call from a group
func (e *Engine) selectPrimaryCall(ctx context.Context, group *models.CallGroup) (*models.Call, error) {
	// Query all calls in this group
	rows, err := e.db.Pool().Query(ctx, `
		SELECT id, error_count, spike_count, signal_db, duration
		FROM calls
		WHERE call_group_id = $1
		ORDER BY
			error_count ASC,
			spike_count ASC,
			signal_db DESC,
			duration DESC
		LIMIT 1
	`, group.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		var call models.Call
		if err := rows.Scan(&call.ID, &call.ErrorCount, &call.SpikeCount, &call.SignalDB, &call.Duration); err != nil {
			return nil, err
		}
		return &call, nil
	}

	return nil, nil
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
