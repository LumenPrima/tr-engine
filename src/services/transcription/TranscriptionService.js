const logger = require('../../utils/logger');
const OpenAI = require('openai');
const fs = require('fs');
const mongoose = require('mongoose');
const { getGridFSBucket } = require('../../config/mongodb');
const os = require('os');
const path = require('path');
const stateEventEmitter = require('../events/emitter');

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
        if (TranscriptionService.instance) {
            return TranscriptionService.instance;
        }
        TranscriptionService.instance = this;
        
        // Initialize state
        this.isShuttingDown = false;
        this.activeRequests = new Set();
        
        // Initialize rate limiting
        this.requestCount = 0;
        this.lastReset = Date.now();
        this.rateLimitWindow = 60 * 1000; // 1 minute
        this.rateLimitMax = 500; // OpenAI's limit of 500 requests per minute
        this.queue = new RequestQueue();

        // Reset rate limit counter periodically
        setInterval(() => {
            const now = Date.now();
            if (now - this.lastReset >= this.rateLimitWindow) {
                this.requestCount = 0;
                this.lastReset = now;
                logger.debug('Rate limit counter reset', {
                    window_ms: this.rateLimitWindow,
                    max_requests: this.rateLimitMax
                });
            }
        }, 1000); // Check every second

        // Handle process shutdown
        process.on('SIGINT', this.shutdown.bind(this));
        process.on('SIGTERM', this.shutdown.bind(this));

        // Initialize OpenAI client with environment config
        if (process.env.OPENAI_API_KEY) {
            try {
                logger.info('Initializing OpenAI client', {
                    config: {
                        api_base: process.env.OPENAI_API_BASE,
                        model: process.env.WHISPER_MODEL,
                        key_prefix: process.env.OPENAI_API_KEY.substring(0, 7) + '...'
                    }
                });
                
                this.openai = new OpenAI({
                    apiKey: process.env.OPENAI_API_KEY,
                    baseURL: process.env.OPENAI_API_BASE
                });
            } catch (error) {
                logger.error('Failed to initialize OpenAI client', {
                    error_details: {
                        message: error.message,
                        stack: error.stack
                    },
                    config: {
                        api_base: process.env.OPENAI_API_BASE,
                        model: process.env.WHISPER_MODEL
                    }
                });
                this.openai = null;
            }
        } else {
            logger.warn('OpenAI API key not configured. Transcription service will be unavailable.');
            this.openai = null;
        }
    }

    async shutdown() {
        if (this.isShuttingDown) {
            return;
        }
        this.isShuttingDown = true;
        logger.info('TranscriptionService shutting down, waiting for active requests to complete...');
        
        // Wait for active requests with a timeout
        const timeout = setTimeout(() => {
            logger.warn('TranscriptionService shutdown timeout, some requests may be incomplete');
            this.activeRequests.clear();
        }, 5000);

        while (this.activeRequests.size > 0) {
            await new Promise(resolve => setTimeout(resolve, 100));
        }
        clearTimeout(timeout);
        
        logger.info('TranscriptionService shutdown complete');
    }

    async processAudioFile(callId, audioPath, audioMessage, retries = 3) {
        if (this.isShuttingDown) {
            throw new Error('Service is shutting down');
        }
        if (!this.openai) {
            throw new Error('Transcription service is not configured. Please set OPENAI_API_KEY environment variable.');
        }
        
        // Track this request
        const requestId = `${callId}_${Date.now()}`;
        this.activeRequests.add(requestId);
        try {
            logger.debug(`Starting transcription for call ${callId}`);
            
            // Validate audio message format
            if (!audioMessage?.srcList) {
                throw new Error('Invalid audio message format: missing required metadata');
            }

            // Get audio file from GridFS
            const gridFSBucket = getGridFSBucket('audioFiles');
            const files = await gridFSBucket.find({ filename: audioMessage.filename }).toArray();
            
            if (!files.length) {
                throw new Error(`Audio file not found: ${audioMessage.filename}`);
            }

            // Get WAV file if available
            const wavFile = files.find(f => f.filename.endsWith('.wav'));
            if (!wavFile) {
                throw new Error('WAV format not available for transcription');
            }

            // Validate file size
            if (wavFile.length === 0) {
                throw new Error('Audio file is empty');
            }

            // Download file to temp buffer for WAV header validation
            const chunks = [];
            const downloadStream = gridFSBucket.openDownloadStream(wavFile._id, { start: 0, end: 43 });
            
            await new Promise((resolve, reject) => {
                downloadStream.on('data', chunk => chunks.push(chunk));
                downloadStream.on('error', reject);
                downloadStream.on('end', resolve);
            });

            const header = Buffer.concat(chunks);
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
                        // Create a temporary file from GridFS stream
                        const tempFilePath = path.join(os.tmpdir(), `temp-${Date.now()}.wav`);
                        const writeStream = fs.createWriteStream(tempFilePath);
                        const downloadStream = gridFSBucket.openDownloadStream(wavFile._id);
                        
                        await new Promise((resolve, reject) => {
                            downloadStream.pipe(writeStream)
                                .on('error', reject)
                                .on('finish', resolve);
                        });

                        try {
                            let whisperResponse;
                            try {
                                // Perform network diagnostics before API call
                                const networkDiagnostics = await new Promise((resolve) => {
                                    const net = require('net');
                                    const tls = require('tls');
                                    const dns = require('dns');
                                    const url = new URL(process.env.OPENAI_API_BASE || 'https://api.openai.com/v1');
                                    
                                    logger.debug('Starting network diagnostics', {
                                        hostname: url.hostname,
                                        port: 443
                                    });

                                    dns.lookup(url.hostname, (err, address) => {
                                        if (err) {
                                            logger.warn('DNS lookup failed for OpenAI API', {
                                                error: err.message,
                                                code: err.code
                                            });
                                            resolve({ success: false, stage: 'dns', error: err });
                                            return;
                                        }

                                        logger.debug('DNS lookup successful', {
                                            hostname: url.hostname,
                                            address
                                        });

                                        // Test TCP and TLS connection
                                        logger.debug('Testing TCP/TLS connection', { address });
                                        
                                        const socket = tls.connect({
                                            host: address,
                                            port: 443,
                                            servername: url.hostname, // Required for SNI
                                            timeout: 5000,
                                            rejectUnauthorized: true
                                        });
                                        
                                        socket.on('secureConnect', async () => {
                                            logger.debug('TLS connection established', {
                                                authorized: socket.authorized,
                                                protocol: socket.getProtocol(),
                                                cipher: socket.getCipher()
                                            });

                                            // Test HTTP layer with a HEAD request
                                            try {
                                                socket.write(
                                                    'HEAD /v1/models HTTP/1.1\r\n' +
                                                    `Host: ${url.hostname}\r\n` +
                                                    'Connection: close\r\n\r\n'
                                                );

                                                const response = await new Promise((resolveHttp) => {
                                                    let data = '';
                                                    socket.on('data', chunk => data += chunk);
                                                    socket.on('end', () => resolveHttp(data));
                                                });

                                                logger.debug('HTTP layer test completed', {
                                                    response_status: response.split('\n')[0]
                                                });

                                                resolve({ 
                                                    success: true, 
                                                    address, 
                                                    port: 443,
                                                    tls: {
                                                        authorized: socket.authorized,
                                                        protocol: socket.getProtocol(),
                                                        cipher: socket.getCipher()
                                                    },
                                                    http: {
                                                        status: response.split('\n')[0]
                                                    }
                                                });
                                            } catch (error) {
                                                logger.warn('HTTP layer test failed', {
                                                    error: error.message
                                                });
                                                resolve({
                                                    success: false,
                                                    stage: 'http',
                                                    error: error.message
                                                });
                                            } finally {
                                                socket.end();
                                            }
                                        });
                                        
                                        socket.on('timeout', () => {
                                            logger.warn('TLS connection timeout');
                                            socket.destroy();
                                            resolve({ success: false, stage: 'tls', error: 'Connection timeout' });
                                        });
                                        
                                        socket.on('error', (error) => {
                                            logger.warn('TLS connection error', {
                                                error: error.message,
                                                code: error.code
                                            });
                                            resolve({ success: false, stage: 'tls', error: error.message });
                                        });
                                    });
                                });

                                if (!networkDiagnostics.success) {
                                    logger.error('Network connectivity check failed', {
                                        diagnostics: networkDiagnostics,
                                        api_base: process.env.OPENAI_API_BASE
                                    });
                                    throw new Error(`Network connectivity check failed at ${networkDiagnostics.stage} stage`);
                                }

                                // Check rate limit
                                if (this.requestCount >= this.rateLimitMax) {
                                    const waitTime = this.rateLimitWindow - (Date.now() - this.lastReset);
                                    logger.warn('Rate limit reached, waiting', {
                                        wait_ms: waitTime,
                                        current_count: this.requestCount,
                                        max_requests: this.rateLimitMax,
                                        call_id: callId
                                    });
                                    await new Promise(resolve => setTimeout(resolve, waitTime));
                                }

                                this.requestCount++;
                                whisperResponse = await this.openai.audio.transcriptions.create({
                                    file: fs.createReadStream(tempFilePath),
                                    model: process.env.WHISPER_MODEL || "guillaumekln/faster-whisper-base.en",
                                    response_format: "verbose_json",
                                    timestamp_granularities: ["word"]
                                });
                            } catch (error) {
                                // Enhance network error information
                                if (error.cause?.code === 'ECONNRESET') {
                                    logger.error('Network connection reset', {
                                        error_details: {
                                            code: error.cause.code,
                                            message: error.message,
                                            stack: error.stack
                                        },
                                        network_info: {
                                            api_base: process.env.OPENAI_API_BASE,
                                            dns_lookup: await new Promise((resolve) => {
                                                require('dns').lookup('api.openai.com', (err, address) => {
                                                    resolve(err ? err.message : address);
                                                });
                                            })
                                        },
                                        request_info: {
                                            file_size: fs.statSync(tempFilePath).size,
                                            model: process.env.WHISPER_MODEL,
                                            attempt: attempt
                                        }
                                    });
                                }
                                throw error;
                            }
                            
                            // Check quality before proceeding
                            const quality = this.assessTranscriptionQuality(whisperResponse, callId);
                            if (quality.needsRetry && attempt < retries) {
                                throw new Error(`Low quality transcription: ${quality.reason}`);
                            }
                            
                            return whisperResponse;
                        } finally {
                            // Clean up temp file
                            fs.unlinkSync(tempFilePath);
                        }
                    };

                    const whisperResponse = await this.queue.add(processTranscription);
                    return await this.saveTranscription(callId, whisperResponse, startTime, audioMessage);
                } catch (error) {
                    lastError = error;
                    // Enhanced error logging
                    if (error.cause) {
                        logger.warn('Transcription attempt failed', {
                            attempt,
                            call_id: callId,
                            error_code: error.cause.code,
                            error_type: error.cause.type,
                            error_message: error.cause.message,
                            error_stack: error.stack,
                            request_details: {
                                model: process.env.WHISPER_MODEL,
                                api_base: process.env.OPENAI_API_BASE,
                                attempt: attempt,
                                call_id: callId
                            }
                        });
                    } else {
                        logger.warn('Transcription attempt failed', {
                            attempt,
                            call_id: callId,
                            error: error.message,
                            stack: error.stack,
                            request_details: {
                                model: process.env.WHISPER_MODEL,
                                api_base: process.env.OPENAI_API_BASE,
                                attempt: attempt,
                                call_id: callId
                            }
                        });
                    }
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
        } finally {
            // Remove request tracking
            this.activeRequests.delete(requestId);
        }
    }

    async saveTranscription(callId, whisperResponse, startTime, audioMessage) {
        if (this.isShuttingDown) {
            throw new Error('Service is shutting down');
        }
        const processingTime = (Date.now() - startTime) / 1000;
        
        // Get source list from audio message
        const srcList = audioMessage.srcList || [];
        
        // Get segments from either format
        const sourceSegments = Array.isArray(whisperResponse.segments) ? 
            whisperResponse.segments :
            whisperResponse.words.map(word => ({
                start: word.start,
                end: word.end,
                text: word.word,
                confidence: 1.0
            }));

        // Convert segments to our schema format and map to sources
        const segments = sourceSegments.map(seg => {
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
            // Save to transcriptions collection
            const transcriptionCollection = mongoose.connection.db.collection('transcriptions');
            await transcriptionCollection.insertOne(transcription);
            
            // Update audio record with transcription
            const audioCollection = mongoose.connection.db.collection('audio');
            
            // Parse call ID - handle both formats (system_talkgroup_timestamp and talkgroup-timestamp)
            let talkgroup, start_time;
            if (callId.includes('_')) {
                // New format: system_talkgroup_timestamp
                const [, tg, ts] = callId.split('_').map(part => parseInt(part));
                talkgroup = tg;
                start_time = ts;
            } else {
                // Old format: talkgroup-timestamp
                const [tg, ts] = callId.split('-').map(part => parseInt(part));
                talkgroup = tg;
                start_time = ts;
            }
            
            await audioCollection.updateOne(
                { 
                    type: 'audio',
                    talkgroup: talkgroup,
                    start_time: start_time,
                    filename: audioMessage.filename
                },
                { 
                    $set: { 
                        transcription: {
                            text: transcription.text,
                            segments: transcription.segments,
                            processing_time: transcription.processing_time,
                            model: transcription.model,
                            timestamp: transcription.timestamp
                        }
                    }
                }
            );
            
            logger.info(`Saved transcription for call ${callId}, duration: ${processingTime}s`);
            stateEventEmitter.emitTranscriptionNew(transcription);
            return transcription;
        } catch (error) {
            logger.error(`Failed to save transcription for call ${callId}:`, error);
            throw error;
        }
    }

    assessTranscriptionQuality(whisperResponse, callId) {
        const MIN_CONFIDENCE = 0.6;
        const MIN_DURATION = 0.5;

        // Validate response structure
        if (!whisperResponse) {
            logger.warn('Received null/undefined whisper response', {
                call_id: callId,
                model: process.env.WHISPER_MODEL
            });
            return {
                needsRetry: true,
                reason: 'Invalid response: received null/undefined'
            };
        }

        // Handle both OpenAI API and local whisper formats
        let segments = [];
        if (Array.isArray(whisperResponse.segments)) {
            // Local whisper format
            segments = whisperResponse.segments;
        } else if (Array.isArray(whisperResponse.words)) {
            // OpenAI API format - convert words to segments
            segments = whisperResponse.words.map(word => ({
                start: word.start,
                end: word.end,
                text: word.word,
                confidence: 1.0 // OpenAI API doesn't provide confidence scores
            }));
        } else {
            logger.warn('Invalid whisper response format', {
                response_type: typeof whisperResponse,
                response_keys: Object.keys(whisperResponse || {}),
                response: JSON.stringify(whisperResponse),
                call_id: callId,
                model: process.env.WHISPER_MODEL
            });
            return {
                needsRetry: true,
                reason: 'Invalid response format: missing segments/words array'
            };
        }

        if (segments.length === 0) {
            logger.warn('Empty segments/words array in response', {
                call_id: callId,
                response: whisperResponse
            });
            return {
                needsRetry: true,
                reason: 'Empty segments array'
            };
        }
        
        // Check for missing confidence scores (only for local whisper)
        const validSegments = Array.isArray(whisperResponse.segments) ? 
            segments.filter(seg => typeof seg.confidence === 'number') :
            segments; // OpenAI format always has confidence = 1.0
        if (validSegments.length === 0) {
            logger.warn('No valid confidence scores in segments', {
                call_id: callId,
                segments: whisperResponse.segments
            });
            return {
                needsRetry: true,
                reason: 'No segments with valid confidence scores'
            };
        }

        // Calculate average confidence (only check for local whisper)
        const avgConfidence = Array.isArray(whisperResponse.segments) ?
            validSegments.reduce((sum, seg) => sum + seg.confidence, 0) / validSegments.length :
            1.0; // OpenAI format always has confidence = 1.0

        if (Array.isArray(whisperResponse.segments) && avgConfidence < MIN_CONFIDENCE) {
            logger.warn('Low confidence score', {
                call_id: callId,
                avg_confidence: avgConfidence,
                threshold: MIN_CONFIDENCE,
                valid_segments: validSegments.length
            });
            return {
                needsRetry: true,
                reason: `Low confidence score: ${avgConfidence.toFixed(2)}`
            };
        }

        // Check for suspiciously short duration
        if (!whisperResponse.duration || whisperResponse.duration < MIN_DURATION) {
            logger.warn('Suspicious duration', {
                call_id: callId,
                duration: whisperResponse.duration,
                threshold: MIN_DURATION
            });
            return {
                needsRetry: true,
                reason: `Suspicious duration: ${whisperResponse.duration || 0}s`
            };
        }

        // Check for empty or very short text
        if (!whisperResponse.text || whisperResponse.text.length < 2) {
            logger.warn('Empty or very short transcription', {
                call_id: callId,
                text_length: whisperResponse.text?.length || 0
            });
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

// Create and export a singleton instance
const instance = new TranscriptionService();
module.exports = TranscriptionService;
module.exports.getInstance = () => instance;
