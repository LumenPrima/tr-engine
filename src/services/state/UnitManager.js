const mongoose = require('mongoose');
const logger = require('../../utils/logger');
const stateEventEmitter = require('../events/emitter');

class UnitManager {
    constructor() {
        // In-memory caches for performance
        this.unitStates = new Map(); // wacn+unit -> state
        this.recentActivity = new Map(); // wacn+unit -> activity array
        this.unitsByTalkgroup = new Map(); // talkgroup -> Set of wacn+unit
        this.wacnBySystem = new Map(); // sys_name -> wacn mapping

        // Initialize after MongoDB connection
        mongoose.connection.on('connected', () => {
            // Listen for WACN updates from SystemManager
            stateEventEmitter.on('system.wacn', ({ sys_name, wacn }) => {
                if (sys_name && wacn) {
                    this.wacnBySystem.set(sys_name, wacn);
                    logger.debug(`Updated WACN mapping for system ${sys_name}: ${wacn}`);
                }
            });

            this.collection = mongoose.connection.db.collection('units');
            
            // Create indexes
            this.setupIndexes().catch(err => 
                logger.error('Error setting up unit indexes:', err)
            );

            // Load existing data into cache
            this.loadCacheFromDB().catch(err => 
                logger.error('Error loading unit cache from DB:', err)
            );
        });

        logger.info('UnitManager initialized');
    }

    async setupIndexes() {
        await this.collection.createIndex({ wacn: 1, unit: 1 }, { unique: true });
        await this.collection.createIndex({ 'status.last_seen': 1 });
        await this.collection.createIndex({ 'status.current_talkgroup': 1 });
        await this.collection.createIndex({ 'systems.sys_name': 1 });
    }

    async loadCacheFromDB() {
        logger.debug('Loading unit cache from database...');
        const units = await this.collection.find({}).toArray();
        
        units.forEach(unit => {
            const unitKey = this.getUnitKey(unit.wacn, unit.unit);
            this.unitStates.set(unitKey, unit);
            
            // Cache recent activity
            this.recentActivity.set(unitKey, unit.recent_activity || []);
            
            // Cache talkgroup mapping
            if (unit.status?.current_talkgroup) {
                if (!this.unitsByTalkgroup.has(unit.status.current_talkgroup)) {
                    this.unitsByTalkgroup.set(unit.status.current_talkgroup, new Set());
                }
                this.unitsByTalkgroup.get(unit.status.current_talkgroup).add(unitKey);
            }

            // Cache WACN mapping for each system
            if (unit.systems) {
                unit.systems.forEach(sys => {
                    if (sys.sys_name) {
                        this.wacnBySystem.set(sys.sys_name, unit.wacn);
                    }
                });
            }
        });
        
        logger.info(`Loaded ${units.length} units into cache`);
    }

    cleanup() {
        logger.debug('Cleaning up UnitManager...');
        this.unitStates.clear();
        this.recentActivity.clear();
        this.unitsByTalkgroup.clear();
        this.wacnBySystem.clear();
        logger.debug('Cleared in-memory caches');
    }
    
    // Generate a unique key for unit+wacn
    getUnitKey(wacn, unit) {
        return `${wacn}:${unit}`;
    }
    
    async processMessage(topic, message, messageId) {
        try {
            logger.debug('Processing message in UnitManager:', {
                topic,
                message: JSON.stringify(message),
                topicParts: topic.split('/')
            });

            if (!message || typeof message !== 'object') {
                logger.debug('Skipping invalid message - not an object');
                return;
            }

            const topicParts = topic.split('/');
            if (topicParts.length < 3) {
                logger.debug('Skipping message with invalid topic format:', topic);
                return;
            }

            const messageType = topicParts[2];
            
            // Handle system messages to update WACN mapping
            if (messageType === 'systems' && Array.isArray(message.systems)) {
                logger.debug('Processing systems message:', message.systems);
                message.systems.forEach(sys => {
                    if (sys && sys.wacn && sys.sys_name) {
                        this.wacnBySystem.set(sys.sys_name, sys.wacn);
                        logger.debug(`Set WACN mapping: ${sys.sys_name} -> ${sys.wacn}`);
                    }
                });
                return;
            }

            const activityType = topicParts[3];
            if (!activityType) {
                logger.debug('Skipping message without activity type:', topicParts);
                return;
            }
            
            // For non-system messages, validate required fields
            if (!message.unit || !message.sys_name) {
                logger.debug(`Skipping ${activityType} message without required unit data:`, {
                    unit: message.unit,
                    sys_name: message.sys_name,
                    message: JSON.stringify(message)
                });
                return;
            }

            // Get WACN for the system
            const wacn = this.wacnBySystem.get(message.sys_name);
            if (!wacn) {
                logger.warn(`No WACN found for system ${message.sys_name}, waiting for systems message`);
                return;
            }
            
            logger.debug(`Processing ${activityType} message for unit ${message.unit} on WACN ${wacn}`);
            
            logger.debug('Calling updateUnitState with:', {
                message: JSON.stringify(message),
                activityType,
                wacn
            });
            
            // Update the unit's current state
            await this.updateUnitState(message, activityType, wacn);
        } catch (err) {
            logger.error('Error processing message in UnitManager:', err);
            throw err;
        }
    }
    
