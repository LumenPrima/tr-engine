const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const activeCallManager = require('../../services/state/ActiveCallManager');
const mongoose = require('mongoose');

// GET / - Get historical calls
router.get('/', async (req, res) => {
    try {
        // Handle historical calls
        const options = {
            limit: parseInt(req.query.limit) || 100,
            offset: parseInt(req.query.offset) || 0,
            startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000),
            endTime: req.query.end ? new Date(req.query.end) : new Date()
        };

        const filter = {
            timestamp: { $gte: options.startTime, $lte: options.endTime }
        };

        if (req.query.sys_name) {
            filter.sys_name = req.query.sys_name;
        }
        if (req.query.talkgroup) {
            filter.talkgroup = parseInt(req.query.talkgroup);
        }
        if (req.query.unit) {
            filter.unit = parseInt(req.query.unit);
        }
        if (req.query.emergency === 'true') {
            filter.emergency = true;
        }

        const collection = mongoose.connection.db.collection('call_start');
        
        const [totalCount, calls] = await Promise.all([
            collection.countDocuments(filter),
            collection.find(filter)
                .limit(options.limit)
                .skip(options.offset)
                .sort({ timestamp: -1 })
                .toArray()
        ]);

        const formattedCalls = calls.map(call => ({
            call_id: call.call_id,
            timestamp: call.timestamp,
            sys_name: call.sys_name,
            talkgroup: call.talkgroup,
            talkgroup_tag: call.talkgroup_tag,
            units: [call.unit].filter(Boolean),
            emergency: call.emergency || false,
            duration: call.call_length,
            audio_type: call.audio_type,
            details: {
                freq: call.freq,
                phase2: call.phase2_tdma || false,
                encrypted: call.encrypted || false
            }
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                pagination: {
                    total: totalCount,
                    limit: options.limit,
                    offset: options.offset,
                    has_more: totalCount > (options.offset + options.limit)
                },
                time_range: {
                    start: options.startTime,
                    end: options.endTime
                },
                count: formattedCalls.length,
                calls: formattedCalls
            }
        });
    } catch (err) {
        logger.error('Error getting historical calls:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve historical calls'
        });
    }
});

// GET /active - Get currently active calls
router.get('/active', async (req, res) => {
    try {
        const filter = {};
        
        if (req.query.sys_name) {
            filter.sys_name = req.query.sys_name;
        }
        if (req.query.talkgroup) {
            filter.talkgroup = parseInt(req.query.talkgroup);
        }
        if (req.query.emergency === 'true') {
            filter.emergency = true;
        }

        const activeCalls = await activeCallManager.getActiveCalls(filter);
        
        const formattedCalls = activeCalls.map(call => ({
            call_id: call.call_id,
            sys_name: call.sys_name,
            talkgroup: call.talkgroup,
            talkgroup_tag: call.talkgroup_tag,
            emergency: call.emergency || false,
            units: call.units || [],
            start_time: call.start_time,
            active: true
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                count: formattedCalls.length,
                calls: formattedCalls
            }
        });
    } catch (err) {
        logger.error('Error getting active calls:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve active calls'
        });
    }
});

// GET /talkgroup/:talkgroup_id - Get historical activity for a specific talkgroup
router.get('/talkgroup/:talkgroup_id', async (req, res) => {
    try {
        const talkgroupId = parseInt(req.params.talkgroup_id);
        const options = {
            limit: parseInt(req.query.limit) || 100,
            startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000),
            endTime: req.query.end ? new Date(req.query.end) : new Date()
        };

        const db = mongoose.connection.db;

        // Query calls and joins concurrently
        const [calls, joins] = await Promise.all([
            db.collection('call_start').find({
                talkgroup: talkgroupId,
                timestamp: { $gte: options.startTime, $lte: options.endTime }
            })
            .limit(options.limit)
            .sort({ timestamp: -1 })
            .toArray(),
            db.collection('join').find({
                talkgroup: talkgroupId,
                timestamp: { $gte: options.startTime, $lte: options.endTime }
            })
            .limit(options.limit)
            .sort({ timestamp: -1 })
            .toArray()
        ]);

        // Transform calls to standardized format
        const formattedCalls = calls.map(call => ({
            call_id: call.call_id,
            timestamp: call.timestamp,
            activity_type: 'call',
            units: [call.unit].filter(Boolean),
            emergency: call.emergency || false,
            duration: call.call_length,
            audio_type: call.audio_type,
            freq: call.freq,
            encrypted: call.encrypted || false
        }));

        // Transform joins to standardized format
        const formattedJoins = joins.map(join => ({
            timestamp: join.timestamp,
            activity_type: 'join',
            unit: join.unit,
            unit_alpha_tag: join.unit_alpha_tag,
            talkgroup: join.talkgroup,
            talkgroup_tag: join.talkgroup_tag
        }));

        // Combine and sort all events by timestamp
        const allEvents = [...formattedCalls, ...formattedJoins]
            .sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                talkgroup: {
                    talkgroup: talkgroupId,
                    talkgroup_tag: calls[0]?.talkgroup_tag
                },
                time_range: {
                    start: options.startTime,
                    end: options.endTime
                },
                count: allEvents.length,
                events: allEvents
            }
        });
    } catch (err) {
        logger.error('Error getting talkgroup call history:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve talkgroup call history'
        });
    }
});

// GET /events - Get currently active events (calls, emergencies, affiliations, system events)
router.get('/events', async (req, res) => {
    try {
        // Get active calls and emergency calls
        const [activeCalls, emergencyCalls] = await Promise.all([
            activeCallManager.getActiveCalls({}),
            activeCallManager.getActiveCalls({ emergency: true })
        ]);

        // Transform calls to standardized format
        const formattedCalls = activeCalls.map(call => ({
            call_id: call.call_id,
            sys_name: call.sys_name,
            talkgroup: call.talkgroup,
            talkgroup_tag: call.talkgroup_tag,
            emergency: call.emergency || false,
            units: call.units || [],
            start_time: call.start_time,
            active: true
        }));

        // Transform emergency calls to standardized format
        const formattedEmergencies = emergencyCalls.map(call => ({
            call_id: call.call_id,
            sys_name: call.sys_name,
            talkgroup: call.talkgroup,
            talkgroup_tag: call.talkgroup_tag,
            units: call.units || [],
            start_time: call.start_time,
            type: 'emergency_call'
        }));

        const totalEvents = formattedCalls.length + formattedEmergencies.length;
        
        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                count: totalEvents,
                events: {
                    calls: formattedCalls,
                    emergencies: formattedEmergencies,
                    affiliations: [], // To be implemented
                    system_events: [] // To be implemented
                }
            }
        });
    } catch (err) {
        logger.error('Error getting active events:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve active events'
        });
    }
});

module.exports = router;
