# Call ID Format Standardization

## Overview

This document outlines the standardization of call_id formats in the system, addressing the need for consistent identification while maintaining flexibility for both single and multi-system deployments.

## Call ID Formats

### Single-System Format
For deployments monitoring a single system:
```
{talkgroup}_{timestamp}
Example: "9177_1738173276"
```

### Multi-System Format
For deployments monitoring multiple systems:
```
{sysnum}_{talkgroup}_{timestamp}
Example: "1_9177_1738173276"
```

### Legacy Formats (Supported for Backward Compatibility)
```
{talkgroup}-{timestamp}
Example: "9177-1738173276"
```

## Implementation Details

### 1. Audio File Storage
- Original audio filenames are preserved to maintain storage compatibility
- Example filename: `9177-1738173276_852200000.0-call_64166.wav`
- MongoDB GridFS handles unique storage regardless of filename

### 2. Call ID Generation
The system determines the appropriate format based on context:

```javascript
// For single-system setups
const callId = `${talkgroup}_${timestamp}`;

// For multi-system setups
const sysNum = short_name.replace('sys', '');
const callId = `${sysNum}_${talkgroup}_${timestamp}`;
```

### 3. Format Parsing
The system supports parsing all formats:

```javascript
// Primary format (3 parts): sysnum_talkgroup_timestamp
[sysNum, talkgroup, timestamp] = callId.split('_');

// Single-system format (2 parts): talkgroup_timestamp
[talkgroup, timestamp] = callId.split('_');

// Legacy format: talkgroup-timestamp
[talkgroup, timestamp] = callId.split('-');
```

## Key Files Modified

1. `src/services/mqtt/message-processor/file-storage.js`
   - Handles initial call_id generation during file storage
   - Determines format based on system context

2. `src/services/mqtt/handlers/audio-handler.js`
   - Manages call_id format in WebSocket broadcasts
   - Maintains consistency with storage format

3. `src/api/routes/transcription.js`
   - Provides parsing and formatting utilities
   - Handles all formats for API operations

## Rationale

1. **Simplicity First**: Single-system deployments use the simpler format without unnecessary system numbers.

2. **Clear System Identification**: Multi-system deployments include system numbers for clear call origin tracking.

3. **Backward Compatibility**: Legacy formats are supported to prevent disruption of existing data and integrations.

4. **Storage Stability**: Original audio filenames are preserved to maintain reliable file retrieval and prevent collisions.

## Migration Notes

- Existing call_ids will continue to work without modification
- New calls will use the appropriate format based on system context
- No database migration required as parsing handles all formats
