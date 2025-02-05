# TR-Engine Testing Tool Design Document

## Overview
A standalone testing application for TR-Engine that generates valid MQTT messages based on real-world examples. The tool provides a web interface for crafting and sending test messages while maintaining complete isolation from the main application.

## Core Principles
- Complete independence from main application
- Based on real message examples
- Validation against known good messages
- Simple, intuitive interface
- Flexible message generation

## Project Structure
```
tr-tester/
├── package.json
├── index.html
├── vite.config.js
├── src/
│   ├── App.jsx              # Main application component
│   ├── main.jsx            # Application entry point
│   ├── mqtt_messages/      # Real message examples
│   │   └── index.js        # Message template exports
│   ├── components/
│   │   ├── ConnectionPanel/
│   │   │   ├── index.jsx   # MQTT connection management
│   │   │   └── styles.css
│   │   ├── MessageBuilder/
│   │   │   ├── index.jsx   # Message creation interface
│   │   │   └── styles.css
│   │   ├── QuickActions/
│   │   │   ├── index.jsx   # Common message buttons
│   │   │   └── styles.css
│   │   └── MessageHistory/
│   │       ├── index.jsx   # Sent message log
│   │       └── styles.css
│   ├── generator/
│   │   ├── messageBuilder.js  # Message generation logic
│   │   ├── topicMapper.js     # Topic mapping logic
│   │   └── validator.js       # Message validation
│   └── utils/
       ├── mqttClient.js     # MQTT connection handling
       └── messageStore.js    # Message history management
```

## Core Dependencies
```json
{
  "dependencies": {
    "mqtt": "^5.0.0",
    "react": "^18.0.0",
    "react-dom": "^18.0.0",
    "@monaco-editor/react": "^4.0.0",
    "tailwindcss": "^3.0.0"
  },
  "devDependencies": {
    "vite": "^5.0.0",
    "@vitejs/plugin-react": "^5.0.0"
  }
}
```

## Key Features

### 1. Message Template System
- Load real message examples as base templates
- Organize messages by type and category
- Support for all TR-Engine message types
- Template modification with field validation

### 2. Message Builder Interface
- Template selector with categorized messages
- Field editor for common parameters
- JSON editor for advanced modifications
- Real-time validation against example structure
- Topic preview and validation

### 3. Connection Management
- MQTT broker connection interface
- Connection status monitoring
- Automatic reconnection
- Connection settings management

### 4. Quick Actions
- Pre-configured message buttons
- Common message sequences
- Emergency message generation
- System status updates

### 5. Message History
- Log of sent messages
- Comparison with original templates
- Success/failure tracking
- Message replay capability

## Implementation Details

### Message Generation Flow
1. Select message template from real examples
2. Modify required fields (timestamps, IDs)
3. Validate against original structure
4. Generate appropriate MQTT topic
5. Send message and log result

### Template Handling
```javascript
// Example message template handling
const templates = {
  call_start: (params = {}) => ({
    ...loadTemplate('calls/call_start.json'),
    instance_id: params.instance_id || 'test-instance',
    call: {
      ...loadTemplate('calls/call_start.json').call,
      id: generateCallId(params),
      talkgroup: params.talkgroup || 58259,
      emergency: params.emergency || false
    }
  })
};
```

### Topic Mapping
```javascript
// Example topic mapping
const topicMap = {
  call_start: '${config.mqtt.topicPrefix}/call_start',
  unit_join: '${config.mqtt.topicPrefix}/units/join',
  system_status: '${config.mqtt.topicPrefix}/systems'
};
```

### Message Validation
- Structure validation against templates
- Required field checking
- Data type validation
- Topic validation
- Relationship validation (e.g., matching IDs)

## User Interface Layout
```
+----------------------------------+
|  MQTT Broker: localhost:1883     |
|  Status: Connected    [Settings] |
+----------------------------------+
|  Message Categories:             |
|  [Calls] [Units] [System]       |
+----------------------------------+
|  Template: [call_start.json ▼]   |
|                                 |
|  Quick Edit:                    |
|  Talkgroup: [58259]             |
|  Emergency: [□]                 |
|                                 |
|  Advanced Editor:               |
|  +----------------------------+ |
|  |{                          | |
|  |  "type": "call_start",    | |
|  |  ...                      | |
|  +----------------------------+ |
|  [Validate] [Send]            |
+----------------------------------+
|  History (Last 10):             |
|  12:01:05 call_start ✓         |
|  12:01:00 system_status ✓      |
+----------------------------------+
```

## Development Setup
```bash
# Initialize project
mkdir tr-tester
cd tr-tester
npm init -y

# Install dependencies
npm install mqtt react react-dom @monaco-editor/react tailwindcss
npm install -D vite @vitejs/plugin-react

# Development
npm run dev

# Build
npm run build
```

## Future Enhancements
1. Message sequence automation
2. Multiple system simulation
3. Load testing capabilities
4. Message timing control
5. Custom template creation
6. Advanced validation rules
7. Network condition simulation
8. Batch message processing

## Next Steps
1. Set up basic project structure
2. Implement template loading system
3. Create basic UI components
4. Add message generation logic
5. Implement MQTT connection
6. Add validation system
7. Create message history
8. Add advanced editing features
