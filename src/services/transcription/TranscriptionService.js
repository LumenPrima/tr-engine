const logger = require('../../utils/logger');
const timestamps = require('../../utils/timestamps');
const OpenAI = require('openai');
const fs = require('fs');
const mongoose = require('mongoose');
const path = require('path');
const os = require('os');

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
        this.lastReset = timestamps.getCurrentUnix();
        this.queue = new RequestQueue();

        // Initialize OpenAI client with environment config
        const apiKey = process.env.OPENAI_API_KEY;
        if (!apiKey) {
            logger.warn('OpenAI API key not found in environment');
        }
        this.openai = new OpenAI({ apiKey });

        logger.info('TranscriptionService initialized');
    }

    async processAudioFile(audioFile) {
        try {
            // Validate audio file
            if (!audioFile || !audioFile.filename) {
                throw new Error('Invalid audio file format');
            }

            // Get audio metadata
            const metadata = await this.getAudioMetadata(audioFile);
            if (!metadata) {
                throw new Error('Invalid call duration in metadata');
            }

            const startTime = timestamps.getCurrentUnix();
            let lastError;

            // Retry loop
            for (let attempt = 1; attempt <= 3; attempt++) {
                try {
                    // Queue the transcription request
                    const processTranscription = async () => {
                        // Create a temporary file from GridFS stream
                        const tempFilePath = path.join(os.tmpdir(), `temp-${timestamps.getCurrentUnix()}.wav`);
                        const writeStream = fs.createWriteStream(tempFilePath);
                        const downloadStream = metadata.gridFSBucket.openDownloadStream(metadata.fileId);

                        await new Promise((resolve, reject) => {
                            downloadStream.pipe(writeStream)
                                .on('error', reject)
                                .on('finish', resolve);
                        });

                        // Process with Whisper API
                        const transcription = await this.transcribeAudio(tempFilePath);

                        // Clean up temp file
                        fs.unlinkSync(tempFilePath);

                        return transcription;
                    };

                    const transcription = await this.queue.add(processTranscription);
                    await this.saveTranscription(audioFile.call_id, transcription, startTime, audioFile);
                    return transcription;

                } catch (error) {
                    lastError = error;
                    logger.error(`Transcription attempt ${attempt} failed:`, error);
                    await new Promise(resolve => setTimeout(resolve, 1000 * attempt));
                }
            }

            throw lastError || new Error('Failed to process transcription after retries');

        } catch (error) {
            logger.error('Error processing audio file:', error);
            throw error;
        }
    }

    async getAudioMetadata(audioFile) {
        try {
            const db = mongoose.connection.db;
            const gridFSBucket = new mongoose.mongo.GridFSBucket(db, {
                bucketName: 'audio'
            });

            // Find the audio file in GridFS
            const files = await db.collection('audio.files')
                .find({ filename: audioFile.filename })
                .toArray();

            if (!files || files.length === 0) {
                logger.error('Audio file not found in GridFS:', audioFile.filename);
                return null;
            }

            const wavFile = files[0];
            if (!wavFile || !wavFile.length) {
                logger.error('Invalid audio file metadata:', wavFile);
                return null;
            }

            return {
                gridFSBucket,
                fileId: wavFile._id,
                duration: wavFile.length
            };

        } catch (error) {
            logger.error('Error getting audio metadata:', error);
            return null;
        }
    }

    async transcribeAudio(filePath) {
        try {
            const transcription = await this.openai.audio.transcriptions.create({
                file: fs.createReadStream(filePath),
                model: process.env.WHISPER_MODEL || "whisper-1",
                language: "en"
            });

            return {
                text: transcription.text,
                duration: transcription.duration || 0
            };

        } catch (error) {
            logger.error('Error transcribing audio:', error);
            throw error;
        }
    }

    async saveTranscription(callId, whisperResponse, startTime, audioMessage) {
        const processingTime = timestamps.diffSeconds(
            timestamps.getCurrentUnix(),
            startTime
        );

        // Get source list from audio message
        const srcList = audioMessage.srcList || [];

        // Parse call ID - handle all formats
        let talkgroup, start_time, sys_num;
        const parts = callId.split('_');

        if (parts.length === 3) {
            // Format: system_talkgroup_timestamp
            [sys_num, talkgroup, start_time] = parts.map(part => parseInt(part));
        } else if (parts.length === 2) {
            // Format: talkgroup_timestamp
            [talkgroup, start_time] = parts.map(part => parseInt(part));
        } else {
            // Try legacy format: talkgroup-starttime
            const legacyParts = callId.split('-');
            if (legacyParts.length === 2) {
                [talkgroup, start_time] = legacyParts.map(part => parseInt(part));

                // Check if talkgroup includes system number (e.g. "2_58259")
                if (talkgroup.toString().includes('_')) {
                    const [sys, tg] = talkgroup.toString().split('_').map(p => parseInt(p));
                    sys_num = sys;
                    talkgroup = tg;
                }
            }
        }

        // Save transcription to MongoDB
        const collection = mongoose.connection.db.collection('transcriptions');
        await collection.insertOne({
            call_id: callId,
            text: whisperResponse.text,
            audio_duration: whisperResponse.duration,
            processing_time: processingTime,
            model: process.env.WHISPER_MODEL,
            timestamp: timestamps.getCurrentUnix(),
            talkgroup: audioMessage.talkgroup,
            talkgroup_tag: audioMessage.talkgroup_tag,
            sys_name: audioMessage.sys_name,
            sys_num: audioMessage.sys_num,
            srcList: srcList
        });

        // Update audio record with transcription
        const audioCollection = mongoose.connection.db.collection('audio');
        await audioCollection.updateOne(
            {
                type: 'audio',
                talkgroup: talkgroup,
                start_time: start_time,
                ...(sys_num ? { sys_num: sys_num } : {}),
                filename: audioMessage.filename
            },
            {
                $set: {
                    transcribed: true,
                    transcription: whisperResponse.text,
                    transcription_model: process.env.WHISPER_MODEL,
                    transcription_time: timestamps.getCurrentUnix()
                }
            }
        );

        logger.info(`Saved transcription for call ${callId}`);
    }

    async getTranscriptionsByTalkgroup(talkgroupId, startDate, endDate, limit = 10) {
        try {
            const collection = mongoose.connection.db.collection('transcriptions');
            const query = { talkgroup: parseInt(talkgroupId) };

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
            logger.error('Error retrieving transcriptions:', error);
            throw error;
        }
    }

    async getTranscriptionStats(talkgroupId, startDate, endDate) {
        try {
            const collection = mongoose.connection.db.collection('transcriptions');
            const match = { };

            if (talkgroupId) {
                match.talkgroup = parseInt(talkgroupId);
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
                        _id: null,
                        total_transcriptions: { $sum: 1 },
                        total_duration: { $sum: '$audio_duration' },
                        avg_duration: { $avg: '$audio_duration' },
                        avg_processing_time: { $avg: '$processing_time' },
                        models_used: { $addToSet: '$model' }
                    }
                }
            ]).toArray();

        } catch (error) {
            logger.error('Error getting transcription stats:', error);
            throw error;
        }
    }
}

module.exports = TranscriptionService;
