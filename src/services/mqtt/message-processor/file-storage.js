const mongoose = require('mongoose');
const logger = require('../../../utils/logger');

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

      uploadStream.on('finish', (fileId) => {
        logger.debug('Successfully stored file', {
          filename,
          size: fileBuffer.length,
          fileId,
          bucket: bucketName
        });
        resolve({ fileId, filename });
      });

      uploadStream.end(fileBuffer);
    });
  }
}

module.exports = new FileStorage();