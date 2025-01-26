const mongoose = require('mongoose');
const { GridFSBucket } = require('mongodb');

// Schema that directly matches our MQTT audio message structure
const AudioMessageSchema = new mongoose.Schema({
    // Message identification
    talkgroup: { type: Number, required: true },
    start_time: { type: Number, required: true },
    sys_name: { type: String, required: true },
    
    // Audio metadata - direct mapping from MQTT message
    freq: Number,
    freq_error: Number,
    signal: Number,
    noise: Number,
    source_num: Number,
    recorder_num: Number,
    tdma_slot: Number,
    phase2_tdma: Boolean,
    stop_time: Number,
    emergency: Boolean,
    priority: Number,
    mode: Number,
    duplex: Boolean,
    encrypted: Boolean,
    call_length: Number,
    
    // Talkgroup information
    talkgroup_tag: String,
    talkgroup_description: String,
    talkgroup_group_tag: String,
    talkgroup_group: String,
    
    // Audio details
    audio_type: String,
    filename: String,
    
    // Frequency and source information
    freq_list: [{
        freq: Number,
        time: Number,
        pos: Number,
        len: Number,
        error_count: Number,
        spike_count: Number
    }],
    
    src_list: [{
        src: Number,
        time: Number,
        pos: Number,
        emergency: Boolean,
        signal_system: String,
        tag: String
    }],
    
    // Storage reference
    gridfs_id: mongoose.Schema.Types.ObjectId,
    
    // Message metadata
    timestamp: { type: Date, default: Date.now },
    instance_id: String
});

// Create indexes for common query patterns
AudioMessageSchema.index({ talkgroup: 1, start_time: -1 });
AudioMessageSchema.index({ sys_name: 1, start_time: -1 });
AudioMessageSchema.index({ emergency: 1 });
AudioMessageSchema.index({ src_list: 1 });  // Index for searching by source unit
AudioMessageSchema.index({ timestamp: 1 });  // For time-based queries

class AudioStorageManager {
    constructor() {
        this.AudioMessage = mongoose.model('AudioMessage', AudioMessageSchema);
        this.gridFSBucket = new GridFSBucket(mongoose.connection.db, {
            bucketName: 'audioFiles'
        });
    }
    
    async processAudioMessage(message) {
        try {
            const audioData = message.call.audio_m4a_base64;
            if (!audioData) {
                console.warn('No audio data in message');
                return;
            }
            
            const metadata = message.call.metadata;
            
            // Store audio file in GridFS
            const buffer = Buffer.from(audioData, 'base64');
            const uploadStream = this.gridFSBucket.openUploadStream(metadata.filename, {
                metadata: {
                    talkgroup: metadata.talkgroup,
                    start_time: metadata.start_time,
                    sys_name: metadata.short_name
                }
            });
            
            await new Promise((resolve, reject) => {
                uploadStream.once('finish', resolve);
                uploadStream.once('error', reject);
                uploadStream.end(buffer);
            });
            
            // Create audio message document
            const audioMessage = new this.AudioMessage({
                // Map all fields from metadata
                talkgroup: metadata.talkgroup,
                start_time: metadata.start_time,
                sys_name: metadata.short_name,
                freq: metadata.freq,
                freq_error: metadata.freq_error,
                signal: metadata.signal,
                noise: metadata.noise,
                source_num: metadata.source_num,
                recorder_num: metadata.recorder_num,
                tdma_slot: metadata.tdma_slot,
                phase2_tdma: Boolean(metadata.phase2_tdma),
                stop_time: metadata.stop_time,
                emergency: Boolean(metadata.emergency),
                priority: metadata.priority,
                mode: metadata.mode,
                duplex: Boolean(metadata.duplex),
                encrypted: Boolean(metadata.encrypted),
                call_length: metadata.call_length,
                talkgroup_tag: metadata.talkgroup_tag,
                talkgroup_description: metadata.talkgroup_description,
                talkgroup_group_tag: metadata.talkgroup_group_tag,
                talkgroup_group: metadata.talkgroup_group,
                audio_type: metadata.audio_type,
                filename: metadata.filename,
                freq_list: metadata.freqList,
                src_list: metadata.srcList.map(src => ({
                    ...src,
                    emergency: Boolean(src.emergency)
                })),
                gridfs_id: uploadStream.id,
                instance_id: message.instance_id,
                timestamp: new Date(message.timestamp * 1000)
            });
            
            await audioMessage.save();
            return audioMessage;
            
        } catch (error) {
            console.error('Error processing audio message:', error);
            throw error;
        }
    }
    
    async getAudioFile(talkgroup, startTime) {
        const audioMessage = await this.AudioMessage.findOne({
            talkgroup,
            start_time: startTime
        });
        
        if (!audioMessage) {
            throw new Error('Audio message not found');
        }
        
        const downloadStream = this.gridFSBucket.openDownloadStream(audioMessage.gridfs_id);
        return {
            stream: downloadStream,
            metadata: audioMessage
        };
    }
    
    async searchAudioMessages(query) {
        const filter = {};
        
        if (query.talkgroup) filter.talkgroup = query.talkgroup;
        if (query.sys_name) filter.sys_name = query.sys_name;
        if (query.emergency) filter.emergency = true;
        if (query.start_time) filter.start_time = { $gte: query.start_time };
        if (query.end_time) filter.stop_time = { $lte: query.end_time };
        if (query.unit) filter['src_list.src'] = query.unit;
        
        return this.AudioMessage.find(filter)
            .sort({ start_time: -1 })
            .limit(query.limit || 50);
    }
}

module.exports = AudioStorageManager;