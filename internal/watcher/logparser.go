package watcher

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LogEvent represents a parsed log event
type LogEvent struct {
	Timestamp time.Time
	Type      LogEventType
	System    string
	Data      interface{}
}

// LogEventType identifies the type of log event
type LogEventType int

const (
	EventUnknown LogEventType = iota
	EventCallStart
	EventCallStop
	EventCallConcluding
	EventTransmission
	EventUnitOnCall
	EventUnitAlias
	EventActiveCalls
	EventActiveCall
	EventDecodeRates
	EventDecodeRate
	EventRecorders
	EventRecorder
	EventPatches
	EventPatch
)

// CallStartEvent contains data from a call start log line
type CallStartEvent struct {
	CallID      string // e.g., "13153C"
	Talkgroup   int
	Freq        float64 // MHz
	RecorderNum int
	TDMA        bool
	Slot        int
	Modulation  string // QPSK, etc.
}

// CallStopEvent contains data from a call stop log line
type CallStopEvent struct {
	CallID      string
	Talkgroup   int
	Freq        float64
	RecorderNum int
	TDMA        bool
	Slot        int
	HzError     int
}

// CallConcludingEvent contains data from a concluding call log line
type CallConcludingEvent struct {
	CallID      string
	Talkgroup   int
	Freq        float64
	LastUpdate  int // seconds since last update
	CallElapsed int // total call duration
}

// TransmissionEvent contains data from a transmission log line
type TransmissionEvent struct {
	CallID    string
	Talkgroup int
	Freq      float64
	UnitID    int64
	AlphaTag  string
	Position  float32
	Length    float32
	Errors    int
	Spikes    int
}

// UnitOnCallEvent contains data when a unit joins a call
type UnitOnCallEvent struct {
	CallID    string
	Talkgroup int
	Freq      float64
	UnitID    int64
}

// UnitAliasEvent contains a discovered unit alias
type UnitAliasEvent struct {
	UnitID   int64
	AlphaTag string
}

// ActiveCallEvent contains data for a single active call
type ActiveCallEvent struct {
	CallID    string
	Talkgroup int
	Freq      float64
	Elapsed   int
	State     string // recording, monitoring, idle
	Encrypted bool
}

// DecodeRateEvent contains decode rate for a system
type DecodeRateEvent struct {
	System   string
	Freq     float64
	MsgPerSec int
}

// RecorderEvent contains recorder status
type RecorderEvent struct {
	SourceNum   int
	SourceFreq  float64
	RecorderNum int
	Type        string // P25, etc.
	State       string // available, recording, idle
}

// PatchEvent contains talkgroup patch info
type PatchEvent struct {
	System     string
	Talkgroups []int
}

// LogParser parses trunk-recorder log lines
type LogParser struct {
	// Compiled regex patterns
	reTimestamp     *regexp.Regexp
	reCallStart     *regexp.Regexp
	reCallStop      *regexp.Regexp
	reCallConclud   *regexp.Regexp
	reTransmission  *regexp.Regexp
	reUnitOnCall    *regexp.Regexp
	reUnitAlias     *regexp.Regexp
	reActiveCalls   *regexp.Regexp
	reActiveCall    *regexp.Regexp
	reDecodeRates   *regexp.Regexp
	reDecodeRate    *regexp.Regexp
	reRecorders     *regexp.Regexp
	reRecorderSrc   *regexp.Regexp
	reRecorder      *regexp.Regexp
	rePatches       *regexp.Regexp
	rePatchSystem   *regexp.Regexp
	rePatch         *regexp.Regexp

	// State for multi-line parsing
	currentSection string
	currentSource  int
	currentFreq    float64
}

