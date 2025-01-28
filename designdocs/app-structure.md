# Radio Monitoring System Architecture

## Overview
The Radio Monitoring System is a Node.js/Express application that processes MQTT messages from trunk-recorder systems, stores historical data, and provides monitoring capabilities. The system is designed to handle high message throughput while maintaining low latency for data processing.

## Directory Structure
```
/tr-engine
├── src/
│   ├── app.js                 # Express application setup
│   ├── config/
│   │   ├── index.js           # Configuration management
│   │   ├── logger.js          # Logging configuration
│   │   └── mongodb.js         # Database connection settings
│   │
│   ├── models/
│   │   ├── call.js           # Call data model
│   │   ├── Talkgroup.js      # Talkgroup data model
│   │   └── processed/        # Processed event models
│   │       └── CallEvent.js
│   │
│   ├── services/
│   │   ├── mqtt/
│   │   │   ├── mqtt-client.js # MQTT connection management
│   │   │   ├── handlers/      # Message handlers
│   │   │   │   └── audio-handler.js
│   │   │   └── message-processor/
│   │   │       ├── collection-manager.js
│   │   │       ├── file-storage.js
│   │   │       ├── index.js
│   │   │       ├── message-transformer.js
│   │   │       └── stats-manager.js
│   │   │
│   │   ├── state/
│   │   │   ├── ActiveCallManager.js
│   │   │   ├── SystemManager.js
│   │   │   ├── TalkgroupManager.js
│   │   │   └── UnitManager.js
│   │   │
│   │   ├── events/
│   │   │   ├── emitter.js    # Application event management
│   │   │   └── handlers.js   # Event handling logic
│   │   │
│   │   ├── monitoring/
│   │   │   └── RecordingMonitor.js
│   │   │
│   │   └── transcription/
│   │       └── TranscriptionService.js
│   │
│   ├── api/
│   │   ├── routes/
│   │   │   ├── audio.js      # Audio file endpoints
│   │   │   ├── calls.js      # Call-related endpoints
│   │   │   ├── index.js      # Main router
│   │   │   ├── systems.js    # System status endpoints
│   │   │   ├── talkgroups.js # Talkgroup endpoints
│   │   │   ├── transcription.js # Transcription endpoints
│   │   │   └── units.js      # Unit tracking endpoints
│   │   │
│   │   ├── middleware/
│   │   │   └── index.js      # Shared middleware
│   │   │
│   │   └── websocket/        # Future WebSocket implementation
│   │       └── server.js     # (Planned)
│   │
│   └── utils/
│       └── logger.js         # Logging utilities
│
├── public/                    # Frontend assets
│   ├── index.html
│   ├── css/
│   │   └── styles.css
│   └── js/
│       ├── app.js
│       ├── utils.js
│       └── modules/
│           ├── calls.js
│           ├── system.js
│           ├── talkgroups.js
│           ├── transcription.js
│           └── units.js
│
└── designdocs/               # Design documentation
    ├── api-usage.md
    ├── app-structure.md
    └── ...
```

## Core Components

### Message Processing Pipeline
The system processes messages through several stages:

1. MQTT Message Reception
   - Messages from trunk-recorder MQTT Status plugin are received
   - Messages are validated and parsed based on topic
   - Raw messages are stored in appropriate collections

2. State Management
   - Messages update real-time state through manager services
   - Changes trigger events for updates
   - State is persisted to MongoDB for durability

3. Real-time Updates (Current Implementation)
   - REST endpoints provide current state
   - Polling-based updates for active calls and events
   - WebSocket implementation planned for future

### Data Storage Strategy

1. Raw Message Collections
   - Time series collections store all incoming messages
   - Organized by message type (calls, systems, units)
   - Efficient historical querying

2. Processed State Collections
   - Active calls with current status and participants
   - System state including configuration and performance
   - Unit state tracking current affiliations and activity

3. Audio Storage
   - GridFS for audio file management
   - Metadata stored with call records
   - Efficient streaming and retrieval

4. Transcription Storage
   - Transcription results linked to audio files
   - Support for both local Whisper and OpenAI API
   - Quality assessment and verification flags

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

### Future WebSocket Implementation (Planned)

The following features are planned for future WebSocket support:

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

## Deployment Considerations

1. Environment Configuration
   - MQTT connection settings
   - MongoDB configuration
   - Transcription service settings (local/OpenAI)
   - API settings

2. Monitoring
   - Message processing metrics
   - System performance monitoring
   - Error tracking and alerting

3. Security Notice
   - Authentication not yet implemented
   - Run in secure, isolated network
   - Docker deployment coming soon

This architecture provides a foundation for building a scalable and maintainable radio monitoring system. Each component is designed to be modular and testable, with clear separation of concerns and well-defined interfaces.
