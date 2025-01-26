const mongoose = require('mongoose');

// Time series collection for all unit activity messages
const UnitActivitySchema = new mongoose.Schema({
    timestamp: { type: Date, required: true },
    sys_name: { type: String, required: true },
    unit: { type: Number, required: true },
    activity_type: { 
        type: String, 
        required: true,
        enum: ['call', 'data', 'join', 'on', 'off']
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

class UnitStateManager {
    constructor() {
        this.UnitActivity = mongoose.model('UnitActivity', UnitActivitySchema);
        this.UnitState = mongoose.model('UnitState', UnitStateSchema);
        
        // Cache current unit states for quick access
        this.unitStateCache = new Map();
    }
    
    // Generate a unique key for the cache
    getUnitKey(sysName, unit) {
        return `${sysName}_${unit}`;
    }
    
    async processMessage(topic, message, messageId) {
        const topicParts = topic.split('/');
        const sysName = topicParts[2];
        const activityType = topicParts[3];
        
        // Extract unit data based on message type
        const unitData = message[message.type];
        if (!unitData?.unit) return;
        
        // Store the raw activity
        await this.storeUnitActivity(sysName, unitData.unit, activityType, message);
        
        // Update the unit's current state
        await this.updateUnitState(sysName, unitData, activityType);
    }
    
    async storeUnitActivity(sysName, unit, activityType, message) {
        const activity = new this.UnitActivity({
            timestamp: new Date(),
            sys_name: sysName,
            unit: unit,
            activity_type: activityType,
            payload: message
        });
        
        await activity.save();
    }
    
    async updateUnitState(sysName, unitData, activityType) {
        const unitKey = this.getUnitKey(sysName, unitData.unit);
        
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
                break;
                
            case 'off':
                updateData.$set['status.online'] = false;
                break;
                
            case 'call':
                updateData.$set['status.current_talkgroup'] = unitData.talkgroup;
                updateData.$set['status.current_talkgroup_tag'] = unitData.talkgroup_tag;
                updateData.$inc = { 'activity_summary.total_calls': 1 };
                updateData.$set['activity_summary.last_call_time'] = new Date();
                break;
                
            case 'join':
                updateData.$set['status.current_talkgroup'] = unitData.talkgroup;
                updateData.$set['status.current_talkgroup_tag'] = unitData.talkgroup_tag;
                updateData.$inc = { 'activity_summary.total_affiliations': 1 };
                updateData.$set['activity_summary.last_affiliation_time'] = new Date();
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
    }
    
    // Helper method to get a unit's current state
    async getUnitState(sysName, unit) {
        const unitKey = this.getUnitKey(sysName, unit);
        return this.unitStateCache.get(unitKey) || 
               await this.UnitState.findOne({ sys_name: sysName, unit: unit });
    }
    
    // Helper method to find all units in a talkgroup
    async getUnitsInTalkgroup(talkgroup) {
        return this.UnitState.find({
            'status.current_talkgroup': talkgroup
        }).sort({ 'status.last_seen': -1 });
    }
    
    // Helper method to get recently active units
    async getActiveUnits(options = {}) {
        const cutoff = new Date(Date.now() - (options.timeWindow || 5 * 60 * 1000));
        return this.UnitState.find({
            'status.last_seen': { $gte: cutoff }
        }).sort({ 'status.last_seen': -1 });
    }
    
    // Helper method to get unit history
    async getUnitHistory(sysName, unit, options = {}) {
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
    }
}

module.exports = UnitStateManager;