// NewLogParser creates a new log parser with compiled patterns
func NewLogParser() *LogParser {
	return &LogParser{
		// Timestamp: [2025-01-03 03:48:12.457502]
		reTimestamp: regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+)\]`),

		// Call start: [butco]    13154C    TG:       9179    Freq: 855.737500 MHz    Starting P25 Recorder Num [0]    TDMA: falseSlot: 0    QPSK: true
		reCallStart: regexp.MustCompile(`\[(\w+)\]\s+(\w+)\s+TG:\s*(\d+)\s+Freq:\s*([\d.]+)\s*MHz\s+Starting P25 Recorder Num \[(\d+)\]\s+TDMA:\s*(\w+)(?:\s*Slot:\s*(\d+))?\s+(\w+):`),

		// Call stop: [butco]    13155C    TG:       9130    Freq: 859.262500 MHz    Stopping P25 Recorder Num [1]    TDMA: falseSlot: 0    TuningErr: +110 Hz
		reCallStop: regexp.MustCompile(`\[(\w+)\]\s+(\w+)\s+TG:\s*(\d+)\s+Freq:\s*([\d.]+)\s*MHz\s+Stopping P25 Recorder Num \[(\d+)\]\s+TDMA:\s*(\w+)(?:\s*Slot:\s*(\d+))?\s+(?:Hz Error|TuningErr):\s*([+-]?\d+)`),

		// Call concluding: [butco]    13155C    TG:       9130    Freq: 859.262500 MHz    Concluding Recorded Call - Last Update: 2s    Recorder last write:3.01713    Call Elapsed: 6
		reCallConclud: regexp.MustCompile(`\[(\w+)\]\s+(\w+)\s+TG:\s*(\d+)\s+Freq:\s*([\d.]+)\s*MHz\s+Concluding Recorded Call - Last Update:\s*(\d+)s.*Call Elapsed:\s*(\d+)`),

		// Transmission: [butco]    13155C    TG:       9130    Freq: 859.262500 MHz    - Transmission src: 921353 (09 SQD 101) pos:  0.00 length:  2.16 errors: 90 spikes: 6
		// Also handles: - Transmission src: 984851 pos:  0.00 length:  2.16
		reTransmission: regexp.MustCompile(`\[(\w+)\]\s+(\w+)\s+TG:\s*(\d+)\s+Freq:\s*([\d.]+)\s*MHz\s+- Transmission src:\s*(-?\d+)(?:\s*\(([^)]*)\))?\s+pos:\s*([\d.]+)\s+length:\s*([\d.]+)(?:\s+errors:\s*(\d+))?(?:\s+spikes:\s*(\d+))?`),

		// Unit on call: [butco]    13154C    TG:       9179    Freq: 855.737500 MHz    Unit ID set via Control Channel, ext: 918127    current: -1     samples: 0
		reUnitOnCall: regexp.MustCompile(`\[(\w+)\]\s+(\w+)\s+TG:\s*(\d+)\s+Freq:\s*([\d.]+)\s*MHz\s+Unit ID set via Control Channel, ext:\s*(\d+)`),

		// Unit alias: [butco]    13155C    TG:       9130    Freq: 859.262500 MHz    New MotoP25_FDMA alias: 921353 (09 SQD 101)
		reUnitAlias: regexp.MustCompile(`New \w+ alias:\s*(\d+)\s*\(([^)]+)\)`),

		// Active calls header
		reActiveCalls: regexp.MustCompile(`Active Calls:\s*(\d+)`),

		// Active call line: [hamco]    59C    TG:      55633    Freq: 853.150000 MHz    Elapsed:   15 State: recording
		reActiveCall: regexp.MustCompile(`\[(\w+)\]\s+(\w+)\s+TG:\s*(\d+)\s+Freq:\s*([\d.]+)\s*MHz\s+Elapsed:\s*(\d+)\s+State:\s*(\w+)(?:\s*:\s*(\w+))?`),

		// Decode rates header
		reDecodeRates: regexp.MustCompile(`Control Channel Decode Rates:`),

		// Decode rate: [butco]    853.037500 MHz    40 msg/sec
		reDecodeRate: regexp.MustCompile(`\[(\w+)\]\s+([\d.]+)\s*MHz\s+(\d+)\s*msg/sec`),

		// Recorders header
		reRecorders: regexp.MustCompile(`Recorders:`),

		// Recorder source: [ Source 0: 855.500000 MHz ]
		reRecorderSrc: regexp.MustCompile(`\[ Source (\d+):\s*([\d.]+)\s*MHz \]`),

		// Recorder: [ 0 ] P25    State: available
		reRecorder: regexp.MustCompile(`\[\s*(\d+)\s*\]\s*(\w+)\s+State:\s*(\w+)`),

		// Patches header
		rePatches: regexp.MustCompile(`Active Patches:`),

		// Patch system: [ butco ] 3 active talkgroup patches:
		rePatchSystem: regexp.MustCompile(`\[\s*(\w+)\s*\]\s*\d+\s*active talkgroup patches:`),

		// Patch: Active Patch of TGIDs -  9130 9139
		rePatch: regexp.MustCompile(`Active Patch of TGIDs\s*-\s*([\d\s]+)`),
	}
}

// ParseLine parses a single log line and returns events
func (p *LogParser) ParseLine(line string) []LogEvent {
	var events []LogEvent

	// Extract timestamp
	tsMatch := p.reTimestamp.FindStringSubmatch(line)
	if tsMatch == nil {
		return nil
	}

	ts, err := time.Parse("2006-01-02 15:04:05.000000", tsMatch[1])
	if err != nil {
		// Try without microseconds
		ts, err = time.Parse("2006-01-02 15:04:05", tsMatch[1][:19])
		if err != nil {
			return nil
		}
	}

	// Check for section headers first
	if p.reActiveCalls.MatchString(line) {
		p.currentSection = "active_calls"
		return nil
	}
	if p.reDecodeRates.MatchString(line) {
		p.currentSection = "decode_rates"
		return nil
	}
	if p.reRecorders.MatchString(line) {
		p.currentSection = "recorders"
		return nil
	}
	if p.rePatches.MatchString(line) {
		p.currentSection = "patches"
		return nil
	}

	// Try to match event patterns

	// Call start
	if m := p.reCallStart.FindStringSubmatch(line); m != nil {
		p.currentSection = ""
		tg, _ := strconv.Atoi(m[3])
		freq, _ := strconv.ParseFloat(m[4], 64)
		recNum, _ := strconv.Atoi(m[5])
		slot := 0
		if m[7] != "" {
			slot, _ = strconv.Atoi(m[7])
		}
		events = append(events, LogEvent{
			Timestamp: ts,
			Type:      EventCallStart,
			System:    m[1],
			Data: CallStartEvent{
				CallID:      m[2],
				Talkgroup:   tg,
				Freq:        freq,
				RecorderNum: recNum,
				TDMA:        m[6] == "true",
				Slot:        slot,
				Modulation:  strings.TrimSuffix(m[8], ":"),
			},
		})
		return events
	}

	// Call stop
	if m := p.reCallStop.FindStringSubmatch(line); m != nil {
		p.currentSection = ""
		tg, _ := strconv.Atoi(m[3])
		freq, _ := strconv.ParseFloat(m[4], 64)
		recNum, _ := strconv.Atoi(m[5])
		slot := 0
		if m[7] != "" {
			slot, _ = strconv.Atoi(m[7])
		}
		hzErr, _ := strconv.Atoi(m[8])
		events = append(events, LogEvent{
			Timestamp: ts,
			Type:      EventCallStop,
			System:    m[1],
			Data: CallStopEvent{
				CallID:      m[2],
				Talkgroup:   tg,
				Freq:        freq,
				RecorderNum: recNum,
				TDMA:        m[6] == "true",
				Slot:        slot,
				HzError:     hzErr,
			},
		})
		return events
	}

	// Call concluding
	if m := p.reCallConclud.FindStringSubmatch(line); m != nil {
		p.currentSection = ""
		tg, _ := strconv.Atoi(m[3])
		freq, _ := strconv.ParseFloat(m[4], 64)
		lastUpdate, _ := strconv.Atoi(m[5])
		elapsed, _ := strconv.Atoi(m[6])
		events = append(events, LogEvent{
			Timestamp: ts,
			Type:      EventCallConcluding,
			System:    m[1],
			Data: CallConcludingEvent{
				CallID:      m[2],
				Talkgroup:   tg,
				Freq:        freq,
				LastUpdate:  lastUpdate,
				CallElapsed: elapsed,
			},
		})
		return events
	}

	// Transmission
	if m := p.reTransmission.FindStringSubmatch(line); m != nil {
		p.currentSection = ""
		tg, _ := strconv.Atoi(m[3])
		freq, _ := strconv.ParseFloat(m[4], 64)
		unitID, _ := strconv.ParseInt(m[5], 10, 64)
		pos, _ := strconv.ParseFloat(m[7], 32)
		length, _ := strconv.ParseFloat(m[8], 32)
		errors := 0
		spikes := 0
		if m[9] != "" {
			errors, _ = strconv.Atoi(m[9])
		}
		if m[10] != "" {
			spikes, _ = strconv.Atoi(m[10])
		}
		events = append(events, LogEvent{
			Timestamp: ts,
			Type:      EventTransmission,
			System:    m[1],
			Data: TransmissionEvent{
				CallID:    m[2],
				Talkgroup: tg,
				Freq:      freq,
				UnitID:    unitID,
				AlphaTag:  m[6],
				Position:  float32(pos),
				Length:    float32(length),
				Errors:    errors,
				Spikes:    spikes,
			},
		})
		return events
	}

	// Unit on call
	if m := p.reUnitOnCall.FindStringSubmatch(line); m != nil {
		p.currentSection = ""
		tg, _ := strconv.Atoi(m[3])
		freq, _ := strconv.ParseFloat(m[4], 64)
		unitID, _ := strconv.ParseInt(m[5], 10, 64)
		events = append(events, LogEvent{
			Timestamp: ts,
			Type:      EventUnitOnCall,
			System:    m[1],
			Data: UnitOnCallEvent{
				CallID:    m[2],
				Talkgroup: tg,
				Freq:      freq,
				UnitID:    unitID,
			},
		})
		return events
	}

	// Unit alias
	if m := p.reUnitAlias.FindStringSubmatch(line); m != nil {
		p.currentSection = ""
		unitID, _ := strconv.ParseInt(m[1], 10, 64)
		events = append(events, LogEvent{
			Timestamp: ts,
			Type:      EventUnitAlias,
			Data: UnitAliasEvent{
				UnitID:   unitID,
				AlphaTag: strings.TrimSpace(m[2]),
			},
		})
		return events
	}

	// Section-specific parsing
	switch p.currentSection {
	case "active_calls":
		if m := p.reActiveCall.FindStringSubmatch(line); m != nil {
			tg, _ := strconv.Atoi(m[3])
			freq, _ := strconv.ParseFloat(m[4], 64)
			elapsed, _ := strconv.Atoi(m[5])
			encrypted := m[7] == "ENCRYPTED"
			events = append(events, LogEvent{
				Timestamp: ts,
				Type:      EventActiveCall,
				System:    m[1],
				Data: ActiveCallEvent{
					CallID:    m[2],
					Talkgroup: tg,
					Freq:      freq,
					Elapsed:   elapsed,
					State:     m[6],
					Encrypted: encrypted,
				},
			})
		}

	case "decode_rates":
		if m := p.reDecodeRate.FindStringSubmatch(line); m != nil {
			freq, _ := strconv.ParseFloat(m[2], 64)
			msgPerSec, _ := strconv.Atoi(m[3])
			events = append(events, LogEvent{
				Timestamp: ts,
				Type:      EventDecodeRate,
				System:    m[1],
				Data: DecodeRateEvent{
					System:    m[1],
					Freq:      freq,
					MsgPerSec: msgPerSec,
				},
			})
		}

	case "recorders":
		if m := p.reRecorderSrc.FindStringSubmatch(line); m != nil {
			p.currentSource, _ = strconv.Atoi(m[1])
			p.currentFreq, _ = strconv.ParseFloat(m[2], 64)
		} else if m := p.reRecorder.FindStringSubmatch(line); m != nil {
			recNum, _ := strconv.Atoi(m[1])
			events = append(events, LogEvent{
				Timestamp: ts,
				Type:      EventRecorder,
				Data: RecorderEvent{
					SourceNum:   p.currentSource,
					SourceFreq:  p.currentFreq,
					RecorderNum: recNum,
					Type:        m[2],
					State:       m[3],
				},
			})
		}

	case "patches":
		if m := p.rePatchSystem.FindStringSubmatch(line); m != nil {
			// Just track we're in a system's patches
		} else if m := p.rePatch.FindStringSubmatch(line); m != nil {
			tgStrs := strings.Fields(m[1])
			var tgs []int
			for _, s := range tgStrs {
				if tg, err := strconv.Atoi(s); err == nil {
					tgs = append(tgs, tg)
				}
			}
			if len(tgs) > 0 {
				events = append(events, LogEvent{
					Timestamp: ts,
					Type:      EventPatch,
					Data: PatchEvent{
						Talkgroups: tgs,
					},
				})
			}
		}
	}

	return events
}

// Reset clears the parser state
func (p *LogParser) Reset() {
	p.currentSection = ""
	p.currentSource = 0
	p.currentFreq = 0
}
