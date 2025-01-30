# TR-Engine Collection Design

## Overview
TR-Engine stores all MQTT messages in MongoDB collections based on the message `type` field. Messages are stored with flattened structure, removing the type wrapper but preserving all data.

## Collection Name Resolution

### Priority Order
1. Use message `type` field if present
2. Fall back to last segment of MQTT topic path if type not found
   - Example: `tr-mqtt/main/systems/newlogstuff` → collection name: `newlogstuff`
   - Example: `tr-mqtt/units/butco2/call` → collection name: `call`

### Special Cases
- If neither type nor meaningful topic segment exists, log error and store in `unclassified` collection
- If topic ends in '#' or '+' (wildcards), use the parent segment

## Message Transformation Rules

### Flattening Strategy
1. On Initial Ingest:
   - Flatten all nested objects to a single level
   - Preserve arrays in their original structure
   - Remove type wrappers
   - Promote top-level fields

2. Post-Processing:
   - Maintain minimal nesting
   - Keep arrays in their original form
   - Allow limited nesting for specific use cases (e.g., transcription segments)

### Example Transformation
Original Message:
```javascript
{
  type: "call_start",
  call: {
    id: "9131_1737430014",
    metadata: {
      frequency: 851350000,
      emergency: false
    },
    units: [976109, 976110]
  }
}
```

Transformed Message:
```javascript
{
  id: "9131_1737430014",
  frequency: 851350000,
  emergency: false,
  units: [976109, 976110],  // Array preserved
  timestamp: 1737430014,
  _processed_at: 1737430015
}
```

### Standard Messages
For all message types except audio:

1. Use `type` field to determine collection name
2. Remove type wrapper
3. Promote top-level fields (timestamp, instance_id)
4. Store the complete inner object plus promoted fields

Example:
```javascript
// Original MQTT Message
{
  "type": "call_start",
  "call": {
    "id": "1_9131_1737430014",
    "call_num": 1571,
    // ... call data
  },
  "timestamp": 1737430019,
  "instance_id": "trunk-recorder"
}

// Stored in 'call_start' collection as:
{
  "id": "1_9131_1737430014",
  "call_num": 1571,
  // ... call data
  "timestamp": 1737430019,
  "instance_id": "trunk-recorder"
}
```

### Audio Messages
Audio messages require special handling due to their binary content and metadata structure.

### Transcription Storage
Transcriptions are stored in two locations:

1. Transcriptions Collection
```javascript
{
  call_id: "9131_1737430014",
  text: "1469 1 with 10.30",
  segments: [
    {
      start_time: 1.18,
      end_time: 18.16,
      text: "1469 1 with 10.30",
      confidence: 0.85
    }
  ],
  metadata: {
    model: "faster-whisper-base.en",
    processing_time: 1.226
  }
}
```

2. Audio Collection (Referenced)
```javascript
{
  call_id: "9131_1737430014",
  // ... other audio metadata
  transcription: {
    id: "trans_12345",  // Reference to transcription document
    text: "1469 1 with 10.30",  // Cached copy of text
    completed_at: 1737430020
  }
}
```

1. Metadata Storage (audio collection):
```javascript
{
  // Promoted metadata fields
  freq: Number,
  freq_error: Number,
  signal: Number,
  noise: Number,
  source_num: Number,
  recorder_num: Number,
  tdma_slot: Number,
  phase2_tdma: Number,
  start_time: Number,
  stop_time: Number,
  emergency: Number,
  priority: Number,
  mode: Number,
  duplex: Number,
  encrypted: Number,
  call_length: Number,
  talkgroup: Number,
  talkgroup_tag: String,
  talkgroup_description: String,
  talkgroup_group_tag: String,
  talkgroup_group: String,
  audio_type: String,
  short_name: String,
  
  // Frequency and source lists
  freqList: [{
    freq: Number,
    time: Number,
    pos: Number,
    len: Number,
    error_count: Number,
    spike_count: Number
  }],
  
  srcList: [{
    src: Number,
    time: Number,
    pos: Number,
    emergency: Number,
    signal_system: String,
    tag: String
  }],
  
  // Original message fields
  filename: String,
  timestamp: Number,
  instance_id: String
}
```

2. Audio Storage (GridFS calls collection):
```javascript
{
  filename: String,  // Original filename
  metadata: {
    talkgroup: Number,
    talkgroup_tag: String,
    start_time: Number,
    stop_time: Number,
    call_length: Number,
    emergency: Boolean,  // Converted from 0/1
    encrypted: Boolean,  // Converted from 0/1
    freq: Number,
    instance_id: String
  },
  // Binary audio data stored in chunks
}
```

## Collection Structure

### Message Collections
Collections are created based on message type or topic fallback:

#### Standard Collections (from type field)
- `call_start` - Call initiation events
- `call_end` - Call termination events
- `calls_active` - Active calls list
- `audio` - Call audio metadata
- `config` - System configuration
- `recorder` - Recorder status
- `systems` - System information
- `rates` - System performance data
- `call` - Unit call events
- `data` - Unit data
- `join` - Unit join events
- `location` - Unit location updates
- `on` - Unit online events
- `off` - Unit offline events
- `ackresp` - Unit acknowledgments

#### Fallback Collections
- `unclassified` - Messages without type or clear topic segment
- Dynamic collections based on topic segments when type not found

### GridFS Collection
- `calls` - Audio files and metadata

### State Collections
- `activeCalls` - Current call states
- `systemStates` - Current system states
- `unitStates` - Current unit states
- `talkgroups` - Talkgroup configurations

### Time Series Collections
- `systemMetrics` - Performance metrics [time-series]
- `unitActivity` - Unit activity history [time-series]

## Indexes

### Standard Collections
```javascript
{
  timestamp: 1,
  instance_id: 1,
  sys_name: 1    // Where applicable
}
```

### Audio Collection
```javascript
{
  talkgroup: 1,
  start_time: 1,
  filename: 1
}
```

### GridFS Calls Collection
```javascript
{
  'metadata.talkgroup': 1,
  'metadata.start_time': 1,
  filename: 1
}
```

## Implementation Notes

### Message Processing
1. Extract `type` field from incoming message
2. Determine target collection
3. Transform message according to rules:
   - Standard: unwrap and promote fields
   - Audio: split into metadata and binary storage

### Audio Processing
1. Store metadata in `audio` collection
2. Convert audio_wav_base64 to binary
3. Store in GridFS with metadata
4. Convert boolean fields for GridFS metadata

### Error Handling
1. Collection Name Resolution
   - Check for `type` field first
   - Parse MQTT topic if type not found
   - Extract last meaningful segment
   - Fall back to `unclassified` if no valid name found
2. Message Structure
2. Ensure required fields present
3. Handle malformed messages gracefully
4. Log processing errors

### Performance Considerations
1. Efficient indexes for common queries
2. Monitoring of collection sizes
3. Archival strategy for old messages
4. GridFS chunk size optimization

## Migration Notes
When migrating from existing collections:
1. Preserve all original data
2. Convert nested structures to flat
3. Ensure proper field promotion
4. Validate data integrity
5. Keep original messages until verified
