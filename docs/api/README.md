# tr-engine API Documentation

Complete API reference for building frontend applications against tr-engine.

## Overview

tr-engine is a backend service that aggregates data from trunk-recorder radio systems. It provides:

- **REST API** for querying historical data (calls, talkgroups, units, systems)
- **WebSocket API** for real-time updates (active calls, unit events, decode rates)
- **Audio streaming** for playback of recorded calls

## Quick Start

### Base URLs

| Service | URL |
|---------|-----|
| REST API | `http://localhost:8080/api/v1` |
| WebSocket | `ws://localhost:8080/api/ws` |
| Dashboard | `http://localhost:8080/dashboard` |
| Swagger UI | `http://localhost:8080/swagger/` |

### Authentication

Currently no authentication is required. All endpoints are publicly accessible.

## Documentation Index

| Document | Description |
|----------|-------------|
| [REST.md](REST.md) | Complete REST API reference with all endpoints |
| [WEBSOCKET.md](WEBSOCKET.md) | WebSocket API for real-time events |
| [MODELS.md](MODELS.md) | Data models and field definitions |
| [TERMINOLOGY.md](TERMINOLOGY.md) | Radio system terminology and concepts |

## Common Patterns

### Pagination

All list endpoints support pagination:

```
GET /api/v1/calls?limit=50&offset=0
```

| Parameter | Type | Default | Max | Description |
|-----------|------|---------|-----|-------------|
| `limit` | int | 50 | 1000 | Results per page |
| `offset` | int | 0 | - | Number of results to skip |

### Time Filtering

Time parameters use RFC3339 format:

```
GET /api/v1/calls?start_time=2024-01-15T10:00:00Z&end_time=2024-01-15T12:00:00Z
```

### Response Format

All responses are JSON. Successful responses return data directly or wrapped:

```json
{
  "calls": [...],
  "count": 50,
  "limit": 50,
  "offset": 0
}
```

Error responses:

```json
{
  "error": "Error message description"
}
```

### HTTP Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Bad request (invalid parameters) |
| 404 | Resource not found |
| 409 | Conflict (ambiguous identifier in multi-system deployment) |
| 500 | Server error |

## Key Concepts

### Entity Relationships

```
System (1) ─────┬───── (*) Talkgroup
                │
                ├───── (*) Unit
                │
                └───── (*) Call ─────┬───── (*) Transmission
                                     │
                                     └───── (*) CallFrequency
```

### ID Types and Lookup Formats

tr-engine uses two types of identifiers:

1. **Database IDs** (`id`) - Auto-incrementing integers
2. **Radio IDs** - Original identifiers from the radio system:
   - `tgid` - Talkgroup ID (e.g., 9178)
   - `unit_id` / `unit_rid` - Radio unit ID (e.g., 9001234)
   - `sysid` - P25 System ID for scoping (e.g., "348")
   - `tr_call_id` - Trunk-recorder's call identifier

**Lookup Formats for Talkgroups and Units:**

| Format | Example | Description |
|--------|---------|-------------|
| `sysid:id` | `348:9178` | Scoped lookup by SYSID and radio ID |
| Plain numeric | `9178` | Lookup by radio ID (returns 409 if ambiguous) |
| `id:number` | `id:123` | Explicit database ID lookup |

For single-system deployments, plain numeric lookups work directly. Multi-system deployments should use `sysid:id` format to avoid ambiguity.

### Audio Access

Audio files are served via the REST API:

```
GET /api/v1/calls/{id}/audio
```

Supported formats (auto-detected by file extension):
- `audio/mpeg` (.mp3)
- `audio/mp4` (.m4a)
- `audio/wav` (.wav)
- `audio/ogg` (.ogg)

Supports HTTP Range requests for seeking.

## Example: Fetch Recent Calls

```javascript
// Fetch 20 most recent calls
const response = await fetch('/api/v1/calls/recent?limit=20');
const data = await response.json();

data.calls.forEach(call => {
  console.log(`${call.tg_alpha_tag}: ${call.duration}s`);

  // Play audio
  const audio = new Audio(`/api/v1/calls/${call.id}/audio`);
  audio.play();
});
```

## Example: Real-time Updates

```javascript
const ws = new WebSocket('ws://localhost:8080/api/ws');

ws.onopen = () => {
  // Subscribe to call events
  ws.send(JSON.stringify({
    action: 'subscribe',
    channels: ['calls', 'units']
  }));
};

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);

  if (msg.event === 'call_end') {
    console.log('Call ended:', msg.data);
  }
};
```

## Rate Limits

No rate limits are currently enforced. However, for performance:
- Limit WebSocket subscriptions to needed channels
- Use pagination for large result sets
- Cache static data (systems, talkgroups) client-side
