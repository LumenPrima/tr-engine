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

        // Call-related collection indexes
        const callCollections = ['call_start', 'call_end', 'calls_active'];
        for (const collection of callCollections) {
            await db.collection(collection).createIndex({ timestamp: 1 });
            await db.collection(collection).createIndex({ instance_id: 1 });
            await db.collection(collection).createIndex({ id: 1 });
            await db.collection(collection).createIndex({ call_num: 1 });
            await db.collection(collection).createIndex({ sys_num: 1 });
            await db.collection(collection).createIndex({ sys_name: 1 });
            await db.collection(collection).createIndex({ talkgroup: 1 });
            await db.collection(collection).createIndex({ unit: 1 });
            await db.collection(collection).createIndex({ start_time: 1 });
            await db.collection(collection).createIndex({ emergency: 1 });
            await db.collection(collection).createIndex({ encrypted: 1 });
            logger.info(`Created indexes for ${collection}`);
        }

        // Audio collection indexes
        await db.collection('audio').createIndex({ timestamp: 1 });
        await db.collection('audio').createIndex({ instance_id: 1 });
        await db.collection('audio').createIndex({ talkgroup: 1 });
        await db.collection('audio').createIndex({ start_time: 1 });
        await db.collection('audio').createIndex({ stop_time: 1 });
        await db.collection('audio').createIndex({ filename: 1 });
        await db.collection('audio').createIndex({ freq: 1 });
        await db.collection('audio').createIndex({ source_num: 1 });
        await db.collection('audio').createIndex({ recorder_num: 1 });
        await db.collection('audio').createIndex({ emergency: 1 });
        await db.collection('audio').createIndex({ encrypted: 1 });
        await db.collection('audio').createIndex({ short_name: 1 });
        logger.info('Created indexes for audio collection');

        // System-related collection indexes
        await db.collection('systems').createIndex({ timestamp: 1 });
        await db.collection('systems').createIndex({ instance_id: 1 });
        await db.collection('systems').createIndex({ sys_num: 1 });
        await db.collection('systems').createIndex({ sys_name: 1 });
        await db.collection('systems').createIndex({ type: 1 });
        await db.collection('systems').createIndex({ sysid: 1 });
        await db.collection('systems').createIndex({ wacn: 1 });
        logger.info('Created indexes for systems collection');

        // Unit-related collection indexes
        const unitCollections = ['call', 'data', 'join', 'location', 'on', 'off', 'ackresp'];
        for (const collection of unitCollections) {
            await db.collection(collection).createIndex({ timestamp: 1 });
            await db.collection(collection).createIndex({ instance_id: 1 });
            await db.collection(collection).createIndex({ sys_name: 1 });
            await db.collection(collection).createIndex({ unit: 1 });
            await db.collection(collection).createIndex({ unit_alpha_tag: 1 });
            await db.collection(collection).createIndex({ talkgroup: 1 });
            logger.info(`Created indexes for ${collection}`);
        }

        // State collection indexes
        await db.collection('activeCalls').createIndex({ id: 1 }, { unique: true });
        await db.collection('activeCalls').createIndex({ sys_name: 1 });
        await db.collection('activeCalls').createIndex({ sys_num: 1 });
        await db.collection('activeCalls').createIndex({ talkgroup: 1 });
        await db.collection('activeCalls').createIndex({ unit: 1 });
        await db.collection('activeCalls').createIndex({ start_time: 1 });
        await db.collection('activeCalls').createIndex({ stop_time: 1 });
        await db.collection('activeCalls').createIndex({ emergency: 1 });
        await db.collection('activeCalls').createIndex({ encrypted: 1 });
        logger.info('Created indexes for activeCalls collection');

        await db.collection('systemStates').createIndex({ sys_name: 1 }, { unique: true });
        await db.collection('systemStates').createIndex({ sys_num: 1 }, { unique: true });
        await db.collection('systemStates').createIndex({ type: 1 });
        await db.collection('systemStates').createIndex({ sysid: 1 });
        await db.collection('systemStates').createIndex({ wacn: 1 });
        await db.collection('systemStates').createIndex({ last_seen: 1 });
        await db.collection('systemStates').createIndex({ connected: 1 });
        logger.info('Created indexes for systemStates collection');

        await db.collection('unitStates').createIndex({ unit: 1, sys_name: 1 }, { unique: true });
        await db.collection('unitStates').createIndex({ unit_alpha_tag: 1 });
        await db.collection('unitStates').createIndex({ last_seen: 1 });
        await db.collection('unitStates').createIndex({ current_talkgroup: 1 });
        await db.collection('unitStates').createIndex({ online: 1 });
        logger.info('Created indexes for unitStates collection');

        await db.collection('talkgroups').createIndex({ talkgroup: 1 }, { unique: true });
        await db.collection('talkgroups').createIndex({ tag: 1 });
        await db.collection('talkgroups').createIndex({ group_tag: 1 });
        await db.collection('talkgroups').createIndex({ sys_name: 1 });
        logger.info('Created indexes for talkgroups collection');

        // GridFS calls collection indexes
        await db.collection('calls.files').createIndex({ 'metadata.talkgroup': 1 });
        await db.collection('calls.files').createIndex({ 'metadata.start_time': 1 });
        await db.collection('calls.files').createIndex({ 'metadata.stop_time': 1 });
        await db.collection('calls.files').createIndex({ 'metadata.emergency': 1 });
        await db.collection('calls.files').createIndex({ 'metadata.encrypted': 1 });
        await db.collection('calls.files').createIndex({ 'metadata.freq': 1 });
        await db.collection('calls.files').createIndex({ filename: 1 });
        await db.collection('calls.chunks').createIndex({ files_id: 1, n: 1 }, { unique: true });
        logger.info('Created indexes for GridFS calls collections');

        // Rates collection indexes
        await db.collection('rates').createIndex({ timestamp: 1 });
        await db.collection('rates').createIndex({ instance_id: 1 });
        await db.collection('rates').createIndex({ sys_name: 1 });
        await db.collection('rates').createIndex({ sys_num: 1 });
        await db.collection('rates').createIndex({ decoderate: 1 });
        await db.collection('rates').createIndex({ control_channel: 1 });
        logger.info('Created indexes for rates collection');

        // Other standard collections
        const otherCollections = ['config', 'recorder', 'unclassified'];
        for (const collection of otherCollections) {
            await db.collection(collection).createIndex({ timestamp: 1 });
            await db.collection(collection).createIndex({ instance_id: 1 });
            await db.collection(collection).createIndex({ sys_name: 1 });
            logger.info(`Created indexes for ${collection}`);
        }

        // Time series collections (indexes auto-created by MongoDB)
        logger.info('Time series collections use auto-created indexes');

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
