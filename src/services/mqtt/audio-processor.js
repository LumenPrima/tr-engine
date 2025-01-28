const logger = require('../../utils/logger');
const mongoose = require('mongoose');
const { getGridFSBucket } = require('../../config/mongodb');

/**
 * Handles processing and storage of audio messages, supporting both base64 and binary formats.
 * This implementation prioritizes compatibility with existing base64 messages while maintaining
 * flexibility for future binary support.
 */
class AudioMessageProcessor {
  constructor() {
    // Track audio processing statistics
    this.stats = {
      processed: 0,
      errors: 0,
      base64Processed: 0,
      binaryProcessed: 0,
      totalBytesProcessed: 0
    };
  }

  /**
   * Process an audio message and store both metadata and audio content
   * @param {string} topic - MQTT topic
   * @param {Object} message - Audio message
   * @returns {Promise<void>}
   */
  async processAudioMessage(topic, message) {
    try {
      // Validate basic message structure
      if (!message?.call?.metadata) {
        throw new Error('Invalid audio message structure');
      }

      logger.debug('AudioMessageProcessor received message:', {
        message_structure: {
          type: message.type,
          has_call: !!message.call,
          has_metadata: !!message.call?.metadata,
          has_wav: !!message.call?.audio_wav_base64,
          has_m4a: !!message.call?.audio_m4a_base64,
          metadata_fields: message.call?.metadata ? Object.keys(message.call.metadata) : [],
          metadata_sample: message.call?.metadata ? {
            filename: message.call.metadata.filename,
            talkgroup: message.call.metadata.talkgroup,
            start_time: message.call.metadata.start_time
          } : null
        }
      });

      const startTime = process.hrtime();

      // First, store the metadata
      const metadata = await this.processMetadata(topic, message);

      // Then handle the audio data
      await this.processAudioData(message, metadata);

      // Update processing statistics
      this.updateStats(startTime, message);

      logger.debug('Successfully processed audio message', {
        filename: message.call.metadata.filename,
        topic
      });

    } catch (err) {
      this.handleError(err, topic, message);
      throw err; // Re-throw for upstream handling
    }
  }

  /**
   * Store audio metadata in the audio collection
   * @param {string} topic - Original MQTT topic
   * @param {Object} message - Complete message
   */
  async processMetadata(topic, message) {
    const audioCollection = await this.getCollection('audio');
    
    if (!message?.call?.metadata) {
      throw new Error('Invalid audio message structure - missing metadata');
    }

    // Extract metadata fields from nested structure
    const metadata = {
      ...message.call.metadata,  // Base metadata fields from nested structure
      timestamp: message.timestamp,
      instance_id: message.instance_id,
      _type: 'audio',
      _mqtt_topic: topic,
      _received_at: new Date(),
      _audio_processed: false,
      _audio_formats: [] // Will be updated with formats stored in GridFS
    };

    // Map fields according to field-mappings.md
    metadata.talkgroup_alpha_tag = metadata.talkgroup_tag;
    metadata.talkgroup_tag = metadata.talkgroup_group_tag;
    metadata.sys_name = metadata.short_name;

    // Remove old field names
    delete metadata.talkgroup_group_tag;
    delete metadata.short_name;

    // Convert numeric booleans to actual booleans
    metadata.emergency = Boolean(metadata.emergency);
    metadata.encrypted = Boolean(metadata.encrypted);
    metadata.phase2_tdma = Boolean(metadata.phase2_tdma);

    // Calculate total duration from freqList
    metadata.total_duration = metadata.freqList?.reduce((sum, freq) => sum + freq.len, 0) || 0;

    logger.debug('Storing audio metadata:', {
      filename: metadata.filename,
      fields: Object.keys(metadata)
    });

    await audioCollection.insertOne(metadata);
    return metadata;
  }

