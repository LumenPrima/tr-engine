const express = require('express');
const router = express.Router();
const { Readable } = require('stream');

class AudioIntegrationManager {
    constructor(activeCallsManager, audioStorageManager) {
        this.activeCallsManager = activeCallsManager;
        this.audioStorageManager = audioStorageManager;
    }

    // Handle incoming audio messages
    async handleAudioMessage(topic, message) {
        try {
            // First, store the audio data
            const audioMessage = await this.audioStorageManager.processAudioMessage(message);
            
            // Update the active call with audio information
            const metadata = message.call.metadata;
            const callId = `${metadata.sys_num}_${metadata.talkgroup}_${metadata.start_time}`;
            
            await this.activeCallsManager.updateCallAudio(callId, {
                has_audio: true,
                audio_metadata: {
                    filename: metadata.filename,
                    audio_type: metadata.audio_type,
                    call_length: metadata.call_length,
                    freq_error: metadata.freq_error,
                    signal: metadata.signal,
                    noise: metadata.noise
                }
            });

            return audioMessage;
        } catch (error) {
            console.error('Error handling audio message:', error);
            throw error;
        }
    }

    // Set up API routes for audio access
    setupRoutes(app) {
        // Get audio file for a specific call
        app.get('/api/v1/audio/:talkgroup/:start_time', async (req, res) => {
            try {
                const { talkgroup, start_time } = req.params;
                const format = req.query.format || 'm4a';
                
                const { stream, metadata } = await this.audioStorageManager.getAudioFile(
                    parseInt(talkgroup),
                    parseInt(start_time)
                );

                // Set appropriate headers
                res.set({
                    'Content-Type': format === 'm4a' ? 'audio/mp4' : 'audio/wav',
                    'Content-Disposition': `attachment; filename="${metadata.filename}"`,
                    'X-Audio-Length': metadata.call_length,
                    'X-Audio-Type': metadata.audio_type
                });

                // Stream the audio file
                stream.pipe(res);
            } catch (error) {
                console.error('Error streaming audio:', error);
                res.status(404).json({
                    error: 'Audio file not found',
                    details: error.message
                });
            }
        });

        // Get audio metadata for a call
        app.get('/api/v1/audio/:talkgroup/:start_time/metadata', async (req, res) => {
            try {
                const { talkgroup, start_time } = req.params;
                
                const audioMessage = await this.audioStorageManager.searchAudioMessages({
                    talkgroup: parseInt(talkgroup),
                    start_time: parseInt(start_time),
                    limit: 1
                });

                if (!audioMessage.length) {
                    return res.status(404).json({
                        error: 'Audio metadata not found'
                    });
                }

                res.json({
                    metadata: audioMessage[0],
                    _links: {
                        audio_file: `/api/v1/audio/${talkgroup}/${start_time}`,
                        call: `/api/v1/calls/${audioMessage[0].sys_name}/${talkgroup}/${start_time}`
                    }
                });
            } catch (error) {
                console.error('Error fetching audio metadata:', error);
                res.status(500).json({
                    error: 'Error fetching audio metadata',
                    details: error.message
                });
            }
        });

        // Search audio recordings
        app.get('/api/v1/audio/search', async (req, res) => {
            try {
                const query = {
                    talkgroup: req.query.talkgroup ? parseInt(req.query.talkgroup) : undefined,
                    sys_name: req.query.system,
                    unit: req.query.unit ? parseInt(req.query.unit) : undefined,
                    emergency: req.query.emergency === 'true',
                    start_time: req.query.start ? parseInt(req.query.start) : undefined,
                    end_time: req.query.end ? parseInt(req.query.end) : undefined,
                    limit: req.query.limit ? parseInt(req.query.limit) : 50
                };

                const recordings = await this.audioStorageManager.searchAudioMessages(query);

                // Format response with HATEOAS links
                const response = recordings.map(recording => ({
                    metadata: recording,
                    _links: {
                        audio_file: `/api/v1/audio/${recording.talkgroup}/${recording.start_time}`,
                        metadata: `/api/v1/audio/${recording.talkgroup}/${recording.start_time}/metadata`
                    }
                }));

                res.json({
                    results: response,
                    count: response.length,
                    query: query
                });
            } catch (error) {
                console.error('Error searching audio recordings:', error);
                res.status(500).json({
                    error: 'Error searching audio recordings',
                    details: error.message
                });
            }
        });

        // Get audio statistics
        app.get('/api/v1/audio/statistics', async (req, res) => {
            try {
                const timeframe = req.query.timeframe || '24h';
                const endTime = Math.floor(Date.now() / 1000);
                let startTime;

                switch (timeframe) {
                    case '1h':
                        startTime = endTime - 3600;
                        break;
                    case '24h':
                        startTime = endTime - 86400;
                        break;
                    case '7d':
                        startTime = endTime - 604800;
                        break;
                    default:
                        startTime = endTime - 86400;
                }

                const aggregateQuery = [
                    {
                        $match: {
                            start_time: { $gte: startTime, $lte: endTime }
                        }
                    },
                    {
                        $group: {
                            _id: null,
                            total_recordings: { $sum: 1 },
                            total_duration: { $sum: '$call_length' },
                            emergency_count: { 
                                $sum: { $cond: ['$emergency', 1, 0] }
                            },
                            avg_signal: { $avg: '$signal' },
                            avg_noise: { $avg: '$noise' },
                            talkgroups: { $addToSet: '$talkgroup' },
                            sources: { $addToSet: '$src_list.src' }
                        }
                    }
                ];

                const stats = await this.audioStorageManager.AudioMessage.aggregate(aggregateQuery);

                if (!stats.length) {
                    return res.json({
                        timeframe,
                        stats: {
                            total_recordings: 0,
                            total_duration: 0,
                            emergency_count: 0
                        }
                    });
                }

                const flattenedStats = stats[0];
                flattenedStats.unique_talkgroups = flattenedStats.talkgroups.length;
                flattenedStats.unique_sources = new Set(flattenedStats.sources.flat()).size;
                
                delete flattenedStats._id;
                delete flattenedStats.talkgroups;
                delete flattenedStats.sources;

                res.json({
                    timeframe,
                    stats: flattenedStats
                });
            } catch (error) {
                console.error('Error fetching audio statistics:', error);
                res.status(500).json({
                    error: 'Error fetching audio statistics',
                    details: error.message
                });
            }
        });
    }
}

module.exports = AudioIntegrationManager;