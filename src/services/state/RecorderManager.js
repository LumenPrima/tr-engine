const mongoose = require('mongoose');
const logger = require('../../utils/logger');
const stateEventEmitter = require('../events/emitter');

class RecorderManager {
    constructor() {
        // In-memory cache of recorder states
        this.recorderStates = new Map(); // id -> state
        this.recordersByFreq = new Map(); // freq -> Set of recorder IDs
        this.recordingRecorders = new Set(); // Set of recorder IDs currently recording
        this.activeCalls = new Map(); // recorder ID -> call info

        // Initialize after MongoDB connection
        mongoose.connection.on('connected', () => {
            this.collection = mongoose.connection.db.collection('recorders');
            
            // Load existing data into cache
            this.loadCacheFromDB().catch(err => 
                logger.error('Error loading recorder cache from DB:', err)
            );
        });

        logger.info('RecorderManager initialized');
    }

    async loadCacheFromDB() {
        if (!this.collection) {
            logger.debug('MongoDB not available, skipping cache load');
            return;
        }

        logger.debug('Loading recorder cache from database...');
        const recorders = await this.collection.find({}).toArray();
        
        recorders.forEach(recorder => {
            this.recorderStates.set(recorder.id, recorder);
            
            // Cache frequency mapping
            if (recorder.freq) {
                if (!this.recordersByFreq.has(recorder.freq)) {
                    this.recordersByFreq.set(recorder.freq, new Set());
                }
                this.recordersByFreq.get(recorder.freq).add(recorder.id);
            }

            // Cache recording state
            if (recorder.rec_state === 1) { // RECORDING
                this.recordingRecorders.add(recorder.id);
            }
        });
        
        logger.info(`Loaded ${recorders.length} recorders into cache`);
    }

    async cleanup() {
        logger.debug('Cleaning up RecorderManager...');
        this.recorderStates.clear();
        this.recordersByFreq.clear();
        this.recordingRecorders.clear();
        logger.debug('Cleared in-memory caches');
        return Promise.resolve();
    }

    async processMessage(topic, message) {
        try {
            const timestamp = new Date(message.timestamp * 1000);
            const instanceId = message.instance_id;

            // Handle call start/end messages to track talkgroup info
            if (message.type === 'call_start') {
                const callInfo = {
                    talkgroup: message.talkgroup,
                    talkgroup_alpha_tag: message.talkgroup_alpha_tag,
                    talkgroup_description: message.talkgroup_description,
                    emergency: message.emergency
                };
                logger.debug('Storing call info:', {
                    type: 'call_start',
                    rec_num: message.rec_num,
                    callInfo: JSON.stringify(callInfo),
                    topic
                });
                this.activeCalls.set(message.rec_num.toString(), callInfo);

                // Find and update the recorder using this rec_num
                for (const [recorderId, recorderState] of this.recorderStates) {
                    if (recorderState.rec_num === message.rec_num) {
                        // Update the recorder with call info
                        const updatedState = {
                            ...recorderState,
                            current_call: {
                                ...callInfo,
                                talkgroup: parseInt(callInfo.talkgroup, 10)
                            }
                        };
                        this.recorderStates.set(recorderId, updatedState);
                        
                        // Emit update event
                        stateEventEmitter.emit('recorder.update', updatedState);
                        break;
                    }
                }
            } else if (message.type === 'call_end') {
                logger.debug('Removing call info:', {
                    type: 'call_end',
                    rec_num: message.rec_num,
                    topic
                });
                this.activeCalls.delete(message.rec_num.toString());

                // Find and update the recorder using this rec_num
                for (const [recorderId, recorderState] of this.recorderStates) {
                    if (recorderState.rec_num === message.rec_num) {
                        // Update the recorder to clear call info
                        const updatedState = {
                            ...recorderState,
                            current_call: null
                        };
                        this.recorderStates.set(recorderId, updatedState);
                        
                        // Emit update event
                        stateEventEmitter.emit('recorder.update', updatedState);
                        break;
                    }
                }
            }
            // Handle recorder state updates
            else if (topic.endsWith('/recorder')) {
                // Individual recorder update is flattened by message transformer
                logger.debug('Processing individual recorder update:', {
                    topic,
                    message: JSON.stringify(message),
                    timestamp,
                    instanceId
                });
                await this.updateRecorderState(message, timestamp, instanceId);
                logger.debug('Processed individual recorder update for:', message.id);
            } else if (topic.endsWith('/recorders') && Array.isArray(message.recorders)) {
                // Bulk update has array of recorders
                for (const recorder of message.recorders) {
                    await this.updateRecorderState(recorder, timestamp, instanceId);
                }
            } else {
                logger.debug('Skipping invalid recorder message:', message);
                return;
            }

            // Emit overall status update
            stateEventEmitter.emit('recorders.status', {
                total: this.recorderStates.size,
                recording: this.recordingRecorders.size,
                idle: Array.from(this.recorderStates.values()).filter(r => r.rec_state === 4).length,
                available: Array.from(this.recorderStates.values()).filter(r => r.rec_state === 7).length,
                timestamp: timestamp
            });

        } catch (err) {
            logger.error('Error processing recorders message:', err);
            throw err;
        }
    }

