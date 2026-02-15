package database

import (
	"context"
	"fmt"
	"time"
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
	qb := newQueryBuilder()

	if filter.InstanceID != nil {
		qb.Add("cm.instance_id = %s", *filter.InstanceID)
	}
	if filter.Severity != nil {
		qb.Add("cm.severity = %s", *filter.Severity)
	}
	if filter.StartTime != nil {
		qb.Add("cm.log_time >= %s", *filter.StartTime)
	} else {
		qb.Add("cm.log_time >= %s", time.Now().Add(-1*time.Hour))
	}
	if filter.EndTime != nil {
		qb.Add("cm.log_time < %s", *filter.EndTime)
	}

	fromClause := "FROM console_messages cm"
	whereClause := qb.WhereClause()

	var total int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) "+fromClause+whereClause, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := fmt.Sprintf(`
		SELECT cm.id, COALESCE(cm.instance_id, ''), COALESCE(cm.severity, ''),
			COALESCE(cm.log_msg, ''), cm.log_time
		%s %s
		ORDER BY cm.log_time DESC
		LIMIT %d OFFSET %d
	`, fromClause, whereClause, filter.Limit, filter.Offset)

	rows, err := db.Pool.Query(ctx, dataQuery, qb.Args()...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var messages []ConsoleMessageAPI
	for rows.Next() {
		var m ConsoleMessageAPI
		if err := rows.Scan(
			&m.ID, &m.InstanceID, &m.Severity,
			&m.LogMsg, &m.LogTime,
		); err != nil {
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
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO console_messages (instance_id, log_time, severity, log_msg, mqtt_timestamp)
		VALUES ($1, $2, $3, $4, $5)
	`, instanceID, logTime, severity, logMsg, mqttTimestamp)
	return err
}
