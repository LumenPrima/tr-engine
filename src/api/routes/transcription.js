const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const mongoose = require('mongoose');

// Get transcription for a specific call
router.get('/calls/:callId/transcription', async (req, res) => {
    try {
        const collection = mongoose.connection.db.collection('transcriptions');
        const transcription = await collection.findOne({ call_id: req.params.callId });
        
        if (!transcription) {
            return res.status(404).json({ error: 'Transcription not found' });
        }

        res.json({
            text: transcription.text,
            metadata: {
                audio_duration: transcription.audio_duration,
                processing_time: transcription.processing_time,
                model: transcription.model,
                timestamp: transcription.timestamp
            }
        });
    } catch (error) {
        logger.error('Error fetching transcription:', error);
        res.status(500).json({ error: 'Internal server error' });
    }
});

// Get recent transcriptions for a talkgroup
router.get('/talkgroups/:talkgroupId/recent_transcriptions', async (req, res) => {
    try {
        const limit = parseInt(req.query.limit) || 10;
        const startDate = req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000);
        const endDate = req.query.end ? new Date(req.query.end) : new Date();
        
        const collection = mongoose.connection.db.collection('transcriptions');
        const transcriptions = await collection.find({
            talkgroup: parseInt(req.params.talkgroupId),
            timestamp: { $gte: startDate, $lte: endDate }
        })
        .sort({ timestamp: -1 })
        .limit(limit)
        .toArray();

        const formattedTranscriptions = transcriptions.map(t => ({
            call_id: t.call_id,
            text: t.text,
            timestamp: t.timestamp,
            metadata: {
                audio_duration: t.audio_duration,
                processing_time: t.processing_time,
                model: t.model
            }
        }));

        res.json({
            status: 'success',
            count: formattedTranscriptions.length,
            transcriptions: formattedTranscriptions
        });
    } catch (error) {
        logger.error('Error fetching recent transcriptions:', error);
        res.status(500).json({ error: 'Internal server error' });
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
            if (req.query.start) match.timestamp.$gte = new Date(req.query.start);
            if (req.query.end) match.timestamp.$lte = new Date(req.query.end);
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
                stats: {
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
            stats
        });
    } catch (error) {
        logger.error('Error fetching transcription stats:', error);
        res.status(500).json({ error: 'Internal server error' });
    }
});

module.exports = router;
