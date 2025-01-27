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
        await db.createCollection('systemMetrics', {
            timeseries: {
                timeField: 'timestamp',
                metaField: 'sys_name',
                granularity: 'minutes'
            }
        });

        await db.createCollection('unitActivity', {
            timeseries: {
                timeField: 'timestamp',
                metaField: 'unit',
                granularity: 'seconds'
            }
        });

        logger.info('Created time series collections');

        // Create state collections with validation
        await db.createCollection('activeCalls', {
            validator: {
                $jsonSchema: {
                    bsonType: "object",
                    required: ["id", "sys_name", "talkgroup", "start_time"],
                    properties: {
                        id: { bsonType: "string" },
                        sys_name: { bsonType: "string" },
                        sys_num: { bsonType: "int" },
                        talkgroup: { bsonType: "int" },
                        unit: { bsonType: "int" },
                        start_time: { bsonType: "long" },
                        stop_time: { bsonType: "long" },
                        emergency: { bsonType: "bool" },
                        encrypted: { bsonType: "bool" }
                    }
                }
            }
        });

        await db.createCollection('systemStates', {
            validator: {
                $jsonSchema: {
                    bsonType: "object",
                    required: ["sys_name", "sys_num", "type"],
                    properties: {
                        sys_name: { bsonType: "string" },
                        sys_num: { bsonType: "int" },
                        type: { bsonType: "string" },
                        sysid: { bsonType: "string" },
                        wacn: { bsonType: "string" },
                        last_seen: { bsonType: "long" },
                        connected: { bsonType: "bool" }
                    }
                }
            }
        });

        await db.createCollection('unitStates', {
            validator: {
                $jsonSchema: {
                    bsonType: "object",
                    required: ["unit", "sys_name"],
                    properties: {
                        unit: { bsonType: "int" },
                        sys_name: { bsonType: "string" },
                        unit_alpha_tag: { bsonType: "string" },
                        last_seen: { bsonType: "long" },
                        current_talkgroup: { bsonType: "int" },
                        online: { bsonType: "bool" }
                    }
                }
            }
        });

        await db.createCollection('talkgroups', {
            validator: {
                $jsonSchema: {
                    bsonType: "object",
                    required: ["talkgroup", "tag"],
                    properties: {
                        talkgroup: { bsonType: "int" },
                        tag: { bsonType: "string" },
                        description: { bsonType: "string" },
                        group_tag: { bsonType: "string" },
                        group: { bsonType: "string" },
                        sys_name: { bsonType: "string" }
                    }
                }
            }
        });

        logger.info('Created state collections');

        // Create GridFS bucket for audio files
        await db.createCollection('calls.files');
        await db.createCollection('calls.chunks');
        
        logger.info('Created GridFS collections for audio storage');

        // Create standard message collections with validation
        const standardCollections = {
            'call_start': ['id', 'call_num', 'sys_name', 'talkgroup', 'start_time'],
            'call_end': ['id', 'call_num', 'sys_name', 'talkgroup', 'stop_time'],
            'calls_active': ['id', 'sys_name', 'talkgroup'],
            'audio': ['filename', 'metadata'],
            'config': ['sys_name'],
            'recorder': ['sys_name'],
            'systems': ['sys_name', 'sys_num', 'type'],
            'rates': ['sys_name', 'sys_num'],
            'call': ['sys_name', 'unit', 'talkgroup'],
            'data': ['sys_name', 'unit'],
            'join': ['sys_name', 'unit'],
            'location': ['sys_name', 'unit'],
            'on': ['sys_name', 'unit'],
            'off': ['sys_name', 'unit'],
            'ackresp': ['sys_name', 'unit'],
            'unclassified': []
        };

        for (const [collection, required] of Object.entries(standardCollections)) {
            await db.createCollection(collection, {
                validator: {
                    $jsonSchema: {
                        bsonType: "object",
                        required: ["timestamp", "instance_id", ...required],
                        properties: {
                            timestamp: { bsonType: "long" },
                            instance_id: { bsonType: "string" },
                            sys_name: { bsonType: "string" },
                            sys_num: { bsonType: "int" },
                            decoderate: { bsonType: "double" },
                            decoderate_interval: { bsonType: "int" },
                            control_channel: { bsonType: "long" }
                        }
                    }
                }
            });
        }

        logger.info('Created standard collections');

        // Create text indexes for search functionality
        await db.collection('activeCalls').createIndex({
            'id': 'text',
            'talkgroup_tag': 'text',
            'talkgroup_description': 'text'
        });

        await db.collection('systemStates').createIndex({
            'sys_name': 'text',
            'type': 'text'
        });

        await db.collection('talkgroups').createIndex({
            'tag': 'text',
            'description': 'text',
            'group_tag': 'text',
            'group': 'text'
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
