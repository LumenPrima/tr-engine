package database

import (
	"context"

	"github.com/snarg/tr-engine/internal/database/sqlcdb"
)

// InsertInstanceConfig stores a snapshot of a TR instance's configuration.
func (db *DB) InsertInstanceConfig(ctx context.Context, instanceID, captureDir, uploadServer string, callTimeout float64, logFile, instanceKey string, configJSON []byte) error {
	ct := float32(callTimeout)
	return db.Q.InsertInstanceConfig(ctx, sqlcdb.InsertInstanceConfigParams{
		InstanceID:   &instanceID,
		CaptureDir:   &captureDir,
		UploadServer: &uploadServer,
		CallTimeout:  &ct,
		LogFile:      &logFile,
		InstanceKey:  &instanceKey,
		ConfigJson:   configJSON,
	})
}
