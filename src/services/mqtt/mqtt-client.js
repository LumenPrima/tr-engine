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
        await messageProcessor.processMessage(topic, payload);
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
