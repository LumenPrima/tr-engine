const mongoose = require('mongoose');
const logger = require('../../utils/logger');
const timestamps = require('../../utils/timestamps');
const stateEventEmitter = require('../events/emitter');

// Schema for persistent unit state
const UnitSchema = new mongoose.Schema({
    // Unit identification
    unit: { type: Number, required: true },
    sys_name: { type: String, required: true },
    unit_alpha_tag: String,
    
    // Current status
    status: {
        online: { type: Boolean, default: false },
        last_seen: String, // ISO 8601 string
        last_activity_type: String,
        current_talkgroup: Number,
        current_talkgroup_tag: String
    },
    
    // Activity tracking
    activity_summary: {
        first_seen: String, // ISO 8601 string
        total_calls: { type: Number, default: 0 },
        total_affiliations: { type: Number, default: 0 },
        last_call_time: String, // ISO 8601 string
        last_affiliation_time: String // ISO 8601 string
    },
    
    // Recent activity history
    recent_activity: [{
        timestamp: String, // ISO 8601 string
        activity_type: String,
        talkgroup: Number,
        talkgroup_tag: String,
        details: mongoose.Schema.Types.Mixed
    }]
}, {
    timestamps: true // Adds createdAt and updatedAt
});

// Indexes for efficient querying
UnitSchema.index({ unit: 1, sys_name: 1 }, { unique: true });
UnitSchema.index({ 'status.last_seen': 1 });
UnitSchema.index({ 'status.current_talkgroup': 1 });

class UnitManager {
    constructor() {
        // Initialize MongoDB model
        try {
            this.Unit = mongoose.model('Unit');
        } catch (error) {
            this.Unit = mongoose.model('Unit', UnitSchema);
        }
        
        // In-memory caches for performance
        this.unitStates = new Map(); // unit+sys_name -> state
        this.recentActivity = new Map(); // unit+sys_name -> activity array
        this.unitsByTalkgroup = new Map(); // talkgroup -> Set of unit+sys_name

        // Load existing data into cache
        this.loadCacheFromDB().catch(err => 
            logger.error('Error loading unit cache from DB:', err)
        );

        logger.info('UnitManager initialized');
    }

    async loadCacheFromDB() {
        logger.debug('Loading unit cache from database...');
        const units = await this.Unit.find({});
        
        units.forEach(unit => {
            const unitKey = this.getUnitKey(unit.sys_name, unit.unit);
            this.unitStates.set(unitKey, unit.toObject());
            
            // Cache recent activity
            this.recentActivity.set(unitKey, unit.recent_activity || []);
            
            // Cache talkgroup mapping
            if (unit.status.current_talkgroup) {
                if (!this.unitsByTalkgroup.has(unit.status.current_talkgroup)) {
                    this.unitsByTalkgroup.set(unit.status.current_talkgroup, new Set());
                }
                this.unitsByTalkgroup.get(unit.status.current_talkgroup).add(unitKey);
            }
        });
        
        logger.info(`Loaded ${units.length} units into cache`);
    }

    cleanup() {
        logger.debug('Cleaning up UnitManager...');
        this.unitStates.clear();
        this.recentActivity.clear();
        this.unitsByTalkgroup.clear();
        logger.debug('Cleared in-memory caches');
    }
    
    // Generate a unique key for unit+system
    getUnitKey(sysName, unit) {
        return `${sysName}:${unit}`;
    }
    
    async processMessage(topic, message, messageId) {
        try {
            const topicParts = topic.split('/');
            const sysName = topicParts[2];
            const activityType = topicParts[3];
            
            if (!message.unit) {
                logger.debug('Skipping message without unit data');
                return;
            }
            
            logger.debug(`Processing ${activityType} message for unit ${message.unit}`);
            
            // Update the unit's current state
            await this.updateUnitState(sysName, message, activityType);
        } catch (err) {
            logger.error('Error processing message in UnitManager:', err);
            throw err;
        }
    }
    
