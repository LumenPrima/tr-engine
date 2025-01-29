# Timestamp Standardization

This document outlines the standardization of timestamp handling across the TR Engine application to ensure consistency and reliability.

## Core Principles

1. **Internal Processing**: Use Unix timestamps (seconds since epoch)
   - All internal state management
   - Database storage
   - Message processing
   - Time calculations
   - Performance-critical operations

2. **API Boundaries**: Use ISO 8601 strings
   - External API responses
   - Client communication
   - Logging
   - Debug output

3. **Database Storage**: Use Unix timestamps
   - Efficient storage
   - Fast queries
   - Consistent with MQTT message format
   - Easy time-based comparisons

4. **Display Layer**: Use locale-specific formatting
   - Human-readable presentation
   - Timezone awareness
   - Cultural preferences

## Implementation Details

### 1. Message Processing Layer

```javascript
// MQTT messages arrive with Unix timestamps
const message = {
    timestamp: 1706543412,  // Unix timestamp (seconds)
    start_time: 1706543400  // Unix timestamp (seconds)
};

// Keep internal processing timestamps as Unix
const processed = {
    ...message,
    _processed_at: timestamps.getCurrentUnix(),  // Unix timestamp
    _mqtt_received_at: timestamps.getCurrentUnix()  // Unix timestamp
};
```

### 2. State Management Layer

```javascript
// Duration calculations (all in seconds)
const duration = timestamps.diffSeconds(
    timestamps.getCurrentUnix(),
    call.start_time
);

// Time window comparisons
const staleThreshold = timestamps.addSeconds(
    timestamps.getCurrentUnix(),
    -300  // 5 minutes ago
);

// Cleanup stale items
if (call.start_time < staleThreshold) {
    // Remove stale call
}
```

### 3. Database Layer

```javascript
// Store as Unix timestamps
const record = {
    timestamp: 1706543412,  // Unix timestamp
    start_time: 1706543400  // Unix timestamp
};

// Query using Unix timestamps
const query = {
    timestamp: {
        $gte: startTimeUnix,
        $lte: endTimeUnix
    }
};
```

### 4. API Layer

```javascript
// Convert to ISO 8601 only when sending responses
const response = {
    status: 'success',
    timestamp: timestamps.getCurrentISO(),
    data: {
        calls: calls.map(call => ({
            ...call,
            timestamp: timestamps.toISO(call.timestamp),
            start_time: timestamps.toISO(call.start_time)
        }))
    }
};
```

### 5. Frontend Layer

```javascript
// Receive ISO strings from API
const call = {
    timestamp: '2025-01-29T14:43:32.000Z',
    start_time: '2025-01-29T14:43:00.000Z'
};

// Convert to Unix for calculations
const unixTimestamp = timestamps.fromISO(call.timestamp);

// Format for display
const displayTime = timestamps.formatForDisplay(unixTimestamp);
```

## Migration Strategy

1. **Message Processing**
   - Keep MQTT timestamps as Unix
   - Validate but don't convert incoming timestamps
   - Add Unix timestamp validation

2. **State Management**
   - Update duration calculations to use Unix timestamps
   - Fix time comparisons to use Unix timestamps
   - Restore stale cleanup functionality

3. **Database Operations**
   - Maintain Unix timestamp storage
   - Update queries to use Unix timestamps
   - Add validation for timestamp fields

4. **API Layer**
   - Convert to ISO 8601 in responses
   - Accept both ISO 8601 and Unix in requests
   - Validate and convert as needed

5. **Frontend**
   - Update display formatting
   - Handle timezone conversions
   - Improve error handling

## Benefits

1. **Performance**
   - Efficient numeric comparisons
   - Reduced conversion overhead
   - Optimized database queries

2. **Reliability**
   - Consistent timestamp handling
   - Accurate time calculations
   - Proper timezone management

3. **Maintainability**
   - Clear boundaries for conversions
   - Centralized timestamp logic
   - Easier debugging

4. **Compatibility**
   - Works with existing MQTT format
   - Supports legacy timestamps
   - Flexible API handling

## Testing Requirements

1. **Unit Tests**
   - Timestamp conversion accuracy
   - Edge case handling
   - Validation functions

2. **Integration Tests**
   - Message processing flow
   - Database operations
   - API responses

3. **Performance Tests**
   - Conversion overhead
   - Query efficiency
   - Time calculation speed

## Conclusion

This standardization maintains Unix timestamps for internal operations while providing appropriate formats at system boundaries. This approach ensures both efficiency and usability while preventing the timestamp conversion issues that could impact system reliability.
