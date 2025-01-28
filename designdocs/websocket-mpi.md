# WebSocket Message Protocol Interface (MPI)

This document defines the WebSocket Message Protocol Interface (MPI) for real-time communication between the client and server.

## Message Format

All messages follow this basic structure:
```javascript
{
  type: string,     // Message type identifier
  timestamp: string, // ISO timestamp
  data?: any,       // Optional payload
  error?: string    // Error message if applicable
}
```

## Client -> Server Messages

### Subscription Management

#### Subscribe to Events
```javascript
{
  type: 'subscribe',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    events: [
      'calls',
      'units', 
      'systems',
      'audio',
      'transcription'
    ]
  }
}
```

#### Unsubscribe from Events
```javascript
{
  type: 'unsubscribe',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    events: ['calls'] // Array of event types to unsubscribe from
  }
}
```

### Audio Channel Management

#### Subscribe to Audio
```javascript
{
  type: 'audio.subscribe',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    talkgroups: number[],  // Array of talkgroup IDs to subscribe to
    format?: 'wav' | 'm4a', // Preferred audio format
    options?: {
      emergencyOnly?: boolean,
      includeMetadata?: boolean
    }
  }
}
```

#### Unsubscribe from Audio
```javascript
{
  type: 'audio.unsubscribe',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    talkgroups?: number[] // Specific talkgroups to unsubscribe from, or all if not specified
  }
}
```

### Data Requests

#### Request Specific Data
```javascript
{
  type: 'request',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    resource: 'active_calls' | 'system_status' | 'unit_status',
    filters?: {
      system?: string,
      emergency?: boolean,
      talkgroup?: string
    }
  }
}
```

#### Request Transcription
```javascript
{
  type: 'transcription.request',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    callId: string,
    talkgroupId?: number
  }
}
```

#### Request Transcription Statistics
```javascript
{
  type: 'transcription.stats.request',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    talkgroupId?: number,
    startDate?: string,
    endDate?: string
  }
}
```

## Server -> Client Messages

### State Updates

#### Initial State
Sent when client first connects:
```javascript
{
  type: 'initial.state',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    calls: [],    // Active calls
    systems: [],  // Active systems
    units: []     // Active units
  }
}
```

#### State Update
```javascript
{
  type: 'state.update',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    resource: 'calls' | 'units' | 'systems',
    action: 'add' | 'update' | 'remove',
    payload: {} // The actual data
  }
}
```

### Audio Updates

#### New Audio Available
```javascript
{
  type: 'audio.new',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    callId: string,
    talkgroup: number,
    audioData: string,     // Base64 encoded audio data
    format: 'wav' | 'm4a', // Audio format
    metadata: {
      filename: string,
      talkgroup_tag: string,
      talkgroup_description: string,
      start_time: number,
      stop_time: number,
      call_length: number,
      emergency: boolean,
      encrypted: boolean,
      freq: number,
      srcList: Array<{
        src: string,
        emergency: boolean,
        signal_system: string,
        tag: string,
        pos: number
      }>,
      audio_type: string,
      short_name: string
    }
  }
}
```

#### Audio Processing Status
```javascript
{
  type: 'audio.update',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    callId: string,
    status: 'processing' | 'ready' | 'error',
    filename?: string,
    duration?: number,
    error?: string
  }
}
```

### Transcription Updates

#### Transcription Status Update
```javascript
{
  type: 'transcription.update',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    callId: string,
    status: 'started' | 'completed' | 'failed',
    text?: string,
    segments?: Array<{
      start_time: number,
      end_time: number,
      text: string,
      confidence: number,
      source?: {
        unit: string,
        emergency: boolean,
        signal_system: string,
        tag: string
      }
    }>,
    error?: string
  }
}
```

#### Transcription Statistics
```javascript
{
  type: 'transcription.stats',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    stats: Array<{
      talkgroup: number,
      count: number,
      avg_confidence: number,
      avg_duration: number,
      avg_processing_time: number,
      emergency_count: number
    }>
  }
}
```

### Response Messages

#### Generic Response
```javascript
{
  type: 'response',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    requestId: string, // To match with request
    resource: string,  // Resource type
    payload: {}       // The requested data
  }
}
```

### Error Messages

#### Error Response
```javascript
{
  type: 'error',
  timestamp: '2025-01-28T22:58:57Z',
  error: 'Error message',
  data: {
    code: string,    // Error code
    details?: any    // Additional error details
  }
}
```

### Connection Status

#### Connection Status Update
```javascript
{
  type: 'connection.status',
  timestamp: '2025-01-28T22:58:57Z',
  data: {
    status: 'connected' | 'disconnected' | 'reconnecting',
    subscriptions?: string[] // Current active subscriptions
  }
}
```

### Keep-Alive

#### Heartbeat
Sent in both directions to keep connection alive:
```javascript
{
  type: 'heartbeat',
  timestamp: '2025-01-28T22:58:57Z'
}
```

## Implementation Notes

1. All timestamps are in ISO 8601 format
2. Audio data is Base64 encoded
3. Subscriptions are maintained per-client
4. Error handling includes both generic errors and resource-specific errors
5. Heartbeat messages should be sent every 30 seconds
6. Clients should implement reconnection logic with exponential backoff
7. Server maintains subscription state for each client
8. Audio subscriptions can be filtered by talkgroup and emergency status
9. Transcription updates are sent in real-time as processing occurs
10. State updates are sent only for changes, after initial state
