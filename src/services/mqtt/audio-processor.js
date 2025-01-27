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

      const startTime = process.hrtime();

      // First, store the metadata
      await this.storeMetadata(topic, message);

      // Then handle the audio data
      await this.processAudioData(message);

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
    
    // Extract and flatten metadata
    const metadata = {
      ...message.call.metadata,
      // Add top-level message fields
      timestamp: message.timestamp,
      instance_id: message.instance_id,
      // Add processing metadata
      _mqtt_topic: topic,
      _received_at: new Date(),
      _audio_processed: false  // Will be updated after audio storage
    };

    // Convert numeric booleans to actual booleans
    metadata.emergency = Boolean(metadata.emergency);
    metadata.encrypted = Boolean(metadata.encrypted);
    metadata.phase2_tdma = Boolean(metadata.phase2_tdma);

    // Calculate total duration from freqList
    metadata.total_duration = metadata.freqList.reduce((sum, freq) => sum + freq.len, 0);

    await audioCollection.insertOne(metadata);
    return metadata;
  }

  /**
   * Process and store the audio data, handling both base64 and binary formats
   * @param {Object} message - Audio message
   * @param {Object} metadata - Previously stored metadata
   */
  async processAudioData(message, metadata) {
    let audioData;
    let audioFormat;

    // Determine the format and extract audio data
    if (message.call.audio_wav_base64) {
      audioData = Buffer.from(message.call.audio_wav_base64, 'base64');
      audioFormat = 'base64';
      this.stats.base64Processed++;
    } else if (Buffer.isBuffer(message.call.audio_data)) {
      audioData = message.call.audio_data;
      audioFormat = 'binary';
      this.stats.binaryProcessed++;
    } else {
      logger.warn('No audio data found in message', {
        filename: metadata.filename
      });
      return;
    }

    // Store audio data in GridFS
    const gridFSBucket = getGridFSBucket('calls');

    // Prepare GridFS metadata
    const gridFSMetadata = {
      talkgroup: metadata.talkgroup,
      talkgroup_tag: metadata.talkgroup_tag,
      start_time: metadata.start_time,
      stop_time: metadata.stop_time,
      call_length: metadata.call_length,
      emergency: Boolean(metadata.emergency),
      encrypted: Boolean(metadata.encrypted),
      freq: metadata.freq,
      instance_id: metadata.instance_id,
      audio_format: audioFormat,
      original_filename: metadata.filename,
      processing_timestamp: new Date()
    };

    // Store the audio file
    await this.storeAudioFile(gridFSBucket, metadata.filename, audioData, gridFSMetadata);

    // Update metadata to indicate audio was processed
    const audioCollection = await this.getCollection('audio');
    await audioCollection.updateOne(
      { filename: metadata.filename },
      { 
        $set: { 
          _audio_processed: true,
          _audio_format: audioFormat,
          _audio_size: audioData.length
        }
      }
    );

    // Update processing stats
    this.stats.totalBytesProcessed += audioData.length;
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