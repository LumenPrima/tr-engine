# TR-ENGINE (Event Network Gateway and Integration Node Engine)

TR-Engine is a foundational building block for radio monitoring systems, transforming raw radio communications into structured, accessible data streams. It provides the core infrastructure needed to build real-time monitoring dashboards, historical analysis tools, and intelligent radio systems.

## Foundation for Solutions

### Real-Time Data Processing
TR-Engine processes and structures radio communications data to enable:
- Building live monitoring dashboards
- Creating real-time alert systems
- Developing audio streaming applications
- Implementing instant transcription displays
- Tracking dynamic activity across systems

### Historical Data Access
The system maintains an organized data store that enables:
- Building advanced search interfaces
- Creating activity correlation tools
- Developing pattern analysis systems
- Implementing audit and review tools
- Constructing timeline analysis applications

### Data Processing Infrastructure
Core processing capabilities that enable:
- Converting voice to searchable text
- Linking related activities and events
- Building usage pattern analyzers
- Creating system health monitors
- Developing trend analysis tools

### Integration Framework
Flexible data access that enables:
- Building custom monitoring interfaces
- Creating specialized analysis tools
- Developing mobile applications
- Integrating with existing systems
- Implementing custom export tools

## Project Status

The project provides core building blocks for radio monitoring applications:

### Available Infrastructure
- ✅ Real-time data processing
  - WebSocket streaming framework
  - State management system
  - Event processing pipeline
  - Audio streaming infrastructure
  - Transcription processing system

- ✅ Data Access Layer
  - Time-based querying
  - Activity tracking
  - Talkgroup filtering
  - Full-text search capability
  - Audio retrieval system

- ✅ Processing Framework
  - Voice transcription pipeline
  - Metadata extraction system
  - Event correlation engine
  - Pattern detection framework
  - System monitoring infrastructure

### Coming Soon
- 🔄 Enhanced Audio Framework
  - Quality analysis tools
  - Signal processing pipeline
  - Coverage analysis system
- 🔄 Advanced Data Tools
  - Report generation framework
  - Batch processing system
  - Analysis pipeline tools

## Getting Started

### Prerequisites

Core services required:
- Node.js 16+
- MongoDB 4.4+
- MQTT Broker
- Local Whisper instance (recommended) or OpenAI API access

### Quick Start

1. Clone and install:
```bash
git clone https://github.com/LumenPrima/tr-engine.git
cd tr-engine
npm install
```

2. Configure environment:
```bash
# .env file
MONGODB_URI=mongodb://localhost:27017/tr-engine
MQTT_BROKER_URL=mqtt://localhost:1883
WHISPER_LOCAL_URL=http://localhost:9000  # For transcription
```

3. Launch:
```bash
npm run dev
```

4. Access the example dashboard at http://localhost:3000/dashboard

## Integration Examples

### Real-Time Data Access
```javascript
// Subscribe to data stream
ws.send({
    type: 'subscribe',
    data: { events: ['call.start', 'unit.status'] }
});

// Access transcription stream
ws.send({
    type: 'transcription.subscribe',
    data: { talkgroups: [123, 456] }
});
```

### Data Retrieval
```javascript
// Access transcribed content
GET /api/search/transcriptions?q=traffic+accident&start=2024-01-01

// Retrieve unit history
GET /api/history/units/1234?timeframe=24h

// Get talkgroup data
GET /api/history/talkgroups/5678?with_audio=true
```

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
