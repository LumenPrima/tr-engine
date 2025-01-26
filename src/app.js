const express = require('express');
const dotenv = require('dotenv');
const mongoose = require('mongoose');
const path = require('path');
const logger = require('./utils/logger');
const mqttClient = require('./services/mqtt/client');
const config = require('./config');
const WebSocketServer = require('./api/websocket/server');
const apiRoutes = require('./api/routes');
const { setupMiddleware } = require('./api/middleware');

// Initialize state managers
const activeCallManager = require('./services/state/ActiveCallManager');
const systemManager = require('./services/state/SystemManager');
const unitManager = require('./services/state/UnitManager');

// Load environment variables
dotenv.config();

class TREngine {
  constructor() {
    this.app = express();
    this.setupMiddleware();
    this.setupRoutes();
  }

  async initialize() {
    try {
      await this.connectMongoDB();
      await this.setupWebSocket();
      await this.connectMQTT();
      await this.startServer();
      this.setupProcessHandlers();
      logger.info('TR-Engine initialized successfully');
      return true;
    } catch (err) {
      logger.error('Failed to initialize TR-Engine:', err);
      if (process.env.NODE_ENV !== 'test') {
        process.exit(1);
      }
      return false;
    }
  }

  setupMiddleware() {
    // Setup API middleware (security, logging, etc.)
    setupMiddleware(this.app);
  }

  setupRoutes() {
    // Mount API routes
    this.app.use('/', apiRoutes);

    // Health check endpoint
    this.app.get('/api/v1/health', (req, res) => {
      const status = {
        status: 'ok',
        timestamp: new Date().toISOString(),
        services: {
          mongodb: mongoose.connection.readyState === 1 ? 'connected' : 'disconnected',
          mqtt: mqttClient.isConnected() ? 'connected' : 'disconnected',
          websocket: this.wss ? 'running' : 'stopped',
          state_managers: {
            active_calls: activeCallManager ? 'running' : 'stopped',
            systems: systemManager ? 'running' : 'stopped',
            units: unitManager ? 'running' : 'stopped'
          }
        }
      };
      res.json(status);
    });

    // Handle 404s
    this.app.use((req, res) => {
      res.status(404).json({
        status: 'error',
        message: 'Not Found',
        path: req.path
      });
    });

    // Handle errors
    this.app.use((err, req, res, next) => {
      logger.error('Server error:', err);
      res.status(err.status || 500).json({
        status: 'error',
        message: err.message || 'Internal Server Error'
      });
    });
  }

  async connectMongoDB() {
    if (!process.env.MONGODB_URI) {
      logger.warn('MONGODB_URI not set - running without database support');
      return;
    }

    try {
      await mongoose.connect(process.env.MONGODB_URI);
      logger.info('Connected to MongoDB');

      mongoose.connection.on('error', err => {
        logger.error('MongoDB connection error:', err);
      });

      mongoose.connection.on('disconnected', () => {
        logger.warn('MongoDB disconnected');
      });

      mongoose.connection.on('reconnected', () => {
        logger.info('MongoDB reconnected');
      });
    } catch (err) {
      logger.warn('MongoDB connection failed:', err.message);
      logger.warn('Application running without database support');
    }
  }

  async setupWebSocket() {
    // Initialize WebSocket server with HTTP server
    this.wss = new WebSocketServer(this.server);
    logger.info('WebSocket server initialized');
  }

  async connectMQTT() {
    if (!process.env.MQTT_BROKER_URL) {
      logger.warn('MQTT_BROKER_URL not set - running without MQTT support');
      return;
    }

    try {
      await mqttClient.connect();
    } catch (err) {
      logger.error('MQTT connection failed:', err);
      logger.warn('Application running without MQTT support');
    }
  }

  async startServer() {
    const port = process.env.PORT || 3000;
    return new Promise((resolve, reject) => {
      this.server = this.app.listen(port, () => {
        logger.info(`TR-Engine server listening on port ${port}`);
        resolve();
      }).on('error', reject);
    });
  }

  setupProcessHandlers() {
    // Handle uncaught exceptions
    process.on('uncaughtException', (err) => {
      logger.error('Uncaught Exception:', err);
      this.shutdown(1);
    });

    process.on('unhandledRejection', (err) => {
      logger.error('Unhandled Rejection:', err);
      this.shutdown(1);
    });

    // Graceful shutdown
    process.on('SIGTERM', () => this.shutdown());
    process.on('SIGINT', () => this.shutdown());
  }

  async shutdown(code = 0) {
    logger.info('Shutting down TR-Engine...');

    // Close WebSocket server
    if (this.wss) {
      await new Promise(resolve => this.wss.close(resolve));
      logger.info('WebSocket server closed');
    }

    // Close MQTT connection
    if (mqttClient.isConnected()) {
      mqttClient.disconnect();
    }

    // Close MongoDB connection
    if (mongoose.connection.readyState === 1) {
      await mongoose.disconnect();
      logger.info('MongoDB disconnected');
    }

    // Close HTTP server
    if (this.server) {
      await new Promise(resolve => this.server.close(resolve));
      logger.info('HTTP server closed');
    }

    // Only exit in production
    if (process.env.NODE_ENV !== 'test') {
      process.exit(code);
    }
  }
}

// Only initialize engine in production
if (process.env.NODE_ENV !== 'test') {
  const engine = new TREngine();
  engine.initialize().catch(err => {
    logger.error('Failed to start TR-Engine:', err);
    process.exit(1);
  });
}

module.exports = {
  TREngine,
  app: new TREngine().app // For backward compatibility
};
