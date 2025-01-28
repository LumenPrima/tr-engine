# API Usage Guide

## Base URL
All API endpoints are relative to: `/api/v1`

## Common Patterns

### Timestamps
All timestamps in requests should be Unix timestamps (seconds since epoch). Responses include both Unix timestamps and ISO 8601 formatted dates.

### Response Format
All endpoints follow this response format:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        // Endpoint-specific data
    }
}
```

Error responses:
```json
{
    "status": "error",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "message": "Error description"
}
```

### Pagination
Many endpoints support pagination with these parameters:
- `limit`: Number of results (default 100)
- `offset`: Starting position (default 0)

Paginated responses include:
```json
{
    "pagination": {
        "total": 1234,
        "limit": 100,
        "offset": 0,
        "has_more": true
    }
}
```

## System Status

### GET /systems
Get status of all systems

Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "count": 2,
    "systems": [
        {
            "name": "monco",
            "sys_name": "monco",
            "sys_num": 1,
            "type": "p25",
            "status": {
                "connected": true,
                "last_seen": "2025-01-28T20:19:47.000Z",
                "current_control_channel": 853062500,
                "current_decoderate": 34.33
            },
            "config": {}
        }
    ]
}
```

### GET /systems/performance
Get system performance statistics

Query Parameters:
- None

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "total_systems": 4,
        "active_systems": 4,
        "system_stats": [
            {
                "name": "monco",
                "sys_name": "monco",
                "sys_num": 0,
                "type": "p25",
                "control_channel": 853062500,
                "decoderate": 34.33,
                "decoderate_interval": 3,
                "recent_rates": [
                    {
                        "timestamp": "2025-01-28T20:19:47.000Z",
                        "decoderate": 29,
                        "control_channel": 853062500
                    }
                ]
            }
        ],
        "aggregate": {
            "average_decoderate": 32.33
        }
    }
}
```

### GET /systems/:sys_name
Get detailed status for specific system

Parameters:
- `sys_name`: System identifier

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "system": {
        "sys_name": "monco",
        "sys_num": 1,
        "type": "p25",
        "sysid": "123",
        "wacn": "BEE00",
        "nac": "293",
        "rfss": "1",
        "site_id": "1",
        "status": {
            "connected": true,
            "last_seen": "2025-01-28T20:19:47.000Z",
            "last_config_update": "2025-01-28T20:19:47.000Z",
            "last_rate_update": "2025-01-28T20:19:47.000Z"
        },
        "performance": {
            "current_control_channel": 853062500,
            "current_decoderate": 34.33,
            "decoderate_interval": 3,
            "recent_rates": []
        },
        "config": {}
    }
}
```

## Audio Files

### GET /audio/:callId
Get audio file for a specific call

Parameters:
- `callId`: Format `talkgroup-timestamp` (e.g., "58366-1738094143")
- `format`: (optional) Preferred format ("wav" or "m4a")

Response streams the audio file with proper headers for range requests.

### GET /audio/archive
Search archived audio recordings

Query Parameters:
- `start`: Start timestamp (default 24h ago)
- `end`: End timestamp (default now)
- `talkgroup`: Filter by talkgroup
- `sys_name`: Filter by system
- `format`: Filter by format
- `emergency`: Filter emergency calls
- `limit`: Results per page (default 100)
- `offset`: Page offset (default 0)

Examples:
```
# Get recent audio files for a talkgroup
GET /audio/archive?talkgroup=58366&start=1738094143&limit=10

# Get emergency calls from last hour
GET /audio/archive?emergency=true&start=1738090543&end=1738094143

# Get all m4a files from a system
GET /audio/archive?sys_name=monco&format=m4a
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "pagination": {
            "total": 1234,
            "limit": 10,
            "offset": 0,
            "has_more": true
        },
        "time_range": {
            "start": "2025-01-28T19:19:47.000Z",
            "end": "2025-01-28T20:19:47.000Z"
        },
        "count": 10,
        "files": [
            {
                "id": "67993648921e2e29f53b372f",
                "filename": "58366-1738094143_853325000.0-call_135975.wav",
                "timestamp": "2025-01-28T20:19:47.000Z",
                "talkgroup": 58366,
                "talkgroup_tag": "57 RTA 366",
                "start_time": 1738094143,
                "stop_time": 1738094148,
                "call_length": 5,
                "emergency": false,
                "encrypted": false,
                "freq": 853325000,
                "audio_type": "digital"
            }
        ]
    }
}
```

