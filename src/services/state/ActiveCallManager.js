const mongoose = require('mongoose');
const logger = require('../../utils/logger');

// Schema for real-time active calls view
const ActiveCallSchema = new mongoose.Schema({
    call_id: { type: String, required: true, unique: true },
    sys_num: Number,
    sys_name: String,
    start_time: { type: Date, required: true },
    last_update: { type: Date, required: true },
    
    // Call details
    talkgroup: Number,
    talkgroup_alpha_tag: String,
    talkgroup_tag: String,
    talkgroup_description: String,
    talkgroup_group: String,
    freq: Number,
    
    // Status flags
    emergency: Boolean,
    encrypted: Boolean,
    
    // Audio information
    audio_type: String,
    has_audio: Boolean,
    audio_file: String,
    
    // Recorder information
    recorder: {
        id: String,           // Recorder identifier (e.g. "0_0")
        src_num: Number,      // Source number
        rec_num: Number,      // Recorder number
        state: String,        // Current state of recording
        freq: Number,         // Frequency being recorded
        squelched: Boolean    // Squelch status
    },
    
    // Participating units (updated from unit messages)
    units: [{
        unit: Number,
        unit_alpha_tag: String,
        joined_at: Date,
        last_seen: Date,
        status: String
    }],
    
    // Source message references
    sources: [{
        type: { type: String, enum: ['call_start', 'calls_active', 'audio', 'unit'] },
        message_id: mongoose.Schema.Types.ObjectId,
        timestamp: Date
    }],
    
    // Derived statistics
    duration: Number,         // Current duration in seconds
    unit_count: Number,       // Number of unique units
    audio_segments: Number    // Number of audio segments received
}, {
    timestamps: true // Adds createdAt and updatedAt
});

// Indexes for efficient querying of active calls
ActiveCallSchema.index({ sys_name: 1, start_time: -1 });
ActiveCallSchema.index({ talkgroup: 1, start_time: -1 });
ActiveCallSchema.index({ last_update: 1 });
ActiveCallSchema.index({ emergency: 1 });

class ActiveCallManager {
    constructor() {
        console.log('[ActiveCallManager] Initializing...');
        this.ActiveCall = mongoose.model('ActiveCall', ActiveCallSchema);
        console.log('[ActiveCallManager] MongoDB model registered');
        
        // In-memory cache of active calls for fastest possible access
        this.activeCallsCache = new Map();
        
        // Track the state of all recorders
        this.recorderStates = new Map();

        console.log('[ActiveCallManager] In-memory caches initialized');
        logger.info('ActiveCallManager initialized');
    }

    async cleanup() {
        logger.debug('Cleaning up ActiveCallManager...');
        this.activeCallsCache.clear();
        this.recorderStates.clear();
    }

