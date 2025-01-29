const logger = require('../../utils/logger');
const stateEventEmitter = require('./emitter');

class EventHandlers {
    constructor(wss) {
        this.wss = wss;
        this.setupEventHandlers();
        logger.info('EventHandlers initialized');
    }

    setupEventHandlers() {
        // Call events
        stateEventEmitter.on('call.start', this.handleCallStart.bind(this));
        stateEventEmitter.on('call.update', this.handleCallUpdate.bind(this));
        stateEventEmitter.on('call.end', this.handleCallEnd.bind(this));
        stateEventEmitter.on('audio.start', this.handleAudioStart.bind(this));
        stateEventEmitter.on('audio.complete', this.handleAudioComplete.bind(this));

        // System events
        stateEventEmitter.on('system.update', this.handleSystemUpdate.bind(this));
        stateEventEmitter.on('system.rates', this.handleSystemRates.bind(this));
        stateEventEmitter.on('system.config', this.handleSystemConfig.bind(this));

        // Unit events
        stateEventEmitter.on('unit.activity', this.handleUnitActivity.bind(this));
        stateEventEmitter.on('unit.location', this.handleUnitLocation.bind(this));
        stateEventEmitter.on('unit.status', this.handleUnitStatus.bind(this));

        // Transcription events
        stateEventEmitter.on('transcription.new', this.handleTranscriptionNew.bind(this));

        // Error events
        stateEventEmitter.on('error', this.handleError.bind(this));
    }

    // Helper method to broadcast to all connected clients
    broadcast(eventType, data) {
        if (!this.wss) {
            logger.warn('WebSocket server not initialized');
            return;
        }

        const message = JSON.stringify({
            type: eventType,
            timestamp: new Date().toISOString(),
            data
        });

        let clientCount = 0;
        this.wss.clients.forEach(client => {
            if (client.readyState === 1) { // WebSocket.OPEN
                client.send(message);
                clientCount++;
            }
        });

        logger.debug(`Broadcasted ${eventType} event to ${clientCount} clients`);
    }

    // Transcription event handler
    handleTranscriptionNew(transcriptionData) {
        try {
            logger.debug('Processing transcription.new event');
            this.broadcast('transcription.new', transcriptionData);
        } catch (err) {
            logger.error('Error handling transcription.new event:', err);
        }
    }

    // Call event handlers
    handleCallStart(callData) {
        try {
            logger.debug('Processing call.start event');
            this.broadcast('call.start', callData);
        } catch (err) {
            logger.error('Error handling call.start event:', err);
        }
    }

    handleCallUpdate(callData) {
        try {
            logger.debug('Processing call.update event');
            this.broadcast('call.update', callData);
        } catch (err) {
            logger.error('Error handling call.update event:', err);
        }
    }

    handleCallEnd(callData) {
        try {
            logger.debug('Processing call.end event');
            this.broadcast('call.end', callData);
        } catch (err) {
            logger.error('Error handling call.end event:', err);
        }
    }

    handleAudioStart(audioData) {
        try {
            logger.debug('Processing audio.start event');
            this.broadcast('audio.start', audioData);
        } catch (err) {
            logger.error('Error handling audio.start event:', err);
        }
    }

    handleAudioComplete(audioData) {
        try {
            logger.debug('Processing audio.complete event');
            this.broadcast('audio.complete', audioData);
        } catch (err) {
            logger.error('Error handling audio.complete event:', err);
        }
    }

    // System event handlers
    handleSystemUpdate(systemData) {
        try {
            logger.debug('Processing system.update event');
            this.broadcast('system.update', systemData);
        } catch (err) {
            logger.error('Error handling system.update event:', err);
        }
    }

    handleSystemRates(ratesData) {
        try {
            logger.debug('Processing system.rates event');
            this.broadcast('system.rates', ratesData);
        } catch (err) {
            logger.error('Error handling system.rates event:', err);
        }
    }

    handleSystemConfig(configData) {
        try {
            logger.debug('Processing system.config event');
            this.broadcast('system.config', configData);
        } catch (err) {
            logger.error('Error handling system.config event:', err);
        }
    }

    // Unit event handlers
    handleUnitActivity(unitData) {
        try {
            logger.debug('Processing unit.activity event');
            this.broadcast('unit.activity', unitData);
        } catch (err) {
            logger.error('Error handling unit.activity event:', err);
        }
    }

    handleUnitLocation(locationData) {
        try {
            logger.debug('Processing unit.location event');
            this.broadcast('unit.location', locationData);
        } catch (err) {
            logger.error('Error handling unit.location event:', err);
        }
    }

    handleUnitStatus(statusData) {
        try {
            logger.debug('Processing unit.status event');
            this.broadcast('unit.status', statusData);
        } catch (err) {
            logger.error('Error handling unit.status event:', err);
        }
    }

    // Error handler
    handleError(error) {
        logger.error('Processing error event:', error);
        this.broadcast('error', {
            message: error.message,
            timestamp: new Date().toISOString()
        });
    }
}

module.exports = EventHandlers;
