package transcribe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/audio"
	"github.com/snarg/tr-engine/internal/database"
	"github.com/snarg/tr-engine/internal/storage"
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

// ProviderPerformance reports aggregate STT provider performance.
type ProviderPerformance struct {
	SampleSize       int                        `json:"sample_size"`
	AvgRealTimeRatio *float64                   `json:"avg_real_time_ratio"`
	AvgProviderMs    *float64                   `json:"avg_provider_ms"`
	ByProvider       map[string]ProviderMetrics `json:"by_provider,omitempty"`
}

// ProviderMetrics reports per-provider aggregate metrics.
type ProviderMetrics struct {
	Count            int      `json:"count"`
	AvgRealTimeRatio *float64 `json:"avg_real_time_ratio"`
	AvgProviderMs    *float64 `json:"avg_provider_ms"`
}

// completionRecord is a single entry in the performance ring buffer.
type completionRecord struct {
	providerMs   int64
	callDuration float32 // seconds
	provider     string
	model        string
}

const perfRingSize = 100

// perfRing is a fixed-size circular buffer for recent completion metrics.
type perfRing struct {
	mu    sync.Mutex
	buf   [perfRingSize]completionRecord
	pos   int
	count int
}

func (r *perfRing) push(rec completionRecord) {
	r.mu.Lock()
	r.buf[r.pos] = rec
	r.pos = (r.pos + 1) % perfRingSize
	if r.count < perfRingSize {
		r.count++
	}
	r.mu.Unlock()
}

func (r *perfRing) snapshot() []completionRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]completionRecord, r.count)
	if r.count < perfRingSize {
		copy(out, r.buf[:r.count])
	} else {
		n := copy(out, r.buf[r.pos:])
		copy(out[n:], r.buf[:r.pos])
	}
	return out
}

func (r *perfRing) performance() *ProviderPerformance {
	records := r.snapshot()
	if len(records) == 0 {
		return nil
	}

	perf := &ProviderPerformance{
		SampleSize: len(records),
		ByProvider: make(map[string]ProviderMetrics),
	}

	var totalProviderMs float64
	var totalRatio float64
	var ratioCount int

	byProvider := make(map[string]struct {
		count      int
		providerMs float64
		ratio      float64
		ratioCount int
	})

	for _, rec := range records {
		totalProviderMs += float64(rec.providerMs)
		if rec.callDuration > 0 {
			ratio := float64(rec.providerMs) / (float64(rec.callDuration) * 1000)
			totalRatio += ratio
			ratioCount++
		}

		entry := byProvider[rec.provider]
		entry.count++
		entry.providerMs += float64(rec.providerMs)
		if rec.callDuration > 0 {
			ratio := float64(rec.providerMs) / (float64(rec.callDuration) * 1000)
			entry.ratio += ratio
			entry.ratioCount++
		}
		byProvider[rec.provider] = entry
	}

	avgMs := totalProviderMs / float64(len(records))
	perf.AvgProviderMs = &avgMs
	if ratioCount > 0 {
		avgRatio := totalRatio / float64(ratioCount)
		perf.AvgRealTimeRatio = &avgRatio
	}

	for name, entry := range byProvider {
		m := ProviderMetrics{Count: entry.count}
		avgMs := entry.providerMs / float64(entry.count)
		m.AvgProviderMs = &avgMs
		if entry.ratioCount > 0 {
			avgRatio := entry.ratio / float64(entry.ratioCount)
			m.AvgRealTimeRatio = &avgRatio
		}
		perf.ByProvider[name] = m
	}

	return perf
}

// EventPublishFunc is a callback for publishing SSE events.
type EventPublishFunc func(eventType string, systemID, tgid int, payload map[string]any)

// WorkerPoolOptions configures the transcription worker pool.
type WorkerPoolOptions struct {
	DB              *database.DB
	AudioDir        string
	TRAudioDir      string
	Store           storage.AudioStore // if set, used instead of AudioDir for file resolution
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

	stopped   atomic.Bool
	completed atomic.Int64
	failed    atomic.Int64
	perf      perfRing
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
	wp.stopped.Store(true)
	close(wp.jobs)
	wp.wg.Wait()
	wp.cancel()
	wp.log.Info().
		Int64("completed", wp.completed.Load()).
		Int64("failed", wp.failed.Load()).
		Msg("transcription worker pool stopped")
}

// Enqueue adds a job to the transcription queue. Returns false if the queue is full
// or the pool has been stopped.
func (wp *WorkerPool) Enqueue(j Job) bool {
	if wp.stopped.Load() {
		return false
	}
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

// Performance returns aggregate provider performance metrics from recent completions.
func (wp *WorkerPool) Performance() *ProviderPerformance {
	return wp.perf.performance()
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

	// 1. Resolve audio file
	var audioPath string

	if wp.opts.Store != nil && job.AudioFilePath != "" {
		// Use storage abstraction — tries local cache first, then S3
		if localPath := wp.opts.Store.LocalPath(job.AudioFilePath); localPath != "" {
			audioPath = localPath
		} else if reader, openErr := wp.opts.Store.Open(ctx, job.AudioFilePath); openErr == nil {
			// Not in local cache — write to temp file for preprocessing/STT
			tmpFile, tmpErr := os.CreateTemp("", "tr-audio-*.tmp")
			if tmpErr != nil {
				reader.Close()
				return errorf("create temp for STT: %w", tmpErr)
			}
			if _, cpErr := io.Copy(tmpFile, reader); cpErr != nil {
				reader.Close()
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return errorf("copy audio to temp: %w", cpErr)
			}
			reader.Close()
			tmpFile.Close()
			audioPath = tmpFile.Name()
			defer os.Remove(audioPath)
		}
	}

	// Fallback to direct file resolution
	if audioPath == "" {
		audioPath = audio.ResolveFile(wp.opts.AudioDir, wp.opts.TRAudioDir, job.AudioFilePath, job.CallFilename)
	}
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
	providerStart := time.Now()
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
	providerMs := int(time.Since(providerStart).Milliseconds())
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
		ProviderMs:    &providerMs,
		Words:         wordsJSON,
	}

	_, err = wp.db.InsertTranscription(ctx, row)
	if err != nil {
		return errorf("db insert: %w", err)
	}

	// Track provider performance
	wp.perf.push(completionRecord{
		providerMs:   int64(providerMs),
		callDuration: job.Duration,
		provider:     wp.provider.Name(),
		model:        wp.provider.Model(),
	})

	// 6. Publish SSE event
	if wp.opts.PublishEvent != nil {
		payload := map[string]any{
			"call_id":     job.CallID,
			"system_id":   job.SystemID,
			"tgid":        job.Tgid,
			"text":        text,
			"word_count":  wordCount,
			"segments":    len(tw.Segments),
			"model":       wp.provider.Model(),
			"duration_ms": durationMs,
			"provider_ms": providerMs,
		}
		if job.Duration > 0 {
			payload["real_time_ratio"] = float64(providerMs) / (float64(job.Duration) * 1000)
		}
		wp.opts.PublishEvent("transcription", job.SystemID, job.Tgid, payload)
	}

	log.Debug().
		Int64("call_id", job.CallID).
		Int("tgid", job.Tgid).
		Int("words", wordCount).
		Int("segments", len(tw.Segments)).
		Int("duration_ms", durationMs).
		Int("provider_ms", providerMs).
		Msg("transcription complete")

	return nil
}

func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