    async processMessage(topic, message, messageId) {
        try {
            const topicParts = topic.split('/');
            const messageType = topicParts[2];
            
            logger.debug(`Processing ${messageType} message for active calls`);
            
            switch(messageType) {
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
                    if (topic.startsWith('tr-mqtt/units/')) {
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
    
    async handleCallStart(message, messageId) {
        try {
            const callId = `${message.sys_num}_${message.talkgroup}_${message.start_time}`;
            
            console.log(`[ActiveCallManager] Processing new call start: ${callId}`);
            logger.debug(`Processing call start for ${callId}`);

            // Use findOneAndUpdate with upsert to handle both new and existing calls
            const activeCall = await this.ActiveCall.findOneAndUpdate(
                { call_id: callId },
                {
                    $setOnInsert: {
                        sys_num: message.sys_num,
                        sys_name: message.sys_name,
                        start_time: new Date(message.start_time * 1000),
                        talkgroup: message.talkgroup,
                        talkgroup_alpha_tag: message.talkgroup_alpha_tag,
                        talkgroup_tag: message.talkgroup_tag,
                        talkgroup_description: message.talkgroup_description,
                        talkgroup_group: message.talkgroup_group
                    },
                    $set: {
                        last_update: new Date(),
                        freq: message.freq,
                        emergency: message.emergency || false,
                        encrypted: message.encrypted || false
                    },
                    $addToSet: {
                        units: {
                            unit: message.unit,
                            unit_alpha_tag: message.unit_alpha_tag,
                            joined_at: new Date(),
                            last_seen: new Date(),
                            status: 'initiator'
                        }
                    },
                    $push: {
                        sources: {
                            type: 'call_start',
                            message_id: messageId,
                            timestamp: new Date()
                        }
                    }
                },
                { 
                    upsert: true,
                    new: true
                }
            );

            // Update cache with new/updated call
            this.activeCallsCache.set(callId, activeCall);
            logger.info(activeCall.isNew ? 
                `New active call created: ${callId}` : 
                `Updated existing call: ${callId}`);
        } catch (err) {
            logger.error('Error handling call start:', err);
            throw err;
        }
    }
    
    async handleCallsActive(message, messageId) {
        try {
            // If calls is null, remove all calls for this system
            if (!message.calls) {
                const sysName = message.instance_id?.split('-')[0];
                if (!sysName) {
                    logger.warn('No system name found in instance_id for calls_active message');
                    return;
                }
                
                const existingCalls = await this.ActiveCall.find({ sys_name: sysName });
                
                if (existingCalls.length > 0) {
                    logger.info(`Removing ${existingCalls.length} active calls for system ${sysName} due to null calls message`);
                    await this.ActiveCall.deleteMany({ sys_name: sysName });
                    existingCalls.forEach(call => {
                        this.activeCallsCache.delete(call.call_id);
                    });
                }
                return;
            }

            // Handle both array and single call formats
            const currentCalls = Array.isArray(message.calls) ? message.calls : [message.calls].filter(Boolean);
            if (currentCalls.length === 0) return;
            
            // Get all existing calls for this system
            const existingCalls = await this.ActiveCall.find({
                sys_name: currentCalls[0].sys_name
            });

            // Create a set of current call IDs
            const currentCallIds = new Set(currentCalls.map(call => 
                `${call.sys_num}_${call.talkgroup}_${call.start_time}`
            ));

            // Remove calls that are no longer active
            const callsToRemove = existingCalls.filter(call => !currentCallIds.has(call.call_id));
            if (callsToRemove.length > 0) {
                logger.info(`Removing ${callsToRemove.length} calls no longer in calls_active message`);
                await Promise.all(callsToRemove.map(async (call) => {
                    logger.debug(`Removing inactive call ${call.call_id}`);
                    await this.ActiveCall.deleteOne({ _id: call._id });
                    this.activeCallsCache.delete(call.call_id);
                }));
            }

            // Update or create current calls
            await Promise.all(currentCalls.map(async (call) => {
                if (!call) return;
                
                const callId = `${call.sys_num}_${call.talkgroup}_${call.start_time}`;
                
                const activeCall = await this.ActiveCall.findOneAndUpdate(
                    { call_id: callId },
                    {
                        $setOnInsert: {
                            sys_num: call.sys_num,
                            sys_name: call.sys_name,
                            start_time: new Date(call.start_time * 1000),
                            talkgroup: call.talkgroup,
                            talkgroup_alpha_tag: call.talkgroup_alpha_tag,
                            talkgroup_tag: call.talkgroup_tag,
                            talkgroup_description: call.talkgroup_description,
                            talkgroup_group: call.talkgroup_group
                        },
                        $set: {
                            last_update: new Date(),
                            freq: call.freq,
                            emergency: call.emergency || false,
                            encrypted: call.encrypted || false
                        },
                        $push: {
                            sources: {
                                type: 'calls_active',
                                message_id: messageId,
                                timestamp: new Date()
                            }
                        }
                    },
                    { 
                        upsert: true,
                        new: true
                    }
                );

                // Update cache
                this.activeCallsCache.set(callId, activeCall);
                logger.debug(`${activeCall.isNew ? 'Created' : 'Updated'} active call: ${callId}`);
            }));
        } catch (err) {
            logger.error('Error handling active calls:', err);
            throw err;
        }
    }
    
    async handleAudioMessage(message, messageId) {
        try {
            const callId = `${message.sys_num}_${message.talkgroup}_${message.start_time}`;
            
            logger.debug(`Processing audio message for ${callId}`);

            // Use findOneAndUpdate with upsert
            const activeCall = await this.ActiveCall.findOneAndUpdate(
                { call_id: callId },
                {
                    $setOnInsert: {
                        sys_num: message.sys_num,
                        sys_name: message.sys_name,
                        start_time: new Date(message.start_time * 1000),
                        talkgroup: message.talkgroup,
                        talkgroup_tag: message.talkgroup_tag
                    },
                    $set: {
                        last_update: new Date(),
                        has_audio: true,
                        audio_file: message.filename,
                        freq: message.freq,
                        emergency: message.emergency || false,
                        encrypted: message.encrypted || false
                    },
                    $inc: {
                        audio_segments: 1
                    },
                    $push: {
                        sources: {
                            type: 'audio',
                            message_id: messageId,
                            timestamp: new Date()
                        }
                    }
                },
                { 
                    upsert: true,
                    new: true
                }
            );

            // Update cache
            this.activeCallsCache.set(callId, activeCall);
            logger.debug(`${activeCall.isNew ? 'Created' : 'Updated'} call with audio: ${callId}`);
        } catch (err) {
            logger.error('Error handling audio message:', err);
            throw err;
        }
    }
    
    async handleUnitMessage(topic, message, messageId) {
        try {
            if (!message.talkgroup) return;
            
            logger.debug(`Processing unit message for talkgroup ${message.talkgroup}`);
            
            // Find and update all active calls for this talkgroup
            const activeCalls = await this.ActiveCall.find({
                talkgroup: message.talkgroup,
                sys_name: message.sys_name
            });

            // Process all calls concurrently
            await Promise.all(activeCalls.map(async (call) => {
                const updatedCall = await this.ActiveCall.findOneAndUpdate(
                    { _id: call._id },
                    {
                        $set: {
                            last_update: new Date()
                        },
                        $push: {
                            sources: {
                                type: 'unit',
                                message_id: messageId,
                                timestamp: new Date()
                            }
                        },
                        $addToSet: {
                            units: {
                                unit: message.unit,
                                unit_alpha_tag: message.unit_alpha_tag,
                                joined_at: new Date(),
                                last_seen: new Date(),
                                status: message.type
                            }
                        }
                    },
                    { new: true }
                );

                if (updatedCall) {
                    this.activeCallsCache.set(updatedCall.call_id, updatedCall);
                    logger.debug(`Updated call ${updatedCall.call_id} with unit ${message.unit}`);
                }
            }));
        } catch (err) {
            logger.error('Error handling unit message:', err);
            throw err;
        }
    }
    
    async handleCallEnd(message, messageId) {
        try {
            const callId = `${message.sys_num}_${message.talkgroup}_${message.start_time}`;
            
            logger.debug(`Processing call end for ${callId}`);
            
            // Remove from active calls and move to call history
            await this.ActiveCall.deleteOne({ call_id: callId });
            this.activeCallsCache.delete(callId);
            
            logger.info(`Call ended and removed from active calls: ${callId}`);
        } catch (err) {
            logger.error('Error handling call end:', err);
            throw err;
        }
    }
    
    async handleRecorderUpdate(message, messageId) {
        try {
            // Update our recorder state cache
            this.recorderStates.set(message.id, {
                ...message,
                last_update: new Date()
            });

            logger.debug(`Recorder ${message.id} state updated to ${message.rec_state_type}`);

            // If the recorder is RECORDING, update the first matching call
            if (message.rec_state_type === 'RECORDING') {
                const updatedCall = await this.ActiveCall.findOneAndUpdate(
                    {
                        freq: message.freq,
                        'recorder.id': { $exists: false }
                    },
                    {
                        $set: {
                            recorder: {
                                id: message.id,
                                src_num: message.src_num,
                                rec_num: message.rec_num,
                                state: message.rec_state_type,
                                freq: message.freq,
                                squelched: message.squelched
                            },
                            last_update: new Date()
                        }
                    },
                    { new: true }
                );

                if (updatedCall) {
                    this.activeCallsCache.set(updatedCall.call_id, updatedCall);
                    logger.debug(`Assigned recorder ${message.id} to call ${updatedCall.call_id}`);
                }
            }
        } catch (err) {
            logger.error('Error handling recorder update:', err);
            throw err;
        }
    }

    async handleRecordersUpdate(message, messageId) {
        try {
            // Update state for all recorders
            message.recorders.forEach(recorder => {
                this.recorderStates.set(recorder.id, {
                    ...recorder,
                    last_update: new Date()
                });
            });

            logger.debug('Updated states for all recorders');

            // Find all active calls
            const activeCalls = await this.ActiveCall.find({});
            
            // Process all calls concurrently
            await Promise.all(activeCalls.map(async (call) => {
                // Find a recorder that matches this call's frequency
                const matchingRecorder = message.recorders.find(rec => 
                    rec.freq === call.freq && rec.rec_state_type === 'RECORDING'
                );

                if (matchingRecorder) {
                    const updatedCall = await this.ActiveCall.findOneAndUpdate(
                        { _id: call._id },
                        {
                            $set: {
                                recorder: {
                                    id: matchingRecorder.id,
                                    src_num: matchingRecorder.src_num,
                                    rec_num: matchingRecorder.rec_num,
                                    state: matchingRecorder.rec_state_type,
                                    freq: matchingRecorder.freq,
                                    squelched: matchingRecorder.squelched
                                },
                                last_update: new Date()
                            }
                        },
                        { new: true }
                    );

                    if (updatedCall) {
                        this.activeCallsCache.set(updatedCall.call_id, updatedCall);
                        logger.debug(`Matched recorder ${matchingRecorder.id} to call ${updatedCall.call_id}`);
                    }
                }
            }));
        } catch (err) {
            logger.error('Error handling recorders update:', err);
            throw err;
        }
    }

    async updateStatistics(sysName = null) {
        try {
            // Clean up stale calls for system
            await this.cleanupStaleCalls(sysName);

            // Update derived statistics for remaining active calls
            const query = {};
            if (sysName) {
                query.sys_name = sysName;
            }
            const activeCalls = await this.ActiveCall.find(query);
            
            // Use Promise.all to handle all updates concurrently
            await Promise.all(activeCalls.map(async (call) => {
                try {
                    const updatedCall = await this.ActiveCall.findOneAndUpdate(
                        { _id: call._id },
                        {
                            $set: {
                                duration: (new Date() - call.start_time) / 1000,
                                unit_count: call.units.length,
                                last_update: new Date()
                            }
                        },
                        { new: true }
                    );

                    if (updatedCall) {
                        this.activeCallsCache.set(updatedCall.call_id, updatedCall);
                    } else {
                        // Call was deleted, remove from cache
                        this.activeCallsCache.delete(call.call_id);
                    }
                } catch (err) {
                    logger.error(`Error updating statistics for call ${call.call_id}:`, err);
                }
            }));
            
            logger.debug(`Updated statistics for ${activeCalls.length} active calls`);
        } catch (err) {
            logger.error('Error updating call statistics:', err);
            throw err;
        }
    }
    
    // Helper method to clean up stale calls for a specific system
    async cleanupStaleCalls(sysName = null) {
        try {
            const staleThreshold = new Date(Date.now() - 5 * 60 * 1000); // 5 minutes
            const query = {
                last_update: { $lt: staleThreshold }
            };
            if (sysName) {
                query.sys_name = sysName;
            }
            const staleCalls = await this.ActiveCall.find(query);

            if (staleCalls.length > 0) {
                logger.info(`Cleaning up ${staleCalls.length} stale calls (not updated in last 5 minutes)`);
                staleCalls.forEach(call => {
                    logger.debug(`Removing stale call ${call.call_id} (last update: ${call.last_update})`);
                });
                await this.ActiveCall.deleteMany({
                    last_update: { $lt: staleThreshold }
                });
                
                // Remove from cache
                staleCalls.forEach(call => {
                    this.activeCallsCache.delete(call.call_id);
                });
            }
        } catch (err) {
            logger.error('Error cleaning up stale calls:', err);
        }
    }

    // Helper method to get current active calls
    async getActiveCalls(filter = {}) {
        try {
            // Clean up stale calls for specific system if filtered
            await this.cleanupStaleCalls(filter.sys_name);

            // Get fresh data from database instead of cache
            const query = {};
            if (filter.sys_name) query.sys_name = filter.sys_name;
            if (filter.talkgroup) query.talkgroup = filter.talkgroup;
            if (filter.emergency === true) query.emergency = true;

            const activeCalls = await this.ActiveCall.find(query)
                .sort({ start_time: -1 });

            // Update cache with fresh data
            activeCalls.forEach(call => {
                this.activeCallsCache.set(call.call_id, call);
            });

            return activeCalls;
        } catch (err) {
            logger.error('Error retrieving active calls:', err);
            throw err;
        }
    }

    // Helper method to clear all active calls (used in tests)
    async clearActiveCalls() {
        try {
            await this.ActiveCall.deleteMany({});
            this.activeCallsCache.clear();
            logger.debug('Cleared all active calls');
        } catch (err) {
            logger.error('Error clearing active calls:', err);
            throw err;
        }
    }
}

// Export singleton instance
const activeCallManager = new ActiveCallManager();
module.exports = activeCallManager;
