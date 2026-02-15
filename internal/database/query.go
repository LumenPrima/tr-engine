package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// QueryResult holds the result of a read-only SQL query.
type QueryResult struct {
	Columns  []string `json:"columns"`
	Rows     [][]any  `json:"rows"`
	RowCount int      `json:"row_count"`
}

// ExecuteReadOnlyQuery runs a SQL query inside a read-only transaction with a
// statement timeout. It returns column names and up to maxRows of results.
func (db *DB) ExecuteReadOnlyQuery(ctx context.Context, sql string, params []any, maxRows int) (*QueryResult, error) {
	if strings.Contains(sql, ";") {
		return nil, fmt.Errorf("multiple statements not allowed")
	}

	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '30s'"); err != nil {
		return nil, fmt.Errorf("set statement timeout: %w", err)
	}

	rows, err := tx.Query(ctx, sql, params...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	// Extract column names from field descriptions.
	fields := rows.FieldDescriptions()
	columns := make([]string, len(fields))
	for i, f := range fields {
		columns[i] = f.Name
	}

	var resultRows [][]any
	for rows.Next() {
		if len(resultRows) >= maxRows {
			break
		}
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		resultRows = append(resultRows, values)
	}
	// Close rows before checking errors or committing — breaking out of
	// the Next() loop early leaves the connection in query mode, which
	// causes "conn busy" on commit.
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	if resultRows == nil {
		resultRows = [][]any{}
	}

	return &QueryResult{
		Columns:  columns,
		Rows:     resultRows,
		RowCount: len(resultRows),
	}, nil
}
