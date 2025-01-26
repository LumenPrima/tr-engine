const mqtt = require('mqtt');
const config = require('./index');

class MQTTClient {
  constructor() {
    this.client = null;
    this.topicHandlers = new Map();
  }

  connect() {
    const options = {
      clientId: config.mqtt.clientId,
      clean: true,
      reconnectPeriod: 5000
    };

    if (config.mqtt.username && config.mqtt.password) {
      options.username = config.mqtt.username;
      options.password = config.mqtt.password;
    }

    this.client = mqtt.connect(config.mqtt.brokerUrl, options);

    this.client.on('connect', () => {
      console.log('Connected to MQTT broker');
      this.subscribeToTopics();
    });

    this.client.on('error', (err) => {
      console.error('MQTT client error:', err);
    });

    this.client.on('close', () => {
      console.warn('MQTT client disconnected');
    });

    this.client.on('message', (topic, message) => {
      this.handleMessage(topic, message);
    });

    return this.client;
  }

  subscribeToTopics() {
    const topics = Object.values(config.mqtt.topics);
    
    topics.forEach(topic => {
      this.client.subscribe(topic, (err) => {
        if (err) {
          console.error(`Error subscribing to ${topic}:`, err);
        } else {
          console.log(`Subscribed to ${topic}`);
        }
      });
    });
  }

  registerTopicHandler(topic, handler) {
    this.topicHandlers.set(topic, handler);
  }

  handleMessage(topic, message) {
    try {
      const handler = this.findHandler(topic);
      if (handler) {
        handler(topic, message);
      } else {
        console.warn(`No handler registered for topic: ${topic}`);
      }
    } catch (err) {
      console.error('Error handling MQTT message:', err);
    }
  }

  findHandler(topic) {
    // Find the most specific matching handler
    for (const [pattern, handler] of this.topicHandlers) {
      if (this.topicMatches(pattern, topic)) {
        return handler;
      }
    }
    return null;
  }

  topicMatches(pattern, topic) {
    const patternParts = pattern.split('/');
    const topicParts = topic.split('/');

    if (patternParts.length > topicParts.length) {
      return false;
    }

    return patternParts.every((part, i) => {
      return part === '#' || part === '+' || part === topicParts[i];
    });
  }

  publish(topic, message) {
    if (!this.client || !this.client.connected) {
      throw new Error('MQTT client not connected');
    }

    return new Promise((resolve, reject) => {
      this.client.publish(topic, JSON.stringify(message), (err) => {
        if (err) {
          reject(err);
        } else {
          resolve();
        }
      });
    });
  }

  disconnect() {
    if (this.client) {
      this.client.end();
      console.log('MQTT client disconnected');
    }
  }
}

// Export singleton instance
const mqttClient = new MQTTClient();
module.exports = mqttClient;
