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
const audioHandler = require('./services/mqtt/handlers/audio-handler');

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

      // Create collections and indexes for all managers
      const collections = {
        // Standard message collections
        'call_start': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'call_end': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'calls_active': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'audio': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 },
          { talkgroup: 1 },
          { start_time: 1 },
          { filename: 1 }
        ],
        'config': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'recorder': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'systems': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'rates': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'call': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'data': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'join': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'location': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'on': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'off': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'ackresp': [
          { timestamp: 1 },
          { instance_id: 1 },
          { sys_name: 1 }
        ],
        'unclassified': [
          { timestamp: 1 },
          { instance_id: 1 }
        ],
        // Manager-specific collections
        'units': [
          { wacn: 1, unit: 1 },
          { 'status.last_seen': 1 },
          { 'status.current_talkgroup': 1 },
          { 'systems.sys_name': 1 }
        ],
        'talkgroups': [
          { wacn: 1, talkgroup: 1 },
          { emergency: 1 },
          { last_heard: 1 },
          { 'systems.sys_name': 1 }
        ],
        'recorders': [
          { id: 1 },
          { 'status.last_update': 1 },
          { freq: 1 },
          { rec_state: 1 }
        ]
      };

      // Create collections and indexes
      for (const [collectionName, indexes] of Object.entries(collections)) {
        try {
          await db.createCollection(collectionName);
        } catch (err) {
          // Collection might already exist, which is fine
          if (!err.message.includes('Collection already exists')) {
            throw err;
          }
        }
        
        const collection = db.collection(collectionName);
        
        // Create indexes with proper options
        const indexOps = indexes.map(index => {
          const options = {};
          
          // Add unique constraint for specific indexes
          if (collectionName === 'units' && 
              index.wacn === 1 && index.unit === 1) {
            options.unique = true;
          }
          if (collectionName === 'talkgroups' && 
              index.wacn === 1 && index.talkgroup === 1) {
            options.unique = true;
          }
          if (collectionName === 'recorders' && 
              index.id === 1) {
            options.unique = true;
            options.sparse = true; // Only index documents where id exists
          }
          
          return {
            key: index,
            ...options
          };
        });

        // Drop existing indexes except _id
        const existingIndexes = await collection.indexes();
        for (const existingIndex of existingIndexes) {
          if (existingIndex.name !== '_id_') {
            await collection.dropIndex(existingIndex.name);
          }
        }

        // Create new indexes
        if (indexOps.length > 0) {
          await collection.createIndexes(indexOps);
        }
      }

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
    // Use the main HTTP server for WebSocket
    this.wss = new WebSocketServer(this.server);
    // Connect AudioHandler to WebSocket server
    audioHandler.setWebSocketServer(this.wss);
    logger.info('WebSocket server initialized and connected to AudioHandler');
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

      // Close HTTP servers
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
