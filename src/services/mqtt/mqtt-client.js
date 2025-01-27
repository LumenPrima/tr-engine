const mqtt = require('mqtt');
const config = require('../../config');
const logger = require('../../utils/logger');
const messageProcessor = require('./message-processor');
const audioProcessor = require('./audio-processor');
const mongoose = require('mongoose');

class MQTTClient {
  constructor() {
    this.client = null;
    this.connected = false;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 10;
    this.collectionCache = new Map(); // Cache for collection instances
  }

  connect() {
    const options = {
      clientId: `${config.mqtt.clientId}-${Date.now()}`, // Ensure unique client ID
      clean: true,
      reconnectPeriod: 5000,
      connectTimeout: 30000,
      username: config.mqtt.username,
      password: config.mqtt.password,
      rejectUnauthorized: false // Only if using self-signed certs
    };

    logger.info(`Connecting to MQTT broker at ${config.mqtt.brokerUrl}`);
    
    this.client = mqtt.connect(config.mqtt.brokerUrl, options);

    this.client.on('connect', () => {
      this.connected = true;
      this.reconnectAttempts = 0;
      logger.info('Connected to MQTT broker');
      this.subscribeToTopics();
    });

    this.client.on('reconnect', () => {
      this.reconnectAttempts++;
      logger.warn(`Attempting to reconnect (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
      
      if (this.reconnectAttempts > this.maxReconnectAttempts) {
        logger.error('Max reconnection attempts reached, stopping client');
        this.client.end();
      }
    });

    this.client.on('error', (err) => {
      logger.error('MQTT client error:', err);
    });

    this.client.on('close', () => {
      this.connected = false;
      logger.warn('MQTT client disconnected');
    });

    this.client.on('message', async (topic, payload) => {
      try {
        await this.handleMessage(topic, payload);
      } catch (err) {
        logger.error('Error processing message:', err);
      }
    });

    return this.client;
  }

  subscribeToTopics() {
    // Subscribe to all tr-mqtt topics
    this.client.subscribe('tr-mqtt/#', { qos: 1 }, (err) => {
      if (err) {
        logger.error('Error subscribing to topics:', err);
      } else {
        logger.info('Subscribed to tr-mqtt/#');
      }
    });
  }

  async handleMessage(topic, payload) {
    try {
      // Only process if MongoDB is connected
      if (mongoose.connection.readyState !== 1) {
        logger.warn('Message received but MongoDB not connected');
        return;
      }

      const message = JSON.parse(payload.toString());
      
      // Handle audio messages specially
      if (message.type === 'audio' && message.call?.audio_wav_base64) {
        await this.handleAudioMessage(topic, message);
        return;
      }

      // Determine collection name from type or topic
      const collectionName = this.resolveCollectionName(topic, message);
      
      // Get or create collection
      const collection = await this.getCollection(collectionName);

      // Transform message for storage
      const transformedMessage = this.transformMessage(message);

      // Store the message
      await collection.insertOne({
        ...transformedMessage,
        _mqtt_topic: topic,       // Store original topic
        _received_at: new Date()  // Add reception timestamp
      });

      logger.debug(`Stored message in collection: ${collectionName}`);

    } catch (err) {
      if (err instanceof SyntaxError) {
        logger.error('Invalid JSON in message:', err);
      } else {
        logger.error('Error processing message:', err);
      }
    }
  }

  resolveCollectionName(topic, message) {
    // Try to use message type first
    if (message.type && typeof message.type === 'string') {
      return message.type.toLowerCase();
    }

    // Fall back to topic parsing
    const segments = topic.split('/');
    const lastSegment = segments[segments.length - 1];

    // Handle wildcards
    if (lastSegment === '#' || lastSegment === '+') {
      return segments[segments.length - 2].toLowerCase();
    }

    // Use last segment if it looks valid
    if (lastSegment && !lastSegment.includes('#') && !lastSegment.includes('+')) {
      return lastSegment.toLowerCase();
    }

    // Ultimate fallback
    return 'unclassified';
  }

  transformMessage(message) {
    // Get the inner payload based on type
    const type = message.type;
    if (!type) return message; // No transformation needed

    // Extract the wrapped content
    const innerContent = message[type.toLowerCase()];
    if (!innerContent) return message; // No wrapped content found

    // Merge top-level fields with inner content
    return {
      ...innerContent,
      timestamp: message.timestamp,
      instance_id: message.instance_id
    };
  }

  async getCollection(name) {
    // Check cache first
    if (this.collectionCache.has(name)) {
      return this.collectionCache.get(name);
    }

    // Get or create collection
    const collection = mongoose.connection.db.collection(name);
    
    // Ensure indexes
    await collection.createIndex({ timestamp: 1 });
    await collection.createIndex({ instance_id: 1 });
    
    // Add to cache
    this.collectionCache.set(name, collection);
    
    return collection;
  }

  async handleAudioMessage(topic, message) {
    try {
      // Store metadata in audio collection
      const audioCollection = await this.getCollection('audio');
      
      // Transform metadata
      const metadata = {
        ...message.call.metadata,
        timestamp: message.timestamp,
        instance_id: message.instance_id,
        _mqtt_topic: topic,
        _received_at: new Date()
      };

      await audioCollection.insertOne(metadata);

      // Store audio in GridFS
      const { getGridFSBucket } = require('../../config/mongodb');
      const gridFSBucket = getGridFSBucket('calls'); // Use 'calls' bucket

      if (message.call.audio_wav_base64) {
        const buffer = Buffer.from(message.call.audio_wav_base64, 'base64');
        const filename = metadata.filename;

        // Convert numeric boolean fields to actual booleans
        const gridFSMetadata = {
          talkgroup: metadata.talkgroup,
          talkgroup_tag: metadata.talkgroup_tag,
          start_time: metadata.start_time,
          stop_time: metadata.stop_time,
          call_length: metadata.call_length,
          emergency: Boolean(metadata.emergency),
          encrypted: Boolean(metadata.encrypted),
          freq: metadata.freq,
          instance_id: metadata.instance_id
        };

        const uploadStream = gridFSBucket.openUploadStream(filename, {
          metadata: gridFSMetadata
        });

        await new Promise((resolve, reject) => {
          uploadStream.on('error', reject);
          uploadStream.on('finish', resolve);
          uploadStream.end(buffer);
        });

        logger.debug(`Stored audio file: ${filename}`);
      }
    } catch (err) {
      logger.error('Error processing audio message:', err);
      throw err;
    }
  }

  async disconnect() {
    if (!this.client) return;

    this.client.options.reconnectPeriod = 0;
    
    return new Promise((resolve) => {
      const timeout = setTimeout(() => {
        logger.warn('MQTT disconnect timed out, forcing closure');
        this.cleanup();
        resolve();
      }, 5000);

      try {
        this.client.removeAllListeners();
        this.client.once('close', () => {
          clearTimeout(timeout);
          this.cleanup();
          resolve();
        });

        this.client.unsubscribe('#', () => {
          this.client.end(true);
        });
      } catch (err) {
        logger.error('Error during MQTT disconnect:', err);
        clearTimeout(timeout);
        this.cleanup();
        resolve();
      }
    });
  }

  cleanup() {
    this.connected = false;
    this.client = null;
    this.collectionCache.clear();
  }

  isConnected() {
    return this.connected;
  }
}

// Export singleton instance
const mqttClient = new MQTTClient();
module.exports = mqttClient;
