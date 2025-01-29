const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const mongoose = require('mongoose');
const TranscriptionService = require('../../services/transcription/TranscriptionService');
const path = require('path');

const transcriptionService = new TranscriptionService();

// Helper function to parse call ID
const parseCallId = (callId) => {
    let talkgroup, targetTime, sysNum;
    
    // Parse call ID formats
    const parts = callId.split('_');
    
    if (parts.length === 3) {
        // Format: sys_num_talkgroup_timestamp (e.g., 2_58259_1738107954)
        [sysNum, talkgroup, targetTime] = parts.map(p => parseInt(p));
    } else if (parts.length === 2) {
        // Format: talkgroup_timestamp (e.g., 58259_1738107954)
        [talkgroup, targetTime] = parts.map(p => parseInt(p));
    } else {
        // Try legacy format: talkgroup-starttime
        const legacyParts = callId.split('-');
        if (legacyParts.length === 2) {
            [talkgroup, targetTime] = legacyParts.map(p => parseInt(p));
        }
    }

    if (isNaN(talkgroup) || isNaN(targetTime)) {
        throw new Error('Invalid call_id format');
    }

    return { talkgroup, targetTime, sysNum };
};

// POST endpoint to transcribe an audio file
router.post('/process/:callId', async (req, res) => {
    try {
        const callId = req.params.callId;
        const { talkgroup, targetTime, sysNum } = parseCallId(callId);
        
        // Get audio metadata
        const audioCollection = mongoose.connection.db.collection('audio');

        // Find audio metadata with closest start_time
        const audioMetadata = await audioCollection.aggregate([
            { 
                $match: {
                    talkgroup: talkgroup,
                    ...(typeof sysNum === 'number' ? { sys_num: sysNum } : {})
                }
            },
            {
                $addFields: {
                    timeDiff: { $abs: { $subtract: ['$start_time', targetTime] } }
                }
            },
            {
                $sort: { timeDiff: 1 }
            },
            {
                $limit: 1
            }
        ]).next();

        if (!audioMetadata) {
            return res.status(404).json({
                status: 'error',
                message: 'Audio metadata not found',
                timestamp: new Date().toISOString()
            });
        }

        // Get audio file path
        const audioPath = path.join(process.env.AUDIO_STORAGE_PATH || 'audio_files', audioMetadata.filename);
        
        // Process transcription
        const transcription = await transcriptionService.processAudioFile(callId, audioPath, audioMetadata);

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: transcription
        });
    } catch (error) {
        logger.error('Error processing transcription:', error);
        res.status(500).json({
            status: 'error',
            message: error.message || 'Failed to process transcription',
            timestamp: new Date().toISOString()
        });
    }
});

// Get transcription for a specific call
router.get('/:callId', async (req, res) => {
    try {
        const collection = mongoose.connection.db.collection('transcriptions');
        const { talkgroup, targetTime, sysNum } = parseCallId(req.params.callId);

        // Find transcription with closest start_time
        const transcription = await collection.aggregate([
            { 
                $match: {
                    talkgroup: talkgroup,
                    ...(typeof sysNum === 'number' ? { sys_name: `sys${sysNum}` } : {})
                }
            },
            {
                $addFields: {
                    timeDiff: { $abs: { $subtract: ['$timestamp', targetTime] } }
                }
            },
            {
                $sort: { timeDiff: 1 }
            },
            {
                $limit: 1
            }
        ]).next();
        
        if (!transcription) {
            return res.status(404).json({
                status: 'error',
                message: 'Transcription not found',
                timestamp: new Date().toISOString()
            });
        }

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                call_id: req.params.callId,
                text: transcription.text,
                audio_duration: transcription.audio_duration,
                processing_time: transcription.processing_time,
                model: transcription.model,
                created_at: new Date(transcription.timestamp * 1000)
            }
        });
    } catch (error) {
        logger.error('Error fetching transcription:', error);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve transcription',
            timestamp: new Date().toISOString()
        });
    }
});

// Get recent transcriptions for a talkgroup
router.get('/:talkgroupId/recent', async (req, res) => {
    try {
        const options = {
            limit: parseInt(req.query.limit) || 10,
            startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000),
            endTime: req.query.end ? new Date(req.query.end) : new Date()
        };

        const collection = mongoose.connection.db.collection('transcriptions');
        const transcriptions = await collection.find({
            talkgroup: parseInt(req.params.talkgroupId),
            timestamp: { 
                $gte: Math.floor(options.startTime.getTime() / 1000),
                $lte: Math.floor(options.endTime.getTime() / 1000)
            }
        })
        .sort({ timestamp: -1 })
        .limit(options.limit)
        .toArray();

        const formattedTranscriptions = transcriptions.map(t => ({
            call_id: typeof t.sys_num === 'number' ? 
                `${t.sys_num}_${t.talkgroup}_${t.timestamp}` : 
                `${t.talkgroup}_${t.timestamp}`,
            text: t.text,
            timestamp: new Date(t.timestamp * 1000),
            metadata: {
                audio_duration: t.audio_duration,
                processing_time: t.processing_time,
                model: t.model
            }
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                count: formattedTranscriptions.length,
                transcriptions: formattedTranscriptions,
                range: {
                    start: options.startTime,
                    end: options.endTime
                }
            }
        });
    } catch (error) {
        logger.error('Error fetching recent transcriptions:', error);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve transcriptions',
            timestamp: new Date().toISOString()
        });
    }
});

