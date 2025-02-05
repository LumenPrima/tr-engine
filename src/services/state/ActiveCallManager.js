const logger = require('../../utils/logger');
const timestamps = require('../../utils/timestamps');
const stateEventEmitter = require('../events/emitter');
const config = require('../../config');

class ActiveCallManager {
    constructor() {
        // In-memory cache of active calls from calls_active messages
        this.activeCallsCache = new Map();
        
        // Additional metadata not in calls_active
        this.audioFiles = new Map(); // call_id -> filename
        this.participatingUnits = new Map(); // call_id -> Set of units
        
        // Track the state of all recorders
        this.recorderStates = new Map();

        logger.info('ActiveCallManager initialized');
    }

    async cleanup() {
        logger.debug('Cleaning up ActiveCallManager...');
        this.activeCallsCache.clear();
        this.recorderStates.clear();
        return Promise.resolve();
    }

    async processMessage(topic, message, messageId) {
        try {
            const topicParts = topic.split('/');
            const messageType = topicParts[2];
            
            logger.debug(`Processing ${messageType} message for active calls`, {
                topic,
                messageType,
                messageId,
                hasMessage: !!message,
                messageKeys: message ? Object.keys(message) : [],
                messageContent: JSON.stringify(message)
            });
            
            switch(messageType) {
                case 'call_start':
                    logger.debug('Call start message content:', {
                        hasCall: !!message?.call,
                        messageContent: JSON.stringify(message)
                    });
                    await this.handleCallStart(message, messageId);
                    break;
                case 'recorder':
                    await this.handleRecorderUpdate(message, messageId);
                    break;
                case 'recorders':
                    await this.handleRecordersUpdate(message, messageId);
                    break;
                case 'call_start':
                    await this.handleCallStart(message, messageId);
                    break;
                case 'calls_active':
                    await this.handleCallsActive(message, messageId);
                    break;
                case 'audio':
                    await this.handleAudioMessage(message, messageId);
                    break;
                case 'call_end':
                    await this.handleCallEnd(message, messageId);
                    break;
                default:
                    if (topic.startsWith(`${config.mqtt.topicPrefix}/units/`)) {
                        await this.handleUnitMessage(topic, message, messageId);
                    }
            }
            
            // Update statistics for affected system
            const sysName = message.sys_name;
            if (sysName) {
                await this.updateStatistics(sysName);
            }
        } catch (err) {
            logger.error('Error processing message in ActiveCallManager:', err);
            throw err;
        }
    }
    
    handleCallStart(message, messageId) {
        try {
            if (!message.id) {
                logger.debug('Received message:', JSON.stringify(message));
                logger.warn('Call start message missing id');
                return;
            }

            const callId = message.id;
            
            logger.debug(`Processing call start for ${callId}`, {
                message_structure: {
                    has_id: !!message.id,
                    message_fields: Object.keys(message)
                }
            });

            // Add unit to participating units
            if (message.unit) {
                if (!this.participatingUnits.has(callId)) {
                    this.participatingUnits.set(callId, new Set());
                }
                this.participatingUnits.get(callId).add(message.unit);
            }

            // Only emit event if this is a new call
            if (!this.activeCallsCache.has(callId)) {
                stateEventEmitter.emitCallStart({
                    call_id: callId,
                    ...message,
                    participating_units: Array.from(this.participatingUnits.get(callId) || [])
                });
            }
            
            logger.info(`Call start processed: ${callId}`);
        } catch (err) {
            logger.error('Error handling call start:', err);
            throw err;
        }
    }
    
