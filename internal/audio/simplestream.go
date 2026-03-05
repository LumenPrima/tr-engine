package audio

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// simplestreamMeta represents the JSON metadata in a simplestream sendJSON packet.
type simplestreamMeta struct {
	Src             int     `json:"src"`
	SrcTag          string  `json:"src_tag"`
	Talkgroup       int     `json:"talkgroup"`
	TalkgroupTag    string  `json:"talkgroup_tag"`
	Freq            float64 `json:"freq"`
	ShortName       string  `json:"short_name"`
	AudioSampleRate int     `json:"audio_sample_rate"`
}

// SimplestreamSource receives UDP packets from trunk-recorder's simplestream
// plugin and produces AudioChunks.
type SimplestreamSource struct {
	listenAddr        string
	defaultSampleRate int
	log               zerolog.Logger

	mu   sync.Mutex
	addr string // actual bound address, set after Start binds
}

// NewSimplestreamSource creates a new SimplestreamSource that listens on the
// given address. Use ":0" for a random port (useful in tests).
func NewSimplestreamSource(listenAddr string, defaultSampleRate int) *SimplestreamSource {
	return &SimplestreamSource{
		listenAddr:        listenAddr,
		defaultSampleRate: defaultSampleRate,
		log:               zerolog.Nop(),
	}
}

// Addr returns the actual listen address after binding. Returns empty string
// until Start has bound the socket.
func (s *SimplestreamSource) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// Start listens for UDP packets, parses them, and sends AudioChunks to out.
// Blocks until ctx is cancelled. Returns nil on clean shutdown.
func (s *SimplestreamSource) Start(ctx context.Context, out chan<- AudioChunk) error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.listenAddr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	// Store the actual bound address
	s.mu.Lock()
	s.addr = conn.LocalAddr().String()
	s.mu.Unlock()

	s.log.Info().Str("addr", s.addr).Msg("simplestream listening")

	// Close the connection when ctx is cancelled to unblock ReadFromUDP
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, 65536) // max UDP packet size
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Check if we were cancelled
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			s.log.Warn().Err(err).Msg("simplestream read error")
			continue
		}

		chunk, ok := s.parsePacket(buf[:n])
		if !ok {
			continue
		}

		select {
		case out <- chunk:
		case <-ctx.Done():
			return nil
		}
	}
}

// parsePacket attempts to parse a simplestream UDP packet.
// It tries sendJSON mode first, then falls back to sendTGID mode.
// Returns false for malformed packets.
func (s *SimplestreamSource) parsePacket(data []byte) (AudioChunk, bool) {
	// Need at least 4 bytes for either mode (JSON length or TGID)
	if len(data) < 4 {
		s.log.Warn().Int("len", len(data)).Msg("simplestream packet too short")
		return AudioChunk{}, false
	}

	// Try sendJSON mode first: [4-byte JSON length LE][JSON][PCM samples]
	jsonLen := int(binary.LittleEndian.Uint32(data[0:4]))
	if jsonLen > 0 && jsonLen < len(data)-4 {
		jsonBytes := data[4 : 4+jsonLen]
		var meta simplestreamMeta
		if json.Unmarshal(jsonBytes, &meta) == nil && meta.Talkgroup > 0 {
			pcmStart := 4 + jsonLen
			pcmData := make([]byte, len(data)-pcmStart)
			copy(pcmData, data[pcmStart:])

			sampleRate := meta.AudioSampleRate
			if sampleRate == 0 {
				sampleRate = s.defaultSampleRate
			}

			return AudioChunk{
				ShortName:  meta.ShortName,
				TGID:       meta.Talkgroup,
				UnitID:     meta.Src,
				Freq:       meta.Freq,
				TGAlphaTag: meta.TalkgroupTag,
				Format:     AudioFormatPCM,
				SampleRate: sampleRate,
				Data:       pcmData,
				Timestamp:  time.Now(),
			}, true
		}
	}

	// Fall back to sendTGID mode: [4-byte TGID LE][int16 PCM samples]
	tgid := int(binary.LittleEndian.Uint32(data[0:4]))
	if tgid <= 0 {
		s.log.Warn().Int("tgid", tgid).Msg("simplestream invalid TGID in sendTGID mode")
		return AudioChunk{}, false
	}

	pcmData := make([]byte, len(data)-4)
	copy(pcmData, data[4:])

	return AudioChunk{
		TGID:       tgid,
		Format:     AudioFormatPCM,
		SampleRate: s.defaultSampleRate,
		Data:       pcmData,
		Timestamp:  time.Now(),
	}, true
}
