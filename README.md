# TR-ENGINE (Event Network Gateway and Integration Node Engine)

⚠️ **Note**: This documentation was generated with AI assistance and may contain inaccuracies in descriptions, setup instructions, and architectural details. Use this as a rough guide only and verify all information before implementation.

A high-throughput trunk radio monitoring and analysis server that ingests MQTT message streams from multiple radio systems, processes them through a distributed event pipeline, and maintains real-time state while archiving to a time-series data store. The system implements an event-driven architecture with a focus on low-latency message processing, efficient state management, and scalable data persistence.

## Project Status

The project is in active development. Current status:

### Completed Features
- ✅ Voice transcription system with OpenAI Whisper integration
- ✅ Advanced search capabilities with multi-field and time-range queries
- ✅ Message validation and error tracking
- ✅ Recording system monitoring
- ✅ Comprehensive monitoring dashboard

### In Progress/Planned
- 🔄 Audio quality management (partial implementation)
  - Signal quality assessment complete
  - Recording comparison pending
- 🔄 Data export system (audio downloads only)
  - Batch export and format options pending
- WebSocket implementation (not started)
  - Real-time monitoring planned
  - Event streaming architecture planned

### Known Issues
- Audio quality comparison may produce inconsistent results
- Real-time updates limited to polling (no WebSocket implementation yet)
- Some transcription results may require manual verification
- Historical data retention policies not yet implemented
- Authentication system not implemented - no access control

## Key Features

- Multi-threaded MQTT message ingestion
- Persistent message queue with fail-safe processing
- Time-series data storage with efficient indexing
- Real-time state management with in-memory caching
- RESTful API for historical queries
- AI-powered voice transcription
- Comprehensive monitoring interface

## Prerequisites

- Node.js 16+
- MongoDB 4.4+
- MQTT Broker (e.g., Mosquitto)

## Installation

Note: These setup instructions are AI-generated and may need verification.

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
NODE_ENV=development
OPENAI_API_KEY=your_api_key  # Required for transcription
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

Note: Test infrastructure is currently in setup phase. Jest is configured but test suites are pending implementation.

## API Endpoints

Note: API documentation is AI-generated and may be incomplete or inaccurate. Refer to the actual code for definitive endpoint specifications.

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

## Future WebSocket Implementation

The following events are planned for future WebSocket support (not yet implemented):

### State Updates
- call.update: Call state changes
- system.update: System status changes
- unit.update: Unit status changes

### Audio Events
- audio.start: New audio recording
- audio.complete: Recording finished

## Architecture

Note: This architectural overview is AI-generated and may not perfectly reflect the actual implementation.

The system processes messages through several stages:

1. MQTT Message Reception
   - Messages are validated and parsed based on topic
   - Raw messages are stored in appropriate collections

2. State Management
   - Messages update real-time state through manager services
   - Changes trigger events for real-time client updates
   - State is persisted to MongoDB

3. Real-time Updates
   - Currently implemented through polling REST endpoints
   - WebSocket implementation planned for future

## Performance Considerations

- Event-driven architecture
- Low-latency message processing
- Efficient state management
- Scalable data persistence
- Concurrent message stream handling

## Security Notice

⚠️ The system currently lacks authentication and access control. It is recommended to run only in a secure, isolated network environment until authentication is implemented.

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

This project is licensed under the ISC License.
