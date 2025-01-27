const mongoose = require('mongoose');
const logger = require('../../utils/logger');

// Current state of each unit
const UnitStateSchema = new mongoose.Schema({
    // Unit identification
    unit: { type: Number, required: true },
    sys_name: { type: String, required: true },
    unit_alpha_tag: String,
    
    // Current status
    status: {
        online: { type: Boolean, default: true },
        last_seen: Date,
        last_activity_type: String,
        current_talkgroup: Number,
        current_talkgroup_tag: String
    },
    
    // Activity tracking
    activity_summary: {
        first_seen: Date,
        total_calls: { type: Number, default: 0 },
        total_affiliations: { type: Number, default: 0 },
        last_call_time: Date,
        last_affiliation_time: Date
    },
    
    // Recent activity cache (for quick access to unit history)
    recent_activity: [{
        timestamp: Date,
        activity_type: String,
        talkgroup: Number,
        talkgroup_tag: String,
        details: mongoose.Schema.Types.Mixed
    }]
});

// Create compound index for unit+system lookup
UnitStateSchema.index({ unit: 1, sys_name: 1 }, { unique: true });
// Index for finding recently active units
UnitStateSchema.index({ 'status.last_seen': 1 });
// Index for finding units by talkgroup
UnitStateSchema.index({ 'status.current_talkgroup': 1 });

class UnitManager {
    constructor() {
        this.UnitState = mongoose.model('UnitState', UnitStateSchema);
        
        // Cache current unit states for quick access
        this.unitStateCache = new Map();

        logger.info('UnitManager initialized');
    }

    async cleanup() {
        logger.debug('Cleaning up UnitManager...');
        this.unitStateCache.clear();
    }
    
