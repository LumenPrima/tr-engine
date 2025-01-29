const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const timestamps = require('../../utils/timestamps');
const activeCallManager = require('../../services/state/ActiveCallManager');
const mongoose = require('mongoose');

// GET / - Get historical calls
router.get('/', async (req, res) => {
    try {
        const options = {
            limit: parseInt(req.query.limit) || 100,
            offset: parseInt(req.query.offset) || 0,
            startTime: req.query.start || timestamps.toISO(timestamps.getCurrentTimeUnix() - 24 * 60 * 60), // 24 hours ago
            endTime: req.query.end || timestamps.toISO(timestamps.getCurrentTimeUnix())
        };

        const filter = {
            timestamp: {
                $gte: timestamps.toUnix(options.startTime),
                $lte: timestamps.toUnix(options.endTime)
            }
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
            call_id: call.id,
            timestamp: timestamps.toISO(call.timestamp),
            start_time: timestamps.toISO(call.timestamp), // Consistent ISO format
            sys_name: call.sys_name,
            talkgroup: call.talkgroup,
            talkgroup_tag: call.talkgroup_alpha_tag,
            units: call.unit ? [call.unit] : [],
            emergency: call.emergency || false,
            duration: call.length,
            audio_type: call.audio_type,
            freq: call.freq,
            phase2: call.phase2_tdma || false,
            encrypted: call.encrypted || false
        }));

        res.json({
            status: 'success',
            timestamp: timestamps.getCurrentTimeISO(),
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
            call_id: call.id,
            sys_name: call.sys_name,
            talkgroup: call.talkgroup,
            talkgroup_tag: call.talkgroup_alpha_tag,
            emergency: call.emergency || false,
            units: call.unit ? [call.unit] : [],
            start_time: call.start_time,
            active: true
        }));

        res.json({
            status: 'success',
            timestamp: timestamps.getCurrentTimeISO(),
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
            startTime: req.query.start || timestamps.toISO(timestamps.getCurrentTimeUnix() - 24 * 60 * 60), // 24 hours ago
            endTime: req.query.end || timestamps.toISO(timestamps.getCurrentTimeUnix())
        };

        const db = mongoose.connection.db;

        // Query calls and joins concurrently
        const [calls, joins] = await Promise.all([
            db.collection('call_start').find({
                talkgroup: talkgroupId,
                timestamp: {
                    $gte: timestamps.toUnix(options.startTime),
                    $lte: timestamps.toUnix(options.endTime)
                }
            })
            .limit(options.limit)
            .sort({ timestamp: -1 })
            .toArray(),
            db.collection('join').find({
                talkgroup: talkgroupId,
                timestamp: {
                    $gte: timestamps.toUnix(options.startTime),
                    $lte: timestamps.toUnix(options.endTime)
                }
            })
            .limit(options.limit)
            .sort({ timestamp: -1 })
            .toArray()
        ]);

        // Transform calls to standardized format
        const formattedCalls = calls.map(call => ({
            call_id: call.id,
            timestamp: timestamps.toISO(call.timestamp),
            activity_type: 'call',
            units: call.unit ? [call.unit] : [],
            emergency: call.emergency || false,
            duration: call.length,
            audio_type: call.audio_type,
            freq: call.freq,
            encrypted: call.encrypted || false
        }));

        // Transform joins to standardized format
        const formattedJoins = joins.map(join => ({
            timestamp: timestamps.toISO(join.timestamp),
            activity_type: 'join',
            unit: join.unit,
            unit_alpha_tag: join.unit_alpha_tag,
            talkgroup: join.talkgroup,
            talkgroup_tag: join.talkgroup_alpha_tag
        }));

        // Combine and sort all events by timestamp
        const allEvents = [...formattedCalls, ...formattedJoins]
            .sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

        res.json({
            status: 'success',
            timestamp: timestamps.getCurrentTimeISO(),
            data: {
                talkgroup: {
                    talkgroup: talkgroupId,
                    talkgroup_tag: calls[0]?.talkgroup_alpha_tag
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

// GET /events - Get currently active events
router.get('/events', async (req, res) => {
    try {
        const [activeCalls, emergencyCalls] = await Promise.all([
            activeCallManager.getActiveCalls({}),
            activeCallManager.getActiveCalls({ emergency: true })
        ]);

        const formattedCalls = activeCalls.map(call => ({
            call_id: call.id,
            sys_name: call.sys_name,
            talkgroup: call.talkgroup,
            talkgroup_tag: call.talkgroup_alpha_tag,
            emergency: call.emergency || false,
            units: call.unit ? [call.unit] : [],
            start_time: call.start_time,
            active: true
        }));

        const formattedEmergencies = emergencyCalls.map(call => ({
            call_id: call.id,
            sys_name: call.sys_name,
            talkgroup: call.talkgroup,
            talkgroup_tag: call.talkgroup_alpha_tag,
            units: call.unit ? [call.unit] : [],
            start_time: call.start_time,
            type: 'emergency_call'
        }));

        const totalEvents = formattedCalls.length + formattedEmergencies.length;
        
        res.json({
            status: 'success',
            timestamp: timestamps.getCurrentTimeISO(),
            data: {
                count: totalEvents,
                events: {
                    calls: formattedCalls,
                    emergencies: formattedEmergencies,
                    affiliations: [],
                    system_events: []
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
