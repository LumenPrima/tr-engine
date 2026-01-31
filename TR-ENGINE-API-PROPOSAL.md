# tr-engine API Proposal: Radio ID-Based Lookups

> **Implementation Status: ✅ COMPLETE**
>
> The core SYSID-based scoping has been implemented:
> - Schema migration to use `sysid` column for talkgroups and units
> - Junction tables (`talkgroup_sites`, `unit_sites`) for site tracking
> - REST API support for `sysid:tgid` and `sysid:unit_id` lookup formats
> - Updated `ListTalkgroups` and `ListUnits` to filter by `sysid` parameter
> - Swagger documentation regenerated
>
> **Not yet implemented**: 409 Conflict response for ambiguous lookups (simple tgid without sysid when collisions exist). Currently, simple numeric lookups resolve to database row ID.

## Summary

This proposal suggests adding support for radio-native identifiers (tgid, RID) as primary lookup keys in the tr-engine API, with optional SYSID scoping for multi-system deployments.

## Problem Statement

### Current State

The tr-engine API uses database-generated IDs for entity lookups:

```
GET /api/v1/talkgroups/123    # 123 is database ID
GET /api/v1/units/456         # 456 is database ID
```

### Issues

1. **ID Mismatch**: WebSocket events provide radio IDs (`tgid`, `unit_id`), but REST endpoints require database IDs. Frontend must maintain a mapping cache or perform extra lookups.

2. **User Mental Model**: Users think in terms of radio IDs. A scanner user knows "TG 9178" - they don't know or care that it's database row 123.

3. **Cross-Reference Friction**: Linking from real-time data (WebSocket) to detail pages requires tgid→database ID resolution.

4. **Favorites/Monitoring**: Features that track talkgroups naturally use tgid, but navigation requires database ID lookup.

## Proposed Solution

### Design Principles

1. **Simple by default**: Single-system deployments (majority of users) should use simple tgid lookups
2. **Explicit when needed**: Multi-system deployments can use composite IDs when collisions exist
3. **Graceful errors**: Collisions return helpful error messages with resolution instructions
4. **Backwards compatible**: Database ID lookups continue to work

### Lookup Resolution Rules

For `GET /api/v1/talkgroups/{identifier}`:

| Scenario | Identifier | Result |
|----------|------------|--------|
| Single system deployment | `9178` | Returns talkgroup |
| Multi-system, no collision | `9178` | Returns talkgroup (only exists in one system) |
| Multi-system, collision exists | `9178` | `409 Conflict` with resolution guidance |
| Explicit SYSID scope | `348:9178` | Returns specific talkgroup |
| Database ID (legacy) | `id:123` | Returns talkgroup by database ID |

### Identifier Format

```
{identifier} = {tgid}              # Simple: 9178
             | {sysid}:{tgid}      # Scoped: 348:9178
             | id:{database_id}    # Legacy: id:123
```

The `id:` prefix for database IDs ensures no ambiguity with numeric tgids.

### Why SYSID, Not WACN?

WACN (Wide Area Communications Network) identifiers like `BEE00` are commonly shared across many P25 systems. For example, `BEE00` is a default WACN used by numerous Motorola-based public safety networks across multiple states for interoperability purposes.

SYSID (System ID) is what actually differentiates P25 networks:
- Ohio MARCS: WACN=`BEE00`, SYSID=`348`
- Another state system: WACN=`BEE00`, SYSID=`XXX`

Sites within the same logical network share both WACN and SYSID (e.g., Butler County and Warren County sites both use SYSID `348`), so talkgroups and units are unique within a SYSID scope.

## API Changes

### Talkgroups

```http
# Lookup by tgid (simple)
GET /api/v1/talkgroups/9178

# Lookup by tgid (SYSID-scoped)
GET /api/v1/talkgroups/348:9178

# Lookup by database ID (legacy/explicit)
GET /api/v1/talkgroups/id:123

# List with tgid filter
GET /api/v1/talkgroups?tgid=9178
GET /api/v1/talkgroups?tgid=348:9178
```

