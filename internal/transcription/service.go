package transcription

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/trunk-recorder/tr-engine/internal/api/ws"
	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
	"github.com/trunk-recorder/tr-engine/internal/metrics"
	"go.uber.org/zap"
)

const (
	pollInterval = 2 * time.Second
	maxRetries   = 3
)

// Service manages transcription processing
type Service struct {
	db            *database.DB
	provider      Provider
	preprocessor  *Preprocessor
	cfg           config.TranscriptionConfig
	audioBasePath string
	hub           *ws.Hub
	logger        *zap.Logger

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	numWorkers int
}

// NewService creates a new transcription service
func NewService(db *database.DB, cfg config.TranscriptionConfig, audioBasePath string, logger *zap.Logger) (*Service, error) {
	provider, err := NewProvider(cfg, audioBasePath, logger)
	if err != nil {
		return nil, err
	}

	// Initialize preprocessor if enabled
	var preprocessor *Preprocessor
	if cfg.Preprocess.Enabled {
		var err error
		preprocessor, err = NewPreprocessor(cfg.Preprocess, logger)
		if err != nil {
			logger.Warn("Failed to initialize audio preprocessor, continuing without preprocessing",
				zap.Error(err),
			)
		}
	}

	numWorkers := cfg.Concurrency
	if numWorkers < 1 {
		numWorkers = 1
	}
	if numWorkers > 10 {
		numWorkers = 10 // Cap at 10 workers
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Service{
		db:            db,
		provider:      provider,
		preprocessor:  preprocessor,
		cfg:           cfg,
		audioBasePath: audioBasePath,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
		numWorkers:    numWorkers,
	}, nil
}

// SetHub sets the WebSocket hub for broadcasting events
func (s *Service) SetHub(hub *ws.Hub) {
	s.hub = hub
}

// Start starts the background worker goroutines
func (s *Service) Start() {
	preprocessStatus := "disabled"
	if s.preprocessor != nil {
		preprocessStatus = "enabled"
	}

	s.logger.Info("Starting transcription service",
		zap.String("provider", s.provider.Name()),
		zap.Int("workers", s.numWorkers),
		zap.Float64("min_duration", s.cfg.MinDuration),
		zap.String("preprocessing", preprocessStatus),
	)

	for i := 0; i < s.numWorkers; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
}

// Stop gracefully stops the service
func (s *Service) Stop() {
	s.logger.Info("Stopping transcription service")
	s.cancel()
	s.wg.Wait()
	if s.provider != nil {
		s.provider.Close()
	}
	if s.preprocessor != nil {
		s.preprocessor.Close()
	}
	s.logger.Info("Transcription service stopped")
}

// QueueCall queues a call for transcription if it meets the criteria.
// callID is the deterministic call ID in sysid:tgid:start_unix format.
func (s *Service) QueueCall(ctx context.Context, callID string, duration float32, priority int) error {
	// Check minimum duration
	if float64(duration) < s.cfg.MinDuration {
		s.logger.Debug("Call too short for transcription",
			zap.String("call_id", callID),
			zap.Float32("duration", duration),
			zap.Float64("min_duration", s.cfg.MinDuration),
		)
		return nil
	}

	if err := s.db.QueueTranscription(ctx, callID, priority); err != nil {
		s.logger.Error("Failed to queue transcription",
			zap.String("call_id", callID),
			zap.Error(err),
		)
		return err
	}

	s.logger.Debug("Queued call for transcription",
		zap.String("call_id", callID),
		zap.Float32("duration", duration),
		zap.Int("priority", priority),
	)

	metrics.TranscriptionQueueDepth.Inc()
	return nil
}

// worker is the main loop for a transcription worker
func (s *Service) worker(id int) {
	defer s.wg.Done()

	s.logger.Debug("Transcription worker started", zap.Int("worker_id", id))

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Debug("Transcription worker stopping", zap.Int("worker_id", id))
			return
		case <-ticker.C:
			s.processNextJob(id)
		}
	}
}

