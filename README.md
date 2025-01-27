# TR-ENGINE (Event Network Gateway and Integration Node Engine)

A high-throughput trunk radio monitoring and analysis server that ingests MQTT message streams from multiple radio systems, processes them through a distributed event pipeline, and maintains real-time state while archiving to a time-series data store. The system implements an event-driven architecture with a focus on low-latency message processing, efficient state management, and scalable data persistence. Built around modern async processing patterns, TR-ENGINE handles concurrent message streams while providing both REST and WebSocket interfaces for real-time monitoring and historical analysis.

## Key Features

- Multi-threaded MQTT message ingestion
- Persistent message queue with fail-safe processing
- Time-series data storage with efficient indexing
- Real-time state management with in-memory caching
- WebSocket-based event streaming
- RESTful API for historical queries

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

## Performance Considerations

- Event-driven architecture
- Low-latency message processing
- Efficient state management
- Scalable data persistence
- Concurrent message stream handling

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

This project is licensed under the ISC License.
