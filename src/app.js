const express = require('express');
const dotenv = require('dotenv');
const mongoose = require('mongoose');
const path = require('path');
const logger = require('./utils/logger');
const mqttClient = require('./services/mqtt/mqtt-client');
const config = require('./config');
const WebSocketServer = require('./api/websocket/server');
const apiRoutes = require('./api/routes');
const { setupMiddleware } = require('./api/middleware');

// Initialize state managers
const activeCallManager = require('./services/state/ActiveCallManager');
const systemManager = require('./services/state/SystemManager');
const unitManager = require('./services/state/UnitManager');
const talkgroupManager = require('./services/state/TalkgroupManager');

// Load environment variables
dotenv.config();

class TREngine {
  constructor() {
    console.log('[TR-Engine] Initializing Express application...');
    this.app = express();
    this.app.use(express.json());
    this.app.use(express.urlencoded({ extended: true }));
    this.app.use(express.static('public')); // Serve static files from public directory
    console.log('[TR-Engine] Setting up middleware...');
    this.setupMiddleware();
    console.log('[TR-Engine] Setting up routes...');
    this.setupRoutes();
    console.log('[TR-Engine] Setting up error handling...');
    this.setupErrorHandling();
    this.isShuttingDown = false;
    console.log('[TR-Engine] Express application setup complete');
  }

  setupErrorHandling() {
    const { setupErrorHandling } = require('./api/middleware');
    setupErrorHandling(this.app);
  }

  async initialize() {
    try {
      console.log('[TR-Engine] Starting initialization sequence...');
      
      console.log('[TR-Engine] Step 1/6: Initializing database...');
      await this.initializeDatabase();
      
      console.log('[TR-Engine] Step 2/6: Initializing state managers...');
      await this.initializeManagers();
      
      console.log('[TR-Engine] Step 3/6: Starting HTTP server...');
      await this.startServer();
      
      console.log('[TR-Engine] Step 4/6: Setting up WebSocket server...');
      await this.setupWebSocket();
      
      console.log('[TR-Engine] Step 5/6: Connecting to MQTT broker...');
      await this.connectMQTT();
      
      console.log('[TR-Engine] Step 6/6: Setting up process handlers...');
      this.setupProcessHandlers();
      
      console.log('[TR-Engine] Initialization completed successfully');
      logger.info('TR-Engine initialized successfully');
      return true;
    } catch (err) {
      console.error('[TR-Engine] Initialization failed:', err.message);
      logger.error('Failed to initialize TR-Engine:', err);
      if (process.env.NODE_ENV !== 'test') {
        process.exit(1);
      }
      return false;
    }
  }

  async initializeDatabase() {
    if (!process.env.MONGODB_URI) {
      logger.warn('MONGODB_URI not set - running without database support');
      return;
    }

    try {
      // Connect to MongoDB
      await mongoose.connect(process.env.MONGODB_URI);
      logger.info('Connected to MongoDB');

      // Set up MongoDB event handlers
      const handleMongoEvent = (event, handler) => {
        mongoose.connection.on(event, (...args) => {
          if (!this.isShuttingDown) {
            handler(...args);
          }
        });
      };

      handleMongoEvent('error', err => {
        logger.error('MongoDB connection error:', err);
      });

      handleMongoEvent('disconnected', () => {
        logger.warn('MongoDB disconnected');
      });

      handleMongoEvent('reconnected', () => {
        logger.info('MongoDB reconnected');
      });

      // Get database instance
      const db = mongoose.connection.db;

      // Create standard message collections with indexes
      const standardCollections = [
        'call_start', 'call_end', 'calls_active', 'audio', 'config',
        'recorder', 'systems', 'rates', 'call', 'data', 'join',
        'location', 'on', 'off', 'ackresp', 'unclassified'
      ];

      for (const collection of standardCollections) {
        await db.createCollection(collection);
        const coll = db.collection(collection);
        await coll.createIndex({ timestamp: 1 });
        await coll.createIndex({ instance_id: 1 });
        if (collection !== 'unclassified') {
          await coll.createIndex({ sys_name: 1 });
        }
      }

      // Create audio-specific indexes
      const audioColl = db.collection('audio');
      await audioColl.createIndex({ talkgroup: 1 });
      await audioColl.createIndex({ start_time: 1 });
      await audioColl.createIndex({ filename: 1 });

      // Create GridFS bucket and indexes for audio files
      const bucket = new mongoose.mongo.GridFSBucket(db, { bucketName: 'audioFiles' });
      
      // Create a small temporary file to ensure collections exist
      const tempBuffer = Buffer.from('temp');
      const uploadStream = bucket.openUploadStream('temp.txt', {
        metadata: { temp: true }
      });
      await new Promise((resolve, reject) => {
        uploadStream.end(tempBuffer, (err) => {
          if (err) reject(err);
          else resolve();
        });
      });
      
      // Now create indexes on the files collection
      const audioFiles = db.collection('audioFiles.files');
      await Promise.all([
        audioFiles.createIndex({ 'metadata.talkgroup': 1 }),
        audioFiles.createIndex({ 'metadata.start_time': 1 }),
        audioFiles.createIndex({ filename: 1 })
      ]);
      
      // Clean up temp file
      await bucket.delete(uploadStream.id);

      logger.info('Database collections and indexes created');

    } catch (err) {
      logger.error('Database initialization failed:', err);
      throw err;
    }
  }

