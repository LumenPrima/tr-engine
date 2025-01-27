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

    // Create indexes for GridFS collections
    await Promise.all([
      // Indexes for the files collection
      gridFSBucket.s.files.createIndex({ 'metadata.callId': 1 }),
      gridFSBucket.s.files.createIndex({ 'metadata.format': 1 }),
      gridFSBucket.s.files.createIndex({ 'metadata.talkgroup': 1 }),
      gridFSBucket.s.files.createIndex({ 'metadata.short_name': 1 }),
      gridFSBucket.s.files.createIndex({ 'metadata.start_time': -1 }),
      gridFSBucket.s.files.createIndex({ 'metadata.emergency': 1 }),
      
      // Compound indexes for common audio queries
      gridFSBucket.s.files.createIndex({ 
        'metadata.short_name': 1, 
        'metadata.talkgroup': 1, 
        'metadata.start_time': -1 
      }),
      gridFSBucket.s.files.createIndex({ 
        'metadata.format': 1, 
        'metadata.start_time': -1 
      })
    ]);

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
