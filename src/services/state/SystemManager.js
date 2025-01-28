const logger = require('../../utils/logger');
const stateEventEmitter = require('../events/emitter');

class SystemManager {
    constructor() {
        // In-memory storage of system states
        this.systemStates = new Map(); // sys_name -> state
        
        // Track recent rates for each system
        this.recentRates = new Map(); // sys_name -> rates array
        
        logger.info('SystemManager initialized');
    }

    cleanup() {
        logger.debug('Cleaning up SystemManager...');
        this.systemStates.clear();
        this.recentRates.clear();
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
                        last_seen: new Date(),
                        last_config_update: new Date()
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
                    timestamp: new Date(),
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
                        last_rate_update: new Date(),
                        last_seen: new Date()
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
                        last_config_update: new Date()
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