    async updateUnitState(sysName, unitData, activityType) {
        try {
            const unitKey = this.getUnitKey(sysName, unitData.unit);
            
            logger.debug(`Updating state for unit ${unitData.unit} (${activityType})`);
            
            // Get current state or create new one
            const currentState = this.unitStates.get(unitKey) || {
                unit: unitData.unit,
                sys_name: sysName,
                unit_alpha_tag: unitData.unit_alpha_tag,
                status: {
                    online: true,
                    last_seen: timestamps.getCurrentTimeISO(),
                    last_activity_type: activityType,
                    current_talkgroup: null,
                    current_talkgroup_tag: null
                },
                activity_summary: {
                    first_seen: timestamps.getCurrentTimeISO(),
                    total_calls: 0,
                    total_affiliations: 0,
                    last_call_time: null,
                    last_affiliation_time: null
                }
            };
            
            // Create new activity entry
            const newActivity = {
                timestamp: timestamps.getCurrentTimeISO(),
                activity_type: activityType,
                talkgroup: unitData.talkgroup,
                talkgroup_tag: unitData.talkgroup_tag,
                details: unitData
            };
            
            // Update recent activity
            if (!this.recentActivity.has(unitKey)) {
                this.recentActivity.set(unitKey, []);
            }
            const activities = this.recentActivity.get(unitKey);
            
            // Only add if different from most recent
            const shouldLogActivity = activities.length === 0 || 
                !this.areSimilarActivities(newActivity, activities[0]);
            
            if (shouldLogActivity) {
                activities.unshift(newActivity);
                if (activities.length > 50) activities.pop(); // Keep last 50
            }
            
            // Update state based on activity type
            const newState = {
                ...currentState,
                unit_alpha_tag: unitData.unit_alpha_tag,
                status: {
                    ...currentState.status,
                    last_seen: timestamps.getCurrentTimeISO(),
                    last_activity_type: activityType
                }
            };
            
            switch (activityType) {
                case 'on':
                    newState.status.online = true;
                    break;
                    
                case 'off':
                    newState.status.online = false;
                    break;
                    
                case 'call':
                    newState.status.current_talkgroup = unitData.talkgroup;
                    newState.status.current_talkgroup_tag = unitData.talkgroup_tag;
                    newState.activity_summary.total_calls++;
                    newState.activity_summary.last_call_time = timestamps.getCurrentTimeISO();
                    break;
                    
                case 'join':
                    newState.status.current_talkgroup = unitData.talkgroup;
                    newState.status.current_talkgroup_tag = unitData.talkgroup_tag;
                    newState.activity_summary.total_affiliations++;
                    newState.activity_summary.last_affiliation_time = timestamps.getCurrentTimeISO();
                    break;
            }
            
            // Update talkgroup tracking
            if (unitData.talkgroup) {
                if (!this.unitsByTalkgroup.has(unitData.talkgroup)) {
                    this.unitsByTalkgroup.set(unitData.talkgroup, new Set());
                }
                this.unitsByTalkgroup.get(unitData.talkgroup).add(unitKey);
            }
            
            // Update both cache and database
            this.unitStates.set(unitKey, newState);
            await this.Unit.findOneAndUpdate(
                { unit: unitData.unit, sys_name: sysName },
                {
                    $set: {
                        ...newState,
                        recent_activity: activities
                    }
                },
                { upsert: true, new: true }
            );
            
            // Emit unit activity event
            stateEventEmitter.emitUnitActivity({
                unit: unitData.unit,
                unit_alpha_tag: unitData.unit_alpha_tag,
                activity_type: activityType,
                talkgroup: unitData.talkgroup,
                talkgroup_tag: unitData.talkgroup_tag,
                sys_name: sysName,
                ...newState
            });

            // Emit specific events based on activity type
            if (activityType === 'location') {
                stateEventEmitter.emitUnitLocation({
                    unit: unitData.unit,
                    unit_alpha_tag: unitData.unit_alpha_tag,
                    sys_name: sysName,
                    ...unitData
                });
            } else if (['on', 'off'].includes(activityType)) {
                stateEventEmitter.emitUnitStatus({
                    unit: unitData.unit,
                    unit_alpha_tag: unitData.unit_alpha_tag,
                    status: activityType === 'on' ? 'online' : 'offline',
                    sys_name: sysName,
                    ...unitData
                });
            }
        } catch (err) {
            logger.error('Error updating unit state:', err);
            throw err;
        }
    }
    
    getUnitState(unit) {
        try {
            // Get all states for this unit across systems
            const states = Array.from(this.unitStates.entries())
                .filter(([key, state]) => state.unit === unit)
                .map(([key, state]) => state)
                .sort((a, b) => new Date(b.status.last_seen) - new Date(a.status.last_seen));

            if (states.length === 0) return null;

            // Combine data from all systems
            return {
                unit: unit,
                unit_alpha_tag: states[0].unit_alpha_tag,
                systems: states.map(s => s.sys_name),
                status: {
                    online: !states.some(s => s.status.last_activity_type === 'off'),
                    last_seen: timestamps.toISO(Math.max(...states.map(s => new Date(s.status.last_seen).getTime() / 1000))),
                    last_activity_type: states[0].status.last_activity_type,
                    current_talkgroup: states[0].status.current_talkgroup,
                    current_talkgroup_tag: states[0].status.current_talkgroup_tag
                },
                activity_summary: {
                    first_seen: timestamps.toISO(Math.min(...states.map(s => new Date(s.activity_summary.first_seen).getTime() / 1000))),
                    total_calls: states.reduce((sum, s) => sum + s.activity_summary.total_calls, 0),
                    total_affiliations: states.reduce((sum, s) => sum + s.activity_summary.total_affiliations, 0),
                    last_call_time: this.getLatestDate(states.map(s => s.activity_summary.last_call_time)),
                    last_affiliation_time: this.getLatestDate(states.map(s => s.activity_summary.last_affiliation_time))
                },
                recent_activity: this.aggregateRecentActivity(
                    states.flatMap(s => this.recentActivity.get(this.getUnitKey(s.sys_name, s.unit)) || [])
                )
            };
        } catch (err) {
            logger.error('Error getting unit state:', err);
            throw err;
        }
    }
    
