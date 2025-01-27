const express = require('express');
const router = express.Router();
const TranscriptionService = require('../../services/transcription/TranscriptionService');
const logger = require('../../utils/logger');

const transcriptionService = new TranscriptionService();

// Get transcription for a specific call
router.get('/calls/:callId/transcription', async (req, res) => {
    try {
        const call = await transcriptionService.getTranscription(req.params.callId);
        if (!call?.call?.metadata?.transcription) {
            return res.status(404).json({ error: 'Transcription not found' });
        }
        res.json(call.call.metadata.transcription);
    } catch (error) {
        logger.error('Error fetching transcription:', error);
        res.status(500).json({ error: 'Internal server error' });
    }
});

// Get recent transcriptions for a talkgroup
router.get('/talkgroups/:talkgroupId/recent_transcriptions', async (req, res) => {
    try {
        const limit = parseInt(req.query.limit) || 10;
        const startDate = req.query.start ? new Date(req.query.start) : null;
        const endDate = req.query.end ? new Date(req.query.end) : null;
        
        const transcriptions = await transcriptionService.getRecentTranscriptions(
            parseInt(req.params.talkgroupId),
            limit,
            startDate,
            endDate
        );
        res.json(transcriptions);
    } catch (error) {
        logger.error('Error fetching recent transcriptions:', error);
        res.status(500).json({ error: 'Internal server error' });
    }
});

// Get transcription statistics
router.get('/stats', async (req, res) => {
    try {
        const talkgroupId = req.query.talkgroup ? parseInt(req.query.talkgroup) : null;
        const startDate = req.query.start ? new Date(req.query.start) : null;
        const endDate = req.query.end ? new Date(req.query.end) : null;
        
        const stats = await transcriptionService.getTranscriptionStats(
            talkgroupId,
            startDate,
            endDate
        );
        res.json(stats);
    } catch (error) {
        logger.error('Error fetching transcription stats:', error);
        res.status(500).json({ error: 'Internal server error' });
    }
});

module.exports = router;