    // Generate a unique key for the cache
    getUnitKey(sysName, unit) {
        return `${sysName}_${unit}`;
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
            
            // Get all states for this unit to check for changes across systems
            const allStates = await this.UnitState.find({ unit: unitData.unit }).sort({ 'status.last_seen': -1 });
            const newActivity = {
                timestamp: new Date(),
                activity_type: activityType,
                talkgroup: unitData.talkgroup,
                talkgroup_tag: unitData.talkgroup_tag,
                details: unitData
            };

            // Get most recent activity across all systems
            const mostRecentActivity = allStates.reduce((latest, state) => {
                if (!state.recent_activity.length) return latest;
                const stateLatest = state.recent_activity[0];
                if (!latest) return stateLatest;
                return new Date(stateLatest.timestamp) > new Date(latest.timestamp) ? stateLatest : latest;
            }, null);

            // Check if this activity is different from the most recent one
            const shouldLogActivity = !mostRecentActivity ||
                !this.areSimilarActivities(newActivity, mostRecentActivity);

            // Build the update based on activity type
            const updateData = {
                $set: {
                    unit: unitData.unit,
                    sys_name: sysName,
                    unit_alpha_tag: unitData.unit_alpha_tag,
                    'status.last_seen': new Date(),
                    'status.last_activity_type': activityType
                }
            };

            // Only add to recent_activity if it's different from the previous one
            if (shouldLogActivity) {
                updateData.$push = {
                    recent_activity: {
                        $each: [newActivity],
                        $slice: -50 // Keep last 50 activities
                    }
                };
            }
            
            // Update specific fields based on activity type
            switch (activityType) {
                case 'on':
                    updateData.$set['status.online'] = true;
                    logger.debug(`Unit ${unitData.unit} came online`);
                    break;
                    
                case 'off':
                    updateData.$set['status.online'] = false;
                    logger.debug(`Unit ${unitData.unit} went offline`);
                    break;
                    
                case 'call':
                    updateData.$set['status.current_talkgroup'] = unitData.talkgroup;
                    updateData.$set['status.current_talkgroup_tag'] = unitData.talkgroup_tag;
                    updateData.$inc = { 'activity_summary.total_calls': 1 };
                    updateData.$set['activity_summary.last_call_time'] = new Date();
                    logger.debug(`Unit ${unitData.unit} started call on talkgroup ${unitData.talkgroup}`);
                    break;
                    
                case 'join':
                    updateData.$set['status.current_talkgroup'] = unitData.talkgroup;
                    updateData.$set['status.current_talkgroup_tag'] = unitData.talkgroup_tag;
                    updateData.$inc = { 'activity_summary.total_affiliations': 1 };
                    updateData.$set['activity_summary.last_affiliation_time'] = new Date();
                    logger.debug(`Unit ${unitData.unit} joined talkgroup ${unitData.talkgroup}`);
                    break;
            }
            
            // Set first_seen if this is a new unit
            updateData.$setOnInsert = {
                'activity_summary.first_seen': new Date()
            };
            
            // Update or create the unit state document
            const unitState = await this.UnitState.findOneAndUpdate(
                { unit: unitData.unit, sys_name: sysName },
                updateData,
                { upsert: true, new: true }
            );
            
            // Update cache
            this.unitStateCache.set(unitKey, unitState);
        } catch (err) {
            logger.error('Error updating unit state:', err);
            throw err;
        }
    }
    
    // Helper method to get a unit's current state across all systems
    async getUnitState(unit) {
        try {
            // Get all states for this unit across systems
            const states = await this.UnitState.find({ unit: unit })
                .sort({ 'status.last_seen': -1 });

            if (!states || states.length === 0) {
                return null;
            }

            // Combine data from all systems
            const combinedState = {
                unit: unit,
                unit_alpha_tag: states[0].unit_alpha_tag, // Use tag from most recently seen system
                systems: states.map(s => s.sys_name),
                status: {
                    online: !states.some(s => s.status.last_activity_type === 'off'),
                    last_seen: states.reduce((latest, s) => 
                        !latest || s.status.last_seen > latest ? s.status.last_seen : latest, null),
                    last_activity_type: states[0].status.last_activity_type,
                    current_talkgroup: states[0].status.current_talkgroup,
                    current_talkgroup_tag: states[0].status.current_talkgroup_tag
                },
                activity_summary: {
                    first_seen: states.reduce((earliest, s) => 
                        !earliest || s.activity_summary.first_seen < earliest ? s.activity_summary.first_seen : earliest, null),
                    total_calls: states.reduce((sum, s) => sum + (s.activity_summary.total_calls || 0), 0),
                    total_affiliations: states.reduce((sum, s) => sum + (s.activity_summary.total_affiliations || 0), 0),
                    last_call_time: states.reduce((latest, s) => 
                        !latest || (s.activity_summary.last_call_time && s.activity_summary.last_call_time > latest) 
                            ? s.activity_summary.last_call_time : latest, null),
                    last_affiliation_time: states.reduce((latest, s) => 
                        !latest || (s.activity_summary.last_affiliation_time && s.activity_summary.last_affiliation_time > latest)
                            ? s.activity_summary.last_affiliation_time : latest, null)
                },
                // Combine, sort, and aggregate recent activity from all systems
                recent_activity: this.aggregateRecentActivity(
                    states.reduce((all, s) => [...all, ...s.recent_activity], [])
                )
            };

            return combinedState;
        } catch (err) {
            logger.error('Error getting unit state:', err);
            throw err;
        }
    }
    
    // Helper method to find all units in a talkgroup
    async getUnitsInTalkgroup(talkgroup) {
        try {
            // Get all unit states in this talkgroup
            const states = await this.UnitState.find({
                'status.current_talkgroup': talkgroup
            }).sort({ 'status.last_seen': -1 });

            // Group states by unit ID
            const unitGroups = states.reduce((groups, state) => {
                if (!groups[state.unit]) {
                    groups[state.unit] = [];
                }
                groups[state.unit].push(state);
                return groups;
            }, {});

            // Combine data for each unit
            return Object.values(unitGroups).map(states => ({
                unit: states[0].unit,
                unit_alpha_tag: states[0].unit_alpha_tag,
                systems: states.map(s => s.sys_name),
                status: {
                    online: !states.some(s => s.status.last_activity_type === 'off'),
                    last_seen: states.reduce((latest, s) => 
                        !latest || s.status.last_seen > latest ? s.status.last_seen : latest, null),
                    last_activity_type: states[0].status.last_activity_type
                }
            }));
        } catch (err) {
            logger.error('Error getting units in talkgroup:', err);
            throw err;
        }
    }
    
    // Helper method to get recently active units (aggregated across systems)
    async getActiveUnits(options = {}) {
        try {
            const cutoff = new Date(Date.now() - (options.timeWindow || 5 * 60 * 1000));
            
            // Get all active unit states
            const activeStates = await this.UnitState.find({
                'status.last_seen': { $gte: cutoff }
            }).sort({ 'status.last_seen': -1 });

            // Group states by unit ID
            const unitGroups = activeStates.reduce((groups, state) => {
                if (!groups[state.unit]) {
                    groups[state.unit] = [];
                }
                groups[state.unit].push(state);
                return groups;
            }, {});

            // Combine data for each unit
            return Object.values(unitGroups).map(states => ({
                unit: states[0].unit,
                unit_alpha_tag: states[0].unit_alpha_tag,
                systems: states.map(s => s.sys_name),
                status: {
                    online: !states.some(s => s.status.last_activity_type === 'off'),
                    last_seen: states.reduce((latest, s) => 
                        !latest || s.status.last_seen > latest ? s.status.last_seen : latest, null),
                    current_talkgroup: states[0].status.current_talkgroup,
                    current_talkgroup_tag: states[0].status.current_talkgroup_tag
                }
            }));
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

    // Clear all units (for testing)
    async clearUnits() {
        try {
            await this.UnitState.deleteMany({});
            this.unitStateCache.clear();
            logger.debug('Cleared all unit data');
        } catch (err) {
            logger.error('Error clearing units:', err);
            throw err;
        }
    }
}

// Export singleton instance
const unitManager = new UnitManager();
module.exports = unitManager;
