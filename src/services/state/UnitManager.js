const mongoose = require('mongoose');
const logger = require('../../utils/logger');

// Time series collection for all unit activity messages
const UnitActivitySchema = new mongoose.Schema({
    timestamp: { type: Date, required: true },
    sys_name: { type: String, required: true },
    unit: { type: Number, required: true },
    activity_type: { 
        type: String, 
        required: true,
        enum: ['call', 'data', 'join', 'on', 'off', 'end', 'location', 'ackresp']
    },
    // Store the complete message payload for historical reference
    payload: mongoose.Schema.Types.Mixed
}, {
    timeseries: {
        timeField: 'timestamp',
        metaField: 'unit',
        granularity: 'seconds'
    }
});

// Current state of each unit
const UnitStateSchema = new mongoose.Schema({
    // Unit identification
    unit: { type: Number, required: true },
    sys_name: { type: String, required: true },
    unit_alpha_tag: String,
    
    // Current status
    status: {
        online: { type: Boolean, default: false },
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
        this.UnitActivity = mongoose.model('UnitActivity', UnitActivitySchema);
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
            
            // Extract unit data based on message type
            const unitData = message[message.type];
            if (!unitData?.unit) {
                logger.debug('Skipping message without unit data');
                return;
            }
            
            logger.debug(`Processing ${activityType} message for unit ${unitData.unit}`);
            
            // Store the raw activity
            await this.storeUnitActivity(sysName, unitData.unit, activityType, message);
            
            // Update the unit's current state
            await this.updateUnitState(sysName, unitData, activityType);
        } catch (err) {
            logger.error('Error processing message in UnitManager:', err);
            throw err;
        }
    }
    
    async storeUnitActivity(sysName, unit, activityType, message) {
        try {
            const activity = new this.UnitActivity({
                timestamp: new Date(),
                sys_name: sysName,
                unit: unit,
                activity_type: activityType,
                payload: message
            });
            
            await activity.save();
            logger.debug(`Stored ${activityType} activity for unit ${unit}`);
        } catch (err) {
            logger.error('Error storing unit activity:', err);
            throw err;
        }
    }
    
    async updateUnitState(sysName, unitData, activityType) {
        try {
            const unitKey = this.getUnitKey(sysName, unitData.unit);
            
            logger.debug(`Updating state for unit ${unitData.unit} (${activityType})`);
            
            // Build the update based on activity type
            const updateData = {
                $set: {
                    unit: unitData.unit,
                    sys_name: sysName,
                    unit_alpha_tag: unitData.unit_alpha_tag,
                    'status.last_seen': new Date(),
                    'status.last_activity_type': activityType
                },
                $push: {
                    recent_activity: {
                        $each: [{
                            timestamp: new Date(),
                            activity_type: activityType,
                            talkgroup: unitData.talkgroup,
                            talkgroup_tag: unitData.talkgroup_tag,
                            details: unitData
                        }],
                        $slice: -50 // Keep last 50 activities
                    }
                }
            };
            
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
    
    // Helper method to get a unit's current state
    async getUnitState(sysName, unit) {
        try {
            const unitKey = this.getUnitKey(sysName, unit);
            return this.unitStateCache.get(unitKey) || 
                   await this.UnitState.findOne({ sys_name: sysName, unit: unit });
        } catch (err) {
            logger.error('Error getting unit state:', err);
            throw err;
        }
    }
    
    // Helper method to find all units in a talkgroup
    async getUnitsInTalkgroup(talkgroup) {
        try {
            return this.UnitState.find({
                'status.current_talkgroup': talkgroup
            }).sort({ 'status.last_seen': -1 });
        } catch (err) {
            logger.error('Error getting units in talkgroup:', err);
            throw err;
        }
    }
    
    // Helper method to get recently active units
    async getActiveUnits(options = {}) {
        try {
            const cutoff = new Date(Date.now() - (options.timeWindow || 5 * 60 * 1000));
            return this.UnitState.find({
                'status.last_seen': { $gte: cutoff }
            }).sort({ 'status.last_seen': -1 });
        } catch (err) {
            logger.error('Error getting active units:', err);
            throw err;
        }
    }
    
    // Helper method to get unit history
    async getUnitHistory(sysName, unit, options = {}) {
        try {
            const query = {
                sys_name: sysName,
                unit: unit
            };
            
            if (options.startTime) {
                query.timestamp = { $gte: options.startTime };
            }
            if (options.endTime) {
                query.timestamp = { ...query.timestamp, $lte: options.endTime };
            }
            if (options.activityTypes) {
                query.activity_type = { $in: options.activityTypes };
            }
            
            return this.UnitActivity.find(query)
                .sort({ timestamp: -1 })
                .limit(options.limit || 100);
        } catch (err) {
            logger.error('Error getting unit history:', err);
            throw err;
        }
    }

    // Clear all units (for testing)
    async clearUnits() {
        try {
            await Promise.all([
                this.UnitActivity.deleteMany({}),
                this.UnitState.deleteMany({})
            ]);
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
