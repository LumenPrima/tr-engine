const mqtt = require('mqtt');
const config = require('../../config');
const logger = require('../../utils/logger');
const messageProcessor = require('./message-processor');

const CONSTANTS = {
  MAX_QUEUE_SIZE: 1000,
  DISCONNECT_TIMEOUT: 5000,
  MAX_RECONNECT_DELAY: 300000 // 5 minutes
};

class QueuedMessage {
  constructor(topic, payload, options) {
    this.topic = topic;
    this.payload = payload;
    this.options = options || {};
    this.timestamp = Date.now();
  }
}

class MetricsCollector {
  constructor() {
    this.reset();
  }

  reset() {
    this.metrics = {
      messagesProcessed: 0,
      messagesSent: 0,
      connectionAttempts: 0,
      errors: 0,
      lastConnectionTime: null,
      averageProcessingTime: 0,
      queueSize: 0
    };
  }

  recordProcessedMessage(processingTime) {
    this.metrics.messagesProcessed++;
    this.metrics.averageProcessingTime = 
      (this.metrics.averageProcessingTime * (this.metrics.messagesProcessed - 1) + processingTime) 
      / this.metrics.messagesProcessed;
  }

  getMetrics() {
    return { ...this.metrics };
  }
}

class MQTTClient {
  constructor() {
    this.client = null;
    this.connected = false;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = config.mqtt.maxReconnectAttempts || 10;
    this.messageQueue = [];
    this.metrics = new MetricsCollector();
  }

  async connect() {
    try {
      const options = {
        clientId: `${config.mqtt.clientId}-${Date.now()}`,
        clean: true,
        reconnectPeriod: config.mqtt.reconnectPeriod || 5000,
        connectTimeout: config.mqtt.connectTimeout || 30000,
        username: config.mqtt.username,
        password: config.mqtt.password,
        rejectUnauthorized: true
      };

      logger.info(`Connecting to MQTT broker at ${config.mqtt.brokerUrl}`);
      
      this.client = mqtt.connect(config.mqtt.brokerUrl, options);

      // Set up event handlers
      this.client.on('connect', this.handleConnect.bind(this));
      this.client.on('reconnect', this.handleReconnect.bind(this));
      this.client.on('close', this.handleClose.bind(this));
      this.client.on('message', this.handleMessage.bind(this));
      this.client.on('error', this.handleError.bind(this));

      return new Promise((resolve, reject) => {
        this.client.once('connect', () => resolve(this.client));
        this.client.once('error', reject);
      });

    } catch (err) {
      logger.error('Error during MQTT connection setup:', err);
      throw err;
    }
  }

  handleConnect() {
    this.connected = true;
    this.reconnectAttempts = 0;
    this.metrics.metrics.lastConnectionTime = Date.now();
    logger.info('Connected to MQTT broker');
    this.subscribeToTopics();
    this.processQueuedMessages();
  }

  handleReconnect() {
    this.reconnectAttempts++;
    this.metrics.metrics.connectionAttempts++;
    logger.warn(`Attempting to reconnect (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
    
    if (this.reconnectAttempts > this.maxReconnectAttempts) {
      logger.error('Max reconnection attempts reached, stopping client');
      this.disconnect();
    }
  }

  handleError(err) {
    this.metrics.metrics.errors++;
    logger.error('MQTT client error:', err);
  }

  handleClose() {
    this.connected = false;
    logger.warn('MQTT client disconnected');
  }

  async handleMessage(topic, payload, packet) {
    const startTime = Date.now();
    
    try {
      await messageProcessor.processMessage(topic, payload);
      const processingTime = Date.now() - startTime;
      this.metrics.recordProcessedMessage(processingTime);
    } catch (err) {
      this.metrics.metrics.errors++;
      logger.error('Error processing message:', err);
    }
  }

  subscribeToTopics() {
    const topic = 'tr-mqtt/#';
    
    this.client.subscribe(topic, { qos: config.mqtt.qos || 1 }, (err) => {
      if (err) {
        logger.error(`Error subscribing to topics: ${topic}`, err);
      } else {
        logger.info(`Subscribed to ${topic}`);
      }
    });
  }

  publish(topic, message, options) {
    return new Promise((resolve, reject) => {
      if (!this.isConnected()) {
        this.queueMessage(topic, message, options);
        resolve(false);
        return;
      }
      
      this.client.publish(topic, message, options || {}, (err) => {
        if (err) {
          this.metrics.metrics.errors++;
          reject(err);
        } else {
          this.metrics.metrics.messagesSent++;
          resolve(true);
        }
      });
    });
  }

  queueMessage(topic, message, options) {
    if (this.messageQueue.length >= CONSTANTS.MAX_QUEUE_SIZE) {
      this.messageQueue.shift();
    }
    this.messageQueue.push(new QueuedMessage(topic, message, options));
    this.metrics.metrics.queueSize = this.messageQueue.length;
  }

  processQueuedMessages() {
    while (this.messageQueue.length > 0 && this.isConnected()) {
      const msg = this.messageQueue.shift();
      this.publish(msg.topic, msg.payload, msg.options).catch(err => {
        logger.error('Error processing queued message:', err);
      });
      this.metrics.metrics.queueSize = this.messageQueue.length;
    }
  }

  async disconnect() {
    if (!this.client) {
      return;
    }

    try {
      this.client.options.reconnectPeriod = 0;
      
      const disconnectPromise = new Promise((resolve) => {
        const timeout = setTimeout(() => {
          logger.warn('MQTT disconnect timed out, forcing closure');
          this.cleanup();
          resolve();
        }, CONSTANTS.DISCONNECT_TIMEOUT);

        this.client.removeAllListeners();
        
        this.client.once('close', () => {
          clearTimeout(timeout);
          this.cleanup();
          resolve();
        });

        this.client.unsubscribe('#', () => {
          this.client.end(true);
        });
      });

      await disconnectPromise;
    } catch (err) {
      logger.error('Error during MQTT disconnect:', err);
      this.cleanup();
    }
  }

  cleanup() {
    this.connected = false;
    this.client = null;
    this.messageQueue = [];
    this.metrics.reset();
  }

  isConnected() {
    return this.connected && this.client && this.client.connected;
  }

  getMetrics() {
    return this.metrics.getMetrics();
  }
}

// Export singleton instance
module.exports = new MQTTClient();