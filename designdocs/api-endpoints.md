# TR-ENGINE API Endpoints Catalog

## Implemented Endpoints

### Root Routes
- [x] GET /hello: Health check endpoint

### Audio Routes (/audio)
See [Audio File Handling Specification](audio-file-handling.md) for detailed documentation.

- [x] GET /audio/call/{call_id}: Retrieve audio recording for a specific call
  - Supports both simple format (talkgroup-timestamp) and full filename
- [x] GET /audio/call/{call_id}/metadata: Get metadata for an audio file
  - Returns available formats and detailed metadata
- [x] DELETE /audio/call/{call_id}: Delete an audio file
  - Removes all format variations of a call recording
- [x] GET /audio/archive: Search archived audio recordings
  - Supports filtering and pagination

### Calls Routes (/calls)
- [x] GET /calls: Get historical calls with filtering
- [x] GET /calls/active: Get currently active calls
- [x] GET /calls/talkgroup/{talkgroup_id}: Get historical activity for a specific talkgroup
- [x] GET /calls/events: Get currently active events (calls, emergencies)

### Systems Routes (/systems)
- [x] GET /systems: Get current status of all systems
- [x] GET /systems/performance: Get system performance statistics
- [x] GET /systems/{sys_name}: Get detailed status for a specific system

### Talkgroups Routes (/talkgroups)
- [x] GET /talkgroups: Get talkgroups with advanced filtering
- [x] GET /talkgroups/{id}: Get talkgroup by ID

### Units Routes (/units)
- [x] GET /units: Get all currently active units
- [x] GET /units/{unit_id}: Get current status for a specific unit
- [x] GET /units/{unit_id}/history: Get complete history for a specific unit
- [x] GET /units/talkgroup/{talkgroup_id}: Get units currently affiliated with a talkgroup

## Planned/Proposed Endpoints (Not Yet Implemented)

### Authentication Routes (/auth)
- [ ] POST /auth/login: User authentication
- [ ] POST /auth/logout: User logout
- [ ] POST /auth/refresh: Refresh authentication token
- [ ] GET /auth/profile: Get user profile information

### Advanced System Management (/system)
- [ ] GET /system/config: Retrieve full system configuration
- [ ] PUT /system/config: Update system configuration
- [ ] GET /system/logs: Retrieve system logs
- [ ] GET /system/metrics/detailed: Comprehensive system performance metrics
- [ ] GET /system/diagnostics: System health and diagnostic information

### Advanced Audio Management (/audio)
- [ ] POST /audio/process: Trigger audio processing job
- [ ] GET /audio/processing/status: Check audio processing queue status
- [x] GET /api/v1/transcription/calls/{call_id}/transcription: Get call transcription with segments
- [x] GET /api/v1/transcription/talkgroups/{talkgroup_id}/recent_transcriptions: Get recent transcriptions for a talkgroup
- [x] GET /api/v1/transcription/stats: Get aggregate transcription statistics
- [ ] POST /audio/export: Export audio files with custom filtering

### AI Analysis (/api/v1/transcription)
- [x] GET /api/v1/transcription/calls/{call_id}/transcription
  - Get transcription for a specific call
  - Returns full transcription text, segments, and metadata
  - Supports both WAV and M4A audio formats
  - Maps transcription segments to source units

- [x] GET /api/v1/transcription/talkgroups/{talkgroup_id}/recent_transcriptions
  - Get recent transcriptions for a talkgroup
  - Supports pagination through limit parameter
  - Optional date range filtering
  - Returns transcriptions sorted by timestamp

- [x] GET /api/v1/transcription/stats
  - Get aggregate transcription statistics
  - Groups by talkgroup
  - Provides metrics:
    - Total transcription count
    - Average confidence scores
    - Average audio duration
    - Average processing time
    - Emergency call counts

- [ ] GET /analysis/calls/{call_id}/sentiment: Get call sentiment analysis
- [ ] GET /analysis/calls/{call_id}/related: Get related call analysis
- [ ] GET /analysis/talkgroups/{talkgroup_id}/sentiment: Get talkgroup sentiment
- [ ] GET /analysis/talkgroups/{talkgroup_id}/history: Get sentiment history
- [ ] GET /analysis/talkgroups/{talkgroup_id}/related: Get related talkgroup activity

### Real-Time Streaming (/stream)
- [ ] WS /stream/audio: WebSocket endpoint for live audio streaming
- [ ] WS /stream/transcriptions: WebSocket endpoint for live transcriptions
- [ ] GET /stream/status: Get current streaming status and subscriptions

### Advanced Call Analysis (/calls/analysis)
- [ ] GET /calls/analysis/trends: Call activity trends and statistics
- [ ] GET /calls/analysis/patterns: Identify communication patterns
- [ ] GET /calls/analysis/anomalies: Detect unusual communication activities

### Recorder Management (/recorders)
- [ ] GET /recorders: List all configured recorders
- [ ] GET /recorders/{recorder_id}: Get specific recorder details
- [ ] GET /recorders/status: Aggregate recorder system status
- [ ] POST /recorders/config: Update recorder configuration

### Advanced Talkgroup Management (/talkgroups/advanced)
- [ ] GET /talkgroups/activity: Comprehensive talkgroup activity analysis
- [ ] GET /talkgroups/performance: Talkgroup communication efficiency metrics
- [ ] POST /talkgroups/tags: Manage talkgroup tags and categorization

### Notification and Alerting (/notifications)
- [ ] GET /notifications: Retrieve system notifications
- [ ] POST /notifications/rules: Create notification rules
- [ ] PUT /notifications/rules/{rule_id}: Update notification rule
- [ ] DELETE /notifications/rules/{rule_id}: Delete notification rule

### Data Export (/export)
- [ ] POST /export/calls: Export call records
- [ ] POST /export/audio: Export audio files
- [ ] POST /export/system-logs: Export system logs
- [ ] GET /export/status/{job_id}: Check export job status

## WebSocket Event Streams (Planned Expansions)
- [ ] system.config_update: System configuration changes
- [ ] system.diagnostics: Real-time system health updates
- [ ] audio.transcription_complete: Audio transcription finished
- [ ] calls.anomaly_detected: Unusual communication pattern alert
- [ ] notifications.new: New system or user-defined notifications

## Notes
- Planned endpoints are based on design documents and potential system requirements
- Implementation priority and feasibility may vary
- Endpoints subject to change based on system evolution and user requirements

## Future Considerations
- Implement robust authentication mechanism
- Develop comprehensive error tracking
- Create advanced audio processing capabilities
- Implement detailed system health monitoring
- Develop flexible notification and alerting system
