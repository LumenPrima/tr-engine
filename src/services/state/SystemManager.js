const logger = require('../../utils/logger');
const stateEventEmitter = require('../events/emitter');

class SystemManager {
    constructor() {
        // In-memory storage of system states
        this.systemStates = new Map(); // sys_name -> state
        
        // Track recent rates for each system
        this.recentRates = new Map(); // sys_name -> rates array
        
        // Track active recorders
        this.activeRecorders = new Map(); // sys_name -> recorders array

        logger.info('SystemManager initialized');
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
            }
        } catch (err) {
            logger.error('Error processing message in SystemManager:', err);
            throw err;
        }
    }
    
    updateSystemState(message) {
        try {
            logger.debug(`Updating state for system ${message.sys_name}`);
            
            // Get current state or create new one
            const currentState = this.systemStates.get(message.sys_name) || {};
            
            // Update state
            const newState = {
                ...currentState,
                sys_num: message.sys_num,
                type: message.type,
                sysid: message.sysid,
                wacn: message.wacn,
                nac: message.nac,
                rfss: message.rfss,
                site_id: message.site_id,
                status: {
                    ...currentState.status,
                    connected: true,
                    last_seen: new Date(),
                    last_config_update: new Date()
                }
            };
            
            // Update storage
            this.systemStates.set(message.sys_name, newState);
            
            // Emit system update event
            stateEventEmitter.emitSystemUpdate(newState);
        } catch (err) {
            logger.error('Error updating system state:', err);
            throw err;
        }
    }
    
    updateSystemRates(message) {
        try {
            logger.debug(`Updating rates for system ${message.sys_name}`);
            
            // Get current state
            const currentState = this.systemStates.get(message.sys_name) || {};
            
            // Update recent rates
            if (!this.recentRates.has(message.sys_name)) {
                this.recentRates.set(message.sys_name, []);
            }
            const rates = this.recentRates.get(message.sys_name);
            rates.push({
                timestamp: new Date(),
                decoderate: message.decoderate,
                control_channel: message.control_channel
            });
            // Keep last 60 readings
            if (rates.length > 60) rates.shift();
            
            // Update state
            const newState = {
                ...currentState,
                current_control_channel: message.control_channel,
                current_decoderate: message.decoderate,
                decoderate_interval: message.decoderate_interval,
                status: {
                    ...currentState.status,
                    last_rate_update: new Date(),
                    last_seen: new Date()
                },
                recent_rates: rates
            };
            
            // Update storage
            this.systemStates.set(message.sys_name, newState);
            
            // Emit system rates event
            stateEventEmitter.emitSystemRates({
                sys_name: message.sys_name,
                decoderate: message.decoderate,
                control_channel: message.control_channel,
                interval: message.decoderate_interval,
                ...newState
            });
        } catch (err) {
            logger.error('Error updating system rates:', err);
            throw err;
        }
    }
    
    updateSystemConfig(message) {
        try {
            logger.debug(`Updating config for system ${message.sys_name}`);
            
            // Get current state
            const currentState = this.systemStates.get(message.sys_name) || {};
            
            // Update state
            const newState = {
                ...currentState,
                config: {
                    system_type: message.system_type,
                    talkgroups_file: message.talkgroups_file,
                    control_channels: message.control_channel ? [message.control_channel] : [],
                    voice_channels: message.channels || [],
                    digital_levels: message.digital_levels,
                    audio_archive: message.audio_archive
                },
                status: {
                    ...currentState.status,
                    last_config_update: new Date()
                }
            };
            
            // Update storage
            this.systemStates.set(message.sys_name, newState);
            
            // Emit system config event
            stateEventEmitter.emitSystemConfig(newState);
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
            const cutoff = Date.now() - (5 * 60 * 1000); // 5 minutes
            return Array.from(this.systemStates.values())
                .filter(system => system.status?.last_seen?.getTime() >= cutoff);
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
