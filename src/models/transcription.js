const mongoose = require('mongoose');

const TranscriptionSegmentSchema = new mongoose.Schema({
    start_time: Number,
    end_time: Number,
    text: String,
    confidence: Number,
    source: {
        unit: Number,
        emergency: Boolean,
        signal_system: String,
        tag: String
    }
}, { _id: false });

const TranscriptionMetadataSchema = new mongoose.Schema({
    model: String,
    processing_time: Number,
    audio_duration: Number,
    timestamp: { type: Date, default: Date.now }
}, { _id: false });

const TranscriptionSchema = new mongoose.Schema({
    call_id: { 
        type: String, 
        required: true, 
        unique: true,
        index: true 
    },
    talkgroup: { 
        type: Number, 
        required: true,
        index: true 
    },
    talkgroup_metadata: {
        tag: { type: String, required: true },
        description: String,
        group_tag: String,
        group: String
    },
    call_metadata: {
        start_time: { type: Number, required: true },
        stop_time: { type: Number, required: true },
        emergency: { type: Boolean, default: false },
        encrypted: { type: Boolean, default: false },
        freq: { type: Number, required: true },
        audio_type: { type: String, required: true }
    },
    transcription: {
        text: { type: String, required: true },
        segments: {
            type: [TranscriptionSegmentSchema],
            validate: [arr => arr.length > 0, 'Transcription must have at least one segment']
        },
        metadata: TranscriptionMetadataSchema
    }
}, { 
    timestamps: true,
    // Add optimistic concurrency control
    strict: true,
    validateBeforeSave: true
});

// Compound indexes for common queries
TranscriptionSchema.index({ 'transcription.text': 'text' });
TranscriptionSchema.index({ talkgroup: 1, 'call_metadata.start_time': -1 });
TranscriptionSchema.index({ 'call_metadata.start_time': 1 }, { 
    expireAfterSeconds: 60 * 60 * 24 * 90 // 90 days TTL
});

// Pre-save validation
TranscriptionSchema.pre('save', function(next) {
    if (this.call_metadata.stop_time <= this.call_metadata.start_time) {
        next(new Error('stop_time must be greater than start_time'));
    }
    next();
});

module.exports = { TranscriptionSchema };