### Units

```http
# Lookup by RID (simple)
GET /api/v1/units/4521

# Lookup by RID (SYSID-scoped)
GET /api/v1/units/348:4521

# Lookup by database ID (legacy/explicit)
GET /api/v1/units/id:456

# List with unit_id filter
GET /api/v1/units?unit_id=4521
GET /api/v1/units?unit_id=348:4521
```

### Calls

Calls continue to use database IDs as primary identifiers since:
- Call IDs are system-generated, not radio-inherent
- Users discover calls via search/filter, not by "knowing" a call ID
- Database IDs are already globally unique

```http
# Calls stay as-is
GET /api/v1/calls/789

# Filter by tgid uses radio ID
GET /api/v1/calls?tgid=9178
GET /api/v1/calls?tgid=348:9178
```

### Error Response for Collisions

When a tgid exists in multiple systems and no scope is provided:

```http
GET /api/v1/talkgroups/9178

HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "error": "ambiguous_identifier",
  "message": "tgid 9178 exists in multiple systems",
  "systems": [
    {"sysid": "348", "name": "Ohio MARCS", "alpha_tag": "OSP Dispatch"},
    {"sysid": "5A1", "name": "Indiana SAFE-T", "alpha_tag": "ISP Channel 1"}
  ],
  "resolution": "Use explicit format: 348:9178 or 5A1:9178"
}
```

## Data Model Considerations

### Network/System Entity

This proposal implies a parent entity grouping sites and scoping tgid/RID uniqueness:

```
Network (P25 System)
├── id (database ID)
├── sysid (string, e.g., "348")
├── wacn (string, e.g., "BEE00") - for reference, not uniqueness
├── name (string, e.g., "Ohio MARCS")
└── sites[] (what TR config calls "systems")
    ├── butco
    └── warco
```

### Current "System" Clarification

The current `system` field in tr-engine represents a **site** (trunk-recorder config entry), not a radio system boundary:

- `butco` = Butler County site (control channels ~853 MHz)
- `warco` = Warren County site (control channels ~858 MHz)
- Both sites belong to the same P25 network (SYSID 348, WACN BEE00)

Sites sharing the same SYSID share talkgroup/unit namespace.

### Uniqueness Constraints

| Entity | Unique Within |
|--------|---------------|
| tgid | SYSID |
| unit_id (RID) | SYSID |
| call_id | tr-engine instance (use database ID) |
| site/system | SYSID |

### Schema Changes

**Talkgroups and Units** - Remove `system_id` foreign key, add `sysid` as the scoping key:

```sql
CREATE TABLE talkgroups (
    id              SERIAL PRIMARY KEY,
    sysid           VARCHAR(16) NOT NULL,
    tgid            INTEGER NOT NULL,
    alpha_tag       VARCHAR(255),
    description     TEXT,
    tg_group        VARCHAR(255),
    tag             VARCHAR(64),
    priority        INTEGER,
    mode            VARCHAR(16),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(sysid, tgid)
);

CREATE TABLE units (
    id              SERIAL PRIMARY KEY,
    sysid           VARCHAR(16) NOT NULL,
    unit_id         BIGINT NOT NULL,
    alpha_tag       VARCHAR(255),
    alpha_tag_source VARCHAR(32),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(sysid, unit_id)
);
```

**Site Tracking** - Junction tables to track which sites have seen each talkgroup/unit:

```sql
CREATE TABLE talkgroup_sites (
    talkgroup_id    INTEGER REFERENCES talkgroups(id) ON DELETE CASCADE,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (talkgroup_id, system_id)
);

CREATE TABLE unit_sites (
    unit_id         INTEGER REFERENCES units(id) ON DELETE CASCADE,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (unit_id, system_id)
);
```

This enables:
- Roaming analysis: which units have been seen at multiple sites
- Coverage patterns: which talkgroups are active at which sites
- Temporal tracking: first/last seen per site

## Migration Path

### Phase 1: Add Radio ID Lookups (Non-Breaking)

