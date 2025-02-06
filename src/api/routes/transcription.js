const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const mongoose = require('mongoose');
const TranscriptionService = require('../../services/transcription/TranscriptionService');
const path = require('path');
const rateLimit = require('express-rate-limit');

// Initialize services
const transcriptionService = TranscriptionService.getInstance();

// Rate limiting middleware
const apiLimiter = rateLimit({
    windowMs: 15 * 60 * 1000, // 15 minutes
    max: 100 // limit each IP to 100 requests per windowMs
});

// Utility functions
const utils = {
    parseCallId: (callId) => {
        let talkgroup, targetTime, sysNum;
        
        // Parse call ID formats
        const parts = callId.split('_');
        
        if (parts.length === 3) {
            // Primary format: sysnum_talkgroup_timestamp
            [sysNum, talkgroup, targetTime] = parts.map(p => parseInt(p));
        } else {
            // Try legacy formats for backward compatibility
            const legacyParts = callId.split('-');
            if (legacyParts.length === 2) {
                // Legacy format: talkgroup-starttime
                [talkgroup, targetTime] = legacyParts.map(p => parseInt(p));
                sysNum = 0; // Default system number for legacy IDs
            } else if (parts.length === 2) {
                // Legacy format: talkgroup_timestamp
                [talkgroup, targetTime] = parts.map(p => parseInt(p));
                sysNum = 0; // Default system number for legacy IDs
            }
        }

        if (isNaN(talkgroup) || isNaN(targetTime)) {
            throw new Error('Invalid call_id format. Expected sysnum_talkgroup_timestamp');
        }

        return { talkgroup, targetTime, sysNum };
    },

    parseTimestamp: (timeParam) => {
        if (!timeParam) return new Date();

        // Try parsing as ISO date first
        let date = new Date(timeParam);
        
        // If invalid date and looks like Unix timestamp, try parsing as Unix timestamp
        if (isNaN(date.getTime()) && /^\d+$/.test(timeParam)) {
            date = new Date(parseInt(timeParam) * 1000);
        }
        
        if (isNaN(date.getTime())) {
            throw new Error('Invalid time format. Please provide either a Unix timestamp or ISO date string');
        }

        return date;
    },

    formatTranscription: (t) => ({
        call_id: t.sys_num ? 
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
    })
};

// Database operations
const db = {
    async findNearestTranscriptions(collection, query, targetTime, limit) {
        return collection.aggregate([
            { $match: query },
            {
                $addFields: {
                    timestampDate: {
                        $cond: {
                            if: { $eq: [{ $type: '$timestamp' }, 'date'] },
                            then: '$timestamp',
                            else: { $toDate: '$timestamp' }
                        }
                    }
                }
            },
            {
                $addFields: {
                    timeDiff: {
                        $abs: {
                            $subtract: [
                                { $toLong: '$timestampDate' },
                                { $multiply: [targetTime, 1000] }
                            ]
                        }
                    }
                }
            },
            { $sort: { timeDiff: 1 } },
            { $limit: limit }
        ]).toArray();
    },

    async getTranscriptionStats(collection, match) {
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

        return stats || {
            total_transcriptions: 0,
            total_duration: 0,
            avg_duration: 0,
            avg_processing_time: 0,
            total_words: 0,
            words_per_second: 0
        };
    }
};

// Error handling middleware
const asyncHandler = (fn) => (req, res, next) =>
    Promise.resolve(fn(req, res, next)).catch(next);

// Response formatting middleware
const formatResponse = (res, data) => {
    res.json({
        status: 'success',
        timestamp: new Date().toISOString(),
        data
    });
};

// Endpoints
// Fixed routes first
router.get('/group', asyncHandler(async (req, res) => {
    const limit = parseInt(req.query.limit) || 10;
    const targetDate = utils.parseTimestamp(req.query.time);
    const targetTime = Math.floor(targetDate.getTime() / 1000);
    
    const collection = mongoose.connection.db.collection('transcriptions');
    const transcriptions = await db.findNearestTranscriptions(collection, {}, targetTime, limit);
    
    formatResponse(res, {
        count: transcriptions.length,
        transcriptions: transcriptions.map(utils.formatTranscription),
        target: new Date(targetTime * 1000)
    });
}));

// Stats endpoint
router.get('/stats', asyncHandler(async (req, res) => {
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

    const stats = await db.getTranscriptionStats(collection, match);
    formatResponse(res, stats);
}));

