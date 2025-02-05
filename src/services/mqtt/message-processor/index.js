const mongoose = require('mongoose');
const logger = require('../../../utils/logger');
const config = require('../../../config');
const ActiveCallManager = require('../../state/ActiveCallManager');
const SystemManager = require('../../state/SystemManager');
const UnitManager = require('../../state/UnitManager');
const TalkgroupManager = require('../../state/TalkgroupManager');
const RecorderManager = require('../../state/RecorderManager');

const messageTransformer = require('./message-transformer');
const collectionManager = require('./collection-manager');
const statsManager = require('./stats-manager');
const audioHandler = require('../handlers/audio-handler');

class MessageProcessor {
  /**
   * Process an incoming message
   * @param {string} topic - MQTT topic
   * @param {Buffer} payload - Message payload
   */
  async processMessage(topic, payload) {
    const startTime = process.hrtime();

    try {
      // Ensure MongoDB is connected
      if (mongoose.connection.readyState !== 1) {
        throw new Error('MongoDB not connected');
      }

      // Parse message
      const originalMessage = JSON.parse(payload.toString());
      const messageId = new mongoose.Types.ObjectId();
      
      // Transform the message
      const transformedMessage = messageTransformer.transformMessage(originalMessage);

      if (originalMessage.type === 'audio') {
        try {
          // Handle the file storage separately
          if (originalMessage.call?.audio_wav_base64 || originalMessage.call?.audio_m4a_base64) {
            await audioHandler.processAudioMessage(topic, originalMessage, transformedMessage);
          }

          // Remove any base64 fields from the transformed message
          Object.keys(transformedMessage).forEach(key => {
            if (key.endsWith('base64')) {
                delete transformedMessage[key];
            }
         });

          // Store the complete message in the audio collection
          const audioCollection = await collectionManager.getCollection('audio');
          await audioCollection.insertOne({
            ...transformedMessage,
            _id: messageId,
            _mqtt_topic: topic,
            _processed_at: new Date()
          });
          
          // Process with state managers
          await ActiveCallManager.processMessage(topic, transformedMessage, messageId);
          
          if (originalMessage.call?.metadata?.talkgroup) {
            await TalkgroupManager.processMessage(topic, transformedMessage, messageId);
          }
        } catch (err) {
          logger.error('Audio processing failed:', {
            error: err.message,
            filename: originalMessage.call?.metadata?.filename,
            stage: err.stage || 'unknown'
          });
          throw err;
        }
      } else {
        // Handle non-audio messages
        const collectionName = collectionManager.resolveCollectionName(topic, originalMessage);
        await collectionManager.storeMessage(collectionName, transformedMessage, topic);

        // Route to appropriate state managers based on topic
        const topicParts = topic.split('/');
        
        if (topicParts[2] === 'systems' || topicParts[2] === 'rates' || topicParts[2] === 'config') {
          await SystemManager.processMessage(topic, transformedMessage, messageId);
        } else if (topic.startsWith(`${config.mqtt.topicPrefix}/units/`)) {
          logger.debug('Routing unit message:', {
            topic,
            message: JSON.stringify(transformedMessage)
          });
          await UnitManager.processMessage(topic, transformedMessage, messageId);
        } else if (['call_start', 'call_end', 'calls_active'].includes(topicParts[2])) {
          await ActiveCallManager.processMessage(topic, transformedMessage, messageId);
          
          if (transformedMessage.talkgroup) {
            await TalkgroupManager.processMessage(topic, transformedMessage, messageId);
          }

          // Route call messages to RecorderManager for talkgroup tracking
          if (transformedMessage.type === 'call_start' || transformedMessage.type === 'call_end') {
            logger.debug('Routing call message to RecorderManager:', {
              topic,
              type: transformedMessage.type,
              rec_num: transformedMessage.rec_num,
              talkgroup: transformedMessage.talkgroup
            });
            await RecorderManager.processMessage(topic, transformedMessage, messageId);
          }
        } else if (topicParts[2] === 'recorder' || topicParts[2] === 'recorders') {
          logger.debug('Processing recorder message:', {
            topic,
            type: transformedMessage.type,
            transformed: JSON.stringify(transformedMessage)
          });
          await RecorderManager.processMessage(topic, transformedMessage, messageId);
        }
      }

      // Update processing statistics
      statsManager.updateStats(originalMessage.type, startTime);

    } catch (err) {
      statsManager.handleError(err, topic);
      throw err;
    }
  }

  /**
   * Get current processing statistics
   * @returns {Object} Processing stats
   */
  getStats() {
    return statsManager.getStats();
  }
}

// Export singleton instance
module.exports = new MessageProcessor();
