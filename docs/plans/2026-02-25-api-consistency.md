# API Consistency Fixes — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix three API consistency issues: Cloudflare-safe composite IDs, call_count on talkgroup units, and unit_id alias on unit-events.

**Architecture:** All changes are additive/backwards-compatible. Dash separator in `ParseCompositeID()` covers all 9 composite-ID endpoints. SQL change in `ListTalkgroupUnits()` adds call count. Struct field addition in `UnitEventAPI` adds alias.

**Tech Stack:** Go, pgx, chi router, OpenAPI 3.0.3

---

### Task 1: Dash Separator in ParseCompositeID

**Files:**
- Modify: `internal/api/composite_id.go:22-45`
- Modify: `internal/api/composite_id_test.go:49-99`

**Step 1: Add test cases for dash separator**

Add these entries to the test table in `composite_id_test.go` (after line 63, before the closing `}`):

```go
{"composite_dash", "3-48686", 3, 48686, false, false},
{"dash_large_ids", "1-1234567", 1, 1234567, false, false},
{"invalid_dash_system", "abc-100", 0, 0, false, true},
{"invalid_dash_entity", "1-abc", 0, 0, false, true},
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run TestParseCompositeID -v`
Expected: FAIL — dash cases return `invalid ID: 3-48686` (parsed as plain int, fails)

**Step 3: Update ParseCompositeID to accept dash**

In `composite_id.go`, replace lines 28-38 (the colon check block) with:

```go
	// Accept both ":" and "-" as separators for composite IDs.
	// Dash is Cloudflare WAF-safe; colon is the canonical format.
	sep := -1
	if idx := strings.IndexByte(raw, ':'); idx > 0 {
		sep = idx
	} else if idx := strings.IndexByte(raw, '-'); idx > 0 {
		// Only treat "-" as separator if both sides are numeric
		left, right := raw[:idx], raw[idx+1:]
		if _, err := strconv.Atoi(left); err == nil {
			if _, err := strconv.Atoi(right); err == nil {
				sep = idx
			}
		}
	}

	if sep > 0 {
		sysID, err := strconv.Atoi(raw[:sep])
		if err != nil {
			return CompositeID{}, fmt.Errorf("invalid system_id in composite ID: %s", raw)
		}
		entID, err := strconv.Atoi(raw[sep+1:])
		if err != nil {
			return CompositeID{}, fmt.Errorf("invalid entity_id in composite ID: %s", raw)
		}
		return CompositeID{SystemID: sysID, EntityID: entID}, nil
	}
```

The dash branch validates both sides are numeric before treating `-` as a separator. This prevents treating negative numbers or non-composite values as composite IDs.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -run TestParseCompositeID -v`
Expected: PASS — all cases including dash

**Step 5: Commit**

```bash
git add internal/api/composite_id.go internal/api/composite_id_test.go
git commit -m "feat(api): accept dash separator in composite IDs for Cloudflare compatibility"
```

---

### Task 2: Add call_count to /talkgroups/{id}/units

**Files:**
- Modify: `internal/database/units.go:25-39` (UnitAPI struct)
- Modify: `internal/database/talkgroups.go:316-367` (ListTalkgroupUnits query)

**Step 1: Add CallCount field to UnitAPI struct**

In `internal/database/units.go`, add after the `LastEventTgTag` field (line 37):

```go
	CallCount      *int       `json:"call_count,omitempty"`