    handleCallsActive(message, messageId) {
        try {
            // Update cache with current calls
            const currentCallIds = new Set();
            
            // Process each active call from the calls array
            if (message.calls && Array.isArray(message.calls)) {
                message.calls.forEach(call => {
                    if (!call || !call.id) return;
                    
                    const callId = call.id;
                    currentCallIds.add(callId);
                    
                    // Add metadata from our tracking
                    const enrichedCall = {
                        ...call,
                        audio_file: this.audioFiles.get(callId),
                        participating_units: Array.from(this.participatingUnits.get(callId) || [])
                    };
                    
                    // Check if call data has changed
                    const existingCall = this.activeCallsCache.get(callId);
                    if (!existingCall || JSON.stringify(existingCall) !== JSON.stringify(enrichedCall)) {
                        this.activeCallsCache.set(callId, enrichedCall);
                        stateEventEmitter.emitCallUpdate(enrichedCall);
                    }
                });
            }
            
            // Remove calls no longer active
            for (const [callId, call] of this.activeCallsCache.entries()) {
                if (!currentCallIds.has(callId)) {
                    stateEventEmitter.emitCallEnd({
                        call_id: callId,
                        ...call
                    });
                    this.activeCallsCache.delete(callId);
                    this.audioFiles.delete(callId);
                    this.participatingUnits.delete(callId);
                }
            }
        } catch (err) {
            logger.error('Error handling active calls:', err);
            throw err;
        }
    }
    
    handleUnitMessage(topic, message, messageId) {
        try {
            if (!message.talkgroup) return;
            
            logger.debug(`Processing unit message for talkgroup ${message.talkgroup}`);
            
            // Find all active calls for this talkgroup
            for (const [callId, call] of this.activeCallsCache.entries()) {
                if (call.talkgroup === message.talkgroup && call.sys_name === message.sys_name) {
                    // Add unit to participating units
                    if (!this.participatingUnits.has(callId)) {
                        this.participatingUnits.set(callId, new Set());
                    }
                    this.participatingUnits.get(callId).add(message.unit);
                    
                    // Emit unit activity event
                    stateEventEmitter.emitUnitActivity({
                        unit: message.unit,
                        unit_alpha_tag: message.unit_alpha_tag,
                        activity_type: message.type,
                        call_id: callId,
                        talkgroup: message.talkgroup,
                        sys_name: message.sys_name,
                        participating_units: Array.from(this.participatingUnits.get(callId))
                    });
                    
                    logger.debug(`Updated call ${callId} with unit ${message.unit}`);
                }
            }
        } catch (err) {
            logger.error('Error handling unit message:', err);
            throw err;
        }
    }
    handleAudioMessage(message, messageId) {
        //this is handled elsewhere
    }
    handleCallEnd(message, messageId) {
        try {
            const callId = message.id;
            
            logger.debug(`Processing call end for ${callId}`);
            
            // Get cached call data before deletion
            const cachedCallData = this.activeCallsCache.get(callId);
            
            // Remove from caches
            this.activeCallsCache.delete(callId);
            this.audioFiles.delete(callId);
            this.participatingUnits.delete(callId);
            
            // Emit call end event with combined data
            stateEventEmitter.emitCallEnd({
                call_id: callId,
                ...(cachedCallData || {}), // Use cached data if available
                ...message // Override with final call data
            });
            
            logger.info(`Call ended and removed from active calls: ${callId}`);
        } catch (err) {
            logger.error('Error handling call end:', err);
            throw err;
        }
    }
    
    handleRecorderUpdate(message, messageId) {
        try {
            // Update our recorder state cache
            this.recorderStates.set(message.id, {
                ...message,
                last_update: timestamps.getCurrentUnix()
            });

            logger.debug(`Recorder ${message.id} state updated to ${message.rec_state_type}`);

            // If the recorder is RECORDING, update matching calls
            if (message.rec_state_type === 'RECORDING') {
                // Find calls on this frequency without a recorder
                for (const [callId, call] of this.activeCallsCache.entries()) {
                    if (call.freq === message.freq && !call.recorder) {
                        // Update call with recorder info
                        const updatedCall = {
                            ...call,
                            recorder: {
                                id: message.id,
                                src_num: message.src_num,
                                rec_num: message.rec_num,
                                state: message.rec_state_type,
                                freq: message.freq,
                                squelched: message.squelched
                            }
                        };
                        
                        this.activeCallsCache.set(callId, updatedCall);
                        stateEventEmitter.emitCallUpdate(updatedCall);
                        
                        logger.debug(`Updated call ${callId} with recorder ${message.id}`);
                        break; // Only update first matching call
                    }
                }
            }
        } catch (err) {
            logger.error('Error handling recorder update:', err);
            throw err;
        }
    }

