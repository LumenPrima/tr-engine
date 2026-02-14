package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (p *Pipeline) handleConfig(payload []byte) error {
	var msg ConfigMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	cfg := &msg.Config

	// log_file can be bool or string depending on TR version — normalize to string
	logFile := string(cfg.LogFile)

	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()

	if err := p.db.InsertInstanceConfig(ctx,
		msg.InstanceID,
		cfg.CaptureDir,
		cfg.UploadServer,
		cfg.CallTimeout,
		logFile,
		cfg.InstanceKey,
		payload,
	); err != nil {
		return fmt.Errorf("insert instance config: %w", err)
	}

	p.log.Info().
		Str("instance_id", msg.InstanceID).
		Str("capture_dir", cfg.CaptureDir).
		Msg("stored instance config")

	return nil
}
