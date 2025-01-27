const logger = require('../../utils/logger');
const mongoose = require('mongoose');
const AudioMessageProcessor = require('./audio-processor');

/**
 * MessageProcessor handles the ingestion and storage of all MQTT messages in our system.
 * It determines the appropriate collection for each message, transforms the data structure,
 * and delegates special message types (like audio) to specialized processors.
 */
class MessageProcessor {
  constructor() {
    // Cache collection instances to avoid repeated lookups
    this.collectionCache = new Map();
    
    // Initialize processing statistics
    this.stats = {
      processed: 0,
      errors: 0,
      processingTimes: [],
      messagesByType: new Map()
    };
  }

  /**
   * Process an incoming MQTT message, routing it to the appropriate handler
   * based on message type and content.
   * 
   * @param {string} topic - MQTT topic
   * @param {Buffer} payload - Raw message payload
   * @returns {Promise<void>}
   */
  async processMessage(topic, payload) {
    const startTime = process.hrtime();

    try {
      // Ensure MongoDB is connected
      if (mongoose.connection.readyState !== 1) {
        throw new Error('MongoDB not connected');
      }

      // Parse message
      const message = JSON.parse(payload.toString());
      
      logger.debug(`Processing message from topic: ${topic}, type: ${message.type}`);

      // Handle message based on type
      if (message.type === 'audio') {
        // Delegate audio messages to specialized processor
        await AudioMessageProcessor.processAudioMessage(topic, message);
      } else {
        // Process standard message
        await this.processStandardMessage(topic, message);
      }

      // Update statistics
      this.updateStats(message.type, startTime);

    } catch (err) {
      this.handleError(err, topic);
    }
  }

  /**
   * Process a standard (non-audio) message by determining its collection
   * and storing the transformed data.
   * 
   * @param {string} topic - MQTT topic
   * @param {Object} message - Parsed message
   */
  async processStandardMessage(topic, message) {
    // Get collection name from message type or topic
    const collectionName = this.resolveCollectionName(topic, message);
    
    // Transform message structure
    const transformedMessage = this.transformMessage(message);
    
    // Store in appropriate collection
    await this.storeMessage(collectionName, transformedMessage, topic);
  }

  /**
   * Determine the appropriate collection name for a message. Uses the message's
   * type field if available, otherwise falls back to the MQTT topic structure.
   * 
   * @param {string} topic - MQTT topic
   * @param {Object} message - Parsed message
   * @returns {string} Collection name
   */
  resolveCollectionName(topic, message) {
    // First try: Use message type if available
    if (message.type && typeof message.type === 'string') {
      return this.sanitizeCollectionName(message.type);
    }

    // Second try: Extract from topic
    const segments = topic.split('/');
    const lastSegment = segments[segments.length - 1];

    // Handle MQTT wildcards
    if (lastSegment === '#' || lastSegment === '+') {
      return this.sanitizeCollectionName(segments[segments.length - 2]);
    }

    // Use last segment if valid
    if (lastSegment && !lastSegment.includes('#') && !lastSegment.includes('+')) {
      return this.sanitizeCollectionName(lastSegment);
    }

    // Final fallback
    logger.warn(`Unable to determine collection name for topic: ${topic}`);
    return 'unclassified';
  }

  /**
   * Make collection names safe for MongoDB by removing invalid characters
   * and ensuring proper formatting.
   * 
   * @param {string} name - Raw collection name
   * @returns {string} Sanitized name
   */
  sanitizeCollectionName(name) {
    return name.toLowerCase()
      .replace(/[^a-z0-9_]/g, '_')  // Replace invalid chars with underscore
      .replace(/^[0-9]/, '_$&')     // Prefix with underscore if starts with number
      .substring(0, 64);            // Ensure name isn't too long
  }

