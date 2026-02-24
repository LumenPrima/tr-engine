package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/snarg/tr-engine/internal/database/sqlcdb"
)

// ConsoleMessageFilter specifies filters for listing console messages.
type ConsoleMessageFilter struct {
	InstanceID *string
	Severity   *string
	StartTime  *time.Time
	EndTime    *time.Time
	Limit      int
	Offset     int
}

// ConsoleMessageAPI represents a console message for API responses.
type ConsoleMessageAPI struct {
	ID         int64     `json:"id"`
	InstanceID string    `json:"instance_id"`
	Severity   string    `json:"severity"`
	LogMsg     string    `json:"log_msg"`
	LogTime    time.Time `json:"log_time"`
}

// ListConsoleMessages returns console messages matching the filter.
func (db *DB) ListConsoleMessages(ctx context.Context, filter ConsoleMessageFilter) ([]ConsoleMessageAPI, int, error) {
	startTime := filter.StartTime
	if startTime == nil {
		t := time.Now().Add(-1 * time.Hour)
		startTime = &t
	}

	const whereClause = `
		WHERE ($1::text IS NULL OR cm.instance_id = $1)
		  AND ($2::text IS NULL OR cm.severity = $2)
		  AND cm.log_time >= $3
		  AND ($4::timestamptz IS NULL OR cm.log_time < $4)`
	args := []any{filter.InstanceID, filter.Severity, *startTime, filter.EndTime}

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM console_messages cm"+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := `
		SELECT cm.id, COALESCE(cm.instance_id, ''), COALESCE(cm.severity, ''),
			COALESCE(cm.log_msg, ''), cm.log_time
		FROM console_messages cm` + whereClause + `
		ORDER BY cm.log_time DESC
		LIMIT $5 OFFSET $6`

	rows, err := db.Pool.Query(ctx, dataQuery, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var messages []ConsoleMessageAPI
	for rows.Next() {
		var m ConsoleMessageAPI
		if err := rows.Scan(&m.ID, &m.InstanceID, &m.Severity, &m.LogMsg, &m.LogTime); err != nil {
			return nil, 0, err
		}
		messages = append(messages, m)
	}
	if messages == nil {
		messages = []ConsoleMessageAPI{}
	}
	return messages, total, rows.Err()
}

func (db *DB) InsertConsoleMessage(ctx context.Context, instanceID string, logTime time.Time, severity, logMsg string, mqttTimestamp time.Time) error {
	return db.Q.InsertConsoleMessage(ctx, sqlcdb.InsertConsoleMessageParams{
		InstanceID:    &instanceID,
		LogTime:       pgtype.Timestamptz{Time: logTime, Valid: true},
		Severity:      &severity,
		LogMsg:        &logMsg,
		MqttTimestamp: pgtype.Timestamptz{Time: mqttTimestamp, Valid: true},
	})
}