## Transcriptions

### GET /transcription/:callId
Get transcription for specific call

Parameters:
- `callId`: Format `talkgroup-timestamp` (e.g., "58366-1738094143")

Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "call_id": "58366-1738094143",
        "text": "Dispatch responding to...",
        "audio_duration": 15.5,
        "processing_time": 2.3,
        "model": "whisper-1",
        "created_at": "2025-01-28T20:19:47.000Z"
    }
}
```

### GET /transcription/group
Get multiple transcriptions near a timestamp

Query Parameters:
- `time`: Target Unix timestamp
- `limit`: Number of results (default 10)

Example:
```
# Get 5 transcriptions closest to a time
GET /transcription/group?time=1738094143&limit=5
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "count": 5,
        "transcriptions": [
            {
                "call_id": "58366-1738094143",
                "talkgroup": 58366,
                "text": "Dispatch responding to...",
                "timestamp": "2025-01-28T20:19:03.000Z",
                "time_diff_seconds": 0,
                "metadata": {
                    "audio_duration": 15.5,
                    "processing_time": 2.3,
                    "model": "whisper-1"
                }
            }
        ],
        "target": "2025-01-28T20:19:03.000Z"
    }
}
```

### GET /transcription/:talkgroupId/group
Get multiple transcriptions near a timestamp for specific talkgroup

Parameters:
- `talkgroupId`: Talkgroup number

Query Parameters:
- `time`: Target Unix timestamp
- `limit`: Number of results (default 10)

Example:
```
# Get 5 transcriptions for talkgroup near time
GET /transcription/58366/group?time=1738094143&limit=5
```

### GET /transcription/:talkgroupId/recent
Get recent transcriptions for a talkgroup

Parameters:
- `talkgroupId`: Talkgroup number

Query Parameters:
- `start`: Start timestamp (default 24h ago)
- `end`: End timestamp (default now)
- `limit`: Number of results (default 10)

Example:
```
# Get recent transcriptions for talkgroup
GET /transcription/58366/recent?limit=5
```

### GET /transcription/stats
Get transcription statistics

Query Parameters:
- `talkgroup`: Filter by talkgroup (optional)
- `start`: Start timestamp (optional)
- `end`: End timestamp (optional)

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "total_transcriptions": 1234,
        "total_duration": 3600,
        "avg_duration": 15.5,
        "avg_processing_time": 2.3,
        "total_words": 12340,
        "words_per_second": 3.4
    }
}
```

## Units

### GET /units
Get all units with pagination

Query Parameters:
- `limit`: Results per page (default 100)
- `offset`: Page offset (default 0)

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "pagination": {
            "total": 1234,
            "limit": 100,
            "offset": 0,
            "has_more": true
        },
        "count": 100,
        "units": [
            {
                "unit": 5783762,
                "sys_name": "monco",
                "unit_alpha_tag": "UNIT 123",
                "status": {
                    "online": true,
                    "last_seen": "2025-01-28T20:19:47.000Z",
                    "current_talkgroup": 58366,
                    "current_talkgroup_tag": "57 RTA 366"
                }
            }
        ]
    }
}
```

### GET /units/active
Get currently active units

Query Parameters:
- `window`: Time window in minutes (default 5)

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "count": 2,
        "units": [
            {
                "unit": 5783762,
                "sys_name": "monco",
                "unit_alpha_tag": "UNIT 123",
                "status": {
                    "online": true,
                    "last_seen": "2025-01-28T20:19:47.000Z",
                    "current_talkgroup": 58366,
                    "current_talkgroup_tag": "57 RTA 366",
                    "active_since": "2025-01-28T20:14:47.000Z"
                }
            }
        ]
    }
}
```

### GET /units/:unit_id
Get unit details

