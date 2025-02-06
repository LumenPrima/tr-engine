# MQTT Topics and Handlers

## Topic Structure
Messages follow two main patterns:
1. `tr-mqtt_main_*` - System-wide messages
2. `tr-mqtt_units_[system]_*` - Unit-specific messages per system

## Main Topics

### Call-Related Messages
| Topic | Handler | Description |
|-------|---------|-------------|
| `tr-mqtt_main_call_start` | CallHandler | New call initiation |
| `tr-mqtt_main_call_end` | CallHandler | Call termination |
| `tr-mqtt_main_calls_active` | CallHandler | Current active calls |
| `tr-mqtt_main_audio` | AudioHandler | Audio data for calls |

### System Status Messages
| Topic | Handler | Description |
|-------|---------|-------------|
| `tr-mqtt_main_systems` | SystemHandler | System configuration and status |
| `tr-mqtt_main_rates` | SystemHandler | System rate information |
| `tr-mqtt_main_config` | SystemHandler | System configuration updates |

### Recorder Messages
| Topic | Handler | Description |
|-------|---------|-------------|
| `tr-mqtt_main_recorder` | RecorderHandler | Individual recorder status |
| `tr-mqtt_main_recorders` | RecorderHandler | Multiple recorders status |
| `tr-mqtt_main_trunk_recorder_status` | RecorderHandler | Trunk recorder status |

## Unit Topics (per system)
Each system (butco, hamco, monco, warco) has the following topics:

### Unit Status Messages
| Topic | Handler | Description |
|-------|---------|-------------|
| `tr-mqtt_units_[system]_on` | UnitStatusHandler | Unit coming online |
| `tr-mqtt_units_[system]_off` | UnitStatusHandler | Unit going offline |
| `tr-mqtt_units_[system]_join` | UnitStatusHandler | Unit joining talkgroup |

### Unit Activity Messages
| Topic | Handler | Description |
|-------|---------|-------------|
| `tr-mqtt_units_[system]_call` | UnitCallHandler | Unit initiating call |
| `tr-mqtt_units_[system]_end` | UnitCallHandler | Unit ending call |
| `tr-mqtt_units_[system]_location` | UnitLocationHandler | Unit location update |

### Unit Data Messages
| Topic | Handler | Description |
|-------|---------|-------------|
| `tr-mqtt_units_[system]_data` | UnitDataHandler | Unit data transmission |
| `tr-mqtt_units_[system]_ackresp` | UnitDataHandler | Unit acknowledgment response |

## Handler Implementation

### Main Handlers

#### CallHandler (src/services/mqtt/handlers/main/call-handler.js)
```javascript
class CallHandler {
  handle(topic, message) {
    const action = topic.split('_').pop(); // call_start, call_end, calls_active
    switch(action) {
      case 'call_start':
        return this.handleCallStart(topic, message);
      case 'call_end':
        return this.handleCallEnd(topic, message);
      case 'calls_active':
        return this.handleCallsActive(topic, message);
    }
  }
}
```

#### SystemHandler (src/services/mqtt/handlers/main/system-handler.js)
```javascript
class SystemHandler {
  handle(topic, message) {
    const action = topic.split('_').pop(); // systems, rates, config
    switch(action) {
      case 'systems':
        return this.handleSystems(topic, message);
      case 'rates':
        return this.handleRates(topic, message);
      case 'config':
        return this.handleConfig(topic, message);
    }
  }
}
```

#### RecorderHandler (src/services/mqtt/handlers/main/recorder-handler.js)
```javascript
class RecorderHandler {
  handle(topic, message) {
    const action = topic.split('_').pop(); // recorder, recorders, trunk_recorder_status
    switch(action) {
      case 'recorder':
        return this.handleRecorder(topic, message);
      case 'recorders':
        return this.handleRecorders(topic, message);
      case 'trunk_recorder_status':
        return this.handleTrunkRecorderStatus(topic, message);
    }
  }
}
```

### Unit Handlers

#### UnitStatusHandler (src/services/mqtt/handlers/units/status-handler.js)
```javascript
class UnitStatusHandler {
  handle(topic, message, system) {
    const action = topic.split('_').pop(); // on, off, join
    switch(action) {
      case 'on':
        return this.handleUnitOn(topic, message, system);
      case 'off':
        return this.handleUnitOff(topic, message, system);
      case 'join':
        return this.handleUnitJoin(topic, message, system);
    }
  }
}
```

#### UnitCallHandler (src/services/mqtt/handlers/units/call-handler.js)
```javascript
class UnitCallHandler {
  handle(topic, message, system) {
    const action = topic.split('_').pop(); // call, end
    switch(action) {
      case 'call':
        return this.handleUnitCall(topic, message, system);
      case 'end':
        return this.handleUnitEnd(topic, message, system);
    }
  }
}
```

#### UnitLocationHandler (src/services/mqtt/handlers/units/location-handler.js)
```javascript
class UnitLocationHandler {
  handle(topic, message, system) {
    return this.handleLocation(topic, message, system);
  }
}
```

#### UnitDataHandler (src/services/mqtt/handlers/units/data-handler.js)
```javascript
class UnitDataHandler {
  handle(topic, message, system) {
    const action = topic.split('_').pop(); // data, ackresp
    switch(action) {
      case 'data':
        return this.handleData(topic, message, system);
      case 'ackresp':
        return this.handleAckResp(topic, message, system);
    }
  }
}
```

## Message Validation

Each handler should implement validation for its specific message format. Example:

```javascript
class CallHandler {
  validateCallStart(message) {
    return message?.call?.id && 
           message?.call?.sys_name &&
           message?.call?.talkgroup;
  }

  validateCallEnd(message) {
    return message?.call?.id &&
           message?.call?.sys_name;
  }

  validateCallsActive(message) {
    return Array.isArray(message?.calls);
  }
}
```

## Event Emission

Handlers emit events that state managers listen to. Example events:

```javascript
// Call events
stateEventEmitter.emitCallStart(callData);
stateEventEmitter.emitCallEnd(callData);
stateEventEmitter.emitCallUpdate(callData);

// Unit events
stateEventEmitter.emitUnitActivity(unitData);
stateEventEmitter.emitUnitLocation(locationData);
stateEventEmitter.emitUnitStatus(statusData);

// System events
stateEventEmitter.emitSystemUpdate(systemData);
stateEventEmitter.emitSystemRates(ratesData);
stateEventEmitter.emitSystemConfig(configData);

// Recorder events
stateEventEmitter.emitRecorderUpdate(recorderData);
stateEventEmitter.emitRecordersUpdate(recordersData);
