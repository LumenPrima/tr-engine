const mongoose = require('mongoose');
const { TranscriptionSchema } = require('../../models/transcription');
const logger = require('../../utils/logger');
const OpenAI = require('openai');
const fs = require('fs');

class TranscriptionService {
    constructor() {
        this.Transcription = mongoose.model('Transcription', TranscriptionSchema);
        
        // Initialize OpenAI client with environment config
        this.openai = new OpenAI({
            apiKey: process.env.OPENAI_API_KEY,
            baseURL: process.env.OPENAI_API_BASE
        });
    }

    async processAudioFile(callId, audioPath) {
        try {
            logger.debug(`Starting transcription for call ${callId}`);
            
            const startTime = Date.now();
            
            // Process with Whisper
            const whisperResponse = await this.openai.audio.transcriptions.create({
                file: fs.createReadStream(audioPath),
                model: process.env.WHISPER_MODEL || "guillaumekln/faster-whisper-base.en",
                response_format: "verbose_json",
                timestamp_granularities: ["word"]
            });
            
            const processingTime = (Date.now() - startTime) / 1000;
            
            // Convert Whisper segments to our schema format
            const segments = whisperResponse.segments.map(seg => ({
                start_time: seg.start,
                end_time: seg.end,
                text: seg.text,
                confidence: seg.confidence
            }));
            
            // Create and store transcription
            const transcription = new this.Transcription({
                call_id: callId,
                transcription: {
                    text: whisperResponse.text,
                    segments: segments,
                    metadata: {
                        model: process.env.WHISPER_MODEL,
                        processing_time: processingTime,
                        audio_duration: whisperResponse.duration
                    }
                }
            });

            await transcription.save();
            logger.info(`Stored transcription for call ${callId}, duration: ${processingTime}s`);
            
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
