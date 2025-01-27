# Multi-System Call Handling Specification

## Overview
This document specifies how TR Engine handles calls and audio recordings that are received from multiple systems for the same talkgroup, ensuring proper data aggregation and quality management.

## Call Identification

### Call ID Format
Calls maintain system-agnostic identification:
- Simple format: `{talkgroup}-{timestamp}`
  Example: "9131-1737430014"
- Full format: `{talkgroup}-{timestamp}_{frequency}-call_{id}.{ext}`
  Example: "9131-1737430014_851350000.0-call_1571.wav"

### Call Uniqueness
- Calls are considered identical if they share the same talkgroup and timestamp
- System-specific identifiers are tracked in metadata but don't affect call identification
- Frequency differences between systems are noted but don't create separate call records

## Data Management

### Data Storage
- All MQTT messages are stored in their original form for complete system record
- Each system's audio recording is stored in GridFS
- Quality metrics are calculated and stored with each recording:
  - Duration of recording
  - Audio quality metrics (if available)
  - Signal strength/clarity
- Best quality version is marked as primary recording for playback
- All recordings are retained for archival purposes
- GridFS metadata includes recording sources and quality metrics

### Metadata Handling
```json
{
  "call_id": "9131-1737430014",
  "talkgroup": 9131,
  "talkgroup_tag": "09 WC HOSP SEC",
  "start_time": 1737430014,
  "duration": 8.12,
  "emergency": false,
  "recording_systems": [
    {
      "sys_name": "butco2",
      "frequency": 851350000.0,
      "signal_quality": 0.95,
      "duration": 8.12
    },
    {
      "sys_name": "warco2",
      "frequency": 851350000.0,
      "signal_quality": 0.87,
      "duration": 7.95
    }
  ],
  "primary_recording": "butco2"
}
```

## Implementation Details

### MQTT Processing
When receiving call messages:
1. Store complete MQTT message in raw message collection
2. Extract call identifier (talkgroup + timestamp)
3. Check for existing call record
4. If call exists:
   - Store new system's audio recording
   - Compare audio quality metrics
   - Update metadata with additional system information
   - Update primary recording designation if new version is higher quality
5. If new call:
   - Create call record with initial system information
   - Store audio file
   - Initialize quality metrics
   - Set as primary recording

### Quality Assessment
Audio quality is determined by:
1. Recording duration (longer is better)
2. Signal strength from MQTT metadata
3. Audio quality metrics if available
4. System priority settings (configurable)

### API Behavior

#### GET /audio/call/{call_id}
- Returns highest quality audio recording
- Query params can specify preferred system
- Response includes recording source in headers

#### GET /audio/call/{call_id}/metadata
Returns comprehensive metadata including:
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
        "recording_systems": [
          {
            "sys_name": "butco2",
            "quality_metrics": {
              "signal_strength": 0.95,
              "duration": 8.12
            }
          },
          {
            "sys_name": "warco2",
            "quality_metrics": {
              "signal_strength": 0.87,
              "duration": 7.95
            }
          }
        ],
        "primary_recording": "butco2"
      }
    }
  }
}
```

#### GET /calls Endpoints
- Return unified call records
- Include recording system information
- Provide quality metrics when available

### Error Handling
- Handle system-specific recording failures
- Track missing data from individual systems
- Log quality differentials between recordings
- Alert on significant quality discrepancies

## Configuration Options
```json
{
  "audio": {
    "retention": {
      "raw_message_retention_days": 90,
      "quality_threshold": 0.15
    },
    "quality_metrics": {
      "duration_weight": 0.4,
      "signal_weight": 0.6
    },
    "system_priorities": {
      "butco2": 1,
      "warco2": 1
    }
  }
}
```

## Future Considerations
- Implement advanced audio quality analysis
- Add support for audio mixing from multiple sources
- Develop system health monitoring based on recording quality
- Create quality trend analysis for system performance
