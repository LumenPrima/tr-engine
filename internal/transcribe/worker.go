package transcribe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/audio"
	"github.com/snarg/tr-engine/internal/database"
)

// Job represents a transcription job enqueued by the ingest pipeline.
type Job struct {
	CallID        int64
	CallStartTime time.Time
	SystemID      int
	Tgid          int
	Duration      float32
	AudioFilePath string          // relative path from audioDir
	CallFilename  string          // TR's absolute path
	SrcList       json.RawMessage // for unit attribution
	TgAlphaTag    string
	TgDescription string
	TgTag         string
	TgGroup       string
}

// QueueStats reports the current state of the transcription queue.
type QueueStats struct {
	Pending   int   `json:"pending"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
}

// EventPublishFunc is a callback for publishing SSE events.
type EventPublishFunc func(eventType string, systemID, tgid int, payload map[string]any)

// WorkerPoolOptions configures the transcription worker pool.
type WorkerPoolOptions struct {
	DB              *database.DB
	AudioDir        string
	TRAudioDir      string
	Provider        Provider
	ProviderTimeout time.Duration // used for per-job context timeout
	Temperature     float64
	Language        string
	Prompt          string
	Hotwords        string
	BeamSize        int
	PreprocessAudio bool
	Workers         int
	QueueSize       int
	MinDuration     float64
	MaxDuration     float64
	PublishEvent    EventPublishFunc
	Log             zerolog.Logger

	// Anti-hallucination (Whisper-specific; ignored by other providers)
	RepetitionPenalty             float64
	NoRepeatNgramSize             int
	ConditionOnPreviousText       *bool
	NoSpeechThreshold             float64
	HallucinationSilenceThreshold float64
	MaxNewTokens                  int
	VadFilter                     bool
}

// WorkerPool manages transcription workers.
type WorkerPool struct {
	jobs     chan Job
	db       *database.DB
	provider Provider
	opts     WorkerPoolOptions
	log      zerolog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	completed atomic.Int64
	failed    atomic.Int64
}

// NewWorkerPool creates a new transcription worker pool.
func NewWorkerPool(opts WorkerPoolOptions) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		jobs:     make(chan Job, opts.QueueSize),
		db:       opts.DB,
		provider: opts.Provider,
		opts:     opts,
		log:      opts.Log,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start launches the worker goroutines.
func (wp *WorkerPool) Start() {
	// Check sox availability at startup
	if wp.opts.PreprocessAudio {
		if CheckSox() {
			wp.log.Info().Msg("audio preprocessing enabled (sox found)")
		} else {
			wp.log.Warn().Msg("PREPROCESS_AUDIO=true but sox not found in PATH; preprocessing disabled")
		}
	}

	for i := 0; i < wp.opts.Workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
	wp.log.Info().Int("workers", wp.opts.Workers).Int("queue_size", wp.opts.QueueSize).Msg("transcription worker pool started")
}

// Stop signals workers to drain and waits for completion.
func (wp *WorkerPool) Stop() {
	close(wp.jobs)
	wp.wg.Wait()
	wp.cancel()
	wp.log.Info().
		Int64("completed", wp.completed.Load()).
		Int64("failed", wp.failed.Load()).
		Msg("transcription worker pool stopped")
}

// Enqueue adds a job to the transcription queue. Returns false if the queue is full.
func (wp *WorkerPool) Enqueue(j Job) bool {
	select {
	case wp.jobs <- j:
		return true
	default:
		return false
	}
}

// Stats returns current queue statistics.
func (wp *WorkerPool) Stats() QueueStats {
	return QueueStats{
		Pending:   len(wp.jobs),
		Completed: wp.completed.Load(),
		Failed:    wp.failed.Load(),
	}
}

// MinDuration returns the minimum call duration for transcription.
func (wp *WorkerPool) MinDuration() float64 { return wp.opts.MinDuration }

// MaxDuration returns the maximum call duration for transcription.
func (wp *WorkerPool) MaxDuration() float64 { return wp.opts.MaxDuration }

// Model returns the configured STT model name.
func (wp *WorkerPool) Model() string { return wp.provider.Model() }

// Workers returns the number of worker goroutines.
func (wp *WorkerPool) Workers() int { return wp.opts.Workers }

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()
	log := wp.log.With().Int("worker", id).Logger()

	for job := range wp.jobs {
		if err := wp.processJob(log, job); err != nil {
			wp.failed.Add(1)
			log.Warn().Err(err).
				Int64("call_id", job.CallID).
				Int("tgid", job.Tgid).
				Msg("transcription failed")
		} else {
			wp.completed.Add(1)
		}
	}
}

func (wp *WorkerPool) processJob(log zerolog.Logger, job Job) error {
	start := time.Now()
	ctx, cancel := context.WithTimeout(wp.ctx, wp.opts.ProviderTimeout+10*time.Second)
	defer cancel()

	// 1. Resolve audio file path
	audioPath := audio.ResolveFile(wp.opts.AudioDir, wp.opts.TRAudioDir, job.AudioFilePath, job.CallFilename)
	if audioPath == "" {
		return errorf("audio file not found: path=%q filename=%q", job.AudioFilePath, job.CallFilename)
	}

	// 2. Audio preprocessing (optional)
	transcribePath := audioPath
	if wp.opts.PreprocessAudio {
		processed, cleanup, err := Preprocess(ctx, audioPath)
		if err != nil {
			log.Warn().Err(err).Msg("preprocessing failed, using original audio")
		} else {
			transcribePath = processed
			defer cleanup()
		}
	}

	// 3. Send to STT provider
	resp, err := wp.provider.Transcribe(ctx, transcribePath, TranscribeOpts{
		Temperature:                   wp.opts.Temperature,
		Language:                      wp.opts.Language,
		Prompt:                        wp.opts.Prompt,
		Hotwords:                      wp.opts.Hotwords,
		BeamSize:                      wp.opts.BeamSize,
		RepetitionPenalty:             wp.opts.RepetitionPenalty,
		NoRepeatNgramSize:             wp.opts.NoRepeatNgramSize,
		ConditionOnPreviousText:       wp.opts.ConditionOnPreviousText,
		NoSpeechThreshold:             wp.opts.NoSpeechThreshold,
		HallucinationSilenceThreshold: wp.opts.HallucinationSilenceThreshold,
		MaxNewTokens:                  wp.opts.MaxNewTokens,
		VadFilter:                     wp.opts.VadFilter,
	})
	if err != nil {
		return errorf("%s: %w", wp.provider.Name(), err)
	}

	text := strings.TrimSpace(resp.Text)
	if text == "" {
		log.Debug().Int64("call_id", job.CallID).Msg("provider returned empty text, skipping")
		return nil
	}

	// 4. Unit attribution — correlate word timestamps with src_list
	totalDuration := float64(job.Duration)
	if resp.Duration > 0 {
		totalDuration = resp.Duration
	}
	transmissions := ParseSrcList(job.SrcList, totalDuration)
	tw := AttributeWords(resp.Words, transmissions, text)

	wordsJSON, err := json.Marshal(tw)
	if err != nil {
		return errorf("marshal words: %w", err)
	}

	wordCount := len(resp.Words)
	if wordCount == 0 {
		// Fallback: count words from text
		wordCount = len(strings.Fields(text))
	}

	durationMs := int(time.Since(start).Milliseconds())

	// 5. Store in DB
	row := &database.TranscriptionRow{
		CallID:        job.CallID,
		CallStartTime: job.CallStartTime,
		Text:          text,
		Source:        "auto",
		IsPrimary:     true,
		Language:      resp.Language,
		Model:         wp.provider.Model(),
		Provider:      wp.provider.Name(),
		WordCount:     wordCount,
		DurationMs:    durationMs,
		Words:         wordsJSON,
	}

	_, err = wp.db.InsertTranscription(ctx, row)
	if err != nil {
		return errorf("db insert: %w", err)
	}

	// 6. Publish SSE event
	if wp.opts.PublishEvent != nil {
		wp.opts.PublishEvent("transcription", job.SystemID, job.Tgid, map[string]any{
			"call_id":     job.CallID,
			"system_id":   job.SystemID,
			"tgid":        job.Tgid,
			"text":        text,
			"word_count":  wordCount,
			"segments":    len(tw.Segments),
			"model":       wp.provider.Model(),
			"duration_ms": durationMs,
		})
	}

	log.Debug().
		Int64("call_id", job.CallID).
		Int("tgid", job.Tgid).
		Int("words", wordCount).
		Int("segments", len(tw.Segments)).
		Int("duration_ms", durationMs).
		Msg("transcription complete")

	return nil
}

func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
