# Message Transformation Strategy for MQTT Message Processing

## Overview

A simple, consistent approach to flattening MQTT messages while preserving arrays and handling large binary data efficiently.

## Core Principles

1. **Simple Flattening**
   - Flatten all nested objects to a single level
   - Keep arrays intact under their original keys
   - No special handling of message types
   - No complex transformations

2. **Binary Data Handling**
   - Store any base64 fields in GridFS first
   - Remove base64 fields before MongoDB storage
   - Use filename from metadata or timestamp-based default

3. **Minimal Metadata**
   - Add `_processed_at` timestamp
   - Add `_mqtt_received_at` timestamp

## Implementation

### Message Flattening
```javascript
flattenObject(obj) {
    if (!obj || typeof obj !== 'object') return obj;
    if (Array.isArray(obj)) return obj;
    
    return Object.entries(obj).reduce((acc, [key, value]) => {
        if (Array.isArray(value)) {
            acc[key] = value;
        } else if (value && typeof value === 'object') {
            Object.assign(acc, this.flattenObject(value));
        } else {
            acc[key] = value;
        }
        return acc;
    }, {});
}
```

### Message Processing Flow
1. Flatten the entire message
2. For audio messages:
   - Store any base64 fields in GridFS
   - Remove base64 fields from message
3. Store processed message in appropriate collection

## Example Transformations

### Standard Message
```javascript
// Input
{
  type: "systems",
  systems: [
    {
      sys_num: 0,
      config: {
        name: "warren",
        type: "p25"
      }
    }
  ]
}

// Output
{
  type: "systems",
  sys_num: 0,
  name: "warren",
  type: "p25",
  systems: [
    {
      sys_num: 0,
      config: {
        name: "warren",
        type: "p25"
      }
    }
  ],
  _processed_at: Date,
  _mqtt_received_at: Date
}
```

### Audio Message with Base64
```javascript
// Input
{
  type: "audio",
  call: {
    audio_wav_base64: "...",
    metadata: {
      freq: 855737500,
      freqList: [
        { len: 5.04 },
        { len: 4.68 }
      ]
    }
  }
}

// Output (after GridFS storage)
{
  type: "audio",
  freq: 855737500,
  freqList: [
    { len: 5.04 },
    { len: 4.68 }
  ],
  _processed_at: Date,
  _mqtt_received_at: Date
}
```

## Notes
- No special cases except base64 handling
- No type-specific transformations
- Simple, predictable output format
- Arrays always stay intact under original keys
- All binary data goes to GridFS before MongoDB storage