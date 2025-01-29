const mongoose = require('mongoose');
const logger = require('../../utils/logger');
const timestamps = require('../../utils/timestamps');
const stateEventEmitter = require('../events/emitter');

// Schema for persistent talkgroup state
const TalkgroupSchema = new mongoose.Schema({
    // Talkgroup identification
    talkgroup: { type: Number, required: true },
    sys_name: { type: String, required: true },
    sys_num: { type: Number, required: true },
    alpha_tag: String,
    description: String,
    category: String,
    group: String,
    
    // Configuration
    emergency: { type: Boolean, default: false },
    encrypted: { type: Boolean, default: false },
    patches: [Number], // Other talkgroups this one is patched with
    
    // Activity tracking
    activity_summary: {
        first_heard: String, // ISO 8601 string
        last_heard: String, // ISO 8601 string
        total_calls: { type: Number, default: 0 },
        total_affiliations: { type: Number, default: 0 },
        total_emergency_calls: { type: Number, default: 0 },
        total_encrypted_calls: { type: Number, default: 0 }
    },
    
    // Recent activity (last 50 events for quick access)
    recent_activity: [{
        timestamp: String, // ISO 8601 string
        activity_type: String,
        unit: Number,
        unit_alpha_tag: String,
        emergency: Boolean,
        encrypted: Boolean,
        details: mongoose.Schema.Types.Mixed
    }],
    
    // Custom configuration
    config: {
        alert_on_activity: { type: Boolean, default: false },
        alert_on_emergency: { type: Boolean, default: true },
        record_audio: { type: Boolean, default: true },
        notes: String
    }
}, {
    timestamps: true
});

// Indexes for efficient querying
TalkgroupSchema.index({ talkgroup: 1, sys_name: 1 }, { unique: true });
TalkgroupSchema.index({ sys_num: 1, talkgroup: 1 });
TalkgroupSchema.index({ emergency: 1 });
TalkgroupSchema.index({ 'activity_summary.last_heard': 1 });
TalkgroupSchema.index({ category: 1 });
TalkgroupSchema.index({ group: 1 });

class TalkgroupManager {
    constructor() {
        // Initialize MongoDB model
        try {
            this.Talkgroup = mongoose.model('Talkgroup');
        } catch (error) {
            this.Talkgroup = mongoose.model('Talkgroup', TalkgroupSchema);
        }
        
        // In-memory caches for performance
        this.talkgroupStates = new Map(); // talkgroup+sys_name -> state
        this.recentActivity = new Map(); // talkgroup+sys_name -> activity array
        this.emergencyTalkgroups = new Set(); // Quick lookup for emergency talkgroups
        this.patchedTalkgroups = new Map(); // talkgroup -> Set of patched talkgroups

        // Load existing data into cache
        this.loadCacheFromDB().catch(err => 
            logger.error('Error loading talkgroup cache from DB:', err)
        );

        logger.info('TalkgroupManager initialized');
    }

    async loadCacheFromDB() {
        logger.debug('Loading talkgroup cache from database...');
        const talkgroups = await this.Talkgroup.find({});
        
        talkgroups.forEach(tg => {
            const tgKey = this.getTalkgroupKey(tg.sys_name, tg.talkgroup);
            this.talkgroupStates.set(tgKey, tg.toObject());
            
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
        });
        
        logger.info(`Loaded ${talkgroups.length} talkgroups into cache`);
    }

    cleanup() {
        logger.debug('Cleaning up TalkgroupManager...');
        this.talkgroupStates.clear();
        this.recentActivity.clear();
        this.emergencyTalkgroups.clear();
        this.patchedTalkgroups.clear();
        logger.debug('Cleared in-memory caches');
    }
    
    // Generate a unique key for talkgroup+system
    getTalkgroupKey(sysName, talkgroup) {
        return `${sysName}:${talkgroup}`;
    }
    
    async processMessage(topic, message, messageId) {
        try {
            const topicParts = topic.split('/');
            const messageType = topicParts[2];
            
            if (!message.talkgroup) {
                logger.debug('Skipping message without talkgroup data');
                return;
            }
            
            logger.debug(`Processing ${messageType} message for talkgroup ${message.talkgroup}`);
            
            // Update the talkgroup's state
            await this.updateTalkgroupState(message, messageType);
        } catch (err) {
            logger.error('Error processing message in TalkgroupManager:', err);
            throw err;
        }
    }
    
    async updateTalkgroupState(message, activityType) {
        try {
            const tgKey = this.getTalkgroupKey(message.sys_name, message.talkgroup);
            
            logger.debug(`Updating state for talkgroup ${message.talkgroup} (${activityType})`);
            
            // Get current state or create new one
            const currentState = this.talkgroupStates.get(tgKey) || {
                talkgroup: message.talkgroup,
                sys_name: message.sys_name,
                sys_num: message.sys_num,
                alpha_tag: message.talkgroup_alpha_tag,
                description: message.talkgroup_description,
                category: message.talkgroup_tag,
                group: message.talkgroup_group,
                emergency: message.emergency || false,
                encrypted: message.encrypted || false,
                patches: [],
                activity_summary: {
                    first_heard: timestamps.getCurrentTimeISO(),
                    last_heard: timestamps.getCurrentTimeISO(),
                    total_calls: 0,
                    total_affiliations: 0,
                    total_emergency_calls: 0,
                    total_encrypted_calls: 0
                },
                config: {
                    alert_on_activity: false,
                    alert_on_emergency: true,
                    record_audio: true
                }
            };
            
            // Create new activity entry
            const newActivity = {
                timestamp: timestamps.getCurrentTimeISO(),
                activity_type: activityType,
                unit: message.unit,
                unit_alpha_tag: message.unit_alpha_tag,
                emergency: message.emergency || false,
                encrypted: message.encrypted || false,
                details: message
            };
            
            // Update recent activity cache (last 50 for quick access)
            if (!this.recentActivity.has(tgKey)) {
                this.recentActivity.set(tgKey, []);
            }
            const activities = this.recentActivity.get(tgKey);
            activities.unshift(newActivity);
            if (activities.length > 50) activities.pop();
            
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
                    last_heard: timestamps.getCurrentTimeISO()
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

            // Update talkgroup state in cache and database
            this.talkgroupStates.set(tgKey, newState);
            await this.Talkgroup.findOneAndUpdate(
                { talkgroup: message.talkgroup, sys_name: message.sys_name },
                {
                    $set: {
                        ...newState,
                        recent_activity: activities
                    }
                },
                { upsert: true, new: true }
            );

            // Emit talkgroup activity event
            stateEventEmitter.emitEvent('talkgroup.activity', {
                talkgroup: message.talkgroup,
                sys_name: message.sys_name,
                activity_type: activityType,
                timestamp: timestamps.getCurrentTimeISO(),
                ...newState
            });

            // Emit emergency event if needed
            if (message.emergency) {
                stateEventEmitter.emitEvent('talkgroup.emergency', {
                    talkgroup: message.talkgroup,
                    sys_name: message.sys_name,
                    unit: message.unit,
                    unit_alpha_tag: message.unit_alpha_tag,
                    timestamp: timestamps.getCurrentTimeISO(),
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
