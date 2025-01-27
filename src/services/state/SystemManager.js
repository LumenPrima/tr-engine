const mongoose = require('mongoose');
const logger = require('../../utils/logger');

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
        // Initialize system state model
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
            }
        } catch (err) {
            logger.error('Error processing message in SystemManager:', err);
            throw err;
        }
    }
    
    async updateSystemState(message) {
        try {
            logger.debug(`Updating state for system ${message.sys_name}`);
            
            // Update or create system state document
            const update = await this.SystemState.findOneAndUpdate(
                { sys_name: message.sys_name },
                {
                    $set: {
                        sys_num: message.sys_num,
                        type: message.type,
                        sysid: message.sysid,
                        wacn: message.wacn,
                        nac: message.nac,
                        rfss: message.rfss,
                        site_id: message.site_id,
                        'status.last_config_update': new Date(),
                        'status.last_seen': new Date(),
                        'status.connected': true
                    }
                },
                { upsert: true, new: true }
            );
            
            // Update cache
            this.systemStateCache.set(message.sys_name, update);
        } catch (err) {
            logger.error('Error updating system state:', err);
            throw err;
        }
    }
    
    async updateSystemRates(message) {
        try {
            logger.debug(`Updating rates for system ${message.sys_name}`);
            
            // Update system state with latest rates
            const update = await this.SystemState.findOneAndUpdate(
                { sys_name: message.sys_name },
                {
                    $set: {
                        current_control_channel: message.control_channel,
                        current_decoderate: message.decoderate,
                        decoderate_interval: message.decoderate_interval,
                        'status.last_rate_update': new Date(),
                        'status.last_seen': new Date()
                    },
                    $push: {
                        recent_rates: {
                            $each: [{
                                timestamp: new Date(),
                                decoderate: message.decoderate,
                                control_channel: message.control_channel
                            }],
                            $slice: -60 // Keep last 60 readings
                        }
                    }
                },
                { new: true }
            );
            
            // Update cache
            this.systemStateCache.set(message.sys_name, update);
        } catch (err) {
            logger.error('Error updating system rates:', err);
            throw err;
        }
    }
    
    async updateSystemConfig(message) {
        try {
            logger.debug(`Updating config for system ${message.sys_name}`);
            
            const update = await this.SystemState.findOneAndUpdate(
                { sys_name: message.sys_name },
                {
                    $set: {
                        'config.system_type': message.system_type,
                        'config.talkgroups_file': message.talkgroups_file,
                        'config.control_channels': message.control_channel ? [message.control_channel] : [],
                        'config.voice_channels': message.channels || [],
                        'config.digital_levels': message.digital_levels,
                        'config.audio_archive': message.audio_archive,
                        'status.last_config_update': new Date()
                    }
                },
                { new: true }
            );
            
            // Update cache
            this.systemStateCache.set(message.sys_name, update);
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
            await this.SystemState.deleteMany({});
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