    async updateRecorderState(recorderData, timestamp, instanceId) {
        try {
            if (!this.collection) {
                logger.warn('MongoDB collection not ready yet');
                return;
            }

            const currentState = this.recorderStates.get(recorderData.id);
            
            // Create new state object with all fields
            const newState = {
                ...currentState, // Preserve existing state including current_call
                id: recorderData.id,
                src_num: recorderData.src_num,
                rec_num: recorderData.rec_num,
                type: recorderData.type,
                duration: recorderData.duration,
                freq: recorderData.freq,
                count: recorderData.count,
                rec_state: recorderData.rec_state,
                rec_state_type: recorderData.rec_state_type,
                squelched: recorderData.squelched,
                instance_id: instanceId,
                status: {
                    last_update: timestamp
                }
            };

            // Only update current_call if we have new call info
            const callInfo = this.activeCalls.get(recorderData.rec_num.toString());
            if (callInfo) {
                newState.current_call = {
                    ...callInfo,
                    talkgroup: parseInt(callInfo.talkgroup, 10)
                };
            }

            // Update frequency mapping
            if (currentState?.freq !== recorderData.freq) {
                // Remove from old frequency set
                if (currentState?.freq) {
                    const oldFreqSet = this.recordersByFreq.get(currentState.freq);
                    if (oldFreqSet) {
                        oldFreqSet.delete(recorderData.id);
                        if (oldFreqSet.size === 0) {
                            this.recordersByFreq.delete(currentState.freq);
                        }
                    }
                }
                
                // Add to new frequency set
                if (recorderData.freq) {
                    if (!this.recordersByFreq.has(recorderData.freq)) {
                        this.recordersByFreq.set(recorderData.freq, new Set());
                    }
                    this.recordersByFreq.get(recorderData.freq).add(recorderData.id);
                }
            }

            // Update recording state tracking
            const wasRecording = this.recordingRecorders.has(recorderData.id);
            const isRecording = recorderData.rec_state === 1;

            if (wasRecording !== isRecording) {
                if (isRecording) {
                    this.recordingRecorders.add(recorderData.id);
                } else {
                    this.recordingRecorders.delete(recorderData.id);
                }

                // Emit state change event
                stateEventEmitter.emit('recorder.stateChange', {
                    id: recorderData.id,
                    previousState: currentState?.rec_state_type || 'UNKNOWN',
                    newState: recorderData.rec_state_type,
                    freq: recorderData.freq,
                    timestamp: timestamp
                });
            }

            // Update cache and database
            this.recorderStates.set(recorderData.id, newState);
            await this.collection.updateOne(
                { id: recorderData.id },
                { $set: newState },
                { upsert: true }
            );

            // Emit individual recorder update
            stateEventEmitter.emit('recorder.update', newState);

        } catch (err) {
            logger.error('Error updating recorder state:', err);
            throw err;
        }
    }

    getRecorderState(id) {
        return this.recorderStates.get(id) || null;
    }

    getAllRecorderStates() {
        return Array.from(this.recorderStates.values());
    }

    getRecordersOnFreq(freq) {
        const recorderIds = this.recordersByFreq.get(freq) || new Set();
        return Array.from(recorderIds).map(id => this.recorderStates.get(id)).filter(Boolean);
    }

    getActiveRecorders() {
        return Array.from(this.recordingRecorders)
            .map(id => this.recorderStates.get(id))
            .filter(Boolean);
    }

    getRecorderStats() {
        const states = Array.from(this.recorderStates.values());
        return {
            total: states.length,
            recording: this.recordingRecorders.size,
            idle: states.filter(r => r.rec_state === 4).length,
            available: states.filter(r => r.rec_state === 7).length,
            timestamp: new Date()
        };
    }
}

// Export singleton instance
const recorderManager = new RecorderManager();
module.exports = recorderManager;
