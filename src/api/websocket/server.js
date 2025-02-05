const WebSocket = require('ws');
const logger = require('../../utils/logger');
const EventHandlers = require('../../services/events/handlers');
const stateEventEmitter = require('../../services/events/emitter');
const activeCallManager = require('../../services/state/ActiveCallManager');
const systemManager = require('../../services/state/SystemManager');
const unitManager = require('../../services/state/UnitManager');
const recorderManager = require('../../services/state/RecorderManager');

class WebSocketServer {
    constructor(server) {
        this.wss = new WebSocket.Server({ 
            server,
            path: '/ws'  // Match the Nginx location
        });
        logger.info('WebSocket server attached to HTTP server on path /ws');
        
        this.eventHandlers = new EventHandlers(this.wss);
        this.subscriptions = new Map(); // Track client subscriptions
        this.audioSubscriptions = new Map(); // Track audio subscriptions
        this.transcriptionSubscriptions = new Map(); // Track transcription subscriptions
        this.setupWebSocketServer();
    }

    close() {
        return new Promise((resolve) => {
            this.wss.close(() => {
                logger.info('WebSocket server closed');
                resolve();
            });
        });
    }

    setupWebSocketServer() {
        this.wss.on('connection', this.handleConnection.bind(this));
        this.wss.on('error', this.handleServerError.bind(this));
    }

    handleConnection(ws, req) {
        const clientIp = req.socket.remoteAddress;
        logger.info(`New WebSocket connection from ${clientIp}`);

        // Initialize client subscriptions
        this.subscriptions.set(ws, new Set());
        this.audioSubscriptions.set(ws, new Map()); // Map talkgroup IDs to options
        this.transcriptionSubscriptions.set(ws, new Set()); // Set of talkgroup IDs

        // Send initial state to the new client
        this.sendInitialState(ws);

        // Setup client event handlers
        ws.on('message', (message) => this.handleClientMessage(ws, message));
        ws.on('close', () => this.handleClientDisconnect(ws));
        ws.on('error', (error) => this.handleClientError(ws, error));

        // Set up recorder event listeners
        stateEventEmitter.on('recorder.stateChange', (data) => {
            const currentSubs = this.subscriptions.get(ws);
            if (currentSubs?.has('recorder.stateChange')) {
                this.sendResponse(ws, 'recorder.stateChange', data);
            }
        });

        stateEventEmitter.on('recorders.status', (data) => {
            const currentSubs = this.subscriptions.get(ws);
            if (currentSubs?.has('recorders.status')) {
                this.sendResponse(ws, 'recorders.status', data);
            }
        });

        stateEventEmitter.on('recorder.update', (data) => {
            const currentSubs = this.subscriptions.get(ws);
            if (currentSubs?.has('recorder.update')) {
                this.sendResponse(ws, 'recorder.update', data);
            }
        });

        // Send connection status
        this.sendResponse(ws, 'connection.status', {
            status: 'connected',
            subscriptions: [],
            transcriptionSubscriptions: []
        });

        // Start heartbeat for this client
        ws.isAlive = true;
        ws.heartbeatInterval = setInterval(() => {
            if (!ws.isAlive) {
                clearInterval(ws.heartbeatInterval);
                return ws.terminate();
            }
            ws.isAlive = false;
            this.sendResponse(ws, 'heartbeat', {});
        }, 30000);
    }

