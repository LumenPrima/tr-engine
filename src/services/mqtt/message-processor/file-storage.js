const mongoose = require('mongoose');
const logger = require('../../../utils/logger');
const TranscriptionService = require('../../transcription/TranscriptionService');

const transcriptionService = new TranscriptionService();

class FileStorage {
  async storeFile(base64Data, message, filename) {
    if (!filename) {
      logger.warn('No filename provided for file storage', { messageType: message?.type });
      return null;
    }

    if (!base64Data) {
      logger.warn(`No file data found for message type: ${message?.type}`, { filename });
      return null;
    }

    logger.debug('storeFile called with:', {
      hasData: !!base64Data,
      dataLength: base64Data?.length,
      messageType: message?.type,
      filename
    });

    const fileBuffer = Buffer.from(base64Data, 'base64');
    const bucketName = `${message?.type || 'unknown'}Files`;
    
    const gridFSBucket = new mongoose.mongo.GridFSBucket(mongoose.connection.db, {
      bucketName
    });

    const essentialMetadata = {
      type: message?.type,
      uploaded_at: new Date(),
      talkgroup: message?.call?.metadata?.talkgroup,
      talkgroup_tag: message?.call?.metadata?.talkgroup_tag,
      start_time: message?.call?.metadata?.start_time,
      stop_time: message?.call?.metadata?.stop_time,
      audio_type: message?.call?.metadata?.audio_type,
      call_length: message?.call?.metadata?.call_length,
      original_topic: message?._mqtt_topic || 'unknown'
    };

    return new Promise((resolve, reject) => {
      const uploadStream = gridFSBucket.openUploadStream(filename, {
        metadata: essentialMetadata
      });

      uploadStream.on('error', (error) => {
        logger.error('Error in GridFS upload stream', {
          filename,
          error: error.message,
          bucketName
        });
        reject(error);
      });

      uploadStream.on('finish', async (fileId) => {
        logger.debug('Successfully stored file', {
          filename,
          size: fileBuffer.length,
          fileId,
          bucket: bucketName
        });

        // Trigger transcription for audio files
        if (bucketName === 'audioFiles' && filename.endsWith('.wav')) {
          try {
            const callId = `${essentialMetadata.talkgroup}-${essentialMetadata.start_time}`;
            
            // Format metadata for transcription
            const audioMessage = {
              filename,
              srcList: message?.call?.metadata?.srcList || [],
              call_length: essentialMetadata.call_length,
              talkgroup: essentialMetadata.talkgroup,
              talkgroup_tag: essentialMetadata.talkgroup_tag,
              sys_name: message?.call?.metadata?.short_name,
              emergency: message?.call?.metadata?.emergency
            };

            // Trigger transcription in background
            transcriptionService.processAudioFile(callId, filename, audioMessage)
              .catch(err => logger.error(`Background transcription failed for ${callId}:`, err));
            
            logger.debug(`Triggered transcription for file: ${filename}`);
          } catch (error) {
            logger.error('Error triggering transcription:', error);
            // Don't reject the promise as file storage was successful
          }
        }

        resolve({ fileId, filename });
      });

      uploadStream.end(fileBuffer);
    });
  }
}

module.exports = new FileStorage();
