# Event-Driven Architecture for MQTT Message Processing

## Current Architecture

### Message Flow
1. MQTT Client receives messages
2. Message Processor:
   - Parses messages
   - Transforms messages (flattening, timestamp validation)
   - Routes to collection manager for storage
   - Routes to state managers for processing
3. Collection Manager:
   - Determines collection names
   - Stores messages in MongoDB
4. State Managers:
   - Process transformed messages
   - Maintain in-memory state
   - Emit events for WebSocket broadcasting

### Issues with Current Approach
1. Tight coupling between components
2. Duplicate database operations
3. Complex message transformation layer
4. State managers receive already transformed data
5. Difficult to add new message handlers
6. Complex routing logic in message processor

## Proposed Event-Driven Architecture

### Message Flow
1. MQTT Client receives messages
2. Message Processor:
   - Parses messages
   - Determines message type and affected subsystems
   - Emits single event with complete message
3. State Managers:
   - Subscribe to message events
   - Extract relevant data from original message structure
   - Manage their own database operations
   - Maintain in-memory state
   - Emit WebSocket events

### Message Structure Preservation
- Keep original nested message format intact
- No transformation or flattening of data
- Each manager knows how to handle its specific message types
- Preserve message metadata (timestamps, instance IDs)

Example message structure:
```javascript
{
  "type": "call_start",
  "call": {
    "id": "3_54623_1738340220",
    "sys_name": "warco",
    "unit": 8308181,
    "talkgroup": 54623,
    // ... other call data
  },
  "timestamp": 1738340228,
  "instance_id": "trunk-recorder"
}
```

### Benefits
1. Loose coupling between components
2. Clear separation of concerns
3. Easier to add new functionality
4. More maintainable codebase
5. Better testability
6. Asynchronous processing capability
7. Each manager owns its data model

## Implementation Changes Required

### 1. Message Processor (src/services/mqtt/message-processor/index.js)
```javascript
class MessageProcessor {
  async processMessage(topic, payload) {
    try {
      const message = JSON.parse(payload.toString());
      const [prefix, type, system, action] = topic.split('/');

      // Emit single event with complete message
      stateEventEmitter.emit('message.received', {
        topic,
        type: message.type,
        data: message,
        system // For unit messages
      });

    } catch (err) {
      logger.error(`Error processing message for ${topic}:`, err);
      stateEventEmitter.emitError(err);
    }
  }
}
```

### State Manager Example
```javascript
class ActiveCallManager {
  constructor() {
    stateEventEmitter.on('message.received', this.handleMessage.bind(this));
  }

  handleMessage({ topic, type, data, system }) {
    // Handle based on message type
    switch(type) {
      case 'call_start':
        if (data.call?.id) {
          this.handleCallStart(data);
        }
        break;
      case 'call_end':
        if (data.call?.id) {
          this.handleCallEnd(data);
        }
        break;
      case 'calls_active':
        if (Array.isArray(data.calls)) {
          this.handleActiveCalls(data);
        }
        break;
    }
  }

  handleCallStart(message) {
    const callId = message.call.id;
    
    // Store complete call data
    this.activeCallsCache.set(callId, {
      ...message.call,
      _received_at: message.timestamp,
      _instance_id: message.instance_id
    });

    // Store in database with original structure
    await this.collection.updateOne(
      { _id: callId },
      { $set: message },
      { upsert: true }
    );
  }
}
```

### 2. State Managers

#### UnitManager (src/services/state/UnitManager.js)
```javascript
class UnitManager {
  constructor() {
    stateEventEmitter.on('message.received', this.handleMessage.bind(this));
  }

  handleMessage({ topic, type, data, system }) {
    // Extract unit data from appropriate nested location
    const unitData = data[type]; // e.g., data.join, data.location
    if (!unitData?.unit) return;

    switch(type) {
      case 'join':
        this.handleUnitJoin(unitData);
        break;
      case 'location':
        this.handleUnitLocation(unitData);
        break;
      case 'call':
        this.handleUnitCall(unitData);
        break;
    }
  }

  handleUnitJoin(data) {
    const unitKey = this.getUnitKey(data.wacn, data.unit);
    
    // Store complete unit data
    this.unitStates.set(unitKey, {
      ...data,
      last_join: new Date()
    });

    // Store in database with original structure
    await this.collection.updateOne(
      { wacn: data.wacn, unit: data.unit },
      { $set: data },
      { upsert: true }
    );
  }
}
```

### 3. Remove Unnecessary Components
- Delete message-transformer.js (no more message transformation)
- Delete collection-manager.js (managers handle their own storage)
- Remove handler classes (single message processor)
- Simplify message flow

### 4. Update Event Emitter (src/services/events/emitter.js)
- Add any new event types needed
- Update event data structures to match raw message formats

### 5. Update Event Handlers (src/services/events/handlers.js)
- Update handlers to work with new event formats
- Add any new event type handlers

## Migration Steps

1. Create new event types in emitter
2. Update state managers to use event listeners
3. Update message processor to emit events
4. Remove old message transformation layer
5. Remove collection manager
6. Update tests to reflect new architecture
7. Update documentation

## Testing Strategy

1. Unit Tests:
   - Test message parsing in processor
   - Test event emission with complete messages
   - Test state manager message handling
   - Test database operations in managers

2. Integration Tests:
   - Test message flow from MQTT to state update
   - Test WebSocket broadcasting
   - Test database persistence
   - Test error handling

3. Load Tests:
   - Test system performance under high message volume
   - Test event handling concurrency
   - Test database performance with original message structures

## Considerations

1. Error Handling:
   - Validate message format at processor level
   - Each manager handles its own data validation
   - Keep original message for debugging
   - Error events include complete message context

2. Performance:
   - No transformation overhead
   - Single event emission per message
   - Managers process only relevant messages
   - Original structure preserved in database

3. Scalability:
   - Easy to add new message types
   - Managers can evolve independently
   - Clear data ownership
   - Flexible message structure support

4. Monitoring:
   - Track raw message flow
   - Monitor manager processing times
   - Log complete message context
   - Track database operation timing