    async sendInitialState(ws) {
        try {
            // Get current state from managers
            const activeCalls = await activeCallManager.getActiveCalls();
            const activeSystems = await systemManager.getActiveSystems();
            const activeUnits = await unitManager.getActiveUnits();
            const recorderStates = await recorderManager.getAllRecorderStates();
            const recorderStats = await recorderManager.getRecorderStats();

            // Send initial state messages
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({
                    type: 'initial.state',
                    timestamp: new Date().toISOString(),
                    data: {
                        calls: activeCalls,
                        systems: activeSystems,
                        units: activeUnits,
                        recorders: {
                            states: recorderStates,
                            stats: recorderStats
                        }
                    }
                }));
                logger.debug('Sent initial state to new client');
            }
        } catch (err) {
            logger.error('Error sending initial state:', err);
        }
    }

    handleClientMessage(ws, message) {
        try {
            const data = JSON.parse(message);
            logger.debug('Received WebSocket message:', data);

            // Reset heartbeat timeout
            ws.isAlive = true;

            // Handle client requests
            switch (data.type) {
                // Subscription management
                case 'subscribe':
                    this.handleSubscribe(ws, data.data?.events);
                    break;
                case 'unsubscribe':
                    this.handleUnsubscribe(ws, data.data?.events);
                    break;
                
                // Audio subscription management
                case 'audio.subscribe':
                    this.handleAudioSubscribe(ws, data.data);
                    break;
                case 'audio.unsubscribe':
                    this.handleAudioUnsubscribe(ws, data.data?.talkgroups);
                    break;

                // Transcription subscription management
                case 'transcription.subscribe':
                    this.handleTranscriptionSubscribe(ws, data.data?.talkgroups);
                    break;
                case 'transcription.unsubscribe':
                    this.handleTranscriptionUnsubscribe(ws, data.data?.talkgroups);
                    break;

                // Data requests
                case 'get.active_calls':
                    this.handleGetActiveCalls(ws);
                    break;
                case 'get.system_status':
                    this.handleGetSystemStatus(ws);
                    break;
                case 'get.unit_status':
                    this.handleGetUnitStatus(ws, data.unit);
                    break;
                case 'get.recorder_status':
                    this.handleGetRecorderStatus(ws, data.recorder);
                    break;
                case 'get.recorder_stats':
                    this.handleGetRecorderStats(ws);
                    break;
                case 'transcription.request':
                    this.handleTranscriptionRequest(ws, data.data);
                    break;
                case 'transcription.stats.request':
                    this.handleTranscriptionStatsRequest(ws, data.data);
                    break;
                
                // Heartbeat response
                case 'heartbeat':
                    ws.isAlive = true;
                    break;

                default:
                    logger.warn(`Unknown message type: ${data.type}`);
                    this.sendError(ws, 'Unknown message type', {
                        code: 'UNKNOWN_MESSAGE_TYPE',
                        type: data.type
                    });
            }
        } catch (err) {
            logger.error('Error handling client message:', err);
            this.sendError(ws, 'Invalid message format');
        }
    }

    handleClientDisconnect(ws) {
        logger.info('Client disconnected from WebSocket');
        // Clean up subscriptions
        this.subscriptions.delete(ws);
        this.audioSubscriptions.delete(ws);
        this.transcriptionSubscriptions.delete(ws);

        // Clean up event listeners
        stateEventEmitter.removeAllListeners('recorder.stateChange');
        stateEventEmitter.removeAllListeners('recorders.status');
        // Clear heartbeat interval
        if (ws.heartbeatInterval) {
            clearInterval(ws.heartbeatInterval);
        }
    }

    handleClientError(ws, error) {
        logger.error('WebSocket client error:', error);
    }

    handleServerError(error) {
        logger.error('WebSocket server error:', error);
    }

    // Client request handlers
    async handleGetActiveCalls(ws) {
        try {
            const calls = await activeCallManager.getActiveCalls();
            this.sendResponse(ws, 'active_calls', calls);
        } catch (err) {
            logger.error('Error getting active calls:', err);
            this.sendError(ws, 'Failed to get active calls');
        }
    }

    async handleGetSystemStatus(ws) {
        try {
            const systems = await systemManager.getActiveSystems();
            this.sendResponse(ws, 'system_status', systems);
        } catch (err) {
            logger.error('Error getting system status:', err);
            this.sendError(ws, 'Failed to get system status');
        }
    }

    // Recorder status handlers
    async handleGetRecorderStatus(ws, recorderId) {
        try {
            const recorderState = await recorderManager.getRecorderState(recorderId);
            this.sendResponse(ws, 'recorder_status', recorderState);
        } catch (err) {
            logger.error('Error getting recorder status:', err);
            this.sendError(ws, 'Failed to get recorder status');
        }
    }

    async handleGetRecorderStats(ws) {
        try {
            const stats = await recorderManager.getRecorderStats();
            this.sendResponse(ws, 'recorder_stats', stats);
        } catch (err) {
            logger.error('Error getting recorder stats:', err);
            this.sendError(ws, 'Failed to get recorder stats');
        }
    }

    async handleGetUnitStatus(ws, unit) {
        try {
            const unitState = await unitManager.getUnitState(unit);
            this.sendResponse(ws, 'unit_status', unitState);
        } catch (err) {
            logger.error('Error getting unit status:', err);
            this.sendError(ws, 'Failed to get unit status');
        }
    }

    // Subscription handlers
    handleSubscribe(ws, events) {
        if (!Array.isArray(events)) {
            return this.sendError(ws, 'Invalid events array', {
                code: 'INVALID_SUBSCRIPTION'
            });
        }

        const currentSubs = this.subscriptions.get(ws);
        events.forEach(event => currentSubs.add(event));
        
        this.sendResponse(ws, 'connection.status', {
            status: 'connected',
            subscriptions: Array.from(currentSubs)
        });
    }

    handleUnsubscribe(ws, events) {
        if (!Array.isArray(events)) {
            return this.sendError(ws, 'Invalid events array', {
                code: 'INVALID_UNSUBSCRIBE'
            });
        }

        const currentSubs = this.subscriptions.get(ws);
        events.forEach(event => currentSubs.delete(event));
        
        this.sendResponse(ws, 'connection.status', {
            status: 'connected',
            subscriptions: Array.from(currentSubs)
        });
    }

    handleAudioSubscribe(ws, data) {
        if (!data?.talkgroups || !Array.isArray(data.talkgroups)) {
            return this.sendError(ws, 'Invalid talkgroups array', {
                code: 'INVALID_AUDIO_SUBSCRIPTION'
            });
        }

        const audioSubs = this.audioSubscriptions.get(ws);
        data.talkgroups.forEach(talkgroup => {
            audioSubs.set(talkgroup, {
                format: data.format || 'm4a',
                options: data.options || {}
            });
        });

        this.sendResponse(ws, 'connection.status', {
            status: 'connected',
            audioSubscriptions: Array.from(audioSubs.keys())
        });
    }

    handleAudioUnsubscribe(ws, talkgroups) {
        const audioSubs = this.audioSubscriptions.get(ws);
        
        if (talkgroups && Array.isArray(talkgroups)) {
            talkgroups.forEach(talkgroup => audioSubs.delete(talkgroup));
        } else {
            audioSubs.clear();
        }

        this.sendResponse(ws, 'connection.status', {
            status: 'connected',
            audioSubscriptions: Array.from(audioSubs.keys())
        });
    }

    // Transcription subscription handlers
    handleTranscriptionSubscribe(ws, talkgroups) {
        if (!talkgroups || !Array.isArray(talkgroups)) {
            return this.sendError(ws, 'Invalid talkgroups array', {
                code: 'INVALID_TRANSCRIPTION_SUBSCRIPTION'
            });
        }

        const transcriptionSubs = this.transcriptionSubscriptions.get(ws);
        talkgroups.forEach(talkgroup => transcriptionSubs.add(talkgroup));
        
        this.sendResponse(ws, 'connection.status', {
            status: 'connected',
            transcriptionSubscriptions: Array.from(transcriptionSubs)
        });
    }

    handleTranscriptionUnsubscribe(ws, talkgroups) {
        const transcriptionSubs = this.transcriptionSubscriptions.get(ws);
        
        if (talkgroups && Array.isArray(talkgroups)) {
            talkgroups.forEach(talkgroup => transcriptionSubs.delete(talkgroup));
        } else {
            transcriptionSubs.clear();
        }

        this.sendResponse(ws, 'connection.status', {
            status: 'connected',
            transcriptionSubscriptions: Array.from(transcriptionSubs)
        });
    }

    // Helper methods for sending responses
    sendResponse(ws, type, data) {
        if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
                type,
                timestamp: new Date().toISOString(),
                data
            }));
        }
    }

    // Method to broadcast audio to subscribed clients
    broadcastAudio(audioData, metadata) {
        const talkgroup = metadata.talkgroup;
        
        this.wss.clients.forEach(client => {
            if (client.readyState !== WebSocket.OPEN) return;

            const audioSubs = this.audioSubscriptions.get(client);
            if (!audioSubs || !audioSubs.has(talkgroup)) return;

            const preferences = audioSubs.get(talkgroup);
            
            // Check emergency filter
            if (preferences.options.emergencyOnly && !metadata.emergency) return;

            // Prepare audio data based on format preference
            const format = preferences.format || 'm4a';
            const audioContent = format === 'wav' ? 
                audioData.audio_wav_base64 : 
                audioData.audio_m4a_base64;

            if (!audioContent) return;

            // Send audio to client
            this.sendResponse(client, 'audio.new', {
                callId: metadata.call_id,
                talkgroup: talkgroup,
                audioData: audioContent,
                format: format,
                metadata: preferences.options.includeMetadata ? metadata : undefined
            });
        });
    }

    // Method to broadcast transcriptions to subscribed clients
    broadcastTranscription(transcription) {
        const talkgroup = transcription.talkgroup;
        
        this.wss.clients.forEach(client => {
            if (client.readyState !== WebSocket.OPEN) return;

            const transcriptionSubs = this.transcriptionSubscriptions.get(client);
            if (!transcriptionSubs || !transcriptionSubs.has(talkgroup)) return;

            this.sendResponse(client, 'transcription.new', transcription);
        });
    }

    sendError(ws, message) {
        if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
                type: 'error',
                timestamp: new Date().toISOString(),
                error: message
            }));
        }
    }

    // Method to broadcast to all clients
    broadcast(type, data) {
        this.wss.clients.forEach(client => {
            if (client.readyState === WebSocket.OPEN) {
                client.send(JSON.stringify({
                    type,
                    timestamp: new Date().toISOString(),
                    data
                }));
            }
        });
    }
}

module.exports = WebSocketServer;