    handleRecordersUpdate(message, messageId) {
        try {
            // Get recorders array from message, ensure it exists and is an array
            const recorders = Array.isArray(message.recorders) ? message.recorders : 
                            (message.recorders ? [message.recorders] : []);
            
            // Update state for all recorders
            recorders.forEach(recorder => {
                if (!recorder || !recorder.id) return;
                this.recorderStates.set(recorder.id, {
                    ...recorder,
                    last_update: timestamps.getCurrentUnix()
                });
            });
    
            logger.debug('Updated states for all recorders');
    
            // Update all active calls with matching recorders
            for (const [callId, call] of this.activeCallsCache.entries()) {
                const matchingRecorder = recorders.find(rec => 
                    rec && rec.freq === call.freq && rec.rec_state_type === 'RECORDING'
                );
    
                if (matchingRecorder) {
                    const updatedCall = {
                        ...call,
                        recorder: {
                            id: matchingRecorder.id,
                            src_num: matchingRecorder.src_num,
                            rec_num: matchingRecorder.rec_num,
                            state: matchingRecorder.rec_state_type,
                            freq: matchingRecorder.freq,
                            squelched: matchingRecorder.squelched
                        }
                    };
                    
                    this.activeCallsCache.set(callId, updatedCall);
                    stateEventEmitter.emitCallUpdate(updatedCall);
                    
                    logger.debug(`Matched recorder ${matchingRecorder.id} to call ${callId}`);
                }
            }
        } catch (err) {
            logger.error('Error handling recorders update:', err);
            throw err;
        }
    }

    updateStatistics(sysName = null) {
        try {
            // Clean up stale calls for system
            this.cleanupStaleCalls(sysName);

            // Update statistics for all matching calls
            let updatedCount = 0;
            for (const [callId, call] of this.activeCallsCache.entries()) {
                if (!sysName || call.sys_name === sysName) {
                    const updatedCall = {
                        ...call,
                        duration: timestamps.diffSeconds(
                            timestamps.getCurrentUnix(),
                            call.start_time
                        ),
                        unit_count: this.participatingUnits.get(callId)?.size || 0
                    };
                    
                    this.activeCallsCache.set(callId, updatedCall);
                    stateEventEmitter.emitCallUpdate(updatedCall);
                    updatedCount++;
                }
            }
            
            logger.debug(`Updated statistics for ${updatedCount} active calls`);
        } catch (err) {
            logger.error('Error updating call statistics:', err);
            throw err;
        }
    }
    
    cleanupStaleCalls(sysName = null) {
        try {
            const staleThreshold = timestamps.addSeconds(
                timestamps.getCurrentUnix(),
                -300 // 5 minutes ago
            );
            let staleCount = 0;
            
            // Find and remove stale calls
            for (const [callId, call] of this.activeCallsCache.entries()) {
                if ((!sysName || call.sys_name === sysName) && 
                    call.start_time < staleThreshold) {
                    this.activeCallsCache.delete(callId);
                    this.audioFiles.delete(callId);
                    this.participatingUnits.delete(callId);
                    staleCount++;
                    
                    logger.debug(`Removing stale call ${callId}`);
                }
            }
            
            if (staleCount > 0) {
                logger.info(`Cleaned up ${staleCount} stale calls`);
            }
        } catch (err) {
            logger.error('Error cleaning up stale calls:', err);
        }
    }

    getActiveCalls(filter = {}) {
        try {
            // Clean up stale calls
            this.cleanupStaleCalls(filter.sys_name);

            // Filter and return active calls
            return Array.from(this.activeCallsCache.values())
                .filter(call => {
                    if (filter.sys_name && call.sys_name !== filter.sys_name) return false;
                    if (filter.talkgroup && call.talkgroup !== filter.talkgroup) return false;
                    if (filter.emergency === true && !call.emergency) return false;
                    return true;
                })
                .sort((a, b) => b.start_time - a.start_time);
        } catch (err) {
            logger.error('Error retrieving active calls:', err);
            throw err;
        }
    }

    clearActiveCalls() {
        try {
            this.activeCallsCache.clear();
            this.audioFiles.clear();
            this.participatingUnits.clear();
            logger.debug('Cleared all active calls and related data');
        } catch (err) {
            logger.error('Error clearing active calls:', err);
            throw err;
        }
    }
}

// Export singleton instance
const activeCallManager = new ActiveCallManager();
module.exports = activeCallManager;
