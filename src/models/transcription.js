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
    call_id: { type: String, required: true, index: true },
    talkgroup: { type: Number, index: true },
    talkgroup_metadata: {
        tag: String,
        description: String,
        group_tag: String,
        group: String
    },
    call_metadata: {
        start_time: Number,
        stop_time: Number,
        emergency: Boolean,
        encrypted: Boolean,
        freq: Number,
        audio_type: String
    },
    transcription: {
        text: String,
        segments: [TranscriptionSegmentSchema],
        metadata: TranscriptionMetadataSchema
    }
}, { timestamps: true });

// Index for efficient queries
TranscriptionSchema.index({ 'transcription.text': 'text' });

module.exports = { TranscriptionSchema };
