package audio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// IdentityLookup resolves a trunk-recorder short name to system and site IDs.
type IdentityLookup interface {
	LookupByShortName(shortName string) (systemID, siteID int, ok bool)
}

// activeStream tracks a live audio stream for a specific talkgroup.
type activeStream struct {
	systemID  int
	siteID    int
	shortName string
	lastChunk time.Time
	seq       uint16
}

// AudioRouter receives AudioChunks, resolves identity (shortName to system/site),
// deduplicates multi-site streams, encodes audio, and publishes AudioFrames to the AudioBus.
type AudioRouter struct {
	bus          *AudioBus
	identity     IdentityLookup
	idleTimeout  time.Duration
	opusBitrate  int // 0 = PCM passthrough, >0 = Opus requested (falls back to PCM if unavailable)
	log          zerolog.Logger

	input chan AudioChunk

	mu            sync.RWMutex
	activeStreams map[string]*activeStream // key: "systemID:tgid"
	encoders      map[string]AudioEncoder  // key: "systemID:tgid"
}

// NewAudioRouter creates an AudioRouter that resolves identity, deduplicates
// multi-site streams, encodes audio, and publishes frames to the given AudioBus.
// opusBitrate controls encoding: 0 = PCM passthrough, >0 = Opus encoding (falls
// back to PCM passthrough if Opus is not available in this build).
func NewAudioRouter(bus *AudioBus, identity IdentityLookup, idleTimeout time.Duration, opusBitrate int) *AudioRouter {
	return &AudioRouter{
		bus:           bus,
		identity:      identity,
		idleTimeout:   idleTimeout,
		opusBitrate:   opusBitrate,
		log:           zerolog.Nop(),
		input:         make(chan AudioChunk, 256),
		activeStreams: make(map[string]*activeStream),
		encoders:      make(map[string]AudioEncoder),
	}
}

// Input returns the channel for sending AudioChunks into the router.
func (r *AudioRouter) Input() chan<- AudioChunk {
	return r.input
}

// ActiveStreamCount returns the number of currently active streams.
func (r *AudioRouter) ActiveStreamCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.activeStreams)
}

// Run starts the router's main loop. It processes incoming chunks, publishes
// frames to the bus, and periodically cleans up idle streams. It blocks until
// ctx is cancelled.
func (r *AudioRouter) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case chunk := <-r.input:
			r.processChunk(chunk)
		case <-ticker.C:
			r.cleanupIdle()
		}
	}
}

// processChunk resolves identity, applies dedup logic, and publishes a frame.
func (r *AudioRouter) processChunk(chunk AudioChunk) {
	systemID := chunk.SystemID
	siteID := chunk.SiteID

	// Resolve identity from short name if system ID is not already set.
	if systemID == 0 && chunk.ShortName != "" {
		var ok bool
		systemID, siteID, ok = r.identity.LookupByShortName(chunk.ShortName)
		if !ok {
			r.log.Debug().
				Str("short_name", chunk.ShortName).
				Msg("dropping chunk: unknown short name")
			return
		}
	}

	// Drop chunks without a valid system or talkgroup.
	if systemID == 0 || chunk.TGID == 0 {
		r.log.Debug().
			Int("system_id", systemID).
			Int("tgid", chunk.TGID).
			Msg("dropping chunk: missing system or talkgroup")
		return
	}

	key := fmt.Sprintf("%d:%d", systemID, chunk.TGID)
	now := time.Now()

	r.mu.Lock()
	stream, exists := r.activeStreams[key]

	if exists {
		// If the existing stream's site differs, check if it's gone idle.
		if stream.siteID != siteID {
			if now.Sub(stream.lastChunk) > r.idleTimeout {
				// Stream is stale; allow takeover by the new site.
				stream.siteID = siteID
				stream.shortName = chunk.ShortName
				stream.lastChunk = now
				stream.seq = 0
			} else {
				// Another site owns this stream; drop (dedup).
				r.mu.Unlock()
				r.log.Debug().
					Str("key", key).
					Int("existing_site", stream.siteID).
					Int("new_site", siteID).
					Msg("dropping chunk: dedup multi-site")
				return
			}
		} else {
			// Same site — update timestamp and increment sequence.
			stream.lastChunk = now
			stream.seq++
		}
	} else {
		// New stream.
		stream = &activeStream{
			systemID:  systemID,
			siteID:    siteID,
			shortName: chunk.ShortName,
			lastChunk: now,
			seq:       0,
		}
		r.activeStreams[key] = stream
	}

	seq := stream.seq

	// Get or create encoder for this stream.
	enc, ok := r.encoders[key]
	if !ok {
		enc = NewEncoder(chunk.SampleRate, r.opusBitrate, r.log)
		r.encoders[key] = enc
	}
	r.mu.Unlock()

	// Encode audio data.
	data, format, err := enc.Encode(chunk.Data)
	if err != nil {
		r.log.Error().Err(err).Str("key", key).Msg("encoding failed, using raw PCM")
		data = chunk.Data
		format = chunk.Format
	}

	// Build and publish frame.
	frame := AudioFrame{
		SystemID: systemID,
		TGID:     chunk.TGID,
		UnitID:   chunk.UnitID,
		Seq:      seq,
		Format:   format,
		Data:     data,
	}

	r.bus.Publish(frame)
}

// cleanupIdle removes streams and their encoders that have been idle longer than idleTimeout.
func (r *AudioRouter) cleanupIdle() {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	for key, stream := range r.activeStreams {
		if now.Sub(stream.lastChunk) > r.idleTimeout {
			if enc, ok := r.encoders[key]; ok {
				enc.Close()
				delete(r.encoders, key)
			}
			delete(r.activeStreams, key)
		}
	}
}
