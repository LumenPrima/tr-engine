const logger = require('../../utils/logger');
const mongoose = require('mongoose');
const activeCallManager = require('../state/ActiveCallManager');
const systemManager = require('../state/SystemManager');
const unitManager = require('../state/UnitManager');
const {
  SystemMessage,
  RatesMessage,
  RecorderMessage,
  CallStartMessage,
  CallEndMessage,
  CallsActiveMessage,
  AudioMessage,
  UnitCallMessage,
  UnitLocationMessage,
  UnitDataMessage,
  UnitJoinMessage,
  UnitEndMessage,
  UnitOnMessage,
  UnitOffMessage,
  UnitAckRespMessage
} = require('../../models/raw/MessageCollections');

class MessageProcessor {
  constructor() {
    // Map message types to their corresponding MongoDB collections
    this.messageCollections = new Map([
      ['systems', SystemMessage],
      ['rates', RatesMessage],
      ['recorder', RecorderMessage],
      ['call_start', CallStartMessage],
      ['call_end', CallEndMessage],
      ['calls_active', CallsActiveMessage],
      ['audio', AudioMessage],
      ['call', UnitCallMessage],
      ['location', UnitLocationMessage],
      ['data', UnitDataMessage],
      ['join', UnitJoinMessage],
      ['end', UnitEndMessage],
      ['on', UnitOnMessage],
      ['off', UnitOffMessage],
      ['ackresp', UnitAckRespMessage]
    ]);
  }

  async handleMessage(topic, payload) {
    try {
      // Only attempt database operations if MongoDB is connected
      if (mongoose.connection.readyState !== 1) {
        logger.debug(`Received message on ${topic} (not saved, MongoDB not connected)`);
        return;
      }

      const message = JSON.parse(payload.toString());
      const topicParts = topic.split('/');
      
      if (topicParts.length < 3) {
        logger.warn(`Invalid topic format: ${topic}`);
        return;
      }

      // Get message type from topic structure
      const messageType = topicParts[2];
      const Collection = this.messageCollections.get(messageType);

      if (!Collection) {
        logger.warn(`No collection mapped for type: ${messageType}`);
        return;
      }

      // Handle the message based on its type
      if (topic === 'tr-mqtt/main/audio') {
        // Extract the base64 audio data and metadata
        const wavData = message.call.audio_wav_base64;
        const m4aData = message.call.audio_m4a_base64;
        const metadata = message.call.metadata;

        // Remove the base64 data from the message to avoid storing it twice
        delete message.call.audio_wav_base64;
        delete message.call.audio_m4a_base64;

        // Calculate total duration from freqList
        const totalDuration = metadata.freqList.reduce((sum, freq) => sum + freq.len, 0);
        metadata.call_length = totalDuration;

        // Create and save the message document
        const doc = new Collection({
          message_type: messageType,
          timestamp: message.timestamp,
          instance_id: message.instance_id,
          topic,
          payload: JSON.stringify({
            ...message,
            topic // Include topic in payload for consistency
          })
        });

        await doc.save();

        // Store the audio data in GridFS if available
        if (mongoose.connection.readyState === 1) {
          const { getGridFSBucket } = require('../../config/mongodb');
          const gridFSBucket = getGridFSBucket();

          // Store WAV if available
          if (wavData) {
            try {
              // Convert base64 to raw binary buffer
              const wavBuffer = Buffer.from(wavData, 'base64');

              // Debug: Save to disk for verification
              const fs = require('fs');
              const debugPath = `/tmp/debug_${metadata.filename}`;
              fs.writeFileSync(debugPath, wavBuffer);
              logger.debug(`Saved WAV file to disk for debug: ${debugPath}`);

              // Store the WAV file in GridFS
              await new Promise((resolve, reject) => {
                const uploadStream = gridFSBucket.openUploadStream(metadata.filename);
                uploadStream.end(wavBuffer);
                uploadStream.once('finish', resolve);
                uploadStream.once('error', reject);
              });

              logger.debug(`Stored WAV file ${metadata.filename} in GridFS`);
            } catch (err) {
              logger.error('Error storing WAV file:', err);
              throw err;
            }
          }

          // Store M4A if available
          if (m4aData) {
            try {
              const m4aBuffer = Buffer.from(m4aData, 'base64');
              const m4aFilename = metadata.filename.replace('.wav', '.m4a');

              // Store the M4A file in GridFS
              await new Promise((resolve, reject) => {
                const uploadStream = gridFSBucket.openUploadStream(m4aFilename);
                uploadStream.end(m4aBuffer);
                uploadStream.once('finish', resolve);
                uploadStream.once('error', reject);
              });

              logger.debug(`Stored M4A file ${m4aFilename} in GridFS`);
            } catch (err) {
              logger.error('Error storing M4A file:', err);
              throw err;
            }
          }
        }

        logger.info(`Stored audio message and files for ${metadata.filename} in ${Collection.collection.name}`);
      } else {
        // For all other messages, store them with stringified payload
        const doc = new Collection({
          message_type: messageType,
          timestamp: message.timestamp,
          instance_id: message.instance_id,
          topic,
          payload: JSON.stringify({
            ...message,
            topic // Include topic in payload for consistency
          })
        });

        await doc.save();
        logger.debug(`Stored message from topic ${topic} in ${Collection.collection.name}`);

        // Update state managers with the processed message
        try {
          const messageId = doc._id;

          // Route message to appropriate state manager(s)
          if (topicParts[1] === 'main') {
            if (['call_start', 'call_end', 'calls_active', 'audio'].includes(topicParts[2])) {
              await activeCallManager.processMessage(topic, message, messageId);
            }
            if (['systems', 'rates', 'config'].includes(topicParts[2])) {
              await systemManager.processMessage(topic, message, messageId);
            }
          } else if (topicParts[1] === 'units') {
            await unitManager.processMessage(topic, message, messageId);
          }
        } catch (stateErr) {
          logger.error('Error updating state managers:', stateErr);
          // Don't throw here - we still successfully stored the raw message
        }
      }
    } catch (err) {
      if (err.name === 'MongoServerError') {
        logger.error('MongoDB error processing message:', err.message);
      } else if (err instanceof SyntaxError) {
        logger.error('Invalid JSON in message:', err.message);
      } else {
        logger.error('Error processing message:', err);
      }
    }
  }

  // Helper method to get messages from any collection
  async getMessages(messageType, query = {}, options = {}) {
    const Collection = this.messageCollections.get(messageType);
    
    if (!Collection) {
      throw new Error(`Unknown collection: ${messageType}`);
    }

    return Collection.find(query, null, options).sort({ timestamp: -1 });
  }

  // Helper method to get the latest message from a collection
  async getLatestMessage(messageType, query = {}) {
    const Collection = this.messageCollections.get(messageType);
    
    if (!Collection) {
      throw new Error(`Unknown collection: ${messageType}`);
    }

    return Collection.findOne(query).sort({ timestamp: -1 });
  }
}

// Export singleton instance
module.exports = new MessageProcessor();