    getUnitsInTalkgroup(talkgroup) {
        try {
            // Get unit keys for this talkgroup
            const unitKeys = this.unitsByTalkgroup.get(talkgroup) || new Set();
            
            // Group states by unit ID
            const unitGroups = {};
            for (const unitKey of unitKeys) {
                const state = this.unitStates.get(unitKey);
                if (!state) continue;
                
                if (!unitGroups[state.unit]) {
                    unitGroups[state.unit] = [];
                }
                unitGroups[state.unit].push(state);
            }

            // Combine data for each unit
            return Object.values(unitGroups).map(states => {
                states.sort((a, b) => new Date(b.status.last_seen) - new Date(a.status.last_seen));
                return {
                    unit: states[0].unit,
                    unit_alpha_tag: states[0].unit_alpha_tag,
                    systems: states.map(s => s.sys_name),
                    status: {
                        online: !states.some(s => s.status.last_activity_type === 'off'),
                        last_seen: timestamps.toISO(Math.max(...states.map(s => new Date(s.status.last_seen).getTime() / 1000))),
                        last_activity_type: states[0].status.last_activity_type
                    }
                };
            });
        } catch (err) {
            logger.error('Error getting units in talkgroup:', err);
            throw err;
        }
    }
    
    getActiveUnits(options = {}) {
        try {
            // Get all states
            const activeStates = Array.from(this.unitStates.values())
                .sort((a, b) => new Date(b.status.last_seen) - new Date(a.status.last_seen));
            
            // Group by unit ID
            const unitGroups = {};
            for (const state of activeStates) {
                if (!unitGroups[state.unit]) {
                    unitGroups[state.unit] = [];
                }
                unitGroups[state.unit].push(state);
            }
            
            // Combine data for each unit
            return Object.values(unitGroups).map(states => {
                states.sort((a, b) => new Date(b.status.last_seen) - new Date(a.status.last_seen));
                return {
                    unit: states[0].unit,
                    unit_alpha_tag: states[0].unit_alpha_tag,
                    systems: states.map(s => s.sys_name),
                    status: {
                        online: !states.some(s => s.status.last_activity_type === 'off'),
                        last_seen: timestamps.toISO(Math.max(...states.map(s => new Date(s.status.last_seen).getTime() / 1000))),
                        current_talkgroup: states[0].status.current_talkgroup,
                        current_talkgroup_tag: states[0].status.current_talkgroup_tag
                    }
                };
            });
        } catch (err) {
            logger.error('Error getting active units:', err);
            throw err;
        }
    }

    // Helper to check if two activities are similar (ignoring timestamp)
    areSimilarActivities(a, b) {
        if (a.activity_type !== b.activity_type) return false;
        if (a.talkgroup !== b.talkgroup) return false;
        
        // Only compare details for location/ackresp/join messages
        if (!['location', 'ackresp', 'join'].includes(a.activity_type)) {
            return false;
        }
        
        // Compare all fields except timestamp, _id, and sys_name
        const compareFields = ['unit', 'unit_alpha_tag', 'talkgroup', 
                             'talkgroup_alpha_tag', 'talkgroup_description', 'talkgroup_group', 
                             'talkgroup_tag', 'talkgroup_patches'];
        
        return compareFields.every(field => 
            JSON.stringify(a.details[field]) === JSON.stringify(b.details[field])
        );
    }

    // Helper method to filter out redundant activities
    aggregateRecentActivity(activities) {
        if (!activities.length) return [];

        // Sort by timestamp descending
        activities.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

        // Filter out activities that are identical to their previous message
        return activities.filter((activity, index) => {
            // Always keep the first activity
            if (index === 0) return true;
            
            // Compare with previous activity
            const prevActivity = activities[index - 1];
            
            // If it's a different type of activity (like a call vs location), keep it
            if (!['location', 'ackresp', 'join'].includes(activity.activity_type)) {
                return true;
            }

            // Only keep if it's different from the previous one
            return !this.areSimilarActivities(activity, prevActivity);
        }).slice(0, 50); // Keep most recent 50 activities
    }

    clearUnits() {
        try {
            this.unitStates.clear();
            this.recentActivity.clear();
            this.unitsByTalkgroup.clear();
            logger.debug('Cleared all unit data');
        } catch (err) {
            logger.error('Error clearing units:', err);
            throw err;
        }
    }

    // Helper method to get latest non-null date from array
    getLatestDate(dates) {
        const validDates = dates.filter(d => d);
        if (validDates.length === 0) return null;
        
        const latestUnix = Math.max(...validDates.map(d => new Date(d).getTime() / 1000));
        return timestamps.toISO(latestUnix);
    }
}

// Export singleton instance
const unitManager = new UnitManager();
module.exports = unitManager;
