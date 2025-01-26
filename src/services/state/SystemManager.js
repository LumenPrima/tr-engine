const mongoose = require('mongoose');
const logger = require('../../utils/logger');

// Raw system configuration messages (time series)
const SystemMessageSchema = new mongoose.Schema({
    timestamp: { type: Date, required: true },
    type: { type: String, required: true },
    instance_id: String,
    payload: mongoose.Schema.Types.Mixed
}, {
    timeseries: {
        timeField: 'timestamp',
        metaField: 'sys_name',
        granularity: 'minutes'  // System configs change infrequently
    }
});

// Raw system performance messages (time series)
const SystemRatesSchema = new mongoose.Schema({
    timestamp: { type: Date, required: true },
    type: { type: String, required: true },
    instance_id: String,
    payload: {
        sys_num: Number,
        sys_name: String,
        decoderate: Number,
        decoderate_interval: Number,
        control_channel: Number
    }
}, {
    timeseries: {
        timeField: 'timestamp',
        metaField: 'sys_name',
        granularity: 'seconds'  // Performance metrics need finer granularity
    }
});

// Current system state (regular collection, one doc per system)
const SystemStateSchema = new mongoose.Schema({
    sys_name: { type: String, required: true, unique: true },
    sys_num: { type: Number, required: true },
    type: String,
    sysid: String,
    wacn: String,
    nac: String,
    rfss: Number,
    site_id: Number,
    
    // Current performance metrics
    current_control_channel: Number,
    current_decoderate: Number,
    decoderate_interval: Number,
    
    // Configuration from config messages
    config: {
        system_type: String,
        talkgroups_file: String,
        control_channels: [Number],
        voice_channels: [Number],
        digital_levels: Number,
        audio_archive: Boolean
    },
    
    // Operational status
    status: {
        connected: Boolean,
        last_seen: Date,
        last_config_update: Date,
        last_rate_update: Date
    },
    
    // Performance history (recent values for quick access)
    recent_rates: [{
        timestamp: Date,
        decoderate: Number,
        control_channel: Number
    }],
    
    // Active recorders for this system
    active_recorders: [{
        id: String,
        src_num: Number,
        rec_num: Number,
        state: String,
        freq: Number,
        last_update: Date
    }]
});

// Create indexes for efficient querying
SystemStateSchema.index({ sys_name: 1 }, { unique: true });
SystemStateSchema.index({ sys_num: 1 }, { unique: true });
SystemStateSchema.index({ 'status.last_seen': 1 });

class SystemManager {
    constructor() {
        // Try to get existing models or create new ones
        try {
            this.SystemMessage = mongoose.model('SystemMessage');
        } catch (error) {
            this.SystemMessage = mongoose.model('SystemMessage', SystemMessageSchema);
        }

        try {
            this.SystemRates = mongoose.model('SystemRates');
        } catch (error) {
            this.SystemRates = mongoose.model('SystemRates', SystemRatesSchema);
        }

        try {
            this.SystemState = mongoose.model('SystemState');
        } catch (error) {
            this.SystemState = mongoose.model('SystemState', SystemStateSchema);
        }
        
        // Cache current state in memory for fastest access
        this.systemStateCache = new Map();

        logger.info('SystemManager initialized');
    }

    async cleanup() {
        logger.debug('Cleaning up SystemManager...');
        this.systemStateCache.clear();
    }
    
    async processMessage(topic, message, messageId) {
        try {
            const topicParts = topic.split('/');
            const messageType = topicParts[2];
            
            logger.debug(`Processing ${messageType} message for system state`);
            
            // Store raw message in appropriate time series collection
            if (messageType === 'systems') {
                await this.storeSystemMessage(message);
                await this.updateSystemState(message);
            } else if (messageType === 'rates') {
                await this.storeRatesMessage(message);
                await this.updateSystemRates(message);
            } else if (messageType === 'config') {
                await this.updateSystemConfig(message);
            }
        } catch (err) {
            logger.error('Error processing message in SystemManager:', err);
            throw err;
        }
    }
    
    async storeSystemMessage(message) {
        try {
            // Store in time series collection
            const systemMessages = message.systems.map(system => ({
                timestamp: new Date(),
                type: 'systems',
                instance_id: message.instance_id,
                payload: JSON.parse(JSON.stringify(system)) // Ensure proper serialization
            }));
            
            await this.SystemMessage.insertMany(systemMessages);
            logger.debug(`Stored ${systemMessages.length} system messages`);
        } catch (err) {
            logger.error('Error storing system message:', err);
            throw err;
        }
    }
    
