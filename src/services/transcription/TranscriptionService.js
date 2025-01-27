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

    async processAudioFile(callId, audioPath, retries = 3) {
        try {
            logger.debug(`Starting transcription for call ${callId}`);
            
            const startTime = Date.now();
            let lastError;
            
            // Retry loop
            for (let attempt = 1; attempt <= retries; attempt++) {
                try {
                    // Process with Whisper
                    const whisperResponse = await this.openai.audio.transcriptions.create({
                        file: fs.createReadStream(audioPath),
                        model: process.env.WHISPER_MODEL || "guillaumekln/faster-whisper-base.en",
                        response_format: "verbose_json",
                        timestamp_granularities: ["word"]
                    });
                    
                    // Check quality before proceeding
                    const quality = this.assessTranscriptionQuality(whisperResponse);
                    if (quality.needsRetry && attempt < retries) {
                        throw new Error(`Low quality transcription: ${quality.reason}`);
                    }
                    
                    return await this.saveTranscription(callId, whisperResponse, startTime);
                } catch (error) {
                    lastError = error;
                    logger.warn(`Transcription attempt ${attempt} failed for call ${callId}:`, error);
                    if (attempt < retries) {
                        const delay = Math.min(1000 * Math.pow(2, attempt - 1), 10000);
                        await new Promise(resolve => setTimeout(resolve, delay));
                    }
                }
            }
            throw lastError;
            
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

    async saveTranscription(callId, whisperResponse, startTime) {
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
    }

    assessTranscriptionQuality(whisperResponse) {
        const MIN_CONFIDENCE = 0.6;
        const MIN_DURATION = 0.5;
        
        // Check overall confidence
        const avgConfidence = whisperResponse.segments.reduce(
            (sum, seg) => sum + seg.confidence, 
            0
        ) / whisperResponse.segments.length;

        if (avgConfidence < MIN_CONFIDENCE) {
            return {
                needsRetry: true,
                reason: `Low confidence score: ${avgConfidence.toFixed(2)}`
            };
        }

        // Check for suspiciously short duration
        if (whisperResponse.duration < MIN_DURATION) {
            return {
                needsRetry: true,
                reason: `Suspicious duration: ${whisperResponse.duration}s`
            };
        }

        // Check for empty or very short text
        if (!whisperResponse.text || whisperResponse.text.length < 2) {
            return {
                needsRetry: true,
                reason: 'Empty or very short transcription'
            };
        }

        return { needsRetry: false };
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
