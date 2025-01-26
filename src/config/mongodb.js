const mongoose = require('mongoose');
const config = require('./index');

const connectDB = async () => {
  try {
    const conn = await mongoose.connect(config.mongodb.uri, config.mongodb.options);
    
    // Add indexes for common queries
    await Promise.all([
      conn.connection.collection('calls').createIndex({ timestamp: 1 }),
      conn.connection.collection('calls').createIndex({ 'metadata.talkgroup': 1 }),
      conn.connection.collection('units').createIndex({ identifier: 1 }),
      conn.connection.collection('systems').createIndex({ systemId: 1 })
    ]);

    console.log(`MongoDB Connected: ${conn.connection.host}`);
    
    // Handle connection events
    mongoose.connection.on('error', err => {
      console.error('MongoDB connection error:', err);
    });

    mongoose.connection.on('disconnected', () => {
      console.warn('MongoDB disconnected. Attempting to reconnect...');
    });

    mongoose.connection.on('reconnected', () => {
      console.log('MongoDB reconnected');
    });

    return conn;
  } catch (err) {
    console.error('Error connecting to MongoDB:', err);
    process.exit(1);
  }
};

const disconnectDB = async () => {
  try {
    await mongoose.disconnect();
    console.log('MongoDB disconnected');
  } catch (err) {
    console.error('Error disconnecting from MongoDB:', err);
    process.exit(1);
  }
};

module.exports = {
  connectDB,
  disconnectDB,
  getConnection: () => mongoose.connection
};
