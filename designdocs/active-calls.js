const mongoose = require('mongoose');
const _ = require('lodash');

// Schema for our real-time active calls view
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
});

// Indexes for efficient querying of active calls
ActiveCallSchema.index({ sys_name: 1, start_time: -1 });
ActiveCallSchema.index({ talkgroup: 1, start_time: -1 });
ActiveCallSchema.index({ last_update: 1 });
ActiveCallSchema.index({ emergency: 1 });

class ActiveCallsManager {
    constructor() {
        this.ActiveCall = mongoose.model('ActiveCall', ActiveCallSchema);
        
        // In-memory cache of active calls for fastest possible access
        this.activeCallsCache = new Map();
    }
    
    // Track the state of all recorders
    recorderStates = new Map();

    async processMessage(topic, message, messageId) {
        const topicParts = topic.split('/');
        const messageType = topicParts[2];
        
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
        
        // Update derived statistics for affected calls
        await this.updateStatistics();
    }
    
    async handleCallStart(message, messageId) {
        const call = message.call;
        const callId = `${call.sys_num}_${call.talkgroup}_${call.start_time}`;
        
        const activeCall = new this.ActiveCall({
            call_id: callId,
            sys_num: call.sys_num,
            sys_name: call.sys_name,
            start_time: new Date(call.start_time * 1000),
            last_update: new Date(),
            
            talkgroup: call.talkgroup,
            talkgroup_alpha_tag: call.talkgroup_alpha_tag,
            talkgroup_tag: call.talkgroup_tag,
            talkgroup_description: call.talkgroup_description,
            talkgroup_group: call.talkgroup_group,
            
            freq: call.freq,
            emergency: call.emergency || false,
            encrypted: call.encrypted || false,
            
            units: [{
                unit: call.unit,
                unit_alpha_tag: call.unit_alpha_tag,
                joined_at: new Date(),
                last_seen: new Date(),
                status: 'initiator'
            }],
            
            sources: [{
                type: 'call_start',
                message_id: messageId,
                timestamp: new Date()
            }]
        });
        
        await activeCall.save();
        this.activeCallsCache.set(callId, activeCall);
    }
    
    async handleCallsActive(message, messageId) {
        for (const call of message.calls) {
            const callId = `${call.sys_num}_${call.talkgroup}_${call.start_time}`;
            const activeCall = await this.ActiveCall.findOne({ call_id: callId });
            
            if (activeCall) {
                // Update call information
                activeCall.last_update = new Date();
                activeCall.freq = call.freq;
                activeCall.emergency = call.emergency;
                activeCall.encrypted = call.encrypted;
                
                // Add source reference
                activeCall.sources.push({
                    type: 'calls_active',
                    message_id: messageId,
                    timestamp: new Date()
                });
                
                await activeCall.save();
                this.activeCallsCache.set(callId, activeCall);
            }
        }
    }
    
    async handleAudioMessage(message, messageId) {
        const metadata = message.call.metadata;
        const callId = `${metadata.sys_num}_${metadata.talkgroup}_${metadata.start_time}`;
        
        const activeCall = await this.ActiveCall.findOne({ call_id: callId });
        if (activeCall) {
            activeCall.has_audio = true;
            activeCall.audio_file = metadata.filename;
            activeCall.audio_segments = (activeCall.audio_segments || 0) + 1;
            
            activeCall.sources.push({
                type: 'audio',
                message_id: messageId,
                timestamp: new Date()
            });
            
            await activeCall.save();
            this.activeCallsCache.set(callId, activeCall);
        }
    }
    
