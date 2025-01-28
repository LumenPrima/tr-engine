const mongoose = require('mongoose');
const logger = require('../../../utils/logger');

class CollectionManager {
  constructor() {
    this.collectionCache = new Map();
  }

  /**
   * Sanitize collection names
   * @param {string} name - Raw collection name
   * @returns {string} Sanitized name
   */
  sanitizeCollectionName(name) {
    return name.toLowerCase()
      .replace(/[^a-z0-9_]/g, '_')
      .replace(/^[0-9]/, '_$&')
      .substring(0, 64);
  }

  /**
   * Get or create a MongoDB collection
   * @param {string} name - Collection name
   * @returns {Collection} MongoDB collection
   */
  async getCollection(name) {
    if (this.collectionCache.has(name)) {
      return this.collectionCache.get(name);
    }

    const collection = mongoose.connection.db.collection(name);
    this.collectionCache.set(name, collection);
    return collection;
  }

  /**
   * Determine the appropriate collection name for a message
   * @param {string} topic - MQTT topic
   * @param {Object} message - Parsed message
   * @returns {string} Collection name
   */
  resolveCollectionName(topic, message) {
    if (message.type && typeof message.type === 'string') {
      return this.sanitizeCollectionName(message.type);
    }

    const segments = topic.split('/');
    const lastSegment = segments[segments.length - 1];

    if (lastSegment === '#' || lastSegment === '+') {
      return this.sanitizeCollectionName(segments[segments.length - 2]);
    }

    if (lastSegment && !lastSegment.includes('#') && !lastSegment.includes('+')) {
      return this.sanitizeCollectionName(lastSegment);
    }

    logger.warn(`Unable to determine collection name for topic: ${topic}`);
    return 'unclassified';
  }

  /**
   * Store a message in its designated MongoDB collection
   * @param {string} collectionName - Target collection
   * @param {Object} message - Transformed message
   * @param {string} topic - Original MQTT topic
   */
  async storeMessage(collectionName, message, topic) {
    const collection = await this.getCollection(collectionName);
    
    const documentToStore = {
      ...message,
      _mqtt_topic: topic,
      _received_at: new Date()
    };

    await collection.insertOne(documentToStore);
    logger.debug(`Stored message in collection: ${collectionName}`);
  }
}

module.exports = new CollectionManager();