  /**
   * Process and store the audio data, handling both base64 and binary formats
   * @param {Object} message - Audio message
   * @param {Object} metadata - Previously stored metadata
   */
  async processAudioData(message, metadata) {
    if (!message?.call) {
      throw new Error('Invalid audio message structure - missing call data');
    }

    const gridFSBucket = getGridFSBucket();
    const audioFormats = [];
    let totalBytes = 0;

    // Common metadata for GridFS
    const baseGridFSMetadata = {
      talkgroup: metadata.talkgroup,
      talkgroup_tag: metadata.talkgroup_tag,
      start_time: metadata.start_time,
      stop_time: metadata.stop_time,
      call_length: metadata.call_length,
      emergency: Boolean(metadata.emergency),
      encrypted: Boolean(metadata.encrypted),
      freq: metadata.freq,
      instance_id: metadata.instance_id,
      processing_timestamp: new Date()
    };

    // Process WAV audio if present in nested structure
    if (message.call.audio_wav_base64) {
      const wavData = Buffer.from(message.call.audio_wav_base64, 'base64');
      const wavFilename = metadata.filename;
      logger.debug('Processing WAV audio:', {
        filename: wavFilename,
        size: wavData.length
      });
      await this.storeAudioFile(gridFSBucket, wavFilename, wavData, {
        ...baseGridFSMetadata,
        audio_format: 'wav'
      });
      audioFormats.push('wav');
      totalBytes += wavData.length;
      this.stats.base64Processed++;
    }

    // Process M4A audio if present in nested structure
    if (message.call.audio_m4a_base64) {
      const m4aData = Buffer.from(message.call.audio_m4a_base64, 'base64');
      const m4aFilename = metadata.filename.replace('.wav', '.m4a');
      logger.debug('Processing M4A audio:', {
        filename: m4aFilename,
        size: m4aData.length
      });
      await this.storeAudioFile(gridFSBucket, m4aFilename, m4aData, {
        ...baseGridFSMetadata,
        audio_format: 'm4a'
      });
      audioFormats.push('m4a');
      totalBytes += m4aData.length;
      this.stats.base64Processed++;
    }

    // Update metadata with audio processing results
    const audioCollection = await this.getCollection('audio');
    await audioCollection.updateOne(
      { filename: metadata.filename },
      { 
        $set: { 
          _audio_processed: true,
          _audio_formats: audioFormats,
          _audio_size: totalBytes
        }
      }
    );

    // Update processing stats
    this.stats.totalBytesProcessed += totalBytes;

    // Log if no audio data found
    if (audioFormats.length === 0) {
      logger.debug('No audio data found in message', {
        filename: metadata.filename,
        available_fields: Object.keys(message.call || {})
      });
    }
  }

  /**
   * Store audio file in GridFS with robust error handling
   * @param {GridFSBucket} bucket - GridFS bucket
   * @param {string} filename - Target filename
   * @param {Buffer} buffer - Audio data
   * @param {Object} metadata - File metadata
   */
  async storeAudioFile(bucket, filename, buffer, metadata) {
    return new Promise((resolve, reject) => {
      // Create upload stream with retry capability
      const upload = () => {
        const uploadStream = bucket.openUploadStream(filename, { metadata });

        uploadStream.on('error', (err) => {
          logger.error(`Error uploading file ${filename}:`, err);
          reject(err);
        });

        uploadStream.on('finish', () => {
          logger.debug(`Successfully uploaded file ${filename}`, {
            size: buffer.length,
            format: metadata.audio_format
          });
          resolve();
        });

        // Write the buffer to GridFS
        uploadStream.end(buffer);
      };

      // Initial upload attempt
      upload();
    });
  }

  /**
   * Get MongoDB collection with automatic index creation
   * @param {string} name - Collection name
   */
  async getCollection(name) {
    const collection = mongoose.connection.db.collection(name);
    
    // Ensure indexes exist
    await collection.createIndex({ timestamp: 1 });
    await collection.createIndex({ instance_id: 1 });
    await collection.createIndex({ filename: 1 }, { unique: true });
    await collection.createIndex({ 
      talkgroup: 1, 
      start_time: 1 
    });

    return collection;
  }

  /**
   * Update processing statistics
   * @param {[number, number]} startTime - Process start time from hrtime
   * @param {Object} message - Processed message
   */
  updateStats(startTime, message) {
    const diff = process.hrtime(startTime);
    const processingTime = (diff[0] * 1e9 + diff[1]) / 1e6; // Convert to milliseconds

    this.stats.processed++;
    this.stats.lastProcessingTime = processingTime;

    // Keep a moving average of processing times
    if (!this.stats.averageProcessingTime) {
      this.stats.averageProcessingTime = processingTime;
    } else {
      this.stats.averageProcessingTime = 
        (this.stats.averageProcessingTime * 0.9) + (processingTime * 0.1);
    }
  }

  /**
   * Handle and log processing errors
   * @param {Error} err - Error object
   * @param {string} topic - MQTT topic
   * @param {Object} message - Original message
   */
  handleError(err, topic, message) {
    this.stats.errors++;
    this.stats.lastError = {
      time: new Date(),
      message: err.message,
      topic,
      filename: message?.call?.metadata?.filename
    };

    logger.error('Error processing audio message:', {
      error: err.message,
      topic,
      filename: message?.call?.metadata?.filename,
      stack: err.stack
    });
  }

  /**
   * Get current processing statistics
   * @returns {Object} Processing stats
   */
  getStats() {
    return {
      ...this.stats,
      uptime: process.uptime(),
      averageProcessingTime: this.stats.averageProcessingTime,
      processingRatio: {
        base64: this.stats.base64Processed / this.stats.processed,
        binary: this.stats.binaryProcessed / this.stats.processed
      }
    };
  }
}

// Export singleton instance
module.exports = new AudioMessageProcessor();