    async updateUnitState(unitData, activityType, wacn) {
        try {
            if (!this.collection) {
                logger.warn('MongoDB collection not ready yet');
                return;
            }

            const unitKey = this.getUnitKey(wacn, unitData.unit);
            
            logger.debug(`Updating state for unit ${unitData.unit} (${activityType})`);
            
            // Get current state or create new one
            const currentState = this.unitStates.get(unitKey) || {
                wacn: wacn,
                unit: unitData.unit,
                systems: [],
                unit_alpha_tag: unitData.unit_alpha_tag,
                status: {
                    online: true,
                    last_seen: new Date(),
                    last_activity_type: activityType,
                    current_talkgroup: null,
                    current_talkgroup_tag: null
                },
                activity_summary: {
                    first_seen: new Date(),
                    total_calls: 0,
                    total_affiliations: 0,
                    last_call_time: null,
                    last_affiliation_time: null
                }
            };

            // Update systems array
            const systemIndex = currentState.systems.findIndex(s => s.sys_name === unitData.sys_name);
            if (systemIndex === -1) {
                currentState.systems.push({
                    sys_name: unitData.sys_name,
                    sys_num: unitData.sys_num,
                    last_seen: new Date()
                });
            } else {
                currentState.systems[systemIndex].last_seen = new Date();
            }
            
            // Create or update activity entry
            const callId = unitData.id?.split('_').slice(1).join('_');
            
            // Get or create activity list
            if (!this.recentActivity.has(unitKey)) {
                this.recentActivity.set(unitKey, []);
            }
            const activities = this.recentActivity.get(unitKey);

            // For call activities, consolidate system details
            if (callId && ['call_start', 'call_end'].includes(activityType)) {
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
                        talkgroup: unitData.talkgroup,
                        talkgroup_tag: unitData.talkgroup_tag,
                        start_time: unitData.start_time,
                        stop_time: unitData.stop_time,
                        length: unitData.length,
                        system_details: []
                    };
                    activities.unshift(activity);
                    if (activities.length > 50) activities.pop();
                }

                // Update or add system details
                const sysDetails = {
                    sys_name: unitData.sys_name,
                    sys_num: unitData.sys_num,
                    freq: unitData.freq,
                    error_count: unitData.error_count,
                    spike_count: unitData.spike_count,
                    freq_error: unitData.freq_error
                };

                const sysIndex = activity.system_details.findIndex(s => s.sys_name === unitData.sys_name);
                if (sysIndex >= 0) {
                    activity.system_details[sysIndex] = sysDetails;
                } else {
                    activity.system_details.push(sysDetails);
                }
            } else {
                // For non-call activities (like affiliations), keep single entry
                const newActivity = {
                    timestamp: new Date(),
                    activity_type: activityType,
                    talkgroup: unitData.talkgroup,
                    talkgroup_tag: unitData.talkgroup_tag,
                    sys_name: unitData.sys_name,
                    sys_num: unitData.sys_num,
                    details: unitData  // Store full message for comparison
                };

                // Only add if different from most recent
                const shouldLogActivity = activities.length === 0 || 
                    !this.areSimilarActivities(newActivity, activities[0]);

                if (shouldLogActivity) {
                    activities.unshift(newActivity);
                    if (activities.length > 50) activities.pop();
                }
            }
            