Add new resolution logic to existing endpoints:
- `/talkgroups/{id}` tries: tgid lookup → database ID lookup
- Single-system deployments work immediately
- Multi-system deployments get helpful collision errors

### Phase 2: Add Network Entity

- Add `networks` table/endpoint with SYSID as key identifier
- Associate sites with networks via SYSID
- Enable SYSID-scoped lookups

### Phase 3: Deprecation (Optional)

- Mark `id:{database_id}` format as deprecated
- Update documentation to prefer radio ID lookups

## Examples

### Single-System Deployment (Common Case)

```javascript
// Frontend can use tgid directly from WebSocket data
const tgid = wsEvent.talkgroup; // 9178

// Direct navigation - no cache lookup needed
window.location = `/talkgroups/${tgid}`;

// API call - no ID translation needed
const tg = await fetch(`/api/v1/talkgroups/${tgid}`);
```

### Multi-System Deployment with Collision

```javascript
// First attempt
const response = await fetch(`/api/v1/talkgroups/9178`);

if (response.status === 409) {
  const error = await response.json();
  // Show user: "TG 9178 exists in: Ohio MARCS (348), Indiana SAFE-T (5A1)"
  // Let them choose, then use explicit format
  const tg = await fetch(`/api/v1/talkgroups/348:9178`);
}
```

### WebSocket Event Integration

```javascript
// WebSocket provides tgid and optionally sysid
ws.on('audio_available', (data) => {
  const identifier = data.sysid
    ? `${data.sysid}:${data.talkgroup}`
    : data.talkgroup;

  // Works for both single and multi-system
  navigateTo(`/talkgroups/${identifier}`);
});
```

## Benefits

1. **Zero friction** for single-system deployments (majority of users)
2. **Natural mental model** - use the IDs users actually see/know
3. **Eliminates client-side caching** of tgid→database ID mappings
4. **Cleaner URLs** - `/talkgroups/9178` vs `/talkgroups/123`
5. **Future-proof** for multi-system aggregation
6. **Graceful degradation** - collisions are errors with clear resolution

## Open Questions

1. Should WebSocket events include `sysid` field for future-proofing?
2. Should there be a global config to require explicit SYSID scoping even without collisions?

---

# Additional API Suggestions

The following suggestions come from frontend development experience and aim to reduce client-side complexity.

## 1. Response Consistency

### Problem

Some endpoints return raw arrays while others wrap in objects:

```javascript
// Inconsistent:
GET /api/v1/calls/123/transmissions  →  [...]           // raw array
GET /api/v1/calls                    →  {calls: [...]}  // wrapped
```

### Suggestion

Standardize on wrapped responses:

```javascript
GET /api/v1/calls/123/transmissions  →  {transmissions: [...]}
GET /api/v1/calls/123/frequencies    →  {frequencies: [...]}
```

Benefits:
- Consistent parsing logic
- Room for metadata (pagination, timing) without breaking changes
- Easier to extend with additional fields

---

## 2. Include Radio IDs in Call Detail

### Problem

Call detail returns `talkgroup_id` (database ID) but not `tgid` (radio ID). Frontend must fetch talkgroup separately just to display the TGID.

### Current Response

```json
{
  "id": 6303,
  "talkgroup_id": 45,
  "system": "butco",
  ...
}
```

### Suggested Response

```json
{
  "id": 6303,
  "talkgroup_id": 45,
  "tgid": 9178,
  "tg_alpha_tag": "Fire Dispatch",
  "system": "butco",
  ...
}
```

Same pattern for units in transmissions - include both `unit_id` (database) and `rid` (radio) plus `unit_alpha_tag`.

---

## 3. Embed Unit Info in Transmissions

### Problem

Transmissions include `unit_id` but no identifying info. To display "Unit 4521 (Medic 42)" instead of just "Unit 4521", frontend must fetch each unit separately.

### Current

```json
{
  "transmissions": [
    {"unit_id": 123, "pos": 0.5, "length": 3.2},
    {"unit_id": 124, "pos": 4.0, "length": 2.1}
  ]
}
```

### Suggested

