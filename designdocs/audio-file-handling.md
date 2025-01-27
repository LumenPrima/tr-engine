# Audio File Handling Specification

## Overview
This document specifies how audio files are handled in the TR Engine system, including storage, naming conventions, and API access patterns.

## File Storage

### Storage System
- Audio files are stored in MongoDB GridFS
- Both WAV and M4A formats are supported
- Original filenames from MQTT messages are preserved
- Metadata is stored alongside each file

### Filename Format
Files follow the format: `{talkgroup}-{timestamp}_{frequency}-call_{id}.{ext}`

Example: `9131-1737430014_851350000.0-call_1571.wav`

Components:
- talkgroup: Numeric talkgroup ID (e.g. "9131")
- timestamp: Unix timestamp (e.g. "1737430014")
- frequency: Frequency in Hz with decimal (e.g. "851350000.0")
- call_id: Sequential call identifier (e.g. "1571")
- ext: File extension ("wav" or "m4a")

### File Metadata
Each stored file includes metadata:
```json
{
  "talkgroup": 9131,
  "talkgroup_tag": "09 WC HOSP SEC",
  "start_time": 1737430014,
  "duration": 8.12,
  "emergency": false
}
```

## API Endpoints

### GET /api/v1/audio/call/{call_id}
Retrieves audio file for a specific call.

Call ID can be specified in two formats:
1. Simple format: `{talkgroup}-{timestamp}`
   - Example: "9131-1737430014"
   - Returns any matching audio file for that call
   
2. Full filename: `{talkgroup}-{timestamp}_{frequency}-call_{id}.{ext}`
   - Example: "9131-1737430014_851350000.0-call_1571.wav"
   - Returns exact audio file matching the filename

Query Parameters:
- format: (optional) Preferred audio format ("wav" or "m4a")
  - If specified, returns file in requested format if available
  - If not specified, prefers m4a over wav

Response Headers:
```
Content-Type: audio/wav or audio/mp4
Content-Length: <file size in bytes>
Content-Disposition: attachment; filename="<original filename>"
Accept-Ranges: bytes
```

### GET /api/v1/audio/call/{call_id}/metadata
Returns metadata about available audio files for a call.

Response Format:
```json
{
  "status": "success",
  "timestamp": "2025-01-27T14:45:59.000Z",
  "metadata": {
    "call_id": "9131-1737430014",
    "formats": {
      "wav": {
        "filename": "9131-1737430014_851350000.0-call_1571.wav",
        "length": 123456,
        "upload_date": "2025-01-27T14:45:59.000Z",
        "md5": "abc123...",
    "metadata": {
      "talkgroup": 9131,
      "talkgroup_tag": "09 WC HOSP SEC",
      "start_time": 1737430014,
      "duration": 8.12,
      "emergency": false,
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
      },
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
      },
      "m4a": {
        // Similar structure for m4a format if available
      }
    }
  }
}
```

### DELETE /api/v1/audio/call/{call_id}
Deletes all audio files associated with a call.

### GET /api/v1/audio/archive
Search archived audio recordings with filtering and pagination.

Query Parameters:
- limit: Maximum number of results (default: 100)
- offset: Pagination offset (default: 0)
- start: Start time for search range (default: 24 hours ago)
- end: End time for search range (default: now)
- unit: Filter by unit ID
- talkgroup: Filter by talkgroup ID
- sys_name: Filter by system name
- format: Filter by audio format

Response Format:
```json
{
  "status": "success",
  "timestamp": "2025-01-27T14:45:59.000Z",
  "data": {
    "pagination": {
      "total": 1234,
      "limit": 100,
      "offset": 0,
      "has_more": true
    },
    "time_range": {
      "start": "2025-01-26T14:45:59.000Z",
      "end": "2025-01-27T14:45:59.000Z"
    },
    "count": 100,
    "files": [
      {
        "id": "mongodb_file_id",
        "call_id": "9131-1737430014_851350000.0-call_1571",
        "format": "wav",
        "timestamp": "2025-01-27T14:45:59.000Z",
        "size": 123456,
        "metadata": {
          "talkgroup": 9131,
          "talkgroup_tag": "09 WC HOSP SEC",
          "duration": 8.12,
          "emergency": false
        }
      }
      // ... more files
    ]
  }
}
```

## Implementation Details

### MQTT Processing
When audio messages are received:
1. Extract WAV/M4A data and metadata from MQTT message
2. Store original message in AudioMessage collection
3. Store audio files in GridFS using original filename
4. For M4A files, use same filename pattern but change extension
5. Include metadata with each stored file
6. Process transcription:
   - Validate WAV file format and duration
   - Send to local OpenAI-compatible endpoint
   - Store transcription in AudioMessage metadata
   - Map transcription segments to source units
   - Retry up to 3 times on failure

### File Retrieval
When searching for files:
1. If full filename provided, do exact match
2. If simple format (talkgroup-timestamp), match start of filename
3. Return files in requested format if specified
4. Prefer M4A over WAV if no format specified

### Error Handling
- 404 if no matching files found
- 400 if invalid call ID format
- 500 for storage/retrieval errors
- All errors include descriptive messages
