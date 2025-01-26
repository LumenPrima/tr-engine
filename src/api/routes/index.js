const express = require('express');
const router = express.Router();

// Test endpoints
router.get('/hello', (req, res) => {
    res.json({
        status: 'success',
        message: 'Hello World',
        timestamp: new Date().toISOString()
    });
});

// Active calls
router.get('/active/calls', (req, res) => {
    const activeCallManager = require('../../services/state/ActiveCallManager');
    activeCallManager.getActiveCalls({})
        .then(calls => {
            res.json({
                status: 'success',
                timestamp: new Date().toISOString(),
                data: {
                    count: calls.length,
                    calls: calls
                }
            });
        })
        .catch(err => {
            res.status(500).json({
                status: 'error',
                message: 'Failed to retrieve active calls',
                timestamp: new Date().toISOString()
            });
        });
});

// System status
router.get('/systems/status', (req, res) => {
    const systemManager = require('../../services/state/SystemManager');
    systemManager.getActiveSystems()
        .then(systems => {
            res.json({
                status: 'success',
                timestamp: new Date().toISOString(),
                data: {
                    count: systems.length,
                    systems: systems
                }
            });
        })
        .catch(err => {
            res.status(500).json({
                status: 'error',
                message: 'Failed to retrieve system status',
                timestamp: new Date().toISOString()
            });
        });
});

// Active units
router.get('/active/units', (req, res) => {
    const unitManager = require('../../services/state/UnitManager');
    const options = {
        timeWindow: parseInt(req.query.window) * 60 * 1000 || 5 * 60 * 1000 // Default 5 minutes
    };
    
    unitManager.getActiveUnits(options)
        .then(units => {
            res.json({
                status: 'success',
                timestamp: new Date().toISOString(),
                data: {
                    count: units.length,
                    window: options.timeWindow / 1000,
                    units: units
                }
            });
        })
        .catch(err => {
            res.status(500).json({
                status: 'error',
                message: 'Failed to retrieve active units',
                timestamp: new Date().toISOString()
            });
        });
});

// Historical calls
router.get('/history/calls', (req, res) => {
    const { CallStartMessage } = require('../../models/raw/MessageCollections');
    const options = {
        limit: parseInt(req.query.limit) || 100,
        offset: parseInt(req.query.offset) || 0,
        startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000),
        endTime: req.query.end ? new Date(req.query.end) : new Date()
    };

    const filter = {
        timestamp: { $gte: options.startTime, $lte: options.endTime }
    };

    Promise.all([
        CallStartMessage.countDocuments(filter),
        CallStartMessage.find(filter)
            .limit(options.limit)
            .skip(options.offset)
            .sort({ timestamp: -1 })
            .exec()
    ])
    .then(([totalCount, calls]) => {
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
                count: calls.length,
                calls: calls.map(call => ({
                    id: call.payload.call.id,
                    timestamp: call.timestamp,
                    sys_name: call.payload.call.sys_name,
                    talkgroup: call.payload.call.talkgroup,
                    talkgroup_tag: call.payload.call.talkgroup_tag,
                    units: [call.payload.call.unit].filter(Boolean),
                    emergency: call.payload.call.emergency || false,
                    duration: call.payload.call.length,
                    audio_type: call.payload.call.audio_type
                }))
            }
        });
    })
    .catch(err => {
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve historical calls',
            timestamp: new Date().toISOString()
        });
    });
});

// Historical unit activity
router.get('/history/unit/:id', (req, res) => {
    const unitManager = require('../../services/state/UnitManager');
    const unitId = parseInt(req.params.id);
    const options = {
        limit: parseInt(req.query.limit) || 100,
        startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000),
        endTime: req.query.end ? new Date(req.query.end) : new Date()
    };

    unitManager.UnitActivity.find({
        unit: unitId,
        timestamp: { 
            $gte: options.startTime, 
            $lte: options.endTime 
        }
    })
    .sort({ timestamp: -1 })
    .limit(options.limit)
    .then(history => {
            if (history.length === 0) {
                res.status(404).json({
                    status: 'error',
                    message: `No activity found for unit ${unitId}`,
                    timestamp: new Date().toISOString()
                });
                return;
            }
            res.json({
                status: 'success',
                timestamp: new Date().toISOString(),
                data: {
                    unit: unitId,
                    time_range: {
                        start: options.startTime,
                        end: options.endTime
                    },
                    count: history.length,
                    history: history
                }
            });
        })
        .catch(err => {
            res.status(500).json({
                status: 'error',
                message: 'Failed to retrieve unit history',
                timestamp: new Date().toISOString()
            });
        });
});

// Talkgroup activity
router.get('/talkgroup/:id', (req, res) => {
    const { CallStartMessage, UnitJoinMessage } = require('../../models/raw/MessageCollections');
    const talkgroupId = parseInt(req.params.id);
    const options = {
        limit: parseInt(req.query.limit) || 100,
        startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000),
        endTime: req.query.end ? new Date(req.query.end) : new Date()
    };

    Promise.all([
        CallStartMessage.find({
            'payload.call.talkgroup': talkgroupId,
            'timestamp': { $gte: options.startTime, $lte: options.endTime }
        })
        .limit(options.limit)
        .sort({ timestamp: -1 })
        .exec(),
        UnitJoinMessage.find({
            'payload.join.talkgroup': talkgroupId,
            'timestamp': { $gte: options.startTime, $lte: options.endTime }
        })
        .limit(options.limit)
        .sort({ timestamp: -1 })
        .exec()
    ])
    .then(([calls, affiliations]) => {
        const events = [
            ...calls.map(call => ({
                id: call.payload.call.id,
                timestamp: call.timestamp,
                type: 'call',
                details: {
                    units: [call.payload.call.unit].filter(Boolean),
                    emergency: call.payload.call.emergency || false,
                    duration: call.payload.call.length,
                    audio_type: call.payload.call.audio_type
                }
            })),
            ...affiliations.map(aff => ({
                id: `${aff.timestamp}_${aff.payload.join.unit}`,
                timestamp: aff.timestamp,
                type: 'affiliation',
                details: {
                    unit: aff.payload.join.unit,
                    unit_tag: aff.payload.join.unit_alpha_tag,
                    talkgroup: aff.payload.join.talkgroup
                }
            }))
        ].sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                talkgroup: {
                    id: talkgroupId,
                    tag: calls[0]?.payload?.call?.talkgroup_tag
                },
                time_range: {
                    start: options.startTime,
                    end: options.endTime
                },
                count: events.length,
                events: events
            }
        });
    })
    .catch(err => {
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve talkgroup activity',
            timestamp: new Date().toISOString()
        });
    });
});

module.exports = router;
