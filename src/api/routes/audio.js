const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const { getGridFSBucket } = require('../../config/mongodb');
const mongoose = require('mongoose');

// GET /call/{call_id}
// Retrieves audio recording for specific call
router.get('/call/:call_id', async (req, res) => {
    try {
        const gridFSBucket = getGridFSBucket();
        const filename = req.params.call_id;
        const format = req.query.format || 'wav'; // Default to WAV if not specified

        // Adjust filename based on format
        const audioFilename = format === 'm4a' ? 
            filename.replace('.wav', '.m4a') : 
            filename;

        // Find the file in GridFS
        const files = await gridFSBucket.find({ filename: audioFilename }).toArray();
        
        if (!files.length) {
            return res.status(404).json({
                status: 'error',
                message: 'Audio file not found'
            });
        }

        const audioFile = files[0];

        // Set appropriate headers
        res.set({
            'Content-Type': format === 'm4a' ? 'audio/mp4' : 'audio/wav',
            'Content-Length': audioFile.length,
            'Content-Disposition': `attachment; filename="${audioFilename}"`,
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
            const downloadStream = gridFSBucket.openDownloadStreamByName(audioFilename, {
                start,
                end: end + 1
            });
            downloadStream.pipe(res);
        } else {
            // Stream the entire file
            const downloadStream = gridFSBucket.openDownloadStreamByName(audioFilename);
            downloadStream.pipe(res);
        }

        logger.debug(`Streaming audio file: ${audioFilename}`);
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
        const gridFSBucket = getGridFSBucket();
        const filename = req.params.call_id;

        // Find both WAV and M4A versions
        const files = await gridFSBucket.find({
            filename: {
                $in: [filename, filename.replace('.wav', '.m4a')]
            }
        }).toArray();

        if (!files.length) {
            return res.status(404).json({
                status: 'error',
                message: 'Audio file not found'
            });
        }

        // Organize metadata by format
        const metadata = {
            call_id: filename.replace('.wav', ''),
            formats: {}
        };

        files.forEach(file => {
            const format = file.filename.endsWith('.m4a') ? 'm4a' : 'wav';
            metadata.formats[format] = {
                filename: file.filename,
                length: file.length,
                upload_date: file.uploadDate,
                md5: file.md5
            };
        });

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            metadata
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
        const gridFSBucket = getGridFSBucket();
        const filename = req.params.call_id;

        // Find both WAV and M4A versions
        const files = await gridFSBucket.find({
            filename: {
                $in: [filename, filename.replace('.wav', '.m4a')]
            }
        }).toArray();

        if (!files.length) {
            return res.status(404).json({
                status: 'error',
                message: 'Audio file not found'
            });
        }

        // Delete each file
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
        const gridFSBucket = getGridFSBucket();
        const options = {
            limit: parseInt(req.query.limit) || 100,
            offset: parseInt(req.query.offset) || 0,
            startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000), // Default to last 24 hours
            endTime: req.query.end ? new Date(req.query.end) : new Date()
        };

        // Build filter based on query parameters
        const filter = {
            'uploadDate': { $gte: options.startTime, $lte: options.endTime }
        };

        if (req.query.unit) {
            filter['metadata.unit'] = parseInt(req.query.unit);
        }
        if (req.query.talkgroup) {
            filter['metadata.talkgroup'] = parseInt(req.query.talkgroup);
        }
        if (req.query.sys_name) {
            filter['metadata.sys_name'] = req.query.sys_name;
        }
        if (req.query.format) {
            filter.filename = new RegExp(`\\.${req.query.format}$`);
        }

        // Get total count for pagination
        const totalCount = await gridFSBucket.find(filter).count();

        // Get paginated audio files
        const files = await gridFSBucket.find(filter)
            .skip(options.offset)
            .limit(options.limit)
            .sort({ uploadDate: -1 })
            .toArray();

        // Transform files to standardized format
        const formattedFiles = files.map(file => ({
            id: file._id.toString(),
            call_id: file.filename.replace(/\.(wav|m4a)$/, ''),
            format: file.filename.split('.').pop(),
            timestamp: file.uploadDate,
            size: file.length,
            metadata: {
                sys_name: file.metadata?.sys_name,
                talkgroup: file.metadata?.talkgroup,
                talkgroup_tag: file.metadata?.talkgroup_tag,
                unit: file.metadata?.unit,
                duration: file.metadata?.duration,
                emergency: file.metadata?.emergency || false
            }
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
