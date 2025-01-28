const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const { getGridFSBucket } = require('../../config/mongodb');
const mongoose = require('mongoose');
const TranscriptionService = require('../../services/transcription/TranscriptionService');
const path = require('path');

const transcriptionService = new TranscriptionService();

// Helper function to find audio metadata and files
const findAudioFiles = async (callIdOrFilename) => {
    const collection = mongoose.connection.db.collection('audio');
    const gridFSBucket = getGridFSBucket('audioFiles');
    
    // Parse talkgroup-starttime pattern
    const parts = callIdOrFilename.split('-');
    if (parts.length !== 2) {
        logger.debug(`Invalid call_id format: ${callIdOrFilename}. Expected talkgroup-starttime`);
        return { metadata: null, files: [] };
    }

    const talkgroup = parseInt(parts[0]);
    const targetTime = parseInt(parts[1]);

    if (isNaN(talkgroup) || isNaN(targetTime)) {
        logger.debug(`Invalid talkgroup or starttime: ${callIdOrFilename}`);
        return { metadata: null, files: [] };
    }

    // Find metadata with closest start_time
    const metadata = await collection.aggregate([
        { 
            $match: { talkgroup: talkgroup }
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

    if (!metadata) {
        logger.debug(`No metadata found for talkgroup ${talkgroup}`);
        return { metadata: null, files: [] };
    }

    // Find audio file
    const files = await gridFSBucket.find({ filename: metadata.filename }).toArray();
    logger.debug(`Found ${files.length} audio files for ${metadata.filename}`);
    return { metadata, files };
};

// Helper function to get preferred format file
const getPreferredFile = (files, requestedFormat) => {
    if (!files.length) return null;

    // If a specific format is requested, try to find it
    if (requestedFormat) {
        const requestedFile = files.find(f => f.filename.endsWith(`.${requestedFormat}`));
        if (requestedFile) return requestedFile;
    }

    // Otherwise prefer m4a over wav
    const m4aFile = files.find(f => f.filename.endsWith('.m4a'));
    return m4aFile || files[0]; // Fallback to first file if no m4a
};

// GET /call/{call_id}
// Retrieves audio recording for specific call
router.get('/call/:call_id', async (req, res) => {
    try {
        const format = req.query.format; // Optional format preference
        const { metadata, files } = await findAudioFiles(req.params.call_id);
        
        if (!metadata || !files.length) {
            return res.status(404).json({
                status: 'error',
                message: 'Audio file not found'
            });
        }

        const audioFile = getPreferredFile(files, format);
        const fileFormat = audioFile.filename.endsWith('.m4a') ? 'm4a' : 'wav';

        // Set appropriate headers
        res.set({
            'Content-Type': fileFormat === 'm4a' ? 'audio/mp4' : 'audio/wav',
            'Content-Length': audioFile.length,
            'Content-Disposition': `attachment; filename="${audioFile.filename}"`,
            'Accept-Ranges': 'bytes'
        });

        // Handle range requests for audio streaming
        const range = req.headers.range;
        if (range) {
            const parts = range.replace(/bytes=/, '').split('-');
            const start = parseInt(parts[0], 10);
            const end = parts[1] ? parseInt(parts[1], 10) : audioFile.length - 1;
            const chunkSize = (end - start) + 1;

            res.status(206);
            res.set({
                'Content-Range': `bytes ${start}-${end}/${audioFile.length}`,
                'Content-Length': chunkSize
            });

            // Create read stream for the range
            const downloadStream = getGridFSBucket('audioFiles').openDownloadStreamByName(audioFile.filename, {
                start,
                end: end + 1
            });
            downloadStream.pipe(res);
        } else {
            // Stream the entire file
            const downloadStream = getGridFSBucket('audioFiles').openDownloadStreamByName(audioFile.filename);
            downloadStream.pipe(res);
            
            // Trigger transcription in the background if not already transcribed
            try {
                const transcriptionCollection = mongoose.connection.db.collection('transcriptions');
                const existingTranscription = await transcriptionCollection.findOne({ call_id: req.params.call_id });
                
                if (!existingTranscription) {
                    logger.debug(`Triggering transcription for call ${req.params.call_id}`);
                    const audioPath = path.join(process.env.AUDIO_STORAGE_PATH || 'audio_files', audioFile.filename);
                    transcriptionService.processAudioFile(req.params.call_id, audioPath, metadata)
                        .catch(err => logger.error(`Background transcription failed for ${req.params.call_id}:`, err));
                }
            } catch (err) {
                logger.error('Error checking/triggering transcription:', err);
            }
        }

        logger.debug(`Streaming audio file: ${audioFile.filename}`);
    } catch (err) {
        logger.error('Error streaming audio file:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to stream audio file'
        });
    }
});

// GET /call/{call_id}/metadata
// Get metadata for an audio file
router.get('/call/:call_id/metadata', async (req, res) => {
    try {
        const { metadata, files } = await findAudioFiles(req.params.call_id);

        if (!metadata || !files.length) {
            return res.status(404).json({
                status: 'error',
                message: 'Audio file not found'
            });
        }

        // Format response
        const response = {
            call_id: req.params.call_id,
            filename: metadata.filename,
            talkgroup: metadata.talkgroup,
            talkgroup_tag: metadata.talkgroup_tag,
            talkgroup_description: metadata.talkgroup_description,
            talkgroup_group: metadata.talkgroup_group,
            talkgroup_group_tag: metadata.talkgroup_group_tag,
            start_time: metadata.start_time,
            stop_time: metadata.stop_time,
            call_length: metadata.call_length,
            emergency: metadata.emergency === 1,
            encrypted: metadata.encrypted === 1,
            freq: metadata.freq,
            audio_type: metadata.audio_type,
            short_name: metadata.short_name,
            srcList: metadata.srcList || [],
            formats: files.map(file => ({
                format: file.filename.endsWith('.m4a') ? 'm4a' : 'wav',
                length: file.length,
                upload_date: file.uploadDate,
                md5: file.md5
            }))
        };

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            metadata: response
        });
    } catch (err) {
        logger.error('Error getting audio metadata:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve audio metadata'
        });
    }
});

