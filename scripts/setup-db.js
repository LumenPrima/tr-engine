#!/usr/bin/env node
const mongoose = require('mongoose');
const dotenv = require('dotenv');
const logger = require('../src/utils/logger');

// Load environment variables
dotenv.config();

// Import models to ensure schemas are registered
require('../src/models/raw/MessageCollections');
require('../src/models/processed/CallEvent');

async function setupDatabase() {
    try {
        // Connect to MongoDB
        await mongoose.connect(process.env.MONGODB_URI);
        logger.info('Connected to MongoDB');

        // Get database instance
        const db = mongoose.connection.db;

        // Create time series collections
        await db.createCollection('systemmessage', {
            timeseries: {
                timeField: 'timestamp',
                metaField: 'sys_name',
                granularity: 'minutes'
            }
        });

        await db.createCollection('systemrates', {
            timeseries: {
                timeField: 'timestamp',
                metaField: 'sys_name',
                granularity: 'seconds'
            }
        });

        await db.createCollection('unitactivity', {
            timeseries: {
                timeField: 'timestamp',
                metaField: 'unit',
                granularity: 'seconds'
            }
        });

        logger.info('Created time series collections');

        // Create GridFS bucket for audio files
        await db.createCollection('audio.files');
        await db.createCollection('audio.chunks');
        
        logger.info('Created GridFS collections for audio storage');

        // Create text indexes for search functionality
        await db.collection('activecall').createIndex({
            'call_id': 'text',
            'talkgroup_alpha_tag': 'text',
            'talkgroup_description': 'text'
        });

        await db.collection('systemstate').createIndex({
            'sys_name': 'text',
            'type': 'text'
        });

        logger.info('Created text search indexes');

        logger.info('Database setup completed successfully');
    } catch (err) {
        logger.error('Error setting up database:', err);
        throw err;
    }
}

// Run setup if this script is run directly
if (require.main === module) {
    setupDatabase();
}

module.exports = setupDatabase;
