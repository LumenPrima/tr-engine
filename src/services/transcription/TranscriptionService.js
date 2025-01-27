const mongoose = require('mongoose');
const { TranscriptionSchema } = require('../../models/transcription');
const logger = require('../../utils/logger');

class TranscriptionService {
    constructor() {
        this.Transcription = mongoose.model('Transcription', TranscriptionSchema);
    }

    async processAudioFile(callId, audioPath) {
        try {
            logger.debug(`Starting transcription for call ${callId}`);
            
            // TODO: Implement Whisper integration
            // For now, store placeholder transcription
            const transcription = new this.Transcription({
                call_id: callId,
                transcription: {
                    text: "Placeholder transcription",
                    segments: [],
                    metadata: {
                        model: "whisper-large-v3",
                        processing_time: 0,
                        audio_duration: 0
                    }
                }
            });

            await transcription.save();
            logger.info(`Stored transcription for call ${callId}`);
            
            return transcription;
        } catch (error) {
            logger.error(`Transcription failed for call ${callId}:`, error);
            throw error;
        }
    }

    async getTranscription(callId) {
        return await this.Transcription.findOne({ call_id: callId });
    }

    async getRecentTranscriptions(talkgroupId, limit = 10) {
        return await this.Transcription.find({ talkgroup: talkgroupId })
            .sort({ 'transcription.metadata.timestamp': -1 })
            .limit(limit);
    }
}

module.exports = TranscriptionService;