// Get transcription statistics
router.get('/stats', async (req, res) => {
    try {
        const collection = mongoose.connection.db.collection('transcriptions');
        const match = {};
        
        if (req.query.talkgroup) {
            match.talkgroup = parseInt(req.query.talkgroup);
        }
        if (req.query.start || req.query.end) {
            match.timestamp = {};
            if (req.query.start) match.timestamp.$gte = Math.floor(new Date(req.query.start).getTime() / 1000);
            if (req.query.end) match.timestamp.$lte = Math.floor(new Date(req.query.end).getTime() / 1000);
        }

        const [stats] = await collection.aggregate([
            { $match: match },
            {
                $group: {
                    _id: null,
                    total_transcriptions: { $sum: 1 },
                    total_duration: { $sum: '$audio_duration' },
                    avg_duration: { $avg: '$audio_duration' },
                    avg_processing_time: { $avg: '$processing_time' },
                    total_words: {
                        $sum: {
                            $size: { $split: ['$text', ' '] }
                        }
                    }
                }
            },
            {
                $project: {
                    _id: 0,
                    total_transcriptions: 1,
                    total_duration: 1,
                    avg_duration: 1,
                    avg_processing_time: 1,
                    total_words: 1,
                    words_per_second: {
                        $divide: ['$total_words', '$total_duration']
                    }
                }
            }
        ]).toArray();

        if (!stats) {
            return res.json({
                status: 'success',
                timestamp: new Date().toISOString(),
                data: {
                    total_transcriptions: 0,
                    total_duration: 0,
                    avg_duration: 0,
                    avg_processing_time: 0,
                    total_words: 0,
                    words_per_second: 0
                }
            });
        }

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: stats
        });
    } catch (error) {
        logger.error('Error fetching transcription stats:', error);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve transcription statistics',
            timestamp: new Date().toISOString()
        });
    }
});

// Get a group of transcriptions closest to a timestamp
router.get('/group', async (req, res) => {
    try {
        const targetTime = parseInt(req.query.time);
        const limit = parseInt(req.query.limit) || 10;

        if (isNaN(targetTime)) {
            return res.status(400).json({
                status: 'error',
                message: 'Invalid timestamp format',
                timestamp: new Date().toISOString()
            });
        }

        const collection = mongoose.connection.db.collection('transcriptions');
        const transcriptions = await collection.aggregate([
            {
                $addFields: {
                    timeDiff: { $abs: { $subtract: ['$timestamp', targetTime] } }
                }
            },
            {
                $sort: { timeDiff: 1 }
            },
            {
                $limit: limit
            }
        ]).toArray();

        const formattedTranscriptions = transcriptions.map(t => ({
            call_id: typeof t.sys_num === 'number' ? 
                `${t.sys_num}_${t.talkgroup}_${t.timestamp}` : 
                `${t.talkgroup}_${t.timestamp}`,
            talkgroup: t.talkgroup,
            text: t.text,
            timestamp: new Date(t.timestamp * 1000),
            time_diff_seconds: t.timeDiff,
            metadata: {
                audio_duration: t.audio_duration,
                processing_time: t.processing_time,
                model: t.model
            }
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                count: formattedTranscriptions.length,
                transcriptions: formattedTranscriptions,
                target: new Date(targetTime * 1000)
            }
        });
    } catch (error) {
        logger.error('Error finding nearest transcriptions:', error);
        res.status(500).json({
            status: 'error',
            message: 'Failed to find nearest transcriptions',
            timestamp: new Date().toISOString()
        });
    }
});

// Get a group of transcriptions closest to a timestamp for a specific talkgroup
router.get('/:talkgroupId/group', async (req, res) => {
    try {
        const talkgroupId = parseInt(req.params.talkgroupId);
        const targetTime = parseInt(req.query.time);
        const limit = parseInt(req.query.limit) || 10;

        if (isNaN(targetTime) || isNaN(talkgroupId)) {
            return res.status(400).json({
                status: 'error',
                message: 'Invalid talkgroup or timestamp format',
                timestamp: new Date().toISOString()
            });
        }

        const collection = mongoose.connection.db.collection('transcriptions');
        const transcriptions = await collection.aggregate([
            {
                $match: { talkgroup: talkgroupId }
            },
            {
                $addFields: {
                    timeDiff: { $abs: { $subtract: ['$timestamp', targetTime] } }
                }
            },
            {
                $sort: { timeDiff: 1 }
            },
            {
                $limit: limit
            }
        ]).toArray();

        const formattedTranscriptions = transcriptions.map(t => ({
            call_id: typeof t.sys_num === 'number' ? 
                `${t.sys_num}_${t.talkgroup}_${t.timestamp}` : 
                `${t.talkgroup}_${t.timestamp}`,
            text: t.text,
            timestamp: new Date(t.timestamp * 1000),
            time_diff_seconds: t.timeDiff,
            metadata: {
                audio_duration: t.audio_duration,
                processing_time: t.processing_time,
                model: t.model
            }
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                count: formattedTranscriptions.length,
                transcriptions: formattedTranscriptions,
                target: new Date(targetTime * 1000)
            }
        });
    } catch (error) {
        logger.error('Error finding nearest transcriptions:', error);
        res.status(500).json({
            status: 'error',
            message: 'Failed to find nearest transcriptions',
            timestamp: new Date().toISOString()
        });
    }
});

module.exports = router;
