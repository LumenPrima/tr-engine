# AI-Enhanced Radio Monitoring Features

## Overview
This document specifies AI-powered features for enhancing radio monitoring capabilities through transcription, sentiment analysis, and real-time audio streaming.

## Voice Transcription

### Storage Schema
Transcriptions are stored in the AudioMessage collection as part of the call metadata:

```json
{
  "payload": {
    "call": {
      "metadata": {
        "filename": "51514-1737999290_856762500.0-call_6008.wav",
        "transcription": {
          "text": "1469 1 with 10.30 13.69 find the scales with Ohio 4",
          "segments": [
            {
              "start_time": 1.18,
              "end_time": 18.16,
              "text": "1469 1 with 10.30 13.69 find the scales with Ohio 4",
              "confidence": 0.85,
              "source": {
                "unit": "9001877",
                "emergency": false,
                "signal_system": "",
                "tag": ""
              }
            }
          ],
          "metadata": {
            "model": "guillaumekln/faster-whisper-base.en",
            "processing_time": 1.226,
            "audio_duration": 18.18,
            "timestamp": "2025-01-27T17:35:23.270Z"
          }
        }
      }
    }
  }
}
```

### Processing Pipeline
1. Audio Recording Complete
   - Audio file is stored in GridFS
   - Transcription job is triggered automatically
   - WAV file is validated (header, format, duration)
   - Transcription is processed using local OpenAI-compatible endpoint
   - Results are stored in AudioMessage collection

2. Error Handling
   - Automatic retries (up to 3 attempts) for failed transcriptions
   - Quality assessment of transcription results:
     - Minimum confidence threshold: 0.6
     - Minimum duration threshold: 0.5s
     - Text length validation
   - Failed transcriptions are logged but don't block audio storage

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
- GET /api/v1/transcription/calls/{call_id}/transcription
  - Get transcription for a specific call
  - Returns full transcription with segments and metadata
- GET /api/v1/transcription/talkgroups/{talkgroup_id}/recent_transcriptions
  - Get recent transcriptions for a talkgroup
  - Supports pagination and date range filtering
- GET /api/v1/transcription/stats
  - Get aggregate transcription statistics
  - Groups by talkgroup with metrics like:
    - Average confidence
    - Average duration
    - Average processing time
    - Emergency call counts

### Sentiment Endpoints
- GET /api/v1/calls/{call_id}/sentiment
- GET /api/v1/talkgroups/{talkgroup_id}/sentiment
- GET /api/v1/talkgroups/{talkgroup_id}/related_activity

### WebSocket Endpoints
- WS /api/v1/stream/audio
- WS /api/v1/stream/transcriptions

## Implementation Considerations

### Performance
- Async transcription processing integrated with audio storage
- Efficient MongoDB indexing on transcription fields
- Optimized queries using MongoDB aggregation pipeline
- Automatic cleanup of temporary audio files

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
