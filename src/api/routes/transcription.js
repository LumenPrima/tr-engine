const express = require('express');
const router = express.Router();
const TranscriptionService = require('../../services/transcription/TranscriptionService');
const logger = require('../../utils/logger');

const transcriptionService = new TranscriptionService();

// Get transcription for a specific call
router.get('/calls/:callId/transcription', async (req, res) => {
    try {
        const transcription = await transcriptionService.getTranscription(req.params.callId);
        if (!transcription) {
            return res.status(404).json({ error: 'Transcription not found' });
        }
        res.json(transcription);
    } catch (error) {
        logger.error('Error fetching transcription:', error);
        res.status(500).json({ error: 'Internal server error' });
    }
});

// Get recent transcriptions for a talkgroup
router.get('/talkgroups/:talkgroupId/recent_transcriptions', async (req, res) => {
    try {
        const limit = parseInt(req.query.limit) || 10;
        const transcriptions = await transcriptionService.getRecentTranscriptions(
            parseInt(req.params.talkgroupId),
            limit
        );
        res.json(transcriptions);
    } catch (error) {
        logger.error('Error fetching recent transcriptions:', error);
        res.status(500).json({ error: 'Internal server error' });
    }
});

module.exports = router;
