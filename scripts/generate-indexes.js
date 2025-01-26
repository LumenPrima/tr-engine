#!/usr/bin/env node
const mongoose = require('mongoose');
const dotenv = require('dotenv');
const logger = require('../src/utils/logger');

// Load environment variables
dotenv.config();

// Import models to ensure schemas are registered
require('../src/models/raw/MessageCollections');
require('../src/models/processed/CallEvent');

async function generateIndexes(shouldDisconnect = true) {
    try {
        // Connect to MongoDB if not already connected
        if (mongoose.connection.readyState === 0) {
            await mongoose.connect(process.env.MONGODB_URI);
            logger.info('Connected to MongoDB');
        }

        const db = mongoose.connection.db;

        // Raw message collections indexes
        const rawCollections = [
            'systemmessage',
            'systemrates',
            'callstartmessage',
            'callendmessage',
            'callsactivemessage',
            'audiomessage',
            'unitcallmessage',
            'unitlocationmessage',
            'unitdatamessage',
            'unitjoinmessage',
            'unitendmessage',
            'unitonmessage',
            'unitoffmessage',
            'unitackrespmessage'
        ];

        // Create indexes for raw message collections
        for (const collection of rawCollections) {
            await db.collection(collection).createIndex({ timestamp: -1 });
            await db.collection(collection).createIndex({ instance_id: 1 });
            await db.collection(collection).createIndex({ type: 1 });
            logger.info(`Created indexes for ${collection}`);
        }

        // Active calls indexes
        await db.collection('activecall').createIndex({ call_id: 1 }, { unique: true });
        await db.collection('activecall').createIndex({ sys_name: 1, start_time: -1 });
        await db.collection('activecall').createIndex({ talkgroup: 1, start_time: -1 });
        await db.collection('activecall').createIndex({ last_update: 1 });
        await db.collection('activecall').createIndex({ emergency: 1 });
        await db.collection('activecall').createIndex({ 'recorder.id': 1 });
        logger.info('Created indexes for activecall collection');

        // System state indexes
        await db.collection('systemstate').createIndex({ sys_name: 1 }, { unique: true });
        await db.collection('systemstate').createIndex({ sys_num: 1 }, { unique: true });
        await db.collection('systemstate').createIndex({ 'status.last_seen': 1 });
        await db.collection('systemstate').createIndex({ 'status.connected': 1 });
        await db.collection('systemstate').createIndex({ type: 1 });
        logger.info('Created indexes for systemstate collection');

        // Unit state indexes
        await db.collection('unitstate').createIndex({ unit: 1, sys_name: 1 }, { unique: true });
        await db.collection('unitstate').createIndex({ 'status.last_seen': 1 });
        await db.collection('unitstate').createIndex({ 'status.current_talkgroup': 1 });
        await db.collection('unitstate').createIndex({ 'status.online': 1 });
        logger.info('Created indexes for unitstate collection');

        // Audio file indexes
        await db.collection('audio.files').createIndex({ filename: 1 });
        await db.collection('audio.files').createIndex({ uploadDate: 1 });
        await db.collection('audio.chunks').createIndex({ files_id: 1, n: 1 }, { unique: true });
        logger.info('Created indexes for GridFS audio collections');

        logger.info('Index generation completed successfully');
    } catch (err) {
        logger.error('Error generating indexes:', err);
        throw err;
    }
}

// Run index generation if this script is run directly
if (require.main === module) {
    generateIndexes().catch(err => {
        logger.error('Failed to generate indexes:', err);
        process.exit(1);
    });
}

module.exports = {
    generateIndexes
};
