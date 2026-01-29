# REST API Reference

Base URL: `/api/v1`

## Systems

### List Systems

```
GET /systems
```

Returns all registered radio systems.

**Response:**
```json
{
  "systems": [
    {
      "id": 1,
      "instance_id": 1,
      "sys_num": 0,
      "short_name": "metro",
      "system_type": "p25",
      "sysid": "1A3",
      "wacn": "BEE00",
      "nac": "1A3",
      "rfss": 1,
      "site_id": 1,
      "config_json": {}
    }
  ],
  "count": 1
}
```

### Get System

```
GET /systems/{id}
```

**Parameters:**
| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| id | path | int | yes | System database ID |

**Response:** Single system object

### List System Talkgroups

```
GET /systems/{id}/talkgroups
```

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| id | path | int | yes | - | System database ID |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

---

## Talkgroups

### List Talkgroups

```
GET /talkgroups
```

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| system | query | int | no | - | Filter by system ID |
| search | query | string | no | - | Search alpha_tag, tgid, group, or tag |
| sort | query | string | no | alpha_tag | Sort field |
| sort_dir | query | string | no | asc | Sort direction: `asc` or `desc` |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

**Sort Fields:** `alpha_tag`, `tgid`, `last_seen`, `first_seen`, `group`

**Response:**
```json
{
  "talkgroups": [
    {
      "id": 8,
      "system_id": 1,
      "tgid": 9178,
      "alpha_tag": "PD Dispatch",
      "description": "City Police Dispatch",
      "group": "Law Enforcement",
      "tag": "Law Dispatch",
      "priority": 0,
      "mode": "D",
      "first_seen": "2024-01-15T10:30:00Z",
      "last_seen": "2024-01-15T12:45:00Z"
    }
  ],
  "limit": 50,
  "offset": 0
}
```

### Get Talkgroup

```
GET /talkgroups/{id}
```

Accepts multiple identifier formats:
- `sysid:tgid` - Scoped lookup (e.g., `348:9178`)
- Plain `tgid` - Radio ID lookup (returns 409 if ambiguous)
- `id:number` - Explicit database ID (e.g., `id:123`)

**Parameters:**
| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| id | path | string | yes | Talkgroup identifier (see formats above) |

**409 Conflict Response** (when tgid exists in multiple systems):
```json
{
  "error": "ambiguous_identifier",
  "message": "tgid 9178 exists in multiple systems",
  "systems": [
    {"sysid": "348", "alpha_tag": "Metro PD Dispatch"},
    {"sysid": "5A1", "alpha_tag": "County PD Dispatch"}
  ],
  "resolution": "Use explicit format: {sysid}:9178"
}
```

### List Talkgroup Calls

```
GET /talkgroups/{id}/calls
```

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| id | path | int | yes | - | Talkgroup database ID |
| start_time | query | RFC3339 | no | - | Filter calls after this time |
| end_time | query | RFC3339 | no | - | Filter calls before this time |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

