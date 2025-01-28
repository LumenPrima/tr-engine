# TR-ENGINE API Documentation

## Base URL
All API endpoints are relative to: `/api/v1`

## Using Parameters

### Time-based Queries
Time parameters can be specified in several ways:
1. Unix timestamp (seconds since epoch)
2. ISO 8601 date string
3. Relative time using 'now' keyword (planned feature)

Example:
```
# Get calls from last hour
GET /calls?start=1737429985&end=1737430027

# Get units active in last 5 minutes
GET /units/active?window=5
```

### Filtering
Most endpoints support multiple filters that can be combined:
```
# Get emergency calls for a specific talkgroup
GET /calls?talkgroup=9468&emergency=true

# Get units in a specific system and talkgroup
GET /units?sys_name=butco2&talkgroup=9131
```

### Pagination
For endpoints returning lists, use limit and offset:
```
# Get first 10 results
GET /calls?limit=10&offset=0

# Get next 10 results
GET /calls?limit=10&offset=10
```

## Common Response Format

Success Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        // Endpoint-specific data
    }
}
```

Error Response:
```json
{
    "status": "error",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "message": "Error description"
}
```

## Detailed API Examples

### Audio Management Examples

#### Complex Audio Search
Find encrypted emergency calls from specific time period:
```
GET /audio/archive?start=1737429985&end=1737430027&encrypted=true&emergency=true
```

Find all calls for a specific unit during their shift:
```
GET /audio/archive?unit=976109&sys_name=butco2&start=1737429985&limit=50
```

Get calls with transcriptions for analysis:
```
GET /audio/archive?talkgroup=9131&has_transcription=true&limit=20
```

#### Audio Format Selection
Get WAV format for specific call:
```
GET /audio/call/9131-1737430014?format=wav
```

Get M4A format with metadata:
```
GET /audio/call/9131-1737430014?format=m4a&include_metadata=true
```

### Active Call Examples



#### GET /calls/active
Get currently active calls.

Example Request:
```
GET /calls/active?sys_name=butco2
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "count": 1,
        "calls": [
            {
                "call_id": "9468_1737429985",
                "sys_name": "butco2",
                "talkgroup": 9468,
                "talkgroup_tag": "09 TA 02",
                "talkgroup_description": "County Regional Transit 2",
                "talkgroup_group": "Butler County (09) Local Government/Public Works",
                "emergency": false,
                "units": [984844],
                "start_time": 1737429985,
                "active": true
            }
        ]
    }
}
```

#### GET /calls/talkgroup/{talkgroup_id}
Get historical activity for a specific talkgroup.

Example Request:
```
GET /calls/talkgroup/9131?limit=2
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "talkgroup": {
            "talkgroup": 9131,
            "talkgroup_tag": "09 WC HOSP SEC"
        },
        "time_range": {
            "start": "2025-01-28T19:19:47.000Z",
            "end": "2025-01-28T20:19:47.000Z"
        },
        "count": 2,
        "events": [
            {
                "call_id": "9131-1737430014",
                "timestamp": "2025-01-28T20:19:03.000Z",
                "activity_type": "call",
                "units": [976109],
                "emergency": false,
                "duration": 8,
                "audio_type": "digital",
                "freq": 851350000,
                "encrypted": false
            },
            {
                "timestamp": "2025-01-28T20:18:47.000Z",
                "activity_type": "join",
                "unit": 976109,
                "unit_alpha_tag": "",
                "talkgroup": 9131,
                "talkgroup_tag": "09 WC HOSP SEC"
            }
        ]
    }
}
```

### Audio Management

#### GET /audio/call/{call_id}
Get audio recording for a specific call.

Example Request:
```
GET /audio/call/9131-1737430014?format=m4a
```

The response will be an audio file stream with appropriate content-type headers.

#### GET /audio/archive
Search archived audio recordings.

Example Request:
```
GET /audio/archive?talkgroup=9131&start=1737430014&emergency=false&limit=2
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "pagination": {
            "total": 123,
            "limit": 2,
            "offset": 0,
            "has_more": true
        },
        "time_range": {
            "start": "2025-01-28T19:19:47.000Z",
            "end": "2025-01-28T20:19:47.000Z"
        },
        "count": 2,
        "files": [
            {
                "id": "9131-1737430014",
                "filename": "9131-1737430014_851350000.0-call_1571.wav",
                "timestamp": "2025-01-28T20:19:47.000Z",
                "talkgroup": 9131,
                "talkgroup_tag": "09 WC HOSP SEC",
                "talkgroup_description": "UC Health West Chester Hospital - Security",
                "talkgroup_group": "Butler County (09) Fire/EMS/Hospitals",
                "start_time": 1737430014,
                "stop_time": 1737430023,
                "call_length": 8,
                "emergency": false,
                "encrypted": false,
                "freq": 851350000,
                "audio_type": "digital"
            }
        ]
    }
}
```

### System Status

#### GET /systems
Get status of all systems.

Example Request:
```
GET /systems
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "count": 2,
    "systems": [
        {
            "name": "butco2",
            "sys_name": "butco2",
            "sys_num": 1,
            "type": "p25",
            "status": {
                "connected": true,
                "last_seen": "2025-01-28T20:19:47.000Z",
                "current_control_channel": 853062500,
                "current_decoderate": 25.67
            },
            "config": {
                "control_channels": [853062500, 853037500, 853287500, 853537500]
            }
        }
    ]
}
```

### Advanced Unit Tracking Examples

#### Active Units with Complex Filters
Get all units in a specific talkgroup that have been active in last 5 minutes:
```
GET /units/active?window=5&talkgroup=9131&sys_name=butco2
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "count": 2,
        "units": [
            {
                "unit": 976109,
                "sys_name": "butco2",
                "unit_alpha_tag": "",
                "status": {
                    "online": true,
                    "last_seen": "2025-01-28T20:19:47.000Z",
                    "current_talkgroup": 9131,
                    "current_talkgroup_tag": "09 WC HOSP SEC"
                }
            },
            {
                "unit": 976001,
                "sys_name": "butco2",
                "unit_alpha_tag": "",
                "status": {
                    "online": true,
                    "last_seen": "2025-01-28T20:19:40.000Z",
                    "current_talkgroup": 9131,
                    "current_talkgroup_tag": "09 WC HOSP SEC"
                }
            }
        ]
    }
}
```

#### Unit History Search
Get unit's activity across multiple talkgroups:
```
GET /units/976109/history?start=1737429985&end=1737430027&include_calls=true
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "unit": 976109,
        "history": [
            {
                "timestamp": "2025-01-28T20:19:47.000Z",
                "activity_type": "call",
                "talkgroup": 9131,
                "talkgroup_tag": "09 WC HOSP SEC",
                "freq": 851350000,
                "emergency": false
            },
            {
                "timestamp": "2025-01-28T20:19:40.000Z",
                "activity_type": "join",
                "talkgroup": 9131,
                "talkgroup_tag": "09 WC HOSP SEC"
            }
        ]
    }
}
```

#### Units in Talkgroup with Activity
Get all units in a talkgroup with their recent activity:
```
GET /units/talkgroup/9131?include_recent=true&window=10
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "talkgroup": 9131,
        "talkgroup_tag": "09 WC HOSP SEC",
        "units": [
            {
                "unit": 976109,
                "unit_alpha_tag": "",
                "status": {
                    "online": true,
                    "last_seen": "2025-01-28T20:19:47.000Z"
                },
                "recent_activity": [
                    {
                        "type": "call",
                        "timestamp": "2025-01-28T20:19:47.000Z",
                        "duration": 8
                    }
                ]
            }
        ]
    }
}
```

### System Monitoring Examples


Get currently active units.

Example Request:
```
GET /units/active?window=5&talkgroup=9131
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "pagination": {
            "total": 45,
            "limit": 100,
            "offset": 0,
            "has_more": false
        },
        "count": 1,
        "window": 300,
        "units": [
            {
                "unit": 976109,
                "sys_name": "butco2",
                "unit_alpha_tag": "",
                "status": {
                    "online": true,
                    "last_seen": "2025-01-28T20:19:47.000Z",
                    "current_talkgroup": 9131,
                    "current_talkgroup_tag": "09 WC HOSP SEC"
                }
            }
        ]
    }
}
```

### System Performance Examples

#### Get System Performance Metrics
Get detailed performance stats for all systems:
```
GET /systems/performance
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "total_systems": 2,
        "active_systems": 2,
        "system_stats": [
            {
                "name": "warco2",
                "sys_name": "warco2",
                "type": "p25",
                "control_channel": 859762500,
                "decoderate": 40,
                "decoderate_interval": 3,
                "recent_rates": [
                    {
                        "timestamp": "2025-01-28T20:19:47.000Z",
                        "decoderate": 40,
                        "control_channel": 859762500
                    }
                ]
            },
            {
                "name": "butco2",
                "sys_name": "butco2",
                "type": "p25",
                "control_channel": 853062500,
                "decoderate": 25.67,
                "decoderate_interval": 3,
                "recent_rates": [
                    {
                        "timestamp": "2025-01-28T20:19:47.000Z",
                        "decoderate": 25.67,
                        "control_channel": 853062500
                    }
                ]
            }
        ],
        "aggregate": {
            "average_decoderate": 32.835
        }
    }
}
```

#### Get Detailed System Configuration
Get configuration details for specific system:
```
GET /systems/butco2
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "system": {
        "sys_name": "butco2",
        "sys_num": 1,
        "type": "p25",
        "sysid": "348",
        "wacn": "BEE00",
        "nac": "340",
        "rfss": 4,
        "site_id": 1,
        "status": {
            "connected": true,
            "last_seen": "2025-01-28T20:19:47.000Z"
        },
        "config": {
            "system_type": "p25",
            "control_channels": [
                853062500,
                853037500,
                853287500,
                853537500
            ],
            "digital_levels": 3,
            "audio_archive": true
        }
    }
}
```

### Call Event Examples

#### Get Historical Emergency Calls
Retrieve past emergency calls across all systems:
```
GET /calls?emergency=true&start=1737429985&end=1737430027
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "count": 1,
        "events": {
            "calls": [
                {
                    "call_id": "9468_1737429985",
                    "sys_name": "butco2",
                    "talkgroup": 9468,
                    "talkgroup_tag": "09 TA 02",
                    "emergency": true,
                    "units": [984844],
                    "start_time": 1737429985,
                    "freq": 855737500,
                    "type": "emergency_call"
                }
            ],
            "emergencies": [
                {
                    "call_id": "9468_1737429985",
                    "sys_name": "butco2",
                    "talkgroup": 9468,
                    "talkgroup_tag": "09 TA 02",
                    "units": [984844],
                    "start_time": 1737429985,
                    "type": "emergency_call"
                }
            ]
        }
    }
}
```

#### Get Call History for Multiple Talkgroups
Search calls across related talkgroups:
```
GET /calls?talkgroup_in=9131,9468&start=1737429985&end=1737430027
```

Example Response:
```json
{
    "status": "success",
    "timestamp": "2025-01-28T20:19:47.000Z",
    "data": {
        "pagination": {
            "total": 2,
            "limit": 100,
            "offset": 0,
            "has_more": false
        },
        "time_range": {
            "start": "2025-01-28T20:19:47.000Z",
            "end": "2025-01-28T20:19:47.000Z"
        },
        "count": 2,
        "calls": [
            {
                "call_id": "9131_1737430014",
                "sys_name": "butco2",
                "talkgroup": 9131,
                "talkgroup_tag": "09 WC HOSP SEC",
                "units": [976109],
                "start_time": 1737430014,
                "duration": 8,
                "freq": 851350000
            },
            {
                "call_id": "9468_1737429985",
                "sys_name": "butco2",
                "talkgroup": 9468,
                "talkgroup_tag": "09 TA 02",
                "units": [984844],
                "start_time": 1737429985,
                "duration": 29,
                "freq": 855737500
            }
        ]
    }
}
```

### Common Query Parameters

Most endpoints support these standard parameters:

#### Time Range Parameters
- `start`: Start timestamp (Unix)
- `end`: End timestamp (Unix)
- `window`: Time window in minutes (for active queries)

#### Filtering Parameters
- `sys_name`: System identifier (e.g., "butco2")
- `talkgroup`: Talkgroup number (e.g., 9131)
- `unit`: Unit identifier (e.g., 976109)
- `emergency`: Boolean filter for emergency status
- `encrypted`: Boolean filter for encrypted calls

#### Pagination Parameters
- `limit`: Number of results per page (default 100)
- `offset`: Starting position (default 0)

## Real-Time Monitoring

Real-time monitoring is implemented via WebSocket connections. Do not poll the REST API endpoints for real-time updates.

### WebSocket Topics

Connect to the WebSocket endpoint and subscribe to relevant topics:

```javascript
// Example WebSocket connection
const ws = new WebSocket('ws://your-server/ws');

// Listen for different event types
ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    switch(data.type) {
        case 'call.start':
            // New call started
            // data includes: talkgroup, units, emergency status
            break;
        case 'call.update':
            // Call status update
            // data includes: audio info, unit updates
            break;
        case 'call.end':
            // Call completed
            // data includes: final duration, stats
            break;
        case 'system.status':
            // System status update 
            // data includes: decoderate, control channel
            break;
    }
};
```

The REST API should be used for:
- Historical data queries
- Configuration management
- Audio file access
- System administration
- Report generation

## Tips and Best Practices

1. **Use Filtering Efficiently**
   - Combine multiple filters to narrow results
   - Always include time bounds for historical queries
   - Use system name when known to reduce response size

2. **Handle Pagination**
   - Check has_more flag for additional results
   - Use reasonable page sizes (10-100 items)
   - Include total count in UI for user feedback

3. **Error Handling**
   - Check status field in all responses
   - Handle 404 for missing resources
   - Implement proper retry logic for 5xx errors

4. **WebSocket Best Practices**
   - Implement reconnection logic
   - Handle connection state changes
   - Buffer messages during reconnection
   - Consider implementing heartbeat mechanism