  /**
   * Transform a message by unwrapping its content and promoting important fields.
   * Preserves all data while simplifying the structure for storage.
   * 
   * @param {Object} message - Original message
   * @returns {Object} Transformed message
   */
  transformMessage(message) {
    // If no type, return as is
    if (!message.type) return message;

    // Get inner content based on type
    const innerContent = message[message.type.toLowerCase()];
    if (!innerContent) {
      logger.warn(`No inner content found for type: ${message.type}`);
      return message;
    }

    // Create transformed message with metadata
    return {
      ...innerContent,                    // Inner message content
      timestamp: message.timestamp,       // Original timestamp
      instance_id: message.instance_id,   // Instance identifier
      _original_type: message.type,       // Preserve message type
      _processed_at: new Date()           // Add processing timestamp
    };
  }

  /**
   * Store a message in its designated MongoDB collection.
   * 
   * @param {string} collectionName - Target collection
   * @param {Object} message - Transformed message
   * @param {string} topic - Original MQTT topic
   */
  async storeMessage(collectionName, message, topic) {
    const collection = await this.getCollection(collectionName);
    
    const documentToStore = {
      ...message,
      _mqtt_topic: topic,       // Preserve original topic
      _received_at: new Date()  // Add reception timestamp
    };

    await collection.insertOne(documentToStore);
    logger.debug(`Stored message in collection: ${collectionName}`);
  }

  /**
   * Get or create a MongoDB collection, ensuring proper indexes exist.
   * 
   * @param {string} name - Collection name
   * @returns {Collection} MongoDB collection
   */
  async getCollection(name) {
    // Check cache first
    if (this.collectionCache.has(name)) {
      return this.collectionCache.get(name);
    }

    // Get or create collection
    const collection = mongoose.connection.db.collection(name);
    
    // Setup standard indexes
    await collection.createIndex({ timestamp: 1 });
    await collection.createIndex({ instance_id: 1 });
    
    // Cache and return
    this.collectionCache.set(name, collection);
    return collection;
  }

  /**
   * Update processing statistics for monitoring.
   * 
   * @param {string} messageType - Type of processed message
   * @param {[number, number]} startTime - Process start time from hrtime
   */
  updateStats(messageType, startTime) {
    // Calculate processing time
    const diff = process.hrtime(startTime);
    const processingTime = (diff[0] * 1e9 + diff[1]) / 1e6; // Convert to milliseconds

    // Update general stats
    this.stats.processed++;
    this.stats.processingTimes.push(processingTime);
    if (this.stats.processingTimes.length > 100) {
      this.stats.processingTimes.shift();
    }

    // Update type-specific counts
    const typeCount = this.stats.messagesByType.get(messageType) || 0;
    this.stats.messagesByType.set(messageType, typeCount + 1);
  }

  /**
   * Handle and log processing errors with appropriate context.
   * 
   * @param {Error} err - Error object
   * @param {string} topic - MQTT topic where error occurred
   */
  handleError(err, topic) {
    this.stats.errors++;
    this.stats.lastError = {
      time: new Date(),
      message: err.message,
      topic,
      stack: err.stack
    };

    if (err instanceof SyntaxError) {
      logger.error(`Invalid JSON in message on topic ${topic}:`, err.message);
    } else if (err.name === 'MongoServerError') {
      logger.error(`MongoDB error processing message on topic ${topic}:`, err.message);
    } else {
      logger.error(`Error processing message on topic ${topic}:`, err);
    }
  }

  /**
   * Get current processing statistics, including both general and audio-specific stats.
   * 
   * @returns {Object} Combined processing stats
   */
  getStats() {
    const times = this.stats.processingTimes;
    const avgTime = times.length ? 
      times.reduce((a, b) => a + b, 0) / times.length : 0;

    return {
      ...this.stats,
      audio: AudioMessageProcessor.getStats(),
      collections: Array.from(this.collectionCache.keys()),
      averageProcessingTime: avgTime,
      messageTypes: Object.fromEntries(this.stats.messagesByType),
      uptime: process.uptime()
    };
  }
}

// Export singleton instance
module.exports = new MessageProcessor();