// Process endpoint
router.post('/process/:callId', apiLimiter, asyncHandler(async (req, res) => {
    const { talkgroup, targetTime, sysNum } = utils.parseCallId(req.params.callId);
    
        // Get audio metadata
        const audioCollection = mongoose.connection.db.collection('audio');
        
        // Try exact match first
        let audioMetadata = await audioCollection.findOne({
            talkgroup: talkgroup,
            start_time: targetTime,
            ...(typeof sysNum === 'number' ? { sys_num: sysNum } : {})
        });

        // If not found, try finding closest match
        if (!audioMetadata) {
            audioMetadata = await audioCollection.aggregate([
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
                { $sort: { timeDiff: 1 } },
                { $limit: 1 }
            ]).next();
        }

        if (!audioMetadata) {
            return res.status(404).json({
                status: 'error',
                message: 'Resource not found',
                timestamp: new Date().toISOString()
            });
        }

    const audioPath = path.join(process.env.AUDIO_STORAGE_PATH || 'audio_files', audioMetadata.filename);
    const transcription = await transcriptionService.processAudioFile(req.params.callId, audioPath, audioMetadata);
    
    formatResponse(res, transcription);
}));

// Parameterized routes last
router.get('/:talkgroupId/recent', asyncHandler(async (req, res) => {
    const options = {
        limit: parseInt(req.query.limit) || 10,
        startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000),
        endTime: req.query.end ? new Date(req.query.end) : new Date()
    };

    const collection = mongoose.connection.db.collection('transcriptions');
    const transcriptions = await collection.find({
        talkgroup: parseInt(req.params.talkgroupId),
        $expr: {
            $and: [
                {
                    $gte: [
                        {
                            $cond: {
                                if: { $eq: [{ $type: '$timestamp' }, 'date'] },
                                then: '$timestamp',
                                else: { $toDate: '$timestamp' }
                            }
                        },
                        options.startTime
                    ]
                },
                {
                    $lte: [
                        {
                            $cond: {
                                if: { $eq: [{ $type: '$timestamp' }, 'date'] },
                                then: '$timestamp',
                                else: { $toDate: '$timestamp' }
                            }
                        },
                        options.endTime
                    ]
                }
            ]
        }
    })
    .sort({ timestamp: -1 })
    .limit(options.limit)
    .toArray();

    formatResponse(res, {
        count: transcriptions.length,
        transcriptions: transcriptions.map(utils.formatTranscription),
        range: {
            start: options.startTime,
            end: options.endTime
        }
    });
}));

// Group by talkgroup endpoint
router.get('/:talkgroupId/group', asyncHandler(async (req, res) => {
    const talkgroupId = parseInt(req.params.talkgroupId);
    if (isNaN(talkgroupId)) {
        throw new Error('Invalid talkgroup format');
    }

    const limit = parseInt(req.query.limit) || 10;
    const targetDate = utils.parseTimestamp(req.query.time);
    const targetTime = Math.floor(targetDate.getTime() / 1000);
    
    const collection = mongoose.connection.db.collection('transcriptions');
    const transcriptions = await db.findNearestTranscriptions(
        collection,
        { talkgroup: talkgroupId },
        targetTime,
        limit
    );
    
    formatResponse(res, {
        count: transcriptions.length,
        transcriptions: transcriptions.map(utils.formatTranscription),
        target: new Date(targetTime * 1000)
    });
}));

// Get specific call endpoint (must be last)
router.get('/:callId', asyncHandler(async (req, res) => {
    const collection = mongoose.connection.db.collection('transcriptions');
    const { talkgroup, targetTime, sysNum } = utils.parseCallId(req.params.callId);

    // Try exact match first
    let transcription = await collection.findOne({ call_id: req.params.callId });
    
    // If not found, try searching by talkgroup and nearest timestamp
    if (!transcription) {
        const [nearestTranscription] = await db.findNearestTranscriptions(
            collection,
            {
                talkgroup: talkgroup,
                ...(typeof sysNum === 'number' ? { sys_name: `sys${sysNum}` } : {})
            },
            targetTime,
            1
        );
        transcription = nearestTranscription;
    }

    if (!transcription) {
        throw new Error('Transcription not found');
    }

    formatResponse(res, {
        call_id: req.params.callId,
        text: transcription.text,
        audio_duration: transcription.audio_duration,
        processing_time: transcription.processing_time,
        model: transcription.model,
        created_at: new Date(transcription.timestamp * 1000)
    });
}));

// Error handling middleware (must be last)
router.use((err, req, res, next) => {
    logger.error('API Error:', err);
    res.status(err.status || 500).json({
        status: 'error',
        message: err.message || 'Internal server error',
        timestamp: new Date().toISOString(),
        details: process.env.NODE_ENV === 'development' ? {
            error: err.message,
            stack: err.stack
        } : undefined
    });
});

module.exports = router;