// DELETE /call/{call_id}
// Delete an audio file and its metadata
router.delete('/call/:call_id', async (req, res) => {
    try {
        const { metadata, files } = await findAudioFiles(req.params.call_id);

        if (!metadata || !files.length) {
            return res.status(404).json({
                status: 'error',
                message: 'Audio file not found'
            });
        }

        // Delete each file
        const gridFSBucket = getGridFSBucket('audioFiles');
        for (const file of files) {
            await gridFSBucket.delete(file._id);
            logger.debug(`Deleted audio file: ${file.filename}`);
        }

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            message: 'Audio files deleted successfully'
        });
    } catch (err) {
        logger.error('Error deleting audio files:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to delete audio files'
        });
    }
});

// GET /archive
// Search archived audio recordings
router.get('/archive', async (req, res) => {
    try {
        const collection = mongoose.connection.db.collection('audio');
        const options = {
            limit: parseInt(req.query.limit) || 100,
            offset: parseInt(req.query.offset) || 0,
            startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000), // Default to last 24 hours
            endTime: req.query.end ? new Date(req.query.end) : new Date()
        };

        // Build filter based on query parameters
        const filter = {
            start_time: { $gte: Math.floor(options.startTime.getTime() / 1000), $lte: Math.floor(options.endTime.getTime() / 1000) }
        };

        if (req.query.talkgroup) {
            filter.talkgroup = parseInt(req.query.talkgroup);
        }
        if (req.query.sys_name) {
            filter.short_name = req.query.sys_name;
        }
        if (req.query.format) {
            filter.filename = new RegExp(`\\.${req.query.format}$`);
        }
        if (req.query.emergency === 'true') {
            filter.emergency = 1;
        }

        // Get total count for pagination
        const totalCount = await collection.countDocuments(filter);

        // Get paginated audio files
        const files = await collection.find(filter)
            .skip(options.offset)
            .limit(options.limit)
            .sort({ start_time: -1 })
            .toArray();

        // Transform files to standardized format
        const formattedFiles = files.map(file => ({
            id: file._id.toString(),
            filename: file.filename,
            timestamp: new Date(file.start_time * 1000),
            talkgroup: file.talkgroup,
            talkgroup_tag: file.talkgroup_tag,
            talkgroup_description: file.talkgroup_description,
            talkgroup_group: file.talkgroup_group,
            talkgroup_group_tag: file.talkgroup_group_tag,
            start_time: file.start_time,
            stop_time: file.stop_time,
            call_length: file.call_length,
            emergency: file.emergency === 1,
            encrypted: file.encrypted === 1,
            freq: file.freq,
            audio_type: file.audio_type,
            short_name: file.short_name,
            srcList: file.srcList || []
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                pagination: {
                    total: totalCount,
                    limit: options.limit,
                    offset: options.offset,
                    has_more: totalCount > (options.offset + options.limit)
                },
                time_range: {
                    start: options.startTime,
                    end: options.endTime
                },
                count: formattedFiles.length,
                files: formattedFiles
            }
        });
    } catch (err) {
        logger.error('Error searching audio archive:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to search audio archive'
        });
    }
});

module.exports = router;