// processNextJob attempts to process the next pending transcription job
func (s *Service) processNextJob(workerID int) {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	// Get next pending job with row locking
	item, err := s.db.GetPendingTranscription(ctx)
	if err != nil {
		s.logger.Error("Failed to get pending transcription", zap.Error(err))
		return
	}
	if item == nil {
		return // No pending jobs
	}

	// Mark as processing
	if err := s.db.MarkTranscriptionProcessing(ctx, item.ID); err != nil {
		s.logger.Error("Failed to mark transcription as processing",
			zap.Int64("queue_id", item.ID),
			zap.Error(err),
		)
		return
	}

	s.logger.Debug("Processing transcription job",
		zap.Int("worker_id", workerID),
		zap.Int64("queue_id", item.ID),
		zap.Int64("call_id", item.CallID),
		zap.Int("attempt", item.Attempts+1),
	)

	// Get call details
	call, err := s.db.GetCallByID(ctx, item.CallID)
	if err != nil {
		s.logger.Error("Failed to get call",
			zap.Int64("call_id", item.CallID),
			zap.Error(err),
		)
		s.markFailed(ctx, item, "failed to get call: "+err.Error())
		return
	}
	if call == nil {
		s.logger.Warn("Call not found for transcription",
			zap.Int64("call_id", item.CallID),
		)
		s.db.DeleteTranscriptionQueueItem(ctx, item.ID)
		return
	}

	// Populate deterministic call ID for logging
	call.PopulateCallID()

	if call.AudioPath == "" {
		s.logger.Warn("Call has no audio path",
			zap.String("call_id", call.CallID),
		)
		s.db.DeleteTranscriptionQueueItem(ctx, item.ID)
		return
	}

	// Build full audio path
	audioPath := filepath.Join(s.audioBasePath, call.AudioPath)

	// Preprocess audio if enabled
	processedPath := audioPath
	if s.preprocessor != nil {
		var err error
		processedPath, err = s.preprocessor.Process(ctx, audioPath)
		if err != nil {
			s.logger.Warn("Audio preprocessing failed, using original file",
				zap.String("call_id", call.CallID),
				zap.Error(err),
			)
			processedPath = audioPath
		} else if processedPath != audioPath {
			// Clean up processed file after we're done
			defer s.preprocessor.Cleanup(processedPath)
		}
	}

	// Perform transcription
	startTime := time.Now()
	result, err := s.provider.Transcribe(ctx, processedPath, s.cfg.Language)
	processingTime := time.Since(startTime)

	if err != nil {
		s.logger.Error("Transcription failed",
			zap.String("call_id", call.CallID),
			zap.String("audio_path", audioPath),
			zap.Error(err),
		)

		// Check if we should retry
		if item.Attempts+1 < s.getMaxRetries() {
			s.db.UpdateTranscriptionQueueStatus(ctx, item.ID, "pending", err.Error())
		} else {
			s.markFailed(ctx, item, err.Error())
		}

		metrics.TranscriptionErrors.WithLabelValues(s.provider.Name()).Inc()
		return
	}

	// Count words
	wordCount := len(strings.Fields(result.Text))

	// Convert provider words to model words
	var words []models.TranscriptionWord
	for _, w := range result.Words {
		words = append(words, models.TranscriptionWord{
			Word:  w.Word,
			Start: w.Start,
			End:   w.End,
		})
	}

	// Save transcription result
	transcription := &models.Transcription{
		CallID:     item.CallID,
		Provider:   s.provider.Name(),
		Model:      result.Model,
		Language:   result.Language,
		Text:       result.Text,
		WordCount:  wordCount,
		DurationMs: result.Duration,
		Words:      words,
	}

	if result.Confidence > 0 {
		transcription.Confidence = &result.Confidence
	}

	if err := s.db.InsertTranscription(ctx, transcription); err != nil {
		s.logger.Error("Failed to save transcription",
			zap.String("call_id", call.CallID),
			zap.Error(err),
		)
		s.markFailed(ctx, item, "failed to save: "+err.Error())
		return
	}

	// Mark as completed and remove from queue
	s.db.UpdateTranscriptionQueueStatus(ctx, item.ID, "completed", "")
	s.db.DeleteTranscriptionQueueItem(ctx, item.ID)

	s.logger.Info("Transcription completed",
		zap.String("call_id", call.CallID),
		zap.Int64("transcription_id", transcription.ID),
		zap.Int("word_count", wordCount),
		zap.Duration("processing_time", processingTime),
	)

	// Track metrics
	metrics.TranscriptionsCompleted.WithLabelValues(s.provider.Name()).Inc()
	metrics.TranscriptionDuration.Observe(processingTime.Seconds())
	metrics.TranscriptionQueueDepth.Dec()

	// Broadcast event with call context
	s.broadcast("transcription_complete", map[string]interface{}{
		"call_id":          call.CallID, // Deterministic: sysid:tgid:start
		"transcription_id": transcription.ID,
		"text":             result.Text,
		"word_count":       wordCount,
		"tgid":             call.TGID,       // For filtering
		"tg_alpha_tag":     call.TGAlphaTag,
		"sysid":            call.TgSysid, // For filtering
		"duration":         call.Duration,
		"language":         result.Language,
		"provider":         s.provider.Name(),
		"words":            words,
	})
}

// markFailed marks a transcription job as failed
func (s *Service) markFailed(ctx context.Context, item *models.TranscriptionQueueItem, errMsg string) {
	s.db.UpdateTranscriptionQueueStatus(ctx, item.ID, "failed", errMsg)
	metrics.TranscriptionQueueDepth.Dec()
}

// getMaxRetries returns the configured max retries
func (s *Service) getMaxRetries() int {
	if s.cfg.RetryCount > 0 {
		return s.cfg.RetryCount
	}
	return maxRetries
}

// broadcast sends an event to WebSocket clients
func (s *Service) broadcast(eventType string, data interface{}) {
	if s.hub == nil {
		return
	}

	event := ws.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Data:      data,
	}

	s.hub.Broadcast(event)
}

// BackfillQueue queues existing calls that haven't been transcribed
func (s *Service) BackfillQueue(ctx context.Context, batchSize int) (int, error) {
	callIDs, err := s.db.GetCallsForTranscriptionBackfill(ctx, s.cfg.MinDuration, batchSize)
	if err != nil {
		return 0, err
	}

	queued := 0
	for _, callID := range callIDs {
		if err := s.db.QueueTranscription(ctx, callID, 0); err != nil {
			s.logger.Error("Failed to queue call for backfill",
				zap.String("call_id", callID),
				zap.Error(err),
			)
			continue
		}
		queued++
	}

	if queued > 0 {
		s.logger.Info("Queued calls for transcription backfill",
			zap.Int("queued", queued),
		)
	}

	return queued, nil
}

// IsEnabled returns whether transcription is enabled
func (s *Service) IsEnabled() bool {
	return s.cfg.Enabled
}
