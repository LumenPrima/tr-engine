const dotenv = require('dotenv');
const path = require('path');

// Load environment variables
dotenv.config();

const config = {
  env: process.env.NODE_ENV || 'development',
  server: {
    port: parseInt(process.env.PORT || '3000', 10),
    apiVersion: process.env.API_VERSION || 'v1',
    apiPrefix: process.env.API_PREFIX || '/api/v1'
  },
  mongodb: {
    uri: process.env.MONGODB_URI || 'mongodb://localhost:27017/tr-engine',
    options: {
      useNewUrlParser: true,
      useUnifiedTopology: true
    }
  },
  mqtt: {
    brokerUrl: process.env.MQTT_BROKER_URL || 'mqtt://localhost:1883',
    clientId: process.env.MQTT_CLIENT_ID || 'tr-engine-client',
    username: process.env.MQTT_USERNAME,
    password: process.env.MQTT_PASSWORD,
    topicPrefix: process.env.MQTT_TOPIC_PREFIX || 'tr-mqtt',
    reconnectPeriod: parseInt(process.env.MQTT_RECONNECT_PERIOD || '5000', 10),
    maxReconnectAttempts: parseInt(process.env.MQTT_MAX_RECONNECT_ATTEMPTS || '10', 10),
    connectTimeout: parseInt(process.env.MQTT_CONNECT_TIMEOUT || '30000', 10),
    qos: parseInt(process.env.MQTT_QOS || '1', 10)
  },
  websocket: {
    port: parseInt(process.env.WS_PORT || '3001', 10)
  },
  logging: {
    level: process.env.LOG_LEVEL || 'info'
  },
  storage: {
    audio: {
      path: process.env.AUDIO_STORAGE_PATH || path.join(__dirname, '../../storage/audio')
    }
  }
};

module.exports = config;