# AI-Enhanced Radio Monitoring Features

## Overview
This document specifies AI-powered features for enhancing radio monitoring capabilities through transcription, sentiment analysis, and real-time audio streaming.

## Voice Transcription

### Storage Schema
```json
{
  "call_id": "9131-1737430014",
  "transcription": {
    "text": "Engine 5 responding to Box 1234",
    "segments": [
      {
        "start_time": 0.0,
        "end_time": 1.2,
        "text": "Engine 5",
        "confidence": 0.95
      },
      {
        "start_time": 1.2,
        "end_time": 2.1,
        "text": "responding to",
        "confidence": 0.98
      },
      {
        "start_time": 2.1,
        "end_time": 3.4,
        "text": "Box 1234",
        "confidence": 0.92
      }
    ],
    "metadata": {
      "model": "whisper-large-v3",
      "processing_time": 2.3,
      "audio_duration": 3.4
    }
  }
}
```

### Processing Pipeline
1. Audio Recording Complete
   - Trigger transcription job
   - Store segments as they're processed
   - Update call metadata with transcription

2. Error Handling
   - Retry failed transcriptions
   - Flag low confidence segments
   - Store processing errors

## Call Sentiment Analysis

### Schema
```json
{
  "call_id": "9131-1737430014",
  "sentiment": {
    "analysis": {
      "summary": "Routine fire response to residential alarm",
      "urgency_level": "normal",
      "key_points": [
        "Initial dispatch",
        "Single unit response",
        "No signs of emergency"
      ]
    },
    "related_calls": [
      {
        "call_id": "9132-1737430020",
        "relationship": "same_incident",
        "talkgroup": "PD Dispatch",
        "relevance_score": 0.85
      }
    ],
    "metadata": {
      "model": "gpt-4",
      "processing_time": 1.2,
      "confidence": 0.88
    }
  }
}
```

### Processing Pipeline
1. Call Completion
   - Gather transcriptions and metadata
   - Identify related calls
   - Generate sentiment analysis

2. Related Call Analysis
   - Time-based correlation
   - Geographic correlation
   - Unit/talkgroup relationships

## Talkgroup Sentiment Analysis

### Schema
```json
{
  "talkgroup_id": 9131,
  "sentiment": {
    "current_status": "urgent_situation",
    "status_description": "Multiple units responding to major incident",
    "contributing_factors": [
      {
        "type": "call_volume",
        "description": "Increased radio traffic",
        "weight": 0.7
      },
      {
        "type": "emergency_markers",
        "description": "Multiple emergency flags",
        "weight": 0.9
      }
    ],
    "related_talkgroups": [
      {
        "talkgroup_id": 9132,
        "relationship": "coordinated_response",
        "activity_correlation": 0.75
      }
    ],
    "timestamp": "2024-01-27T15:45:00Z",
    "valid_until": "2024-01-27T16:00:00Z"
  }
}
```

### Analysis Triggers
1. Time-based Updates
   - Regular interval updates (e.g., every 15 minutes)
   - Activity threshold triggers
   - Emergency event triggers

2. Context Sources
   - Recent call transcriptions
   - Call volume patterns
   - Emergency markers
   - Unit activity

## Real-Time Audio Streaming

### WebSocket Events

#### Subscribe to Talkgroups
```json
{
  "type": "subscribe",
  "talkgroups": [9131, 9132],
  "format": "m4a",
  "options": {
    "include_metadata": true,
    "include_transcription": true
  }
}
```

#### Audio Stream Event
```json
{
  "type": "audio_stream",
  "call_id": "9131-1737430014",
  "talkgroup": 9131,
  "chunk": {
    "sequence": 1,
    "timestamp": "2024-01-27T15:45:00.123Z",
    "duration": 0.5,
    "format": "m4a",
    "data": "base64_encoded_audio_chunk"
  },
  "metadata": {
    "unit": 104,
    "emergency": false,
    "live_transcription": "Engine 5"
  }
}
```

### Stream Management
1. Client Connections
   - Connection authentication
   - Talkgroup subscription management
   - Client resource monitoring

2. Audio Delivery
   - Chunk size optimization
   - Format conversion
   - Metadata inclusion
   - Optional live transcription

## API Endpoints

### Transcription Endpoints
- GET /api/v1/calls/{call_id}/transcription
- GET /api/v1/talkgroups/{talkgroup_id}/recent_transcriptions

### Sentiment Endpoints
- GET /api/v1/calls/{call_id}/sentiment
- GET /api/v1/talkgroups/{talkgroup_id}/sentiment
- GET /api/v1/talkgroups/{talkgroup_id}/related_activity

### WebSocket Endpoints
- WS /api/v1/stream/audio
- WS /api/v1/stream/transcriptions

## Implementation Considerations

### Performance
- Async transcription processing
- Caching of sentiment analysis
- Efficient audio streaming
- Resource usage monitoring

### Accuracy
- Model selection and tuning
- Domain-specific training
- Error rate monitoring
- Confidence thresholds

### Storage
- Transcription retention policy
- Sentiment history retention
- Audio chunk caching
- Index optimization

### Privacy
- PII detection and handling
- Access control
- Data retention policies
- Audit logging
