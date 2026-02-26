# Timestamp Normalization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Normalize all API timestamp fields to RFC 3339 by converting Unix seconds in `freq_list`/`src_list` JSONB at read time.

**Architecture:** Add a conversion helper in the database package that transforms `time` fields from int64 Unix seconds to RFC 3339 strings. Apply it in two places: the `CallAPI` scan path (list/detail endpoints) and the `GetCallFrequencies`/`GetCallTransmissions` unmarshal path. Update the `CallFrequencyAPI`/`CallTransmissionAPI` structs to use `*string` for the time field with a custom RFC 3339 value. Update OpenAPI spec and frontend consumers.

**Tech Stack:** Go (time package, json.RawMessage manipulation), OpenAPI 3.0.3, vanilla JS

---

### Task 1: Add JSONB timestamp conversion helper

**Files:**
- Create: `internal/database/jsontime.go`
- Create: `internal/database/jsontime_test.go`

**Step 1: Write the failing test**

```go
// internal/database/jsontime_test.go
package database

import (
	"encoding/json"
	"testing"
)

func TestNormalizeSrcFreqTimestamps_FreqList(t *testing.T) {
	input := json.RawMessage(`[{"freq":154875000,"time":1713207802,"pos":0.0,"len":3.24,"error_count":50,"spike_count":3}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	if err := json.Unmarshal(out, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	timeVal, ok := entries[0]["time"].(string)
	if !ok {
		t.Fatalf("time should be string, got %T: %v", entries[0]["time"], entries[0]["time"])
	}
	if timeVal != "2024-04-15T18:03:22Z" {
		t.Errorf("time = %q, want %q", timeVal, "2024-04-15T18:03:22Z")
	}
	// Other fields preserved
	if entries[0]["freq"].(float64) != 154875000 {
		t.Error("freq not preserved")
	}
}

func TestNormalizeSrcFreqTimestamps_SrcList(t *testing.T) {
	input := json.RawMessage(`[{"src":104,"tag":"09 7COM3","time":1713207802,"pos":0.0,"duration":3.5,"emergency":0}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	if err := json.Unmarshal(out, &entries); err != nil {
		t.Fatal(err)
	}
	timeVal := entries[0]["time"].(string)
	if timeVal != "2024-04-15T18:03:22Z" {
		t.Errorf("time = %q, want %q", timeVal, "2024-04-15T18:03:22Z")
	}
}

func TestNormalizeSrcFreqTimestamps_NilAndEmpty(t *testing.T) {
	if out := NormalizeSrcFreqTimestamps(nil); out != nil {
		t.Errorf("nil input should return nil, got %s", out)
	}
	if out := NormalizeSrcFreqTimestamps(json.RawMessage(`null`)); string(out) != "null" {
		t.Errorf("null input should return null, got %s", out)
	}
	if out := NormalizeSrcFreqTimestamps(json.RawMessage(`[]`)); string(out) != "[]" {
		t.Errorf("empty array should return [], got %s", out)
	}
}

func TestNormalizeSrcFreqTimestamps_ZeroTime(t *testing.T) {
	input := json.RawMessage(`[{"freq":154875000,"time":0,"pos":0.0}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	json.Unmarshal(out, &entries)
	// time=0 should be omitted (or null) since it's meaningless
	if _, exists := entries[0]["time"]; exists {
		t.Errorf("time=0 should be omitted, got %v", entries[0]["time"])
	}
}

func TestNormalizeSrcFreqTimestamps_AlreadyString(t *testing.T) {
	// If time is already a string (future-proof), pass through unchanged
	input := json.RawMessage(`[{"freq":154875000,"time":"2024-04-15T18:03:22Z","pos":0.0}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	json.Unmarshal(out, &entries)
	if entries[0]["time"] != "2024-04-15T18:03:22Z" {
		t.Errorf("string time should pass through, got %v", entries[0]["time"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/database/ -run TestNormalizeSrcFreqTimestamps -v`
Expected: FAIL — `NormalizeSrcFreqTimestamps` not defined

**Step 3: Write the implementation**

```go
// internal/database/jsontime.go
package database

import (
	"encoding/json"
	"time"
)

// NormalizeSrcFreqTimestamps converts int64 Unix "time" fields in a
// freq_list or src_list JSONB array to RFC 3339 strings. Returns the
// input unchanged on nil, null, empty array, or decode error.
func NormalizeSrcFreqTimestamps(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return raw
	}

	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return raw // malformed — pass through
	}

	changed := false
	for _, entry := range entries {
		v, ok := entry["time"]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case float64: // JSON numbers decode as float64
			if t == 0 {
				delete(entry, "time")
			} else {
				entry["time"] = time.Unix(int64(t), 0).UTC().Format(time.RFC3339)
			}
			changed = true
		case string:
			// already a string — leave as-is
		}
	}

	if !changed {
		return raw
	}

	out, err := json.Marshal(entries)
	if err != nil {
		return raw
	}
	return out
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/database/ -run TestNormalizeSrcFreqTimestamps -v`
Expected: PASS (all 5 subtests)

**Step 5: Commit**

```bash
git add internal/database/jsontime.go internal/database/jsontime_test.go
git commit -m "feat: add JSONB timestamp normalization helper for freq_list/src_list"
```

---

### Task 2: Apply conversion in CallAPI scan paths

**Files:**
- Modify: `internal/database/queries_calls.go` — lines 157, 213, 412 (after scanning SrcList/FreqList)

There are three scan sites that populate `CallAPI.SrcList` and `CallAPI.FreqList`:
1. `ListCalls()` — line ~157
2. `GetCallByID()` — line ~213
3. `GetCallGroupCallsByID()` — line ~412

**Step 1: Write a test**

This is best tested via integration against the live DB or by verifying the helper is called. Since the helper is already unit-tested, we add a focused test that round-trips a CallAPI struct with known JSONB.

```go
// Add to internal/database/jsontime_test.go
func TestNormalizeSrcFreqTimestamps_PreservesOtherFields(t *testing.T) {
	// Simulate what comes out of DB: full src_list entry
	input := json.RawMessage(`[{"src":104,"tag":"09 7COM3","time":1713207802,"pos":0.5,"duration":3.5,"emergency":0,"signal_system":""}]`)
	out := NormalizeSrcFreqTimestamps(input)
	var entries []map[string]any
	json.Unmarshal(out, &entries)
	e := entries[0]
	if e["src"].(float64) != 104 {
		t.Error("src not preserved")
	}
	if e["tag"] != "09 7COM3" {
		t.Error("tag not preserved")
	}
	if e["pos"].(float64) != 0.5 {
		t.Error("pos not preserved")
	}
	if e["duration"].(float64) != 3.5 {
		t.Error("duration not preserved")
	}
	if e["emergency"].(float64) != 0 {
		t.Error("emergency not preserved")
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/database/ -run TestNormalizeSrcFreqTimestamps -v`

**Step 3: Apply normalization after each scan site**

In `queries_calls.go`, after each `rows.Scan(...)` block that populates a `CallAPI`, add:

```go
c.SrcList = NormalizeSrcFreqTimestamps(c.SrcList)
c.FreqList = NormalizeSrcFreqTimestamps(c.FreqList)
```

Three locations:
1. **ListCalls()** — after line 161 (`); err != nil {` block), inside the `for rows.Next()` loop, before `calls = append(calls, c)` (before line 168)
2. **GetCallByID()** — after line 217 (the Scan error check), before the audioPath check (before line 221)
3. **GetCallGroupCallsByID()** — after the scan error check in its loop, before appending

**Step 4: Run tests**

Run: `go test ./internal/database/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/database/queries_calls.go internal/database/jsontime_test.go
git commit -m "feat: normalize freq_list/src_list timestamps to RFC 3339 in call API responses"
```

---

### Task 3: Apply conversion in GetCallFrequencies/GetCallTransmissions

**Files:**
- Modify: `internal/database/queries_calls.go` — `CallFrequencyAPI.Time` and `CallTransmissionAPI.Time` types
- Modify: `internal/database/calls.go` — `GetCallFrequencies()` and `GetCallTransmissions()`

These endpoints unmarshal the JSONB into typed structs. The `Time` field is currently `*int64`. Change it to `*string` and convert during unmarshal.

**Step 1: Update the struct types**

In `queries_calls.go`, change:

```go
// Before:
type CallFrequencyAPI struct {
	Freq       int64    `json:"freq"`
	Time       *int64   `json:"time,omitempty"`
	// ...
}

type CallTransmissionAPI struct {
	Src          int      `json:"src"`
	Tag          string   `json:"tag,omitempty"`
	Time         *int64   `json:"time,omitempty"`
	// ...
}

// After:
type CallFrequencyAPI struct {
	Freq       int64    `json:"freq"`
	Time       *string  `json:"time,omitempty"`
	// ...
}

type CallTransmissionAPI struct {
	Src          int      `json:"src"`
	Tag          string   `json:"tag,omitempty"`
	Time         *string  `json:"time,omitempty"`
	// ...
}
```

**Step 2: Update GetCallFrequencies/GetCallTransmissions to convert**

In `calls.go`, after unmarshalling, the `Time` field will contain the raw JSON number as a string (since we changed the type). Instead, use the `NormalizeSrcFreqTimestamps` helper on the raw JSONB before unmarshalling:

```go
func (db *DB) GetCallFrequencies(ctx context.Context, callID int64) ([]CallFrequencyAPI, error) {
	raw, err := db.Q.GetCallFreqList(ctx, callID)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return []CallFrequencyAPI{}, nil
	}
	normalized := NormalizeSrcFreqTimestamps(raw)
	var freqs []CallFrequencyAPI
	if err := json.Unmarshal(normalized, &freqs); err != nil {
		return nil, err
	}
	if freqs == nil {
		freqs = []CallFrequencyAPI{}
	}
	return freqs, nil
}
```

Same pattern for `GetCallTransmissions`.

**Step 3: Run tests**

Run: `go test ./internal/database/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/database/queries_calls.go internal/database/calls.go
git commit -m "feat: normalize timestamps in /calls/{id}/frequencies and /calls/{id}/transmissions"
```

---

### Task 4: Update OpenAPI spec

**Files:**
- Modify: `openapi.yaml` — CallFrequency and CallTransmission schemas

**Step 1: Update CallFrequency.time**

Change from:
```yaml
time:
  type: integer
  description: Unix timestamp of when this frequency segment started
  example: 1713207802
```

To:
```yaml
time:
  type: string
  format: date-time
  description: When this frequency segment started (RFC 3339)
  example: "2024-04-15T18:03:22Z"
```

**Step 2: Update CallTransmission.time**

Change from:
```yaml
time:
  type: integer
  description: Unix timestamp of when this unit keyed up
  example: 1713207802
```

To:
```yaml
time:
  type: string
  format: date-time
  description: When this unit keyed up (RFC 3339)
  example: "2024-04-15T18:03:22Z"
```

**Step 3: Commit**

```bash
git add openapi.yaml
git commit -m "docs: update openapi spec — freq_list/src_list timestamps now RFC 3339"
```

---

### Task 5: Update web frontends

**Files:**
- Modify: `web/irc-radio-live.html` — lines 2679, 2919, 3705
- Modify: `web/scanner.html` — line 1680

These files currently do `new Date(tx.time * 1000)` to convert Unix seconds to JS Date. After the change, `tx.time` will be an RFC 3339 string like `"2024-04-15T18:03:22Z"`, which `new Date()` parses natively.

**Step 1: Update irc-radio-live.html**

Three locations. Change each from:
```javascript
const txTime = tx.time ? new Date(tx.time * 1000) : ...fallback...;
```
To:
```javascript
const txTime = tx.time ? new Date(tx.time) : ...fallback...;
```

Specific lines:
- Line 2679: `new Date(tx.time * 1000)` → `new Date(tx.time)`
- Line 2919: `new Date(tx.time * 1000)` → `new Date(tx.time)`
- Line 3705: `new Date(tx.time * 1000)` → `new Date(tx.time)`

**Step 2: Update scanner.html**

- Line 1680: `new Date(first.time * 1000)` → `new Date(first.time)`

**Step 3: Verify no other web files multiply .time by 1000**

Search for any remaining `* 1000` patterns on `.time` fields. The grep from earlier confirms these are the only 4 locations.

**Step 4: Commit**

```bash
git add web/irc-radio-live.html web/scanner.html
git commit -m "fix: update frontends for RFC 3339 timestamps in src_list/freq_list"
```

---

### Task 6: Build and verify

**Step 1: Build**

Run: `bash build.sh`
Expected: Clean build, no errors

**Step 2: Run all tests**

Run: `go test ./... -v`
Expected: All pass

**Step 3: Commit (if any fixups needed)**