```

**Step 2: Rewrite ListTalkgroupUnits SQL to include call counts**

In `internal/database/talkgroups.go`, replace the entire `ListTalkgroupUnits` function (lines 316-367) with:

```go
// ListTalkgroupUnits returns units affiliated with a talkgroup within a time window.
func (db *DB) ListTalkgroupUnits(ctx context.Context, systemID, tgid, windowMinutes, limit, offset int) ([]UnitAPI, int, error) {
	window := strconv.Itoa(windowMinutes) + " minutes"

	var total int
	err := db.Pool.QueryRow(ctx, `
		SELECT count(DISTINCT u)
		FROM calls c, unnest(c.unit_ids) AS u
		WHERE c.system_id = $1 AND c.tgid = $2 AND c.start_time > now() - $3::interval
	`, systemID, tgid, window).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.Pool.Query(ctx, `
		WITH unit_calls AS (
			SELECT uid, count(*) AS call_count
			FROM calls c, unnest(c.unit_ids) AS uid
			WHERE c.system_id = $1 AND c.tgid = $2 AND c.start_time > now() - $3::interval
			GROUP BY uid
		)
		SELECT u.system_id, COALESCE(s.name, ''), s.sysid,
			u.unit_id, COALESCE(u.alpha_tag, ''), COALESCE(u.alpha_tag_source, ''),
			u.first_seen, u.last_seen,
			u.last_event_type, u.last_event_time, u.last_event_tgid,
			uc.call_count
		FROM units u
		JOIN systems s ON s.system_id = u.system_id
		JOIN unit_calls uc ON uc.uid = u.unit_id
		ORDER BY uc.call_count DESC, u.unit_id
		LIMIT $4 OFFSET $5
	`, systemID, tgid, window, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var units []UnitAPI
	for rows.Next() {
		var u UnitAPI
		if err := rows.Scan(
			&u.SystemID, &u.SystemName, &u.Sysid,
			&u.UnitID, &u.AlphaTag, &u.AlphaTagSource,
			&u.FirstSeen, &u.LastSeen,
			&u.LastEventType, &u.LastEventTime, &u.LastEventTgid,
			&u.CallCount,
		); err != nil {
			return nil, 0, err
		}
		units = append(units, u)
	}
	if units == nil {
		units = []UnitAPI{}
	}
	return units, total, rows.Err()
}
```

Key changes from original:
- CTE `unit_calls` computes `count(*)` per unit in one pass
- `JOIN unit_calls` replaces `WHERE IN (subquery)` — same filtering, adds count
- Default sort changed to `call_count DESC` (most active units first)
- Scans `uc.call_count` into new `u.CallCount` field

**Step 3: Build and verify**

Run: `go build ./...`
Expected: PASS — no compile errors

**Step 4: Commit**

```bash
git add internal/database/units.go internal/database/talkgroups.go
git commit -m "feat(api): add call_count to talkgroup units endpoint"
```

---

### Task 3: Add unit_id Alias to Unit-Events Response

**Files:**
- Modify: `internal/database/unit_events.go:40-54` (UnitEventAPI struct)
- Modify: `internal/database/unit_events.go` (all Scan sites that populate UnitEventAPI)

**Step 1: Add UnitID field to UnitEventAPI struct**

In `internal/database/unit_events.go`, add after the `UnitRID` field (line 47):

```go
	UnitID        int       `json:"unit_id"`
```

**Step 2: Set UnitID = UnitRID after every Scan**

There are multiple places where `UnitEventAPI` is populated by scanning rows. After each `rows.Scan(...)` block that populates a `UnitEventAPI`, add `e.UnitID = e.UnitRID`.

Find all Scan sites by searching for `&e.UnitRID` or `&ev.UnitRID` in `unit_events.go`. There should be scan blocks in:
- `ListUnitEvents()` (~line 90)
- `ListUnitEventsGlobal()` (~line 165)

After each `rows.Scan(...)` call (before the append), add:

```go
		e.UnitID = e.UnitRID
```

(Use the actual variable name — `e` or `ev` — matching whichever is used in that function.)

**Step 3: Build and verify**

Run: `go build ./...`
Expected: PASS — no compile errors

**Step 4: Commit**

```bash
git add internal/database/unit_events.go
git commit -m "feat(api): add unit_id alias alongside unit_rid in unit-events response"
```

---

### Task 4: Update OpenAPI Spec

**Files:**
- Modify: `openapi.yaml`

**Step 1: Document dash separator in ID format section**

In `openapi.yaml`, find the ID format description (~lines 62-65) and update:

```yaml
    - **Talkgroup ID**: `{system_id}:{tgid}` (e.g., `1:9178`) or
      `{system_id}-{tgid}` (e.g., `1-9178`) or plain `{tgid}` if
      unambiguous across systems. The dash format avoids issues with
      WAF rules that block colons in URL paths.
    - **Unit ID**: `{system_id}:{unit_id}` (e.g., `1:924003`) or
      `{system_id}-{unit_id}` (e.g., `1-924003`) or plain `{unit_id}`
      if unambiguous.
```

**Step 2: Add unit_id to UnitEvent schema**

In `openapi.yaml`, find the UnitEvent schema (~line 2791). Add `unit_id` to the required array and add the field after `unit_rid`:

Change the required line:
```yaml
      required: [id, event_type, unit_rid, unit_id, time]
```

Add after the `unit_rid` property (~after line 2816):
```yaml
        unit_id:
          type: integer
          description: "Radio unit ID (alias for unit_rid, consistent with other endpoints)"
          example: 1234567
```

**Step 3: Add call_count to Unit schema**

Find the Unit schema in openapi.yaml (search for the `Unit:` schema definition). Add `call_count` as an optional property:

```yaml
        call_count:
          type: integer
          description: "Number of calls within the requested time window (only present on /talkgroups/{id}/units)"
          example: 42
```

**Step 4: Commit**

```bash
git add openapi.yaml
git commit -m "docs(openapi): document dash separator, unit_id alias, and call_count field"
```

---

### Task 5: Build, Deploy, Verify

**Step 1: Full build**

Run: `go build ./... && go test ./internal/api/ -v`
Expected: All pass

**Step 2: Deploy**

```bash
./deploy-dev.sh
```

**Step 3: Verify all three fixes via Tailnet**

Test dash separator:
```
GET http://tr-dashboard.pizzly-manta.ts.net:8000/api/v1/talkgroups/3-48686/units?window=1440&limit=3
```
Expected: 200 with units list (same as colon format)

Test call_count:
```
GET http://tr-dashboard.pizzly-manta.ts.net:8000/api/v1/talkgroups/3:48686/units?window=1440&limit=3
```
Expected: Each unit has `call_count` field, sorted by most active first

Test unit_id alias:
```
GET http://tr-dashboard.pizzly-manta.ts.net:8000/api/v1/unit-events?system_id=3&tgid=48686&limit=1
```
Expected: Response contains both `unit_rid` and `unit_id` with same value

Test Cloudflare path (dash):
```
GET https://tr-engine.luxprimatech.com/api/v1/talkgroups/3-48686
```
Expected: 200 (no longer blocked by WAF)

**Step 4: Final commit + push**

```bash
git push
```
