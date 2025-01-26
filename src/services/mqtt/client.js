const mqtt = require('mqtt');
const config = require('../../config');
const logger = require('../../utils/logger');
const messageProcessor = require('./processor');

class MQTTClient {
  constructor() {
    this.client = null;
    this.connected = false;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 10;
  }

  connect() {
    const options = {
      clientId: config.mqtt.clientId,
      clean: true,
      reconnectPeriod: 5000,
      connectTimeout: 30000,
      username: config.mqtt.username,
      password: config.mqtt.password
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
      logger.warn(`Attempting to reconnect to MQTT broker (attempt ${this.reconnectAttempts})`);
      
      if (this.reconnectAttempts > this.maxReconnectAttempts) {
        logger.error('Max reconnection attempts reached. Stopping MQTT client.');
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
        logger.debug(`Received message on topic: ${topic}`);
        await messageProcessor.handleMessage(topic, payload);
      } catch (err) {
        logger.error('Error processing MQTT message:', err);
      }
    });

    return this.client;
  }

  subscribeToTopics() {
    const topics = [
      'tr-mqtt/main/call_start',
      'tr-mqtt/main/call_end',
      'tr-mqtt/main/calls_active',
      'tr-mqtt/main/audio',
      'tr-mqtt/main/systems',
      'tr-mqtt/main/rates',
      'tr-mqtt/units/#'
    ];

    topics.forEach(topic => {
      this.client.subscribe(topic, (err) => {
        if (err) {
          logger.error(`Error subscribing to ${topic}:`, err);
        } else {
          logger.info(`Subscribed to ${topic}`);
        }
      });
    });
  }

  publish(topic, message) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('MQTT client not connected'));
        return;
      }

      this.client.publish(topic, JSON.stringify(message), (err) => {
        if (err) {
          logger.error(`Error publishing to ${topic}:`, err);
          reject(err);
        } else {
          logger.debug(`Published message to ${topic}`);
          resolve();
        }
      });
    });
  }

  async disconnect() {
    if (!this.client) {
      return;
    }

    // Prevent reconnection attempts during shutdown
    this.client.options.reconnectPeriod = 0;
    
    return new Promise((resolve, reject) => {
      // Set a timeout to force resolution after 5 seconds
      const timeout = setTimeout(() => {
        logger.warn('MQTT disconnect timed out after 5 seconds, forcing closure');
        this.connected = false;
        this.client = null;
        resolve();
      }, 5000);

      try {
        // Remove all listeners to prevent any lingering callbacks
        this.client.removeAllListeners();

        // Add one-time close handler
        this.client.once('close', () => {
          clearTimeout(timeout);
          this.connected = false;
          this.client = null;
          resolve();
        });

        // Unsubscribe from all topics first
        this.client.unsubscribe('#', (err) => {
          if (err) {
            logger.warn('Error unsubscribing from topics:', err);
          }
          // Force close the client connection with clean=true to prevent reconnect
          this.client.end(true, {}, () => {
            // Additional cleanup after end
            this.connected = false;
            this.client = null;
          });
        });
      } catch (err) {
        clearTimeout(timeout);
        logger.error('Error during MQTT disconnect:', err);
        this.connected = false;
        this.client = null;
        resolve(); // Resolve anyway to allow shutdown to continue
      }
    });
  }

  isConnected() {
    return this.connected;
  }
}

// Export singleton instance
const mqttClient = new MQTTClient();
module.exports = mqttClient;
