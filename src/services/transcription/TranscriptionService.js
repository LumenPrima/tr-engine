const logger = require('../../utils/logger');
const OpenAI = require('openai');
const fs = require('fs');
const mongoose = require('mongoose');
const { getGridFSBucket } = require('../../config/mongodb');

// Queue for managing transcription requests
class RequestQueue {
    constructor() {
        this.queue = [];
        this.processing = false;
    }

    async add(task) {
        return new Promise((resolve, reject) => {
            this.queue.push({ task, resolve, reject });
            this.process();
        });
    }

    async process() {
        if (this.processing || this.queue.length === 0) return;
        this.processing = true;

        const { task, resolve, reject } = this.queue.shift();
        try {
            const result = await task();
            resolve(result);
        } catch (error) {
            reject(error);
        } finally {
            this.processing = false;
            this.process();
        }
    }
}

class TranscriptionService {
    constructor() {
        // Initialize rate limiting and queue
        this.requestCount = 0;
        this.lastReset = Date.now();
        this.queue = new RequestQueue();

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
            if (!audioMessage?.srcList) {
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
            const expectedDuration = audioMessage.call_length;
            if (expectedDuration < 0.1) {
                throw new Error('Invalid call duration in metadata');
            }
            
            const startTime = Date.now();
            let lastError;
            
            // Retry loop
            for (let attempt = 1; attempt <= retries; attempt++) {
                try {
                    // Queue the transcription request
                    const processTranscription = async () => {
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
                        
                        return whisperResponse;
                    };

                    const whisperResponse = await this.queue.add(processTranscription);
                    return await this.saveTranscription(callId, whisperResponse, startTime, audioMessage);
                } catch (error) {
                    lastError = error;
                    logger.warn(`Transcription attempt ${attempt} failed for call ${callId}:`, error);
                    if (attempt < retries) {
                        // Increase max backoff time to 30 seconds
                        const delay = Math.min(1000 * Math.pow(2, attempt - 1), 30000);
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
        const srcList = audioMessage.srcList || [];
        
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

        // Create transcription document
        const transcription = {
            call_id: callId,
            text: whisperResponse.text,
            segments: segments,
            audio_duration: whisperResponse.duration,
            processing_time: processingTime,
            model: process.env.WHISPER_MODEL,
            timestamp: new Date(),
            talkgroup: audioMessage.talkgroup,
            talkgroup_tag: audioMessage.talkgroup_tag,
            sys_name: audioMessage.sys_name,
            emergency: audioMessage.emergency || false,
            filename: audioMessage.filename
        };

        try {
            const collection = mongoose.connection.db.collection('transcriptions');
            await collection.insertOne(transcription);
            
            logger.info(`Saved transcription for call ${callId}, duration: ${processingTime}s`);
            return transcription;
        } catch (error) {
            logger.error(`Failed to save transcription for call ${callId}:`, error);
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
        try {
            const collection = mongoose.connection.db.collection('transcriptions');
            return await collection.findOne({ call_id: callId });
        } catch (error) {
            logger.error('Error getting transcription:', error);
            return null;
        }
    }

    async getRecentTranscriptions(talkgroupId, limit = 10, startDate = null, endDate = null) {
        try {
            const collection = mongoose.connection.db.collection('transcriptions');
            const query = { talkgroup: talkgroupId };
            
            if (startDate || endDate) {
                query.timestamp = {};
                if (startDate) query.timestamp.$gte = startDate;
                if (endDate) query.timestamp.$lte = endDate;
            }
            
            return await collection.find(query)
                .sort({ timestamp: -1 })
                .limit(limit)
                .toArray();
        } catch (error) {
            logger.error('Error getting recent transcriptions:', error);
            return [];
        }
    }

    async getTranscriptionStats(talkgroupId = null, startDate = null, endDate = null) {
        try {
            const collection = mongoose.connection.db.collection('transcriptions');
            const match = {};
            
            if (talkgroupId) {
                match.talkgroup = talkgroupId;
            }
            
            if (startDate || endDate) {
                match.timestamp = {};
                if (startDate) match.timestamp.$gte = startDate;
                if (endDate) match.timestamp.$lte = endDate;
            }

            return await collection.aggregate([
                { $match: match },
                {
                    $group: {
                        _id: '$talkgroup',
                        count: { $sum: 1 },
                        avg_confidence: { 
                            $avg: { 
                                $avg: '$segments.confidence' 
                            }
                        },
                        avg_duration: { 
                            $avg: '$audio_duration' 
                        },
                        avg_processing_time: { 
                            $avg: '$processing_time' 
                        },
                        emergency_count: {
                            $sum: { 
                                $cond: ['$emergency', 1, 0]
                            }
                        }
                    }
                },
                { $sort: { count: -1 } }
            ]).toArray();
        } catch (error) {
            logger.error('Error getting transcription stats:', error);
            return [];
        }
    }
}

module.exports = TranscriptionService;
