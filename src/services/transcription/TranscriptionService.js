const logger = require('../../utils/logger');
const OpenAI = require('openai');
const fs = require('fs');
const { AudioMessage } = require('../../models/raw/MessageCollections');

class TranscriptionService {
    constructor() {
        // Initialize OpenAI client with environment config
        if (process.env.OPENAI_API_KEY) {
            this.openai = new OpenAI({
                apiKey: process.env.OPENAI_API_KEY,
                baseURL: process.env.OPENAI_API_BASE
            });
        } else {
            logger.warn('OpenAI API key not configured. Transcription service will be unavailable.');
            this.openai = null;
        }
    }

    async processAudioFile(callId, audioPath, audioMessage, retries = 3) {
        if (!this.openai) {
            throw new Error('Transcription service is not configured. Please set OPENAI_API_KEY environment variable.');
        }
        try {
            logger.debug(`Starting transcription for call ${callId}`);
            
            // Validate audio message format
            if (!audioMessage?.call?.metadata?.srcList) {
                throw new Error('Invalid audio message format: missing required metadata');
            }

            // Validate audio file
            if (!fs.existsSync(audioPath)) {
                throw new Error(`Audio file not found: ${audioPath}`);
            }

            // Check file format and size
            const fileStats = fs.statSync(audioPath);
            if (fileStats.size === 0) {
                throw new Error('Audio file is empty');
            }
            
            // Validate file extension
            if (!audioPath.toLowerCase().endsWith('.wav')) {
                throw new Error('Invalid file format: only WAV files are supported');
            }
            
            // Basic WAV header validation
            const header = Buffer.alloc(44);
            const fd = fs.openSync(audioPath, 'r');
            fs.readSync(fd, header, 0, 44, 0);
            fs.closeSync(fd);
            
            if (header.toString('ascii', 0, 4) !== 'RIFF' || 
                header.toString('ascii', 8, 12) !== 'WAVE') {
                throw new Error('Invalid WAV file format');
            }

            // Validate audio duration matches metadata
            const expectedDuration = audioMessage.call.metadata.call_length;
            if (expectedDuration < 0.1) {
                throw new Error('Invalid call duration in metadata');
            }
            
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
                    
                    return await this.saveTranscription(callId, whisperResponse, startTime, audioMessage);
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
        } catch (error) {
            logger.error(`Transcription failed for call ${callId}:`, error);
            throw error;
        }
    }

    async saveTranscription(callId, whisperResponse, startTime, audioMessage) {
        const processingTime = (Date.now() - startTime) / 1000;
        
        // Get source list from audio message
        const srcList = audioMessage.call.metadata.srcList || [];
        
        // Convert Whisper segments to our schema format and map to sources
        const segments = whisperResponse.segments.map(seg => {
            // Find the source active during this segment
            const source = srcList.find(src => 
                seg.start >= src.pos && 
                seg.start < (src.pos + (srcList[srcList.indexOf(src) + 1]?.pos || Infinity))
            );
            
            return {
                start_time: seg.start,
                end_time: seg.end,
                text: seg.text,
                confidence: seg.confidence,
                source: source ? {
                    unit: source.src,
                    emergency: Boolean(source.emergency),
                    signal_system: source.signal_system,
                    tag: source.tag
                } : null
            };
        });

        // Add transcription to call metadata
        audioMessage.call.metadata.transcription = {
            text: whisperResponse.text,
            segments: segments,
            metadata: {
                model: process.env.WHISPER_MODEL,
                processing_time: processingTime,
                audio_duration: whisperResponse.duration,
                timestamp: new Date()
            }
        };

        try {
            // Update the call message with transcription
            const result = await AudioMessage.findOneAndUpdate(
                { 'payload.call.metadata.filename': audioMessage.call.metadata.filename },
                { $set: { 'payload.call.metadata.transcription': audioMessage.call.metadata.transcription } },
                { new: true }
            );
            
            logger.info(`Updated call ${callId} with transcription, duration: ${processingTime}s`);
            return result;
        } catch (error) {
            logger.error(`Failed to update call ${callId} with transcription:`, error);
            throw error;
        }
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
        const result = await AudioMessage.findOne({ 
            'payload.call.metadata.filename': callId 
        });
        return result?.payload;
    }

    async getRecentTranscriptions(talkgroupId, limit = 10, startDate = null, endDate = null) {
        const query = { 
            'payload.call.metadata.talkgroup': talkgroupId,
            'payload.call.metadata.transcription': { $exists: true }
        };
        
        if (startDate || endDate) {
            query['payload.call.metadata.transcription.metadata.timestamp'] = {};
            if (startDate) {
                query['payload.call.metadata.transcription.metadata.timestamp'].$gte = startDate;
            }
            if (endDate) {
                query['payload.call.metadata.transcription.metadata.timestamp'].$lte = endDate;
            }
        }
        
        const results = await AudioMessage.find(query)
            .sort({ 'payload.call.metadata.transcription.metadata.timestamp': -1 })
            .limit(limit);
            
        return results.map(doc => doc.payload);
    }

    async getTranscriptionStats(talkgroupId = null, startDate = null, endDate = null) {
        const match = {
            'payload.call.metadata.transcription': { $exists: true }
        };
        
        if (talkgroupId) {
            match['payload.call.metadata.talkgroup'] = talkgroupId;
        }
        
        if (startDate || endDate) {
            match['payload.call.metadata.transcription.metadata.timestamp'] = {};
            if (startDate) {
                match['payload.call.metadata.transcription.metadata.timestamp'].$gte = startDate;
            }
            if (endDate) {
                match['payload.call.metadata.transcription.metadata.timestamp'].$lte = endDate;
            }
        }

        return await AudioMessage.aggregate([
            { $match: match },
            { $group: {
                _id: '$payload.call.metadata.talkgroup',
                count: { $sum: 1 },
                avg_confidence: { 
                    $avg: { 
                        $avg: '$payload.call.metadata.transcription.segments.confidence' 
                    }
                },
                avg_duration: { 
                    $avg: '$payload.call.metadata.transcription.metadata.audio_duration' 
                },
                avg_processing_time: { 
                    $avg: '$payload.call.metadata.transcription.metadata.processing_time' 
                },
                emergency_count: {
                    $sum: { 
                        $cond: ['$payload.call.metadata.emergency', 1, 0]
                    }
                }
            }},
            { $sort: { count: -1 } }
        ]);
    }
}

module.exports = TranscriptionService;
