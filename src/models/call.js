const mongoose = require('mongoose');

// Schema for call transcription segments
const TranscriptionSegmentSchema = new mongoose.Schema({
    start_time: Number,
    end_time: Number,
    text: String,
    confidence: Number,
    source: {
        unit: String,
        emergency: Boolean,
        signal_system: String,
        tag: String
    }
});

// Schema for call metadata
const CallMetadataSchema = new mongoose.Schema({
    filename: String,
    talkgroup: Number,
    talkgroup_tag: String,
    start_time: Date,
    call_length: Number,
    emergency: Boolean,
    freqList: [{
        freq: Number,
        len: Number
    }],
    srcList: [{
        src: String,
        pos: Number,
        emergency: Boolean,
        signal_system: String,
        tag: String
    }],
    transcription: {
        text: String,
        segments: [TranscriptionSegmentSchema],
        metadata: {
            model: String,
            processing_time: Number,
            audio_duration: Number,
            timestamp: Date
        }
    }
});

// Schema for call messages
const CallSchema = new mongoose.Schema({
    message_type: String,
    type: String,
    timestamp: Date,
    instance_id: String,
    topic: String,
    call: {
        metadata: CallMetadataSchema
    }
});

// Create indexes
CallSchema.index({ 'call.metadata.filename': 1 });
CallSchema.index({ 'call.metadata.talkgroup': 1 });
CallSchema.index({ 'call.metadata.transcription.metadata.timestamp': 1 });

// Export the model
const CallMessage = mongoose.model('CallMessage', CallSchema);

module.exports = { CallMessage };
