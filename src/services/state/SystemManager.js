const logger = require('../../utils/logger');
const timestamps = require('../../utils/timestamps');
const stateEventEmitter = require('../events/emitter');

class SystemManager {
    constructor() {
        // In-memory storage of system states
        this.systemStates = new Map(); // sys_name -> state
        
        // Track recent rates for each system
        this.recentRates = new Map(); // sys_name -> rates array
        
        // Track active recorders for each system
        this.activeRecorders = new Map(); // sys_name -> recorder[]
        
        // Load initial state from MongoDB
        this.loadInitialState().catch(err => {
            logger.error('Failed to load initial system state:', err);
        });
        
        logger.info('SystemManager initialized');
    }

    async loadInitialState() {
        try {
            const mongoose = require('mongoose');
            const db = mongoose.connection;
            
            // Wait for MongoDB connection
            if (db.readyState !== 1) {
                logger.debug('Waiting for MongoDB connection...');
                await new Promise(resolve => {
                    db.once('connected', resolve);
                });
            }

            logger.debug('MongoDB connected, attempting to load system state');
            
            // List all collections to verify 'systems' exists
            const collections = await db.db.listCollections().toArray();
            logger.debug('Available collections:', collections.map(c => c.name));

            // Try both 'systems' and 'system' collections
            let systemsCollection = db.collection('systems');
            let latestSystem = await systemsCollection.findOne(
                { type: 'systems' },
                { sort: { _processed_at: -1 } }
            );

            if (!latestSystem) {
                logger.debug('No document found in "systems" collection, trying "system"');
                systemsCollection = db.collection('system');
                latestSystem = await systemsCollection.findOne(
                    { type: 'systems' },
                    { sort: { _processed_at: -1 } }
                );
            }

            logger.debug('Latest system document:', latestSystem);

            if (latestSystem && latestSystem.systems) {
                logger.debug('Loading initial system state from MongoDB');
                await this.updateSystemState(latestSystem);
                logger.debug('Current system states:', Array.from(this.systemStates.keys()));
            } else {
                // Try to find any document with systems array
                const pipeline = [
                    { $match: { systems: { $exists: true, $ne: null } } },
                    { $sort: { _processed_at: -1 } },
                    { $limit: 1 }
                ];
                
                for (const collName of collections.map(c => c.name)) {
                    const coll = db.collection(collName);
                    const doc = await coll.findOne(pipeline);
                    if (doc && doc.systems) {
                        logger.debug(`Found system data in collection: ${collName}`);
                        await this.updateSystemState(doc);
                        logger.debug('Current system states:', Array.from(this.systemStates.keys()));
                        return;
                    }
                }
                
                logger.warn('No system data found in any collection');
            }
        } catch (err) {
            logger.error('Error loading initial system state:', err);
            throw err;
        }
    }

    cleanup() {
        logger.debug('Cleaning up SystemManager...');
        this.systemStates.clear();
        this.recentRates.clear();
        this.activeRecorders.clear();
    }
    
    async processMessage(topic, message, messageId) {
        try {
            const topicParts = topic.split('/');
            const messageType = topicParts[2];
            
            logger.debug(`Processing ${messageType} message for system state`);
            
            switch(messageType) {
                case 'systems':
                    await this.updateSystemState(message);
                    break;
                case 'rates':
                    await this.updateSystemRates(message);
                    break;
                case 'config':
                    await this.updateSystemConfig(message);
                    break;
                case 'recorders':
                    await this.updateRecorders(message);
                    break;
            }
        } catch (err) {
            logger.error('Error processing message in SystemManager:', err);
            throw err;
        }
    }
    
    updateSystemState(message) {
        try {
            if (!message.systems || !Array.isArray(message.systems)) {
                logger.warn('Systems message missing systems array');
                return;
            }

            // Process each system in the array
            message.systems.forEach(system => {
                logger.debug(`Updating state for system ${system.sys_name}`);
                
                // Get current state or create new one
                const currentState = this.systemStates.get(system.sys_name) || {};
                
                // Update state
                const newState = {
                    ...currentState,
                    sys_name: system.sys_name,
                    name: system.sys_name,
                    sys_num: system.sys_num,
                    type: system.type,
                    sysid: system.sysid,
                    wacn: system.wacn,
                    nac: system.nac,
                    rfss: system.rfss,
                    site_id: system.site_id,
                    status: {
                        ...currentState.status,
                        connected: true,
                        last_seen: message._processed_at || message._mqtt_received_at || timestamps.getCurrentTimeISO(),
                        last_config_update: message._processed_at || message._mqtt_received_at || timestamps.getCurrentTimeISO()
                    }
                };
                
                // Update storage
                this.systemStates.set(system.sys_name, newState);
                
                // Emit system update event
                stateEventEmitter.emitSystemUpdate(newState);
            });
        } catch (err) {
            logger.error('Error updating system state:', err);
            throw err;
        }
    }

