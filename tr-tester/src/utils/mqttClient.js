import mqtt from 'mqtt';

class MQTTClient {
  constructor() {
    this.client = null;
    this.onMessageCallbacks = new Set();
    this.onConnectCallbacks = new Set();
    this.onDisconnectCallbacks = new Set();
  }

  connect(settings) {
    const { host, port, username, password } = settings;
    const url = `mqtt://${host}:${port}`;
    
    if (this.client) {
      this.disconnect();
    }

    this.client = mqtt.connect(url, {
      username,
      password,
      keepalive: 30,
      reconnectPeriod: 5000,
    });

    this.client.on('connect', () => {
      console.log('Connected to MQTT broker');
      this.onConnectCallbacks.forEach(cb => cb());
    });

    this.client.on('message', (topic, message) => {
      const payload = message.toString();
      console.log('Received message:', { topic, payload });
      this.onMessageCallbacks.forEach(cb => cb(topic, payload));
    });

    this.client.on('error', (error) => {
      console.error('MQTT error:', error);
    });

    this.client.on('close', () => {
      console.log('Disconnected from MQTT broker');
      this.onDisconnectCallbacks.forEach(cb => cb());
    });
  }

  disconnect() {
    if (this.client) {
      this.client.end();
      this.client = null;
    }
  }

  subscribe(topic) {
    if (this.client) {
      this.client.subscribe(topic, (err) => {
        if (err) {
          console.error('Error subscribing to topic:', topic, err);
        } else {
          console.log('Subscribed to topic:', topic);
        }
      });
    }
  }

  publish(topic, message) {
    if (this.client) {
      const messageStr = JSON.stringify(message);
      this.client.publish(topic, messageStr, (err) => {
        if (err) {
          console.error('Error publishing message:', err);
        } else {
          console.log('Published message to topic:', topic);
          // Emit message event for history tracking
          this.onMessageCallbacks.forEach(cb => cb(topic, messageStr));
        }
      });
    }
  }

  onMessage(callback) {
    this.onMessageCallbacks.add(callback);
    return () => this.onMessageCallbacks.delete(callback);
  }

  onConnect(callback) {
    this.onConnectCallbacks.add(callback);
    return () => this.onConnectCallbacks.delete(callback);
  }

  onDisconnect(callback) {
    this.onDisconnectCallbacks.add(callback);
    return () => this.onDisconnectCallbacks.delete(callback);
  }

  isConnected() {
    return this.client?.connected || false;
  }
}

// Create a singleton instance
const mqttClient = new MQTTClient();
export default mqttClient;
