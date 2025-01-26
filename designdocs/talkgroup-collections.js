const mongoose = require('mongoose');

// Base talkgroup information - relatively static data
const TalkgroupSchema = new mongoose.Schema({
    talkgroup_id: { type: Number, required: true, unique: true },
    
    // Basic metadata
    alpha_tag: { type: String, required: true },  // Display name
    description: String,
    
    // Hierarchical organization
    tag: { type: String, required: true },        // Primary category
    group: { type: String, required: true },      // Department/organization
    subgroup: String,                             // Optional subdivision
    
    // Operational settings
    priority: { type: Number, default: 5 },
    emergency_enabled: { type: Boolean, default: true },
    record_audio: { type: Boolean, default: true },
    
    // Parent/child relationships for organizational hierarchy
    parent_group: { type: String, sparse: true },
    child_groups: [String],
    
    // Metadata
    created_at: { type: Date, default: Date.now },
    updated_at: { type: Date, default: Date.now },
    
    // System-specific configuration
    system_configs: [{
        sys_name: String,
        enabled: { type: Boolean, default: true },
        priority_override: Number,
        record_settings: {
            record_audio: Boolean,
            monitor_emergency: Boolean
        }
    }]
});

// Active talkgroup state - dynamic real-time data
const TalkgroupStateSchema = new mongoose.Schema({
    talkgroup_id: { type: Number, required: true, unique: true },
    
    // Current state
    active: { type: Boolean, default: false },
    last_activity: Date,
    current_patch: { type: String, sparse: true },  // ID of active patch group
    
    // Real-time metrics
    active_units: [{
        unit: Number,
        joined_at: Date
    }],
    active_calls: [{
        call_id: String,
        start_time: Date
    }],
    
    // Short-term statistics (last 24 hours)
    daily_stats: {
        total_calls: { type: Number, default: 0 },
        total_time: { type: Number, default: 0 },  // seconds
        emergency_calls: { type: Number, default: 0 },
        unique_units: { type: Set, default: new Set() },
        last_reset: { type: Date, default: Date.now }
    }
});

// Talkgroup patches (temporary merges)
const TalkgroupPatchSchema = new mongoose.Schema({
    patch_id: { type: String, required: true, unique: true },
    created_at: { type: Date, default: Date.now },
    active: { type: Boolean, default: true },
    ended_at: Date,
    
    // Member talkgroups
    members: [{
        talkgroup_id: Number,
        joined_at: { type: Date, default: Date.now }
    }],
    
    // Patch history
    events: [{
        timestamp: Date,
        event_type: {
            type: String,
            enum: ['created', 'member_added', 'member_removed', 'ended']
        },
        talkgroup_id: Number,
        details: mongoose.Schema.Types.Mixed
    }]
});

// Historical activity tracking
const TalkgroupActivitySchema = new mongoose.Schema({
    talkgroup_id: Number,
    timestamp: Date,
    hour: Number,  // 0-23, for hourly aggregation
    day: Number,   // 0-6, for daily patterns
    
    // Activity metrics
    call_count: { type: Number, default: 0 },
    total_time: { type: Number, default: 0 },
    unique_units: { type: Set, default: new Set() },
    emergency_count: { type: Number, default: 0 },
    
    // System metrics
    system_metrics: {
        decode_errors: { type: Number, default: 0 },
        audio_issues: { type: Number, default: 0 }
    }
}, {
    timeseries: {
        timeField: 'timestamp',
        metaField: 'talkgroup_id',
        granularity: 'hours'
    }
});

// Create indexes for common query patterns
TalkgroupSchema.index({ tag: 1 });
TalkgroupSchema.index({ group: 1 });
TalkgroupSchema.index({ 'system_configs.sys_name': 1 });
TalkgroupSchema.index({ alpha_tag: 'text', description: 'text' });

TalkgroupStateSchema.index({ active: 1, last_activity: -1 });
TalkgroupStateSchema.index({ current_patch: 1 }, { sparse: true });

TalkgroupPatchSchema.index({ active: 1, created_at: -1 });
TalkgroupPatchSchema.index({ 'members.talkgroup_id': 1 });