    updateRecorders(message) {
        try {
            if (!message.recorders || !Array.isArray(message.recorders)) {
                logger.warn('Recorders message missing recorders array');
                return;
            }

            // Process each recorder
            message.recorders.forEach(recorder => {
                const sysName = recorder.sys_name;
                if (!this.activeRecorders.has(sysName)) {
                    this.activeRecorders.set(sysName, []);
                }
                const recorders = this.activeRecorders.get(sysName);
                
                // Update or add recorder
                const index = recorders.findIndex(r => r.id === recorder.id);
                if (index >= 0) {
                    recorders[index] = recorder;
                } else {
                    recorders.push(recorder);
                }
            });
        } catch (err) {
            logger.error('Error updating recorders:', err);
            throw err;
        }
    }
    
    updateSystemRates(message) {
        try {
            if (!message.rates || !Array.isArray(message.rates)) {
                logger.warn('Rates message missing rates array');
                return;
            }

            // Process each system's rates
            message.rates.forEach(rate => {
                logger.debug(`Updating rates for system ${rate.sys_name}`);
                
                // Get current state
                const currentState = this.systemStates.get(rate.sys_name) || {};
                
                // Update recent rates
                if (!this.recentRates.has(rate.sys_name)) {
                    this.recentRates.set(rate.sys_name, []);
                }
                const rates = this.recentRates.get(rate.sys_name);
                rates.push({
                    timestamp: timestamps.getCurrentTimeISO(),
                    decoderate: rate.decoderate,
                    control_channel: rate.control_channel
                });
                // Keep last 60 readings
                if (rates.length > 60) rates.shift();
                
                // Update state
                const newState = {
                    ...currentState,
                    sys_name: rate.sys_name,
                    name: rate.sys_name,
                    current_control_channel: rate.control_channel,
                    current_decoderate: rate.decoderate,
                    decoderate_interval: rate.decoderate_interval,
                    status: {
                        ...currentState.status,
                        last_rate_update: timestamps.getCurrentTimeISO(),
                        last_seen: timestamps.getCurrentTimeISO()
                    },
                    recent_rates: rates
                };
                
                // Update storage
                this.systemStates.set(rate.sys_name, newState);
                
                // Emit system rates event
                stateEventEmitter.emitSystemRates({
                    sys_name: rate.sys_name,
                    decoderate: rate.decoderate,
                    control_channel: rate.control_channel,
                    interval: rate.decoderate_interval,
                    ...newState
                });
            });
        } catch (err) {
            logger.error('Error updating system rates:', err);
            throw err;
        }
    }
    
    updateSystemConfig(message) {
        try {
            if (!message.config?.systems || !Array.isArray(message.config.systems)) {
                logger.warn('Config message missing systems array');
                return;
            }

            // Process each system's config
            message.config.systems.forEach(system => {
                logger.debug(`Updating config for system ${system.sys_name}`);
                
                // Get current state
                const currentState = this.systemStates.get(system.sys_name) || {};
                
                // Update state
                const newState = {
                    ...currentState,
                    sys_name: system.sys_name,
                    name: system.sys_name,
                    config: {
                        system_type: system.system_type,
                        talkgroups_file: system.talkgroups_file,
                        control_channels: system.control_channel ? [system.control_channel] : [],
                        voice_channels: system.channels || [],
                        digital_levels: system.digital_levels,
                        audio_archive: system.audio_archive
                    },
                    status: {
                        ...currentState.status,
                        last_config_update: timestamps.getCurrentTimeISO()
                    }
                };
                
                // Update storage
                this.systemStates.set(system.sys_name, newState);
                
                // Emit system config event
                stateEventEmitter.emitSystemConfig(newState);
            });
        } catch (err) {
            logger.error('Error updating system config:', err);
            throw err;
        }
    }
    
    getSystemState(sysName) {
        try {
            return this.systemStates.get(sysName);
        } catch (err) {
            logger.error('Error getting system state:', err);
            throw err;
        }
    }
    
    getActiveSystems() {
        try {
            // Return all systems in the state map since they were loaded from MongoDB
            return Array.from(this.systemStates.values())
                .map(system => ({
                    ...system,
                    active_recorders: this.activeRecorders.get(system.sys_name) || []
                }));
        } catch (err) {
            logger.error('Error getting active systems:', err);
            throw err;
        }
    }

    clearSystems() {
        try {
            this.systemStates.clear();
            this.recentRates.clear();
            this.activeRecorders.clear();
            logger.debug('Cleared all system data');
        } catch (err) {
            logger.error('Error clearing systems:', err);
            throw err;
        }
    }
}

// Export singleton instance
const systemManager = new SystemManager();
module.exports = systemManager;
