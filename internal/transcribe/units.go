package transcribe

import (
	"encoding/json"
	"strings"
)

// Transmission represents a unit's transmission within a call, parsed from src_list.
type Transmission struct {
	Src      int     `json:"src"`       // unit/radio ID
	Tag      string  `json:"tag"`       // unit alpha tag
	Pos      float64 `json:"pos"`       // start position in audio (seconds)
	Duration float64 `json:"duration"`  // transmission duration (seconds)
}

// AttributedWord is a Whisper word enriched with unit attribution.
type AttributedWord struct {
	Word   string  `json:"word"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Src    int     `json:"src"`               // unit/radio ID (0 if unattributed)
	SrcTag string  `json:"src_tag,omitempty"`  // unit alpha tag
}

// Segment groups consecutive words from the same unit.
type Segment struct {
	Src    int     `json:"src"`
	SrcTag string  `json:"src_tag,omitempty"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Text   string  `json:"text"`
}

// TranscriptionWords is the structure stored in the transcriptions.words JSONB column.
type TranscriptionWords struct {
	Words    []AttributedWord `json:"words"`
	Segments []Segment        `json:"segments"`
}

// ParseSrcList parses the src_list JSONB from a call record into Transmission entries.
// Computes duration for each entry as the gap to the next entry's position.
// totalDuration is used for the last entry's duration calculation.
func ParseSrcList(srcListJSON json.RawMessage, totalDuration float64) []Transmission {
	if len(srcListJSON) == 0 || string(srcListJSON) == "null" {
		return nil
	}

	var raw []struct {
		Src       int     `json:"src"`
		Tag       string  `json:"tag"`
		Pos       float64 `json:"pos"`
		Emergency int     `json:"emergency"`
	}
	if err := json.Unmarshal(srcListJSON, &raw); err != nil {
		return nil
	}

	txs := make([]Transmission, len(raw))
	for i, r := range raw {
		dur := totalDuration - r.Pos
		if i+1 < len(raw) {
			dur = raw[i+1].Pos - r.Pos
		}
		if dur < 0 {
			dur = 0
		}
		txs[i] = Transmission{
			Src:      r.Src,
			Tag:      r.Tag,
			Pos:      r.Pos,
			Duration: dur,
		}
	}
	return txs
}

// AttributeWords correlates word timestamps with transmission boundaries
// to determine which radio unit said each word.
//
// For each word, the midpoint (start+end)/2 is compared against transmission
// boundaries [Pos, Pos+Duration). Words falling outside all transmissions
// are attributed to the nearest transmission.
func AttributeWords(words []Word, transmissions []Transmission) *TranscriptionWords {
	if len(words) == 0 {
		return &TranscriptionWords{
			Words:    []AttributedWord{},
			Segments: []Segment{},
		}
	}

	// If no transmission data, attribute all words to src=0 (unknown)
	if len(transmissions) == 0 {
		attributed := make([]AttributedWord, len(words))
		for i, w := range words {
			attributed[i] = AttributedWord{
				Word:  w.Word,
				Start: w.Start,
				End:   w.End,
				Src:   0,
			}
		}
		seg := buildSegments(attributed)
		return &TranscriptionWords{Words: attributed, Segments: seg}
	}

	attributed := make([]AttributedWord, len(words))
	for i, w := range words {
		mid := (w.Start + w.End) / 2
		src, tag := findTransmission(mid, transmissions)
		attributed[i] = AttributedWord{
			Word:   w.Word,
			Start:  w.Start,
			End:    w.End,
			Src:    src,
			SrcTag: tag,
		}
	}

	segments := buildSegments(attributed)
	return &TranscriptionWords{Words: attributed, Segments: segments}
}

// findTransmission finds which transmission a timestamp falls within.
// Returns (src, tag). Falls back to nearest transmission if no exact match.
func findTransmission(t float64, txs []Transmission) (int, string) {
	// Try exact containment first
	for _, tx := range txs {
		if t >= tx.Pos && t < tx.Pos+tx.Duration {
			return tx.Src, tx.Tag
		}
	}

	// Fall back to nearest transmission by start position
	bestIdx := 0
	bestDist := abs(t - txs[0].Pos)
	for i := 1; i < len(txs); i++ {
		d := abs(t - txs[i].Pos)
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return txs[bestIdx].Src, txs[bestIdx].Tag
}

// buildSegments groups consecutive attributed words by the same src into segments.
func buildSegments(words []AttributedWord) []Segment {
	if len(words) == 0 {
		return []Segment{}
	}

	var segments []Segment
	cur := Segment{
		Src:    words[0].Src,
		SrcTag: words[0].SrcTag,
		Start:  words[0].Start,
		End:    words[0].End,
		Text:   strings.TrimSpace(words[0].Word),
	}

	for i := 1; i < len(words); i++ {
		w := words[i]
		if w.Src == cur.Src {
			// Same unit — extend segment
			cur.End = w.End
			cur.Text += " " + strings.TrimSpace(w.Word)
		} else {
			// New unit — finalize current segment, start new one
			cur.Text = strings.TrimSpace(cur.Text)
			segments = append(segments, cur)
			cur = Segment{
				Src:    w.Src,
				SrcTag: w.SrcTag,
				Start:  w.Start,
				End:    w.End,
				Text:   strings.TrimSpace(w.Word),
			}
		}
	}
	cur.Text = strings.TrimSpace(cur.Text)
	segments = append(segments, cur)

	return segments
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