    async storeRatesMessage(message) {
        try {
            // Store in time series collection
            const ratesMessages = message.rates.map(rate => ({
                timestamp: new Date(),
                type: 'rates',
                instance_id: message.instance_id,
                payload: JSON.parse(JSON.stringify(rate)) // Ensure proper serialization
            }));
            
            await this.SystemRates.insertMany(ratesMessages);
            logger.debug(`Stored ${ratesMessages.length} rate messages`);
        } catch (err) {
            logger.error('Error storing rates message:', err);
            throw err;
        }
    }
    
    async updateSystemState(message) {
        try {
            for (const system of message.systems) {
                logger.debug(`Updating state for system ${system.sys_name}`);
                
                // Update or create system state document
                const update = await this.SystemState.findOneAndUpdate(
                    { sys_name: system.sys_name },
                    {
                        $set: {
                            sys_num: system.sys_num,
                            type: system.type,
                            sysid: system.sysid,
                            wacn: system.wacn,
                            nac: system.nac,
                            rfss: system.rfss,
                            site_id: system.site_id,
                            'status.last_config_update': new Date(),
                            'status.last_seen': new Date(),
                            'status.connected': true
                        }
                    },
                    { upsert: true, new: true }
                );
                
                // Update cache
                this.systemStateCache.set(system.sys_name, update);
            }
        } catch (err) {
            logger.error('Error updating system state:', err);
            throw err;
        }
    }
    
    async updateSystemRates(message) {
        try {
            for (const rate of message.rates) {
                logger.debug(`Updating rates for system ${rate.sys_name}`);
                
                // Update system state with latest rates
                const update = await this.SystemState.findOneAndUpdate(
                    { sys_name: rate.sys_name },
                    {
                        $set: {
                            current_control_channel: rate.control_channel,
                            current_decoderate: rate.decoderate,
                            decoderate_interval: rate.decoderate_interval,
                            'status.last_rate_update': new Date(),
                            'status.last_seen': new Date()
                        },
                        $push: {
                            recent_rates: {
                                $each: [{
                                    timestamp: new Date(),
                                    decoderate: rate.decoderate,
                                    control_channel: rate.control_channel
                                }],
                                $slice: -60 // Keep last 60 readings
                            }
                        }
                    },
                    { new: true }
                );
                
                // Update cache
                this.systemStateCache.set(rate.sys_name, update);
            }
        } catch (err) {
            logger.error('Error updating system rates:', err);
            throw err;
        }
    }
    
    async updateSystemConfig(message) {
        try {
            const config = message.config;
            
            // Update each system mentioned in the config
            for (const system of config.systems) {
                logger.debug(`Updating config for system ${system.sys_name}`);
                
                const update = await this.SystemState.findOneAndUpdate(
                    { sys_name: system.sys_name },
                    {
                        $set: {
                            'config.system_type': system.system_type,
                            'config.talkgroups_file': system.talkgroups_file,
                            'config.control_channels': system.control_channel ? [system.control_channel] : [],
                            'config.voice_channels': system.channels || [],
                            'config.digital_levels': system.digital_levels,
                            'config.audio_archive': system.audio_archive,
                            'status.last_config_update': new Date()
                        }
                    },
                    { new: true }
                );
                
                // Update cache
                this.systemStateCache.set(system.sys_name, update);
            }
        } catch (err) {
            logger.error('Error updating system config:', err);
            throw err;
        }
    }
    
    // Helper method to get current system state
    async getSystemState(sysName) {
        try {
            return this.systemStateCache.get(sysName) || 
                   await this.SystemState.findOne({ sys_name: sysName });
        } catch (err) {
            logger.error('Error getting system state:', err);
            throw err;
        }
    }
    
    // Helper method to get all active systems
    async getActiveSystems() {
        try {
            const cutoff = new Date(Date.now() - 5 * 60 * 1000); // 5 minutes
            return this.SystemState.find({
                'status.last_seen': { $gte: cutoff }
            });
        } catch (err) {
            logger.error('Error getting active systems:', err);
            throw err;
        }
    }

    // Clear all systems (for testing)
    async clearSystems() {
        try {
            await Promise.all([
                this.SystemMessage.deleteMany({}),
                this.SystemRates.deleteMany({}),
                this.SystemState.deleteMany({})
            ]);
            this.systemStateCache.clear();
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
