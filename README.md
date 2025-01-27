# TR-ENGINE
(ENGINE: ENGINE, Next-Generation Integration for Network Events)

A self-referential recursive acronym that's as interconnected as the trunk radio systems it monitors - a meta-monitoring server that processes its own kind of trunked communications (MQTT messages) to analyze actual trunked radio communications. Like a digital ouroboros, it's a system monitoring system, recording the recorders, and tracking the trackers.

At its core, TR-ENGINE is a real-time trunk radio monitoring and analysis server that processes MQTT messages from radio systems, stores historical data, and provides real-time monitoring capabilities.

## Features

- MQTT message ingestion and processing
- Real-time state management
- WebSocket-based real-time updates
- REST API for data access
- Audio file management with GridFS
- Comprehensive monitoring and metrics

## Prerequisites

- Node.js 16+
- MongoDB 4.4+
- MQTT Broker (e.g., Mosquitto)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/LumenPrima/tr-engine.git
cd tr-engine
```

2. Install dependencies:
```bash
npm install
```

3. Create a .env file:
```bash
MONGODB_URI=mongodb://localhost:27017/tr-engine
MQTT_BROKER_URL=mqtt://localhost:1883
MQTT_CLIENT_ID=tr-engine-client
PORT=3000
WS_PORT=3001
NODE_ENV=development
```

4. Initialize the database:
```bash
npm run db:init
```

## Development

Start the development server with auto-reload:
```bash
npm run dev
```

Run tests:
```bash
# Run all tests
npm test

# Run specific test suites
npm run test:unit
npm run test:integration
npm run test:performance

# Run tests with coverage
npm run test:coverage
```

Code formatting and linting:
```bash
# Format code
npm run format

# Check code style
npm run lint
```

## API Endpoints

### Real-time Monitoring
- GET /api/v1/active/calls - Get all active calls
- GET /api/v1/events/active - Get active events
- GET /api/v1/systems/status - Get system status

### Historical Data
- GET /api/v1/history/unit/{unit_id} - Get unit history
- GET /api/v1/history/talkgroup/{talkgroup_id} - Get talkgroup history
- GET /api/v1/audio/{call_id} - Stream audio file

### System Management
- GET /api/v1/system/health - Get system health status
- GET /api/v1/system/stats - Get system statistics

## WebSocket Events

### State Updates
- call.update: Call state changes
- system.update: System status changes
- unit.update: Unit status changes

### Audio Events
- audio.start: New audio recording
- audio.complete: Recording finished

## Architecture

The system processes messages through several stages:

1. MQTT Message Reception
   - Messages are validated and parsed based on topic
   - Raw messages are stored in appropriate collections

2. State Management
   - Messages update real-time state through manager services
   - Changes trigger events for real-time client updates
   - State is persisted to MongoDB

3. Real-time Updates
   - WebSocket connections maintain client synchronization
   - State changes are broadcast to subscribers
   - Clients can request initial state through REST API

## Performance

The system is designed to handle high message throughput:
- Asynchronous message processing
- In-memory caching for active state
- Efficient database indexing
- Batch operations where appropriate

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

This project is licensed under the ISC License.
