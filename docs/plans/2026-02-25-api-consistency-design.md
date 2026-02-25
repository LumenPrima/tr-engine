# API Consistency Fixes — Design

## Summary

Three targeted fixes to reduce API consumer friction discovered during a talkgroup investigation session.

## Changes

### 1. Dash Separator for Composite IDs

**Problem:** Cloudflare WAF blocks colons in URL paths, returning 403 for requests like `/talkgroups/3:48686`. Affects 9 endpoints (4 talkgroup, 5 unit) that use `system_id:entity_id` composite IDs.

**Fix:** Update `ParseCompositeID()` in `internal/api/composite_id.go` to accept `-` as an alternative separator. `3-48686` and `3:48686` both resolve identically. Colon remains the canonical format; dash is the Cloudflare-safe alternative.

**Affected endpoints:**
- `GET/PATCH /talkgroups/{id}`
- `GET /talkgroups/{id}/calls`
- `GET /talkgroups/{id}/units`
- `GET/PATCH /units/{id}`
- `GET /units/{id}/calls`
- `GET /units/{id}/events`

### 2. Add `call_count` to `/talkgroups/{id}/units`

**Problem:** The endpoint returns units that talked on a talkgroup within a time window, but not how many calls each unit made. Consumers must fetch all calls and count client-side.

**Fix:** Modify `ListTalkgroupUnits()` SQL in `internal/database/talkgroups.go` to count calls per unit. Add `CallCount` field to `UnitAPI` struct. The existing unnest query already touches the right data — add a GROUP BY with COUNT.

### 3. Add `unit_id` Alias to Unit-Events Response

**Problem:** Every endpoint returns `unit_id` for the radio identifier, except `/unit-events` which returns `unit_rid` (matching the P25 "Radio ID" term from trunk-recorder's MQTT messages and the DB column name). Consumers expect `unit_id` and get confused.

**Fix:** Add `unit_id` field to `UnitEventAPI` struct alongside existing `unit_rid`. Both fields return the same value. `unit_rid` is preserved for backwards compatibility; `unit_id` is the consistent name. Document in openapi.yaml.

## Files

- `internal/api/composite_id.go` — dash separator parsing
- `internal/api/composite_id_test.go` — test dash format
- `internal/database/talkgroups.go` — call_count in ListTalkgroupUnits SQL
- `internal/database/unit_events.go` — UnitEventAPI struct + scan
- `openapi.yaml` — document dash separator, call_count field, unit_id alias