    async handleUnitMessage(topic, message, messageId) {
        const unitData = message[message.type]; // Extract unit data based on message type
        if (!unitData?.talkgroup) return;
        
        // Find all active calls for this talkgroup
        const activeCalls = await this.ActiveCall.find({
            talkgroup: unitData.talkgroup,
            sys_name: unitData.sys_name
        });
        
        for (const call of activeCalls) {
            // Update unit information
            const unitIndex = call.units.findIndex(u => u.unit === unitData.unit);
            if (unitIndex === -1) {
                call.units.push({
                    unit: unitData.unit,
                    unit_alpha_tag: unitData.unit_alpha_tag,
                    joined_at: new Date(),
                    last_seen: new Date(),
                    status: message.type // join, leave, etc.
                });
            } else {
                call.units[unitIndex].last_seen = new Date();
                call.units[unitIndex].status = message.type;
            }
            
            call.sources.push({
                type: 'unit',
                message_id: messageId,
                timestamp: new Date()
            });
            
            await call.save();
            this.activeCallsCache.set(call.call_id, call);
        }
    }
    
    async handleCallEnd(message, messageId) {
        const call = message.call;
        const callId = `${call.sys_num}_${call.talkgroup}_${call.start_time}`;
        
        // Remove from active calls and move to call history
        await this.ActiveCall.deleteOne({ call_id: callId });
        this.activeCallsCache.delete(callId);
    }
    
    async handleRecorderUpdate(message, messageId) {
        const recorder = message.recorder;
        
        // Update our recorder state cache
        this.recorderStates.set(recorder.id, {
            ...recorder,
            last_update: new Date()
        });

        // If the recorder is RECORDING, find the matching active call
        if (recorder.rec_state_type === 'RECORDING') {
            const activeCalls = await this.ActiveCall.find({
                freq: recorder.freq,
                'recorder.id': { $exists: false } // Calls without a recorder assigned
            });

            // Update the first matching call with recorder information
            if (activeCalls.length > 0) {
                const call = activeCalls[0];
                call.recorder = {
                    id: recorder.id,
                    src_num: recorder.src_num,
                    rec_num: recorder.rec_num,
                    state: recorder.rec_state_type,
                    freq: recorder.freq,
                    squelched: recorder.squelched
                };
                await call.save();
                this.activeCallsCache.set(call.call_id, call);
            }
        }
    }

    async handleRecordersUpdate(message, messageId) {
        // Update state for all recorders
        for (const recorder of message.recorders) {
            this.recorderStates.set(recorder.id, {
                ...recorder,
                last_update: new Date()
            });
        }

        // Match recorders to active calls based on frequency
        const activeCalls = await this.ActiveCall.find({});
        for (const call of activeCalls) {
            // Find a recorder that matches this call's frequency
            const matchingRecorder = message.recorders.find(rec => 
                rec.freq === call.freq && rec.rec_state_type === 'RECORDING'
            );

            if (matchingRecorder) {
                call.recorder = {
                    id: matchingRecorder.id,
                    src_num: matchingRecorder.src_num,
                    rec_num: matchingRecorder.rec_num,
                    state: matchingRecorder.rec_state_type,
                    freq: matchingRecorder.freq,
                    squelched: matchingRecorder.squelched
                };
                await call.save();
                this.activeCallsCache.set(call.call_id, call);
            }
        }
    }

    async updateStatistics() {
        // Update derived statistics for all active calls
        const activeCalls = await this.ActiveCall.find({});
        
        for (const call of activeCalls) {
            call.duration = (new Date() - call.start_time) / 1000;
            call.unit_count = call.units.length;
            
            await call.save();
            this.activeCallsCache.set(call.call_id, call);
        }
    }
    
    // Helper method to get current active calls
    async getActiveCalls(filter = {}) {
        return Array.from(this.activeCallsCache.values())
            .filter(call => {
                // Apply any filtering logic
                if (filter.sys_name && call.sys_name !== filter.sys_name) return false;
                if (filter.talkgroup && call.talkgroup !== filter.talkgroup) return false;
                if (filter.emergency === true && !call.emergency) return false;
                return true;
            })
            .sort((a, b) => b.start_time - a.start_time);
    }
}

module.exports = ActiveCallsManager;