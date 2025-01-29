const EventEmitter = require('events');
const logger = require('../../utils/logger');

class StateEventEmitter extends EventEmitter {
    constructor() {
        super();
        this.setMaxListeners(0); // Allow unlimited listeners
        logger.info('StateEventEmitter initialized');
    }

    // Call state events
    emitCallStart(callData) {
        logger.debug(`Emitting call.start event for call ${callData.call_id}`);
        this.emit('call.start', callData);
    }

    emitCallUpdate(callData) {
        logger.debug(`Emitting call.update event for call ${callData.call_id}`);
        this.emit('call.update', callData);
    }

    emitCallEnd(callData) {
        logger.debug(`Emitting call.end event for call ${callData.call_id}`);
        this.emit('call.end', callData);
    }

    emitAudioStart(audioData) {
        logger.debug(`Emitting audio.start event for call ${audioData.call_id}`);
        this.emit('audio.start', audioData);
    }

    emitAudioComplete(audioData) {
        logger.debug(`Emitting audio.complete event for call ${audioData.call_id}`);
        this.emit('audio.complete', audioData);
    }

    // System state events
    emitSystemUpdate(systemData) {
        logger.debug(`Emitting system.update event for system ${systemData.sys_name}`);
        this.emit('system.update', systemData);
    }

    emitSystemRates(ratesData) {
        logger.debug(`Emitting system.rates event for system ${ratesData.sys_name}`);
        this.emit('system.rates', ratesData);
    }

    emitSystemConfig(configData) {
        logger.debug(`Emitting system.config event for system ${configData.sys_name}`);
        this.emit('system.config', configData);
    }

    // Unit state events
    emitUnitActivity(unitData) {
        logger.debug(`Emitting unit.activity event for unit ${unitData.unit}`);
        this.emit('unit.activity', unitData);
    }

    emitUnitLocation(locationData) {
        logger.debug(`Emitting unit.location event for unit ${locationData.unit}`);
        this.emit('unit.location', locationData);
    }

    emitUnitStatus(statusData) {
        logger.debug(`Emitting unit.status event for unit ${statusData.unit}`);
        this.emit('unit.status', statusData);
    }

    // Error events
    emitError(error) {
        logger.error('Emitting error event:', error);
        this.emit('error', error);
    }

    // Helper method to emit any event
    emitEvent(eventName, data) {
        logger.debug(`Emitting ${eventName} event`);
        this.emit(eventName, data);
    }

    // Transcription event
    emitTranscriptionNew(transcriptionData) {
        logger.debug(`Emitting transcription.new event for call ${transcriptionData.call_id}`);
        this.emit('transcription.new', transcriptionData);
    }
}

// Export singleton instance
const stateEventEmitter = new StateEventEmitter();
module.exports = stateEventEmitter;
