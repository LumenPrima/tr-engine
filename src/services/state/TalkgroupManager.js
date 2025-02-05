const mongoose = require('mongoose');
const logger = require('../../utils/logger');
const stateEventEmitter = require('../events/emitter');

class TalkgroupManager {
    constructor() {
        // In-memory caches for performance
        this.talkgroupStates = new Map(); // wacn+talkgroup -> state
        this.recentActivity = new Map(); // wacn+talkgroup -> activity array
        this.emergencyTalkgroups = new Set(); // Quick lookup for emergency talkgroups
        this.patchedTalkgroups = new Map(); // talkgroup -> Set of patched talkgroups
        this.wacnBySystem = new Map(); // sys_name -> wacn mapping

        // Listen for WACN updates from SystemManager
        stateEventEmitter.on('system.wacn', ({ sys_name, wacn }) => {
            if (sys_name && wacn) {
                this.wacnBySystem.set(sys_name, wacn);
                logger.debug(`Updated WACN mapping for system ${sys_name}: ${wacn}`);
            }
        });

        // Only initialize MongoDB if it's available
        if (process.env.MONGODB_URI) {
            mongoose.connection.on('connected', () => {
                this.collection = mongoose.connection.db.collection('talkgroups');
                
                // Load existing data into cache
                this.loadCacheFromDB().catch(err => 
                    logger.error('Error loading talkgroup cache from DB:', err)
                );
            });
        }

        logger.info('TalkgroupManager initialized');
    }

    async loadCacheFromDB() {
        if (!this.collection) {
            logger.debug('MongoDB not available, skipping cache load');
            return;
        }

        logger.debug('Loading talkgroup cache from database...');
        const talkgroups = await this.collection.find({}).toArray();
        
        talkgroups.forEach(tg => {
            const tgKey = this.getTalkgroupKey(tg.wacn, tg.talkgroup);
            this.talkgroupStates.set(tgKey, tg);
            
            // Cache recent activity
            this.recentActivity.set(tgKey, tg.recent_activity || []);
            
            // Cache emergency talkgroups
            if (tg.emergency) {
                this.emergencyTalkgroups.add(tg.talkgroup);
            }
            
            // Cache patches
            if (tg.patches?.length > 0) {
                this.patchedTalkgroups.set(tg.talkgroup, new Set(tg.patches));
            }

            // Cache WACN mapping for each system
            if (tg.systems) {
                tg.systems.forEach(sys => {
                    if (sys.sys_name) {
                        this.wacnBySystem.set(sys.sys_name, tg.wacn);
                    }
                });
            }
        });
        
        logger.info(`Loaded ${talkgroups.length} talkgroups into cache`);
    }

    async cleanup() {
        logger.debug('Cleaning up TalkgroupManager...');
        this.talkgroupStates.clear();
        this.recentActivity.clear();
        this.emergencyTalkgroups.clear();
        this.patchedTalkgroups.clear();
        this.wacnBySystem.clear();
        logger.debug('Cleared in-memory caches');
        return Promise.resolve();
    }
    
    // Generate a unique key for talkgroup+wacn
    getTalkgroupKey(wacn, talkgroup) {
        return `${wacn}:${talkgroup}`;
    }
    
    async processMessage(topic, message, messageId) {
        try {
            const topicParts = topic.split('/');
            const messageType = topicParts[2];
            
            // Handle system messages to update WACN mapping
            if (messageType === 'systems' && message.systems) {
                message.systems.forEach(sys => {
                    if (sys.wacn && sys.sys_name) {
                        this.wacnBySystem.set(sys.sys_name, sys.wacn);
                    }
                });
                return;
            }
            
            if (!message.talkgroup || !message.sys_name) {
                logger.debug('Skipping message without required talkgroup data');
                return;
            }
            
            // Get WACN for the system
            const wacn = this.wacnBySystem.get(message.sys_name);
            if (!wacn) {
                logger.warn(`No WACN found for system ${message.sys_name}, waiting for systems message`);
                return;
            }
            
            logger.debug(`Processing ${messageType} message for talkgroup ${message.talkgroup} on WACN ${wacn}`);
            
            // Update the talkgroup's state
            await this.updateTalkgroupState(message, messageType, wacn);
        } catch (err) {
            logger.error('Error processing message in TalkgroupManager:', err);
            throw err;
        }
    }
    