```json
{
  "transmissions": [
    {"unit_id": 123, "rid": 4521, "unit_alpha_tag": "Medic 42", "pos": 0.5, "length": 3.2},
    {"unit_id": 124, "rid": 4522, "unit_alpha_tag": "Engine 7", "pos": 4.0, "length": 2.1}
  ]
}
```

Or provide a `units` lookup object:

```json
{
  "transmissions": [...],
  "units": {
    "123": {"rid": 4521, "alpha_tag": "Medic 42"},
    "124": {"rid": 4522, "alpha_tag": "Engine 7"}
  }
}
```

---

## 4. Null Instead of Sentinel Values

### Problem

Some fields use magic numbers as "no data" indicators:
- `signal_db: 999` means "unknown"
- `noise_db: 999` means "unknown"
- `duration: -1` or `0` may mean "unknown"

Frontend must know these conventions and filter them out.

### Suggestion

Use `null` for missing/unknown values:

```json
{
  "signal_db": null,
  "noise_db": null,
  "duration": null
}
```

Or omit the fields entirely. Document which fields are optional.

Benefits:
- Standard JSON null semantics
- No magic number documentation needed
- TypeScript can use `number | null` instead of runtime checks for `>= 900`

---

## 5. Decode Rate Context

### Problem

`rate_update` WebSocket events provide a decode rate number, but without context it's hard to display meaningfully.

### Current

```json
{"type": "rate_update", "system": "butco", "rate": 38.5}
```

### Suggested Options

**Option A**: Include max in event

```json
{"type": "rate_update", "system": "butco", "rate": 38.5, "max_rate": 40}
```

**Option B**: Include in system info endpoint

```json
GET /api/v1/systems/butco
{
  "name": "butco",
  "control_channel": {
    "protocol": "P25_Phase1",
    "max_decode_rate": 40
  }
}
```

**Option C**: Document the constant

If it's always 40 for P25 Phase 1 CCS, document this in the API spec.

---

## 6. WebSocket Subscription Feedback

### Problem

After sending a subscription message, there's no confirmation. Frontend doesn't know if subscription was accepted, if filters are valid, or current subscription state.

### Current Flow

```
Client → {"type": "subscribe", "talkgroups": [9178, 9445]}
Server → (silence, then events start arriving)
```

### Suggested Flow

```
Client → {"type": "subscribe", "talkgroups": [9178, 9445]}
Server → {"type": "subscribed", "talkgroups": [9178, 9445], "systems": ["*"], "units": ["*"]}
```

Or for errors:

```
Client → {"type": "subscribe", "talkgroups": [99999]}
Server → {"type": "subscribe_error", "message": "Unknown talkgroup: 99999"}
```

Benefits:
- Confirmation that subscription is active
- Debug visibility into current filter state
- Graceful error handling for invalid filters

---

## 7. Call Audio URL in Call Object

### Problem

To get audio URL, frontend must construct it from call_id:

```javascript
const audioUrl = `/api/v1/calls/${call.call_id}/audio`
```

### Suggestion

Include it in the call object:

```json
{
  "id": 6303,
  "audio_url": "/api/v1/calls/6303/audio",
  "audio_available": true,
  ...
}
```

Benefits:
- URL construction stays server-side (can change format without breaking clients)
- `audio_available` flag indicates if audio file exists
- Could include multiple formats in future: `{"m4a": "...", "wav": "..."}`

---

## Summary Table

| Issue | Impact | Effort | Priority |
|-------|--------|--------|----------|
| Radio IDs in call detail | High - extra fetches | Low | High |
| Unit info in transmissions | High - N+1 queries | Low | High |
| Null vs sentinel values | Medium - bug source | Low | Medium |
| Response consistency | Low - minor annoyance | Low | Low |
| Decode rate context | Low - display only | Low | Low |
| WebSocket subscription feedback | Medium - debugging | Medium | Medium |
| Audio URL in response | Low - convenience | Low | Low |

---

*Proposal drafted for tr-engine development team*
*From: tr-dashboard frontend development*