**Response:**
```json
{
  "calls": [
    {
      "id": 12345,
      "tr_call_id": "1705312200_850387500_9178",
      "system_id": 1,
      "talkgroup_id": 8,
      "tgid": 9178,
      "tg_alpha_tag": "PD Dispatch",
      "start_time": "2024-01-15T10:30:00Z",
      "stop_time": "2024-01-15T10:30:45Z",
      "duration": 45.2,
      "encrypted": false,
      "emergency": false,
      "audio_path": "metro/2024/1/15/9178-1705312200.m4a",
      "audio_url": "/api/v1/calls/12345/audio",
      "units": [
        {"unit_rid": 9001234, "alpha_tag": "Unit 123"},
        {"unit_rid": 9001235, "alpha_tag": "Unit 124"}
      ]
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### Talkgroup Encryption Stats

```
GET /talkgroups/encryption-stats
```

Get encryption statistics per talkgroup over a time window.

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| hours | query | int | no | 24 | Hours to look back (1-168) |

**Response:**
```json
{
  "stats": {
    "9178": {"encrypted": 0, "clear": 156},
    "9180": {"encrypted": 45, "clear": 12}
  },
  "hours": 24
}
```

---

## Units

### List Units

```
GET /units
```

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| system | query | int | no | - | Filter by system ID |
| search | query | string | no | - | Search alpha_tag or unit_id |
| sort | query | string | no | alpha_tag | Sort field |
| sort_dir | query | string | no | asc | Sort direction: `asc` or `desc` |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

**Sort Fields:** `alpha_tag`, `unit_id`, `last_seen`, `first_seen`

**Response:**
```json
{
  "units": [
    {
      "id": 1729,
      "system_id": 1,
      "unit_id": 9001234,
      "alpha_tag": "Unit 123",
      "alpha_tag_source": "radioreference",
      "first_seen": "2024-01-15T08:00:00Z",
      "last_seen": "2024-01-15T12:45:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### Get Unit

```
GET /units/{id}
```

Accepts multiple identifier formats:
- `sysid:unit_id` - Scoped lookup (e.g., `348:9001234`)
- Plain `unit_id` - Radio ID lookup (returns 409 if ambiguous)
- `id:number` - Explicit database ID (e.g., `id:1729`)

**Parameters:**
| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| id | path | string | yes | Unit identifier (see formats above) |

**Response:**
```json
{
  "id": 1729,
  "sysid": "348",
  "unit_id": 9001234,
  "alpha_tag": "Unit 123",
  "alpha_tag_source": "radioreference",
  "first_seen": "2024-01-15T08:00:00Z",
  "last_seen": "2024-01-15T12:45:00Z"
}
```

**409 Conflict Response** (when unit_id exists in multiple systems):
```json
{
  "error": "ambiguous_identifier",
  "message": "unit_id 9001234 exists in multiple systems",
  "systems": [
    {"sysid": "348", "alpha_tag": "Unit 123"},
    {"sysid": "5A1", "alpha_tag": "Car 234"}
  ],
  "resolution": "Use explicit format: {sysid}:9001234"
}
```

### List Unit Events

```
GET /units/{id}/events
```

Get unit activity events (affiliations, registrations, calls, etc.)

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| id | path | int | yes | - | Unit database ID |
| type | query | string | no | - | Filter by event type |
| talkgroup | query | int | no | - | Filter by talkgroup ID (TGID) |
| start_time | query | RFC3339 | no | - | Filter events after this time |
| end_time | query | RFC3339 | no | - | Filter events before this time |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

**Event Types:** `on`, `off`, `join`, `call`, `ackresp`, `end`, `leave`, `data`, `status_update`

**Response:**
```json
{
  "events": [
    {
      "id": 98765,
      "instance_id": 1,
      "system_id": 1,
      "unit_id": 1729,
      "unit_rid": 9001234,
      "event_type": "call",
      "talkgroup_id": 8,
      "tgid": 9178,
      "time": "2024-01-15T10:30:00Z",
      "metadata_json": {}
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### List Unit Calls

```
GET /units/{id}/calls
```

Get calls that include transmissions from this unit.

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| id | path | int | yes | - | Unit database ID |
| start_time | query | RFC3339 | no | - | Filter calls after this time |
| end_time | query | RFC3339 | no | - | Filter calls before this time |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

### List Active Units

```
GET /units/active
```

Get units with recent activity within a time window.

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| window | query | int | no | 5 | Minutes to look back (1-60) |
| system | query | int/string | no | - | Filter by system ID or short_name |
| sys_name | query | string | no | - | Alternative system name filter |
| talkgroup | query | int | no | - | Filter by talkgroup ID |
| sort | query | string | no | last_seen | Sort field |
| sort_dir | query | string | no | desc | Sort direction |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

**Sort Fields:** `alpha_tag`, `unit_id`, `last_seen`, `first_seen`

**Response:**
```json
{
  "units": [
    {
      "id": 1729,
      "system_id": 1,
      "unit_id": 9001234,
      "alpha_tag": "Unit 123",
      "alpha_tag_source": "radioreference",
      "first_seen": "2024-01-15T08:00:00Z",
      "last_seen": "2024-01-15T12:45:00Z",
      "last_event_type": "call",
      "last_event_tgid": 9178,
      "last_event_tg_tag": "PD Dispatch",
      "last_event_time": "2024-01-15T12:45:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0,
  "window": 5
}
```

---

## Calls

### List Calls

```
GET /calls
```

Returns calls with audio recordings only.

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| system | query | int | no | - | Filter by system ID |
| talkgroup | query | int | no | - | Filter by talkgroup ID |
| start_time | query | RFC3339 | no | - | Filter calls after this time |
| end_time | query | RFC3339 | no | - | Filter calls before this time |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

**Response:**
```json
{
  "calls": [
    {
      "id": 12345,
      "tr_call_id": "1705312200_850387500_9178",
      "system_id": 1,
      "talkgroup_id": 8,
      "tgid": 9178,
      "tg_alpha_tag": "PD Dispatch",
      "start_time": "2024-01-15T10:30:00Z",
      "stop_time": "2024-01-15T10:30:45Z",
      "duration": 45.2,
      "encrypted": false,
      "emergency": false,
      "audio_path": "metro/2024/1/15/9178-1705312200.m4a",
      "audio_url": "/api/v1/calls/12345/audio",
      "units": [
        {"unit_rid": 9001234, "alpha_tag": "Unit 123"}
      ]
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### Get Call

```
GET /calls/{id}
```

**Parameters:**
| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| id | path | string/int | yes | Call `tr_call_id` or database ID |

The endpoint accepts both:
- Trunk-recorder call ID (string): `1705312200_850387500_9178`
- Database row ID (int): `12345`

**Response:** Full call object with all fields.

### Stream Call Audio

```
GET /calls/{id}/audio
```

Stream the audio file for a call.

**Parameters:**
| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| id | path | string/int | yes | Call `tr_call_id` or database ID |

**Response:**
- Content-Type: `audio/mpeg`, `audio/mp4`, `audio/wav`, or `audio/ogg`
- Content-Length: File size in bytes
- Accept-Ranges: `bytes` (supports range requests for seeking)

**Example:**
```javascript
const audio = new Audio('/api/v1/calls/12345/audio');
audio.play();
```

### List Call Transmissions

```
GET /calls/{id}/transmissions
```

Get individual unit transmissions (srcList) within a call, ordered by position.

**Parameters:**
| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| id | path | string/int | yes | Call `tr_call_id` or database ID |

**Response:**
```json
{
  "transmissions": [
    {
      "id": 54321,
      "call_id": 12345,
      "unit_id": 1729,
      "unit_rid": 9001234,
      "start_time": "2024-01-15T10:30:00Z",
      "stop_time": "2024-01-15T10:30:15Z",
      "duration": 15.2,
      "position": 0.0,
      "emergency": false,
      "error_count": 0,
      "spike_count": 0
    },
    {
      "id": 54322,
      "call_id": 12345,
      "unit_id": 1730,
      "unit_rid": 9001235,
      "start_time": "2024-01-15T10:30:18Z",
      "stop_time": "2024-01-15T10:30:30Z",
      "duration": 12.0,
      "position": 15.5,
      "emergency": false,
      "error_count": 0,
      "spike_count": 0
    }
  ],
  "count": 2
}
```

### List Call Frequencies

```
GET /calls/{id}/frequencies
```

Get frequency usage (freqList) during a call, ordered by position.

**Parameters:**
| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| id | path | string/int | yes | Call `tr_call_id` or database ID |

**Response:**
```json
{
  "frequencies": [
    {
      "id": 11111,
      "call_id": 12345,
      "freq": 850387500,
      "time": "2024-01-15T10:30:00Z",
      "position": 0.0,
      "duration": 20.5,
      "error_count": 0,
      "spike_count": 0
    }
  ],
  "count": 1
}
```

### List Active Calls

```
GET /calls/active
```

Get currently active calls (calls without a stop_time).

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| system | query | int/string | no | - | Filter by system ID or short_name |
| sys_name | query | string | no | - | Alternative system name filter |
| talkgroup | query | int | no | - | Filter by talkgroup ID |
| emergency | query | bool | no | - | Filter by emergency status |
| encrypted | query | bool | no | - | Filter by encryption status |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

### Get Real-time Active Calls

```
GET /calls/active/realtime
```

Get active calls from in-memory tracker. Only available in MQTT mode.

**Response:**
```json
{
  "calls": [...],
  "count": 5
}
```

### List Recent Calls

```
GET /calls/recent
```

Get recently completed calls with full metadata.

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| limit | query | int | no | 50 | Results (1-200) |

**Response:**
```json
{
  "calls": [
    {
      "id": 12345,
      "call_id": 12345,
      "tr_call_id": "1705312200_850387500_9178",
      "call_num": 42,
      "start_time": "2024-01-15T10:30:00Z",
      "stop_time": "2024-01-15T10:30:45Z",
      "duration": 45.2,
      "system": "metro",
      "tgid": 9178,
      "tg_alpha_tag": "PD Dispatch",
      "freq": 850387500,
      "encrypted": false,
      "emergency": false,
      "audio_path": "metro/2024/1/15/9178-1705312200.m4a",
      "audio_url": "/api/v1/calls/12345/audio",
      "has_audio": true,
      "units": [
        {"unit_id": 9001234, "unit_tag": "Unit 123"}
      ]
    }
  ],
  "count": 1
}
```

---

## Call Groups

Call groups are deduplicated calls from multiple recorders capturing the same transmission.

### List Call Groups

```
GET /call-groups
```

**Parameters:**
| Name | In | Type | Required | Default | Description |
|------|-----|------|----------|---------|-------------|
| start_time | query | RFC3339 | no | - | Filter after this time |
| end_time | query | RFC3339 | no | - | Filter before this time |
| limit | query | int | no | 50 | Results per page |
| offset | query | int | no | 0 | Page offset |

**Response:**
```json
{
  "call_groups": [
    {
      "id": 100,
      "system_id": 1,
      "talkgroup_id": 8,
      "tgid": 9178,
      "start_time": "2024-01-15T10:30:00Z",
      "end_time": "2024-01-15T10:30:45Z",
      "primary_call_id": 12345,
      "call_count": 2,
      "encrypted": false,
      "emergency": false
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### Get Call Group

```
GET /call-groups/{id}
```

**Parameters:**
| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| id | path | int | yes | Call group database ID |

**Response:**
```json
{
  "call_group": {
    "id": 100,
    "system_id": 1,
    "talkgroup_id": 8,
    "tgid": 9178,
    "start_time": "2024-01-15T10:30:00Z",
    "end_time": "2024-01-15T10:30:45Z",
    "primary_call_id": 12345,
    "call_count": 2,
    "encrypted": false,
    "emergency": false
  },
  "calls": [
    { "id": 12345, ... },
    { "id": 12346, ... }
  ]
}
```

---

## Recorders

### List Recorders

```
GET /recorders
```

Get recorder states from in-memory cache (MQTT mode) or watch mode provider.

**Response:**
```json
{
  "recorders": [
    {
      "id": 1,
      "rec_num": 0,
      "state": 1,
      "state_name": "recording",
      "freq": 850387500,
      "tgid": 9178,
      "unit_id": 9001234,
      "call_count": 42
    }
  ],
  "count": 1
}
```

**State Values:**
| Value | Name | Description |
|-------|------|-------------|
| 0 | available | Ready to record |
| 1 | recording | Currently recording a call |
| 2 | idle | Idle/monitoring |

---

## Statistics

### Get Overall Stats

```
GET /stats
```

**Response:**
```json
{
  "total_systems": 1,
  "total_talkgroups": 250,
  "total_units": 2575,
  "total_calls": 21859,
  "active_calls": 5,
  "calls_last_hour": 434,
  "calls_last_24h": 12576,
  "audio_files": 21859,
  "audio_bytes": 1065565793
}
```

### Get Decode Rates

```
GET /stats/rates
```

Get system decode rate measurements over time.

**Parameters:**
| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| start_time | query | RFC3339 | no | Filter after this time |
| end_time | query | RFC3339 | no | Filter before this time |

**Response:**
```json
{
  "rates": [
    {
      "system_id": 1,
      "short_name": "metro",
      "time": "2024-01-15T10:30:00Z",
      "decode_rate": 98.5,
      "control_channel": 851012500
    }
  ],
  "count": 1
}
```

### Get Activity Summary

```
GET /stats/activity
```

**Response:**
```json
{
  "systems": 1,
  "talkgroups": 250,
  "units": 2575,
  "calls_24h": 12576,
  "system_activity": [
    {"system": "metro", "call_count": 12576}
  ]
}
```