    async updateTalkgroupState(message, activityType, wacn) {
        try {
            if (!this.collection) {
                logger.warn('MongoDB collection not ready yet');
                return;
            }

            const tgKey = this.getTalkgroupKey(wacn, message.talkgroup);
            
            logger.debug(`Updating state for talkgroup ${message.talkgroup} (${activityType})`);
            
            // Get current state or create new one
            const currentState = this.talkgroupStates.get(tgKey) || {
                wacn: wacn,
                talkgroup: message.talkgroup,
                systems: [],
                alpha_tag: message.talkgroup_alpha_tag,
                description: message.talkgroup_description,
                category: message.talkgroup_tag,
                group: message.talkgroup_group,
                emergency: message.emergency || false,
                encrypted: message.encrypted || false,
                patches: [],
                activity_summary: {
                    first_heard: new Date(),
                    last_heard: new Date(),
                    total_calls: 0,
                    total_affiliations: 0,
                    total_emergency_calls: 0,
                    total_encrypted_calls: 0
                }
            };

            // Update systems array
            const systemIndex = currentState.systems.findIndex(s => s.sys_name === message.sys_name);
            if (systemIndex === -1) {
                currentState.systems.push({
                    sys_name: message.sys_name,
                    sys_num: message.sys_num,
                    last_heard: new Date()
                });
            } else {
                currentState.systems[systemIndex].last_heard = new Date();
            }
            
            // Create or update activity entry
            const callId = message.id?.split('_').slice(1).join('_');
            if (!callId) {
                logger.warn('Message missing call ID, skipping activity tracking');
                return;
            }

            // Get or create activity list
            if (!this.recentActivity.has(tgKey)) {
                this.recentActivity.set(tgKey, []);
            }
            const activities = this.recentActivity.get(tgKey);

            // Find existing activity or create new one
            let activity = activities.find(a => 
                a.activity_type === activityType && 
                a.call_id === callId &&
                Math.abs(new Date(a.timestamp).getTime() - new Date().getTime()) < 10000 // Within 10 seconds
            );

            if (!activity) {
                activity = {
                    timestamp: new Date(),
                    activity_type: activityType,
                    call_id: callId,
                    unit: message.unit,
                    unit_alpha_tag: message.unit_alpha_tag,
                    emergency: message.emergency || false,
                    encrypted: message.encrypted || false,
                    start_time: message.start_time,
                    stop_time: message.stop_time,
                    length: message.length,
                    system_details: []
                };
                activities.unshift(activity);
                if (activities.length > 50) activities.pop();
            }

            // Update or add system details
            const sysDetails = {
                sys_name: message.sys_name,
                sys_num: message.sys_num,
                freq: message.freq,
                error_count: message.error_count,
                spike_count: message.spike_count,
                freq_error: message.freq_error,
                call_filename: message.call_filename
            };

            const sysIndex = activity.system_details.findIndex(s => s.sys_name === message.sys_name);
            if (sysIndex >= 0) {
                activity.system_details[sysIndex] = sysDetails;
            } else {
                activity.system_details.push(sysDetails);
            }
            
            // Update state based on activity type
            const newState = {
                ...currentState,
                alpha_tag: message.talkgroup_alpha_tag || currentState.alpha_tag,
                description: message.talkgroup_description || currentState.description,
                category: message.talkgroup_tag || currentState.category,
                group: message.talkgroup_group || currentState.group,
                emergency: message.emergency || currentState.emergency,
                encrypted: message.encrypted || currentState.encrypted,
                activity_summary: {
                    ...currentState.activity_summary,
                    last_heard: new Date()
                }
            };
            
            // Update activity counters
            if (activityType === 'call') {
                newState.activity_summary.total_calls++;
                if (message.emergency) {
                    newState.activity_summary.total_emergency_calls++;
                }
                if (message.encrypted) {
                    newState.activity_summary.total_encrypted_calls++;
                }
            } else if (activityType === 'affiliation') {
                newState.activity_summary.total_affiliations++;
            }

            // Update talkgroup state in cache
            this.talkgroupStates.set(tgKey, newState);
            
            // Update database if available
            if (this.collection) {
                await this.collection.updateOne(
                    { wacn: wacn, talkgroup: message.talkgroup },
                    {
                        $set: {
                            ...newState,
                            recent_activity: activities
                        }
                    },
                    { upsert: true }
                );
            }

            // Emit talkgroup activity event
            stateEventEmitter.emitEvent('talkgroup.activity', {
                talkgroup: message.talkgroup,
                sys_name: message.sys_name,
                activity_type: activityType,
                ...newState
            });

            // Emit emergency event if needed
            if (message.emergency) {
                stateEventEmitter.emitEvent('talkgroup.emergency', {
                    talkgroup: message.talkgroup,
                    sys_name: message.sys_name,
                    unit: message.unit,
                    unit_alpha_tag: message.unit_alpha_tag,
                    ...newState
                });
            }
        } catch (err) {
            logger.error('Error updating talkgroup state:', err);
            throw err;
        }
    }
}

// Export singleton instance
const talkgroupManager = new TalkgroupManager();
module.exports = talkgroupManager;
