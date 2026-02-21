package transcribe

import (
	"encoding/json"
	"testing"
)

func TestBuildSegments_PunctuationPreserved(t *testing.T) {
	// Word tokens lack punctuation, but fullText has it.
	words := []AttributedWord{
		{Word: "Air", Start: 0.0, End: 0.3, Src: 1},
		{Word: "2", Start: 0.3, End: 0.5, Src: 1},
		{Word: "pilot", Start: 0.5, End: 0.9, Src: 1},
		{Word: "weather", Start: 1.0, End: 1.4, Src: 1},
		{Word: "check", Start: 1.4, End: 1.8, Src: 1},
	}
	fullText := "Air 2 pilot, weather check."

	segments := buildSegments(words, fullText)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Text != "Air 2 pilot, weather check." {
		t.Errorf("expected %q, got %q", "Air 2 pilot, weather check.", segments[0].Text)
	}
}

func TestBuildSegments_MultiSegmentPunctuation(t *testing.T) {
	// Two units — punctuation should land in the correct segment.
	words := []AttributedWord{
		{Word: "Air", Start: 0.0, End: 0.3, Src: 1, SrcTag: "Unit1"},
		{Word: "2", Start: 0.3, End: 0.5, Src: 1, SrcTag: "Unit1"},
		{Word: "pilot", Start: 0.5, End: 0.9, Src: 1, SrcTag: "Unit1"},
		{Word: "weather", Start: 1.0, End: 1.4, Src: 2, SrcTag: "Unit2"},
		{Word: "check", Start: 1.4, End: 1.8, Src: 2, SrcTag: "Unit2"},
	}
	fullText := "Air 2 pilot, weather check."

	segments := buildSegments(words, fullText)

	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}
	if segments[0].Text != "Air 2 pilot," {
		t.Errorf("segment 0: expected %q, got %q", "Air 2 pilot,", segments[0].Text)
	}
	if segments[0].Src != 1 {
		t.Errorf("segment 0: expected src=1, got src=%d", segments[0].Src)
	}
	if segments[1].Text != "weather check." {
		t.Errorf("segment 1: expected %q, got %q", "weather check.", segments[1].Text)
	}
	if segments[1].Src != 2 {
		t.Errorf("segment 1: expected src=2, got src=%d", segments[1].Src)
	}
}

func TestBuildSegments_SingleSegment(t *testing.T) {
	words := []AttributedWord{
		{Word: "hello", Start: 0.0, End: 0.5, Src: 1},
		{Word: "world", Start: 0.5, End: 1.0, Src: 1},
	}
	fullText := "Hello, world!"

	segments := buildSegments(words, fullText)

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Text != "Hello, world!" {
		t.Errorf("expected %q, got %q", "Hello, world!", segments[0].Text)
	}
}

func TestBuildSegments_EmptyFullTextFallback(t *testing.T) {
	// When fullText is empty, fall back to joining word tokens.
	words := []AttributedWord{
		{Word: "hello", Start: 0.0, End: 0.5, Src: 1},
		{Word: "world", Start: 0.5, End: 1.0, Src: 1},
	}

	segments := buildSegments(words, "")

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Text != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", segments[0].Text)
	}
}

func TestBuildSegments_RepeatedWordsMatchSequentially(t *testing.T) {
	// "go go go" — each "go" should match sequentially in fullText.
	words := []AttributedWord{
		{Word: "go", Start: 0.0, End: 0.3, Src: 1},
		{Word: "go", Start: 0.3, End: 0.6, Src: 2},
		{Word: "go", Start: 0.6, End: 0.9, Src: 1},
	}
	fullText := "Go, go, go!"

	segments := buildSegments(words, fullText)

	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segments))
	}
	if segments[0].Text != "Go," {
		t.Errorf("segment 0: expected %q, got %q", "Go,", segments[0].Text)
	}
	if segments[1].Text != "go," {
		t.Errorf("segment 1: expected %q, got %q", "go,", segments[1].Text)
	}
	if segments[2].Text != "go!" {
		t.Errorf("segment 2: expected %q, got %q", "go!", segments[2].Text)
	}
}

func TestAttributeWords_WithFullText(t *testing.T) {
	// Integration test: AttributeWords with transmissions and fullText.
	srcList := json.RawMessage(`[{"src":100,"tag":"Engine 1","pos":0.0},{"src":200,"tag":"Dispatch","pos":1.0}]`)
	transmissions := ParseSrcList(srcList, 2.0)

	words := []Word{
		{Word: "responding", Start: 0.1, End: 0.5},
		{Word: "to", Start: 0.5, End: 0.7},
		{Word: "scene", Start: 0.7, End: 0.9},
		{Word: "copy", Start: 1.1, End: 1.4},
		{Word: "that", Start: 1.4, End: 1.7},
	}
	fullText := "Responding to scene. Copy that."

	tw := AttributeWords(words, transmissions, fullText)

	if len(tw.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(tw.Segments))
	}
	if tw.Segments[0].Text != "Responding to scene." {
		t.Errorf("segment 0: expected %q, got %q", "Responding to scene.", tw.Segments[0].Text)
	}
	if tw.Segments[0].Src != 100 {
		t.Errorf("segment 0: expected src=100, got src=%d", tw.Segments[0].Src)
	}
	if tw.Segments[1].Text != "Copy that." {
		t.Errorf("segment 1: expected %q, got %q", "Copy that.", tw.Segments[1].Text)
	}
	if tw.Segments[1].Src != 200 {
		t.Errorf("segment 1: expected src=200, got src=%d", tw.Segments[1].Src)
	}
}

func TestAttributeWords_EmptyWords(t *testing.T) {
	tw := AttributeWords(nil, nil, "some text")
	if len(tw.Words) != 0 {
		t.Errorf("expected 0 words, got %d", len(tw.Words))
	}
	if len(tw.Segments) != 0 {
		t.Errorf("expected 0 segments, got %d", len(tw.Segments))
	}
}

func TestAttributeWords_NoTransmissions(t *testing.T) {
	words := []Word{
		{Word: "hello", Start: 0.0, End: 0.5},
		{Word: "world", Start: 0.5, End: 1.0},
	}
	fullText := "Hello, world!"

	tw := AttributeWords(words, nil, fullText)

	if len(tw.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(tw.Segments))
	}
	if tw.Segments[0].Src != 0 {
		t.Errorf("expected src=0, got src=%d", tw.Segments[0].Src)
	}
	if tw.Segments[0].Text != "Hello, world!" {
		t.Errorf("expected %q, got %q", "Hello, world!", tw.Segments[0].Text)
	}
}

func TestMapWordPositions_CaseInsensitive(t *testing.T) {
	words := []AttributedWord{
		{Word: "HELLO"},
		{Word: "World"},
	}
	fullText := "Hello, World!"

	positions := mapWordPositions(words, fullText)

	if positions[0] != 0 {
		t.Errorf("expected position 0 for HELLO, got %d", positions[0])
	}
	if positions[1] != 7 {
		t.Errorf("expected position 7 for World, got %d", positions[1])
	}
}