class TalkgroupManager {
    constructor() {
        this.Talkgroup = mongoose.model('Talkgroup', TalkgroupSchema);
        this.TalkgroupState = mongoose.model('TalkgroupState', TalkgroupStateSchema);
        this.TalkgroupPatch = mongoose.model('TalkgroupPatch', TalkgroupPatchSchema);
        this.TalkgroupActivity = mongoose.model('TalkgroupActivity', TalkgroupActivitySchema);
        
        // Cache for active talkgroup states
        this.stateCache = new Map();
        // Cache for active patches
        this.patchCache = new Map();
    }
    
    async processCallActivity(callData) {
        const talkgroupId = callData.talkgroup;
        
        // Update talkgroup state
        const state = await this.TalkgroupState.findOneAndUpdate(
            { talkgroup_id: talkgroupId },
            {
                $set: {
                    active: true,
                    last_activity: new Date()
                },
                $push: {
                    active_calls: {
                        call_id: callData.call_id,
                        start_time: new Date(callData.start_time)
                    }
                },
                $inc: {
                    'daily_stats.total_calls': 1,
                    'daily_stats.emergency_calls': callData.emergency ? 1 : 0
                }
            },
            { upsert: true, new: true }
        );
        
        // Update cache
        this.stateCache.set(talkgroupId, state);
        
        // Record activity for historical tracking
        const hour = new Date().getHours();
        const day = new Date().getDay();
        
        await this.TalkgroupActivity.updateOne(
            {
                talkgroup_id: talkgroupId,
                timestamp: new Date(),
                hour,
                day
            },
            {
                $inc: {
                    call_count: 1,
                    total_time: callData.call_length || 0,
                    emergency_count: callData.emergency ? 1 : 0
                },
                $addToSet: {
                    unique_units: callData.unit
                }
            },
            { upsert: true }
        );
    }
    
    async createPatch(memberTalkgroups) {
        const patchId = `patch_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
        
        const patch = new this.TalkgroupPatch({
            patch_id: patchId,
            members: memberTalkgroups.map(tg => ({
                talkgroup_id: tg
            })),
            events: [{
                timestamp: new Date(),
                event_type: 'created',
                details: { member_count: memberTalkgroups.length }
            }]
        });
        
        await patch.save();
        
        // Update state for all member talkgroups
        await this.TalkgroupState.updateMany(
            { talkgroup_id: { $in: memberTalkgroups } },
            { $set: { current_patch: patchId } }
        );
        
        // Update cache
        this.patchCache.set(patchId, patch);
        
        return patch;
    }
    
    async searchTalkgroups(query) {
        const filter = {};
        
        if (query.tag) filter.tag = query.tag;
        if (query.group) filter.group = query.group;
        if (query.text) {
            filter.$text = { $search: query.text };
        }
        
        if (query.active) {
            // Join with state collection to filter active talkgroups
            const activeTalkgroups = await this.TalkgroupState.find(
                { active: true },
                { talkgroup_id: 1 }
            );
            filter.talkgroup_id = {
                $in: activeTalkgroups.map(t => t.talkgroup_id)
            };
        }
        
        return this.Talkgroup.find(filter)
            .sort(query.sort || { alpha_tag: 1 })
            .skip(query.skip || 0)
            .limit(query.limit || 50);
    }
    
    async getTalkgroupStatistics(timeframe = '24h') {
        const cutoff = new Date();
        switch (timeframe) {
            case '24h':
                cutoff.setHours(cutoff.getHours() - 24);
                break;
            case '7d':
                cutoff.setDate(cutoff.getDate() - 7);
                break;
            case '30d':
                cutoff.setDate(cutoff.getDate() - 30);
                break;
        }
        
        // Aggregate activity statistics
        const stats = await this.TalkgroupActivity.aggregate([
            {
                $match: {
                    timestamp: { $gte: cutoff }
                }
            },
            {
                $group: {
                    _id: '$talkgroup_id',
                    total_calls: { $sum: '$call_count' },
                    total_time: { $sum: '$total_time' },
                    emergency_calls: { $sum: '$emergency_count' },
                    unique_units: { $addToSet: '$unique_units' }
                }
            },
            {
                $sort: { total_calls: -1 }
            }
        ]);
        
        // Get current active counts
        const activeCount = await this.TalkgroupState.countDocuments({ active: true });
        const emergencyCount = await this.TalkgroupState.countDocuments({
            active: true,
            'daily_stats.emergency_calls': { $gt: 0 }
        });
        
        return {
            current: {
                active_talkgroups: activeCount,
                emergency_talkgroups: emergencyCount
            },
            timeframe_stats: stats
        };
    }
}

module.exports = TalkgroupManager;