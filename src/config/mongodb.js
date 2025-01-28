const mongoose = require('mongoose');
const config = require('./index');
const logger = require('../utils/logger');

const connectDB = async () => {
  try {
    console.log(`[MongoDB] Attempting to connect to ${config.mongodb.uri}`);
    const conn = await mongoose.connect(config.mongodb.uri, config.mongodb.options);
    console.log(`[MongoDB] Successfully connected to ${conn.connection.host}`);
    
    // Set up GridFS bucket for audio files
    const gridFSBucket = new mongoose.mongo.GridFSBucket(conn.connection.db, {
      bucketName: 'audioFiles'
    });

    // Create GridFS collections and indexes
    const db = conn.connection.db;
    
    // Ensure GridFS collections exist by writing a dummy file
    const tempBucket = new mongoose.mongo.GridFSBucket(db, {
      bucketName: 'audioFiles'
    });
    
    try {
      // Create a small temporary file to ensure collections are created
      const tempBuffer = Buffer.from('temp');
      const uploadStream = tempBucket.openUploadStream('temp.txt', {
        metadata: { temp: true }
      });
      await new Promise((resolve, reject) => {
        uploadStream.end(tempBuffer, (err) => {
          if (err) reject(err);
          else resolve();
        });
      });
      
      // Now that collections exist, create indexes
      const audioFiles = db.collection('audioFiles.files');
      await Promise.all([
        // Indexes for the files collection
        audioFiles.createIndex({ 'metadata.callId': 1 }),
        audioFiles.createIndex({ 'metadata.format': 1 }),
        audioFiles.createIndex({ 'metadata.talkgroup': 1 }),
        audioFiles.createIndex({ 'metadata.short_name': 1 }),
        audioFiles.createIndex({ 'metadata.start_time': -1 }),
        audioFiles.createIndex({ 'metadata.emergency': 1 }),
        
        // Compound indexes for common audio queries
        audioFiles.createIndex({ 
          'metadata.short_name': 1, 
          'metadata.talkgroup': 1, 
          'metadata.start_time': -1 
        }),
        audioFiles.createIndex({ 
          'metadata.format': 1, 
          'metadata.start_time': -1 
        })
      ]);
      
      // Clean up temp file
      await tempBucket.delete(uploadStream.id);
      
    } catch (err) {
      logger.error('Error setting up GridFS:', err);
      throw err;
    }

    logger.info(`MongoDB Connected: ${conn.connection.host}`);
    
    // Handle connection events
    mongoose.connection.on('error', err => {
      console.error('[MongoDB] Connection error:', err.message);
      logger.error('MongoDB connection error:', err);
    });

    mongoose.connection.on('disconnected', () => {
      console.warn('[MongoDB] Disconnected from database. Attempting to reconnect...');
      logger.warn('MongoDB disconnected. Attempting to reconnect...');
    });

    mongoose.connection.on('reconnected', () => {
      console.log('[MongoDB] Successfully reconnected to database');
      logger.info('MongoDB reconnected');
    });

    return conn;
  } catch (err) {
    logger.error('Error connecting to MongoDB:', err);
    throw err; // Let the application handle the error
  }
};

const disconnectDB = async () => {
  try {
    await mongoose.disconnect();
    logger.info('MongoDB disconnected');
  } catch (err) {
    logger.error('Error disconnecting from MongoDB:', err);
    throw err;
  }
};

// Helper to get GridFS bucket
const getGridFSBucket = () => {
  if (mongoose.connection.readyState !== 1) {
    throw new Error('MongoDB not connected');
  }
  return new mongoose.mongo.GridFSBucket(mongoose.connection.db, {
    bucketName: 'audioFiles'
  });
};

module.exports = {
  connectDB,
  disconnectDB,
  getConnection: () => mongoose.connection,
  getGridFSBucket
};
