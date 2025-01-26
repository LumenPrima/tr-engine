# Radio Monitoring System Architecture

## Overview
The Radio Monitoring System is a Node.js/Express application that processes MQTT messages from radio systems, stores historical data, and provides real-time monitoring capabilities. The system is designed to handle high message throughput while maintaining low latency for real-time updates.

## Directory Structure
```
/radio-monitor
├── src/
│   ├── app.js                 # Express application setup
│   ├── config/
│   │   ├── index.js           # Configuration management
│   │   ├── mqtt.js            # MQTT connection settings
│   │   └── mongodb.js         # Database connection settings
│   │
│   ├── models/
│   │   ├── raw/               # Raw message schemas
│   │   │   ├── CallMessage.js
│   │   │   ├── SystemMessage.js
│   │   │   ├── UnitMessage.js
│   │   │   └── RecorderMessage.js
│   │   │
│   │   └── processed/         # Processed data schemas
│   │       ├── ActiveCall.js
│   │       ├── SystemState.js
│   │       ├── UnitState.js
│   │       └── AudioFile.js
│   │
│   ├── services/
│   │   ├── mqtt/
│   │   │   ├── client.js      # MQTT connection management
│   │   │   ├── parser.js      # Message parsing and validation 
│   │   │   └── handlers/      # Topic-specific message handlers
│   │   │       ├── calls.js
│   │   │       ├── systems.js
│   │   │       ├── units.js
│   │   │       └── recorders.js
│   │   │
│   │   ├── state/
│   │   │   ├── ActiveCallManager.js
│   │   │   ├── SystemManager.js
│   │   │   └── UnitManager.js
│   │   │
│   │   ├── audio/
│   │   │   ├── storage.js     # GridFS audio file management
│   │   │   └── processor.js   # Audio processing utilities
│   │   │
│   │   └── events/
│   │       ├── emitter.js     # Application event management
│   │       └── handlers.js     # Event handling logic
│   │
│   ├── api/
│   │   ├── routes/
│   │   │   ├── calls.js       # Call-related endpoints
│   │   │   ├── systems.js     # System status endpoints
│   │   │   ├── units.js       # Unit tracking endpoints
│   │   │   └── audio.js       # Audio file endpoints
│   │   │
│   │   ├── middleware/
│   │   │   ├── auth.js        # Authentication (future use)
│   │   │   ├── validation.js  # Request validation
│   │   │   └── error.js       # Error handling
│   │   │
│   │   └── websocket/
│   │       ├── server.js      # WebSocket server setup
│   │       └── handlers.js     # WebSocket event handlers
│   │
│   └── utils/
│       ├── logger.js          # Logging configuration
│       ├── metrics.js         # Performance monitoring
│       └── helpers.js         # Shared utility functions
│
├── scripts/
│   ├── setup-db.js            # Database initialization
│   └── generate-indexes.js    # Index creation script
│
└── tests/
    ├── unit/                  # Unit tests
    ├── integration/           # Integration tests
    └── fixtures/              # Test data
```

## Core Components

### Message Processing Pipeline
The system processes messages through several stages:

1. MQTT Message Reception
   - The MQTT client connects to the radio system broker
   - Messages are validated and parsed based on topic
   - Raw messages are stored in appropriate collections

2. State Management
   - Messages update real-time state through manager services
   - Changes trigger events for real-time client updates
   - State is persisted to MongoDB for durability

3. Real-time Updates
   - WebSocket connections maintain client synchronization
   - State changes are immediately broadcast to subscribers
   - Clients can request initial state through REST API

### Data Storage Strategy

1. Raw Message Collections
   - Time series collections store all incoming messages
   - Organized by message type (calls, systems, units)
   - Efficient historical querying and retention management

2. Processed State Collections
   - Active calls with current status and participants
   - System state including configuration and performance
   - Unit state tracking current affiliations and activity

3. Audio Storage
   - GridFS for audio file management
   - Metadata stored with call records
   - Efficient streaming and retrieval

## API Layer

### REST Endpoints

1. Real-time Monitoring
   - GET /api/v1/active/calls
   - GET /api/v1/events/active
   - GET /api/v1/systems/status

2. Historical Data
   - GET /api/v1/history/unit/{unit_id}
   - GET /api/v1/history/talkgroup/{talkgroup_id}
   - GET /api/v1/audio/{call_id}

3. System Management
   - GET /api/v1/system/health
   - GET /api/v1/system/stats

### WebSocket Events

1. State Updates
   - call.update: Call state changes
   - system.update: System status changes
   - unit.update: Unit status changes

2. Audio Events
   - audio.start: New audio recording
   - audio.complete: Recording finished

## Performance Considerations

1. Message Processing
   - Asynchronous message handling
   - Batch processing where appropriate
   - In-memory caching for active state

2. Database Operations
   - Efficient indexes for common queries
   - Time series collections for historical data
   - Bulk operations for batch updates

3. Real-time Updates
   - WebSocket connection management
   - Event batching and throttling
   - Client-side state reconciliation

## Deployment Considerations

1. Environment Configuration
   - MQTT connection settings
   - MongoDB configuration
   - API rate limits and caching

2. Monitoring
   - Message processing metrics
   - System performance monitoring
   - Error tracking and alerting

3. Maintenance
   - Database cleanup procedures
   - Audio file retention policies
   - Backup and recovery procedures

## Development Workflow

1. Local Development
   - Environment setup with Docker
   - Development database seeding
   - Test message simulation

2. Testing Strategy
   - Unit tests for core logic
   - Integration tests for API endpoints
   - Performance testing for message processing

3. Deployment Pipeline
   - Code quality checks
   - Automated testing
   - Deployment automation

This architecture provides a foundation for building a scalable and maintainable radio monitoring system. Each component is designed to be modular and testable, with clear separation of concerns and well-defined interfaces.