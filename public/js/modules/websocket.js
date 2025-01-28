import { getWsBaseUrl } from '../utils.js';

// WebSocket connection manager
class WebSocketManager {
    constructor() {
        this.ws = null;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectDelay = 1000; // Start with 1 second
        this.subscriptions = new Set();
        this.audioSubscriptions = new Map(); // talkgroup -> options
        this.handlers = new Map();
        this.isConnected = false;
        this.pendingReconnect = null;
    }

    connect() {
        const wsUrl = getWsBaseUrl();
        console.log('Connecting to WebSocket at:', wsUrl);
        this.ws = new WebSocket(wsUrl);
        this.setupEventHandlers();
    }

    setupEventHandlers() {
        this.ws.onopen = () => {
            console.log('WebSocket connected');
            this.isConnected = true;
            this.reconnectAttempts = 0;
            this.reconnectDelay = 1000;

            // Resubscribe to previous subscriptions
            if (this.subscriptions.size > 0) {
                this.ws.send(JSON.stringify({
                    type: 'subscribe',
                    timestamp: new Date().toISOString(),
                    data: {
                        events: Array.from(this.subscriptions)
                    }
                }));
            }

            // Resubscribe to audio channels
            if (this.audioSubscriptions.size > 0) {
                this.ws.send(JSON.stringify({
                    type: 'audio.subscribe',
                    timestamp: new Date().toISOString(),
                    data: {
                        talkgroups: Array.from(this.audioSubscriptions.keys()),
                        format: 'm4a', // Default to m4a
                        options: {
                            includeMetadata: true
                        }
                    }
                }));
            }
        };

        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            this.isConnected = false;
            this.handleReconnect();
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };

        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                this.handleMessage(message);
            } catch (error) {
                console.error('Error handling WebSocket message:', error);
            }
        };
    }

    handleReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('Max reconnection attempts reached');
            return;
        }

        if (this.pendingReconnect) {
            clearTimeout(this.pendingReconnect);
        }

        this.pendingReconnect = setTimeout(() => {
            this.reconnectAttempts++;
            this.reconnectDelay = Math.min(this.reconnectDelay * 2, 30000); // Max 30 seconds
            console.log(`Attempting to reconnect (${this.reconnectAttempts}/${this.maxReconnectAttempts})...`);
            this.connect();
        }, this.reconnectDelay);
    }

    subscribe(events) {
        if (!Array.isArray(events)) {
            events = [events];
        }

        events.forEach(event => this.subscriptions.add(event));

        if (this.isConnected) {
            this.ws.send(JSON.stringify({
                type: 'subscribe',
                timestamp: new Date().toISOString(),
                data: { events }
            }));
        }
    }

    unsubscribe(events) {
        if (!Array.isArray(events)) {
            events = [events];
        }

        events.forEach(event => this.subscriptions.delete(event));

        if (this.isConnected) {
            this.ws.send(JSON.stringify({
                type: 'unsubscribe',
                timestamp: new Date().toISOString(),
                data: { events }
            }));
        }
    }

    subscribeToAudio(talkgroups, options = {}) {
        if (!Array.isArray(talkgroups)) {
            talkgroups = [talkgroups];
        }

        talkgroups.forEach(talkgroup => {
            this.audioSubscriptions.set(talkgroup, options);
        });

        if (this.isConnected) {
            this.ws.send(JSON.stringify({
                type: 'audio.subscribe',
                timestamp: new Date().toISOString(),
                data: {
                    talkgroups,
                    format: options.format || 'm4a',
                    options: {
                        emergencyOnly: options.emergencyOnly || false,
                        includeMetadata: options.includeMetadata || true
                    }
                }
            }));
        }
    }

    unsubscribeFromAudio(talkgroups) {
        if (!talkgroups) {
            this.audioSubscriptions.clear();
        } else {
            if (!Array.isArray(talkgroups)) {
                talkgroups = [talkgroups];
            }
            talkgroups.forEach(talkgroup => {
                this.audioSubscriptions.delete(talkgroup);
            });
        }

        if (this.isConnected) {
            this.ws.send(JSON.stringify({
                type: 'audio.unsubscribe',
                timestamp: new Date().toISOString(),
                data: { talkgroups }
            }));
        }
    }

    on(type, handler) {
        if (!this.handlers.has(type)) {
            this.handlers.set(type, new Set());
        }
        this.handlers.get(type).add(handler);
    }

    off(type, handler) {
        if (this.handlers.has(type)) {
            this.handlers.get(type).delete(handler);
        }
    }

    handleMessage(message) {
        // Handle heartbeat messages
        if (message.type === 'heartbeat') {
            this.ws.send(JSON.stringify({
                type: 'heartbeat',
                timestamp: new Date().toISOString()
            }));
            return;
        }

        // Handle connection status updates
        if (message.type === 'connection.status') {
            console.log('Connection status:', message.data);
            return;
        }

        // Handle audio messages
        if (message.type === 'audio.new') {
            this.handleAudioMessage(message.data);
            return;
        }

        // Notify all handlers for this message type
        const handlers = this.handlers.get(message.type);
        if (handlers) {
            handlers.forEach(handler => {
                try {
                    handler(message.data);
                } catch (error) {
                    console.error('Error in message handler:', error);
                }
            });
        }
    }

    handleAudioMessage(data) {
        // Create audio element
        const audio = new Audio();
        
        // Convert base64 to blob
        const byteCharacters = atob(data.audioData);
        const byteNumbers = new Array(byteCharacters.length);
        for (let i = 0; i < byteCharacters.length; i++) {
            byteNumbers[i] = byteCharacters.charCodeAt(i);
        }
        const byteArray = new Uint8Array(byteNumbers);
        const blob = new Blob([byteArray], { 
            type: data.format === 'wav' ? 'audio/wav' : 'audio/mp4' 
        });
        
        // Create object URL and set as audio source
        const url = URL.createObjectURL(blob);
        audio.src = url;
        
        // Clean up object URL after audio loads
        audio.onload = () => {
            URL.revokeObjectURL(url);
        };

        // Notify handlers
        const handlers = this.handlers.get('audio.new');
        if (handlers) {
            handlers.forEach(handler => {
                try {
                    handler({ audio, metadata: data.metadata });
                } catch (error) {
                    console.error('Error in audio handler:', error);
                }
            });
        }

        // Auto-play if no handlers are registered
        if (!handlers || handlers.size === 0) {
            audio.play().catch(console.error);
        }
    }

    close() {
        if (this.ws) {
            this.ws.close();
        }
        if (this.pendingReconnect) {
            clearTimeout(this.pendingReconnect);
        }
    }
}

// Create and export singleton instance
const wsManager = new WebSocketManager();
export default wsManager;