Parameters:
- `unit_id`: Unit identifier

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "unit": 5783762,
        "sys_name": "monco",
        "unit_alpha_tag": "UNIT 123",
        "status": {
            "online": true,
            "last_seen": "2025-01-28T20:19:47.000Z",
            "current_talkgroup": 58366,
            "current_talkgroup_tag": "57 RTA 366"
        },
        "history": {
            "total_calls": 123,
            "total_talkgroups": 5,
            "most_active_talkgroup": 58366
        }
    }
}
```

### GET /units/talkgroup/:talkgroup_id
Get units in talkgroup

Parameters:
- `talkgroup_id`: Talkgroup number

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "talkgroup": 58366,
        "talkgroup_tag": "57 RTA 366",
        "count": 5,
        "units": [
            {
                "unit": 5783762,
                "unit_alpha_tag": "UNIT 123",
                "status": {
                    "online": true,
                    "last_seen": "2025-01-28T20:19:47.000Z"
                }
            }
        ]
    }
}
```

## Calls

### GET /calls
Get historical calls

Query Parameters:
- `start`: Start timestamp (default 24h ago)
- `end`: End timestamp (default now)
- `sys_name`: Filter by system
- `talkgroup`: Filter by talkgroup
- `unit`: Filter by unit
- `emergency`: Filter emergency calls
- `limit`: Results per page (default 100)
- `offset`: Page offset (default 0)

Example:
```
# Get emergency calls for a talkgroup
GET /calls?talkgroup=58366&emergency=true&limit=10
```

### GET /calls/active
Get currently active calls

Query Parameters:
- `sys_name`: Filter by system (optional)
- `talkgroup`: Filter by talkgroup (optional)
- `emergency`: Filter emergency calls (optional)

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "count": 2,
        "calls": [
            {
                "call_id": "58366-1738094143",
                "sys_name": "monco",
                "talkgroup": 58366,
                "talkgroup_tag": "57 RTA 366",
                "emergency": false,
                "units": [5783762],
                "start_time": 1738094143,
                "active": true
            }
        ]
    }
}
```

### GET /calls/talkgroup/:talkgroup_id
Get historical activity for talkgroup

Parameters:
- `talkgroup_id`: Talkgroup number

Query Parameters:
- `start`: Start timestamp (default 24h ago)
- `end`: End timestamp (default now)
- `limit`: Number of results (default 100)

Example:
```
# Get last 10 calls and joins for a talkgroup
GET /calls/talkgroup/58366?limit=10

# Get activity in time range
GET /calls/talkgroup/58366?start=1738090543&end=1738094143
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "talkgroup": {
            "talkgroup": 58366,
            "talkgroup_tag": "57 RTA 366"
        },
        "time_range": {
            "start": "2025-01-28T19:19:47.000Z",
            "end": "2025-01-28T20:19:47.000Z"
        },
        "count": 10,
        "events": [
            {
                "call_id": "58366-1738094143",
                "timestamp": "2025-01-28T20:19:03.000Z",
                "activity_type": "call",
                "units": [5783762],
                "emergency": false,
                "duration": 5,
                "audio_type": "digital",
                "freq": 853325000,
                "encrypted": false
            },
            {
                "timestamp": "2025-01-28T20:18:47.000Z",
                "activity_type": "join",
                "unit": 5783762,
                "unit_alpha_tag": "UNIT 123",
                "talkgroup": 58366,
                "talkgroup_tag": "57 RTA 366"
            }
        ]
    }
}
```

### GET /calls/events
Get currently active events (calls, emergencies, etc.)

Query Parameters:
- None

## Health Check

### GET /hello
Get system health and stats

Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "system": {
        "hostname": "server1",
        "platform": "linux",
        "arch": "x64",
        "cpus": 8,
        "uptime_hours": 720,
        "memory": {
            "total_gb": 32.0,
            "used_gb": 16.5,
            "free_gb": 15.5,
            "usage_percent": 51
        }
    },
    "process": {
        "pid": 12345,
        "node_version": "v18.0.0",
        "uptime_hours": 48,
        "memory_mb": 256.5
    }
}
