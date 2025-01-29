# Timestamp Standardization

This document outlines the standardization of timestamp handling across the TR Engine application to ensure consistency and reliability.

## Current State

The application currently uses a mix of timestamp formats:

1. **Database Layer (MongoDB)**
   - Unix timestamps (seconds since epoch)
   - Used for efficient storage and querying

2. **API Layer**
   - Mixed usage of:
     - ISO 8601 strings
     - Unix timestamps
     - JavaScript Date objects
   - Inconsistent format in responses

3. **Frontend**
   - Receives mixed formats from API
   - Uses JavaScript Date objects internally
   - Displays using locale-specific formatting

4. **Message Processing**
   - MQTT messages with Unix timestamps
   - Processor adds timestamps as JavaScript Date objects

## Standardization Plan

### 1. Storage Layer (MongoDB)

**Standard: Unix Timestamps (seconds since epoch)**

Rationale:
- Space-efficient storage
- Timezone-independent
- Efficient for queries and comparisons
- Consistent with existing MQTT message timestamps

Implementation:
- Continue storing timestamps as Unix timestamps
- Ensure all new collections follow this standard
- Convert any existing non-compliant timestamps during data migration

### 2. API Layer

**Standard: ISO 8601 Strings**

Rationale:
- Human-readable format
- Includes timezone information
- Industry standard for REST APIs
- Easily parsed by any client
- Self-documenting

Implementation:

Input Handling:
```javascript
// Accept ISO 8601 strings in requests
const timestamp = new Date(req.query.timestamp).getTime() / 1000;
```

Output Formatting:
```javascript
// Return ISO 8601 strings in responses
{
  timestamp: new Date(unixTimestamp * 1000).toISOString()
}
```

### 3. Frontend Layer

**Standard: ISO 8601 Strings for API Communication, Date Objects for Display**

Rationale:
- Consistent with API standard
- Maintains timezone information
- Allows flexible formatting for display

Implementation:

API Communication:
```javascript
// When sending to API
const isoString = new Date().toISOString();

// When receiving from API
const date = new Date(isoString);
```

Display Formatting:
```javascript
// Use utility functions for consistent display
function formatTimestamp(isoString) {
  const date = new Date(isoString);
  return date.toLocaleTimeString();
}
```

### 4. Message Processing Layer

**Standard: Unix Timestamps for Storage, ISO 8601 for Logging/Display**

Rationale:
- Maintains compatibility with MQTT messages
- Consistent with database storage
- Human-readable logs

Implementation:
```javascript
const processed = {
  original_timestamp: message.timestamp, // Unix timestamp
  processed_at: new Date().toISOString(), // ISO 8601 for logs
  // ... other fields
};
```

## Migration Plan

1. **Create Utility Functions**
   - Centralize timestamp conversion logic
   - Ensure consistent handling across the application

```javascript
// utils/timestamps.js
export const timestamps = {
  // Convert ISO 8601 to Unix timestamp
  toUnix: (isoString) => Math.floor(new Date(isoString).getTime() / 1000),
  
  // Convert Unix timestamp to ISO 8601
  toISO: (unixTimestamp) => new Date(unixTimestamp * 1000).toISOString(),
  
  // Format for display
  formatForDisplay: (isoString) => new Date(isoString).toLocaleString()
};
```

2. **Update API Routes**
   - Modify all endpoints to accept ISO 8601 input
   - Ensure all responses use ISO 8601
   - Use utility functions for conversions

3. **Update Frontend**
   - Modify API calls to handle ISO 8601
   - Update display formatting
   - Use utility functions consistently

4. **Update Message Processing**
   - Ensure MQTT message timestamps are properly converted
   - Use ISO 8601 for logs and debugging
   - Maintain Unix timestamps for storage

## Testing Considerations

1. **Unit Tests**
   - Test timestamp conversion utilities
   - Verify API input/output formatting
   - Test edge cases (invalid formats, timezone handling)

2. **Integration Tests**
   - Verify end-to-end timestamp handling
   - Test timezone scenarios
   - Validate database queries

3. **Frontend Tests**
   - Test display formatting
   - Verify API communication
   - Test user timezone handling

## Implementation Phases

1. **Phase 1: Utility Functions**
   - Create timestamp utility module
   - Add tests
   - Document usage

2. **Phase 2: API Standardization**
   - Update API routes
   - Add input validation
   - Update response formatting

3. **Phase 3: Frontend Updates**
   - Modify API calls
   - Update display formatting
   - Add client-side validation

4. **Phase 4: Message Processing**
   - Update MQTT message handling
   - Standardize logging
   - Verify storage format

## Benefits

1. **Consistency**
   - Predictable timestamp handling
   - Reduced bugs from format mismatches
   - Easier debugging

2. **Maintainability**
   - Centralized timestamp logic
   - Clear standards for new code
   - Better documentation

3. **User Experience**
   - Consistent display formatting
   - Proper timezone handling
   - Reliable sorting and filtering

## Conclusion

This standardization will improve code quality, reduce bugs, and provide a better developer and user experience. The use of Unix timestamps for storage and ISO 8601 for API communication provides an optimal balance of efficiency and usability.