  async initializeManagers() {
    try {
      // Initialize state managers in order
      logger.info('Initializing state managers...');

      // Initialize managers in order
      // Note: SystemManager doesn't need DB cache as it manages real-time state only
      logger.info('SystemManager ready');

      // Load cached data for other managers
      await Promise.all([
        unitManager.loadCacheFromDB?.() || Promise.resolve(),
        talkgroupManager.loadCacheFromDB?.() || Promise.resolve()
      ]);
      logger.info('UnitManager and TalkgroupManager initialized');

      // Active call manager last since it depends on unit and talkgroup state
      await activeCallManager.loadCacheFromDB?.() || Promise.resolve();
      logger.info('ActiveCallManager initialized');

    } catch (err) {
      logger.error('Manager initialization failed:', err);
      throw err;
    }
  }

  setupMiddleware() {
    // Setup API middleware (security, logging, etc.)
    setupMiddleware(this.app);
  }

  setupRoutes() {
    // Root path serves status page
    this.app.get('/', (req, res) => res.sendFile('index.html', { root: 'public' }));

    // Mount API routes with versioning
    console.log('[TR-Engine] Mounting API routes at /api/v1');
    this.app.use('/api/v1', apiRoutes);
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
      console.log(`[TR-Engine] Starting HTTP server on port ${port}...`);
      this.server = this.app.listen(port, '0.0.0.0', () => {
        console.log(`[TR-Engine] HTTP server is now listening on 0.0.0.0:${port}`);
        logger.info(`TR-Engine server listening on 0.0.0.0:${port}`);
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
    // Prevent multiple shutdown attempts
    if (this.isShuttingDown) {
      logger.warn('Shutdown already in progress...');
      return;
    }
    this.isShuttingDown = true;
    
    logger.info('Shutting down TR-Engine...');

    // Set a timeout to force exit after 10 seconds
    const forceExitTimeout = setTimeout(() => {
      logger.error('Shutdown timed out after 10 seconds, forcing exit');
      process.exit(1);
    }, 10000);

    try {
      // Create an array of cleanup tasks
      const cleanupTasks = [];

      // First cleanup state managers
      cleanupTasks.push(
        activeCallManager.cleanup().then(() => {
          logger.info('ActiveCallManager cleaned up');
        })
      );
      cleanupTasks.push(
        systemManager.cleanup().then(() => {
          logger.info('SystemManager cleaned up');
        })
      );
      cleanupTasks.push(
        unitManager.cleanup().then(() => {
          logger.info('UnitManager cleaned up');
        })
      );
      cleanupTasks.push(
        talkgroupManager.cleanup().then(() => {
          logger.info('TalkgroupManager cleaned up');
        })
      );

      // Close WebSocket server
      if (this.wss) {
        cleanupTasks.push(
          new Promise(resolve => {
            this.wss.close(() => {
              logger.info('WebSocket server closed');
              resolve();
            });
          })
        );
      }

      // Close MQTT connection
      if (mqttClient.isConnected()) {
        cleanupTasks.push(
          mqttClient.disconnect().then(() => {
            logger.info('MQTT client disconnected');
          })
        );
      }

      // Close MongoDB connection
      if (mongoose.connection.readyState === 1) {
        cleanupTasks.push(
          mongoose.disconnect().then(() => {
            logger.info('MongoDB disconnected');
          })
        );
      }

      // Close HTTP server
      if (this.server) {
        cleanupTasks.push(
          new Promise(resolve => {
            this.server.close(() => {
              logger.info('HTTP server closed');
              resolve();
            });
          })
        );
      }

      // Wait for all cleanup tasks to complete
      await Promise.all(cleanupTasks);
      
      clearTimeout(forceExitTimeout);
      logger.info('Cleanup completed successfully');

      // Only exit in production
      if (process.env.NODE_ENV !== 'test') {
        process.exit(code);
      }
    } catch (err) {
      clearTimeout(forceExitTimeout);
      logger.error('Error during shutdown:', err);
      
      if (process.env.NODE_ENV !== 'test') {
        process.exit(1);
      }
    }
  }
}

// Create single instance for both export and running
const engine = new TREngine();

// Only initialize in production
if (process.env.NODE_ENV !== 'test') {
  engine.initialize().catch(err => {
    logger.error('Failed to start TR-Engine:', err);
    process.exit(1);
  });
}

module.exports = {
  TREngine,
  app: engine.app // Share the same instance
};