            // Update state based on activity type
            const newState = {
                ...currentState,
                unit_alpha_tag: unitData.unit_alpha_tag,
                status: {
                    ...currentState.status,
                    last_seen: new Date(),
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
                    newState.activity_summary.last_call_time = new Date();
                    break;
                    
                case 'join':
                    newState.status.current_talkgroup = unitData.talkgroup;
                    newState.status.current_talkgroup_tag = unitData.talkgroup_tag;
                    newState.activity_summary.total_affiliations++;
                    newState.activity_summary.last_affiliation_time = new Date();
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
            await this.collection.updateOne(
                { wacn: wacn, unit: unitData.unit },
                {
                    $set: {
                        ...newState,
                        recent_activity: activities
                    }
                },
                { upsert: true }
            );
            
            // For call events, only emit if this is the first system to report it
            if (callId) {
                const isFirstSystem = !activities.some(a => 
                    a !== activity && // Not the current activity
                    a.call_id === callId && // Same call
                    a.activity_type === activityType && // Same type (start/end)
                    new Date(a.timestamp).getTime() < new Date().getTime() // Earlier timestamp
                );

                if (isFirstSystem) {
                    stateEventEmitter.emitUnitActivity({
                        unit: unitData.unit,
                        unit_alpha_tag: unitData.unit_alpha_tag,
                        activity_type: activityType,
                        talkgroup: unitData.talkgroup,
                        talkgroup_tag: unitData.talkgroup_tag,
                        wacn: wacn,
                        call_id: callId,
                        systems: [unitData.sys_name],
                        ...newState
                    });
                }
            } else {
                // For non-call events, handle differently based on type
                if (activityType === 'location') {
                    // Always emit location events as they might have different coordinates
                    stateEventEmitter.emitUnitLocation({
                        unit: unitData.unit,
                        unit_alpha_tag: unitData.unit_alpha_tag,
                        wacn: wacn,
                        sys_name: unitData.sys_name,
                        ...unitData
                    });
                } else if (['on', 'off'].includes(activityType)) {
                    // For status events, only emit if status actually changed
                    const prevStatus = currentState.status?.online;
                    const newStatus = activityType === 'on';
                    if (prevStatus !== newStatus) {
                        stateEventEmitter.emitUnitStatus({
                            unit: unitData.unit,
                            unit_alpha_tag: unitData.unit_alpha_tag,
                            status: newStatus ? 'online' : 'offline',
                            wacn: wacn,
                            sys_name: unitData.sys_name,
                            ...unitData
                        });
                    }
                } else {
                    // For other events (like affiliations), emit normally
                    stateEventEmitter.emitUnitActivity({
                        unit: unitData.unit,
                        unit_alpha_tag: unitData.unit_alpha_tag,
                        activity_type: activityType,
                        talkgroup: unitData.talkgroup,
                        talkgroup_tag: unitData.talkgroup_tag,
                        wacn: wacn,
                        sys_name: unitData.sys_name,
                        ...newState
                    });
                }
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
                .sort((a, b) => b.status.last_seen - a.status.last_seen);

            if (states.length === 0) return null;

            // Combine data from all systems
            return {
                unit: unit,
                wacn: states[0].wacn,
                unit_alpha_tag: states[0].unit_alpha_tag,
                systems: states[0].systems,
                status: {
                    online: !states.some(s => s.status.last_activity_type === 'off'),
                    last_seen: new Date(Math.max(...states.map(s => s.status.last_seen.getTime()))),
                    last_activity_type: states[0].status.last_activity_type,
                    current_talkgroup: states[0].status.current_talkgroup,
                    current_talkgroup_tag: states[0].status.current_talkgroup_tag
                },
                activity_summary: {
                    first_seen: new Date(Math.min(...states.map(s => s.activity_summary.first_seen.getTime()))),
                    total_calls: states.reduce((sum, s) => sum + s.activity_summary.total_calls, 0),
                    total_affiliations: states.reduce((sum, s) => sum + s.activity_summary.total_affiliations, 0),
                    last_call_time: this.getLatestDate(states.map(s => s.activity_summary.last_call_time)),
                    last_affiliation_time: this.getLatestDate(states.map(s => s.activity_summary.last_affiliation_time))
                },
                recent_activity: this.aggregateRecentActivity(
                    states.flatMap(s => this.recentActivity.get(this.getUnitKey(s.wacn, s.unit)) || [])
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
                states.sort((a, b) => b.status.last_seen - a.status.last_seen);
                return {
                    unit: states[0].unit,
                    wacn: states[0].wacn,
                    unit_alpha_tag: states[0].unit_alpha_tag,
                    systems: states[0].systems,
                    status: {
                        online: !states.some(s => s.status.last_activity_type === 'off'),
                        last_seen: new Date(Math.max(...states.map(s => s.status.last_seen.getTime()))),
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
            const cutoff = Date.now() - (options.timeWindow || 5 * 60 * 1000);
            
            // Get all active states
            const activeStates = Array.from(this.unitStates.values())
                .filter(state => state.status.last_seen.getTime() >= cutoff)
                .sort((a, b) => b.status.last_seen - a.status.last_seen);
            
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
                states.sort((a, b) => b.status.last_seen - a.status.last_seen);
                return {
                    unit: states[0].unit,
                    wacn: states[0].wacn,
                    unit_alpha_tag: states[0].unit_alpha_tag,
                    systems: states[0].systems,
                    status: {
                        online: !states.some(s => s.status.last_activity_type === 'off'),
                        last_seen: new Date(Math.max(...states.map(s => s.status.last_seen.getTime()))),
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

        // Compare unit and talkgroup fields
        const compareFields = ['unit', 'unit_alpha_tag', 'talkgroup', 'talkgroup_tag'];
        return compareFields.every(field => a.details[field] === b.details[field]);
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
            this.wacnBySystem.clear();
            logger.debug('Cleared all unit data');
        } catch (err) {
            logger.error('Error clearing units:', err);
            throw err;
        }
    }

    // Helper method to get latest non-null date from array
    getLatestDate(dates) {
        const validDates = dates.filter(d => d);
        return validDates.length > 0 ? new Date(Math.max(...validDates.map(d => d.getTime()))) : null;
    }
}

// Export singleton instance
const unitManager = new UnitManager();
module.exports = unitManager;
