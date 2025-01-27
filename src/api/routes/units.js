const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const unitManager = require('../../services/state/UnitManager');

// GET /active - Get all currently active units
router.get('/active', async (req, res) => {
    try {
        const window = parseInt(req.query.window);
        const options = {
            timeWindow: !isNaN(window) ? window * 60 * 1000 : 5 * 60 * 1000 // Default 5 minutes if window is invalid
        };

        const activeUnits = await unitManager.getActiveUnits(options);
        
        // Transform unit data for response
        const formattedUnits = activeUnits.map(unit => ({
            id: unit.unit,
            sys_name: unit.sys_name,
            unit_tag: unit.unit_alpha_tag,
            status: {
                online: unit.status.online,
                last_seen: unit.status.last_seen,
                current_talkgroup: unit.status.current_talkgroup,
                current_talkgroup_tag: unit.status.current_talkgroup_tag
            }
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                count: formattedUnits.length,
                window: options.timeWindow / 1000, // seconds
                units: formattedUnits
            }
        });
    } catch (err) {
        logger.error('Error getting active units:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve active units'
        });
    }
});

// GET / - Get all units
router.get('/', async (req, res) => {
    try {
        const units = await unitManager.UnitState.find().sort({ 'status.last_seen': -1 });
        
        // Transform unit data for response
        const formattedUnits = units.map(unit => ({
            id: unit.unit,
            sys_name: unit.sys_name,
            unit_tag: unit.unit_alpha_tag,
            status: {
                online: unit.status.online,
                last_seen: unit.status.last_seen,
                current_talkgroup: unit.status.current_talkgroup,
                current_talkgroup_tag: unit.status.current_talkgroup_tag
            }
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                count: formattedUnits.length,
                units: formattedUnits
            }
        });
    } catch (err) {
        logger.error('Error getting all units:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve units'
        });
    }
});

// GET /:unit_id - Get current status and recent activity for a specific unit
router.get('/:unit_id', async (req, res) => {
    try {
        const unitId = parseInt(req.params.unit_id);
        // Allow -1 as a valid unit ID, but reject other invalid values
        if (isNaN(unitId) || (unitId !== -1 && unitId < 0)) {
            return res.status(400).json({
                status: 'error',
                message: 'Invalid unit ID format'
            });
        }
        const unitState = await unitManager.getUnitState(unitId);

        if (!unitState) {
            return res.status(404).json({
                status: 'error',
                message: 'Unit not found'
            });
        }

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            unit: unitState
        });
    } catch (err) {
        logger.error('Error getting unit status:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve unit status'
        });
    }
});

// GET /:unit_id/history - Get complete history for a specific unit
router.get('/:unit_id/history', async (req, res) => {
    try {
        const unitId = parseInt(req.params.unit_id);
        // Allow -1 as a valid unit ID, but reject other invalid values
        if (isNaN(unitId) || (unitId !== -1 && unitId < 0)) {
            return res.status(400).json({
                status: 'error',
                message: 'Invalid unit ID format'
            });
        }
        const options = {
            limit: parseInt(req.query.limit) || 100,
            startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000), // Default to last 24 hours
            endTime: req.query.end ? new Date(req.query.end) : new Date(),
            activityTypes: req.query.types ? req.query.types.split(',') : ['calls', 'status', 'affiliations'] // Default to all types
        };

        const history = await unitManager.getUnitHistory(unitId, options);

        // Transform history entries to standardized format
        const formattedHistory = history.map(entry => ({
            id: entry.id || `${entry.timestamp}_${entry.type}`,
            timestamp: entry.timestamp,
            type: entry.type,
            details: {
                ...(entry.type === 'call' && {
                    talkgroup: entry.talkgroup,
                    talkgroup_tag: entry.talkgroup_tag,
                    emergency: entry.emergency || false,
                    audio_url: entry.audio_url
                }),
                ...(entry.type === 'status' && {
                    old_status: entry.old_status,
                    new_status: entry.new_status
                }),
                ...(entry.type === 'affiliation' && {
                    talkgroup: entry.talkgroup,
                    talkgroup_tag: entry.talkgroup_tag
                })
            }
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            data: {
                unit: unitId,
                time_range: {
                    start: options.startTime,
                    end: options.endTime
                },
                count: formattedHistory.length,
                history: formattedHistory
            }
        });
    } catch (err) {
        logger.error('Error getting unit history:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve unit history'
        });
    }
});

// GET /talkgroup/:talkgroup_id
// Get all units currently affiliated with a talkgroup
router.get('/talkgroup/:talkgroup_id', async (req, res) => {
    try {
        const talkgroupId = parseInt(req.params.talkgroup_id);
        if (isNaN(talkgroupId)) {
            return res.status(400).json({
                status: 'error',
                message: 'Invalid talkgroup ID format'
            });
        }
        const units = await unitManager.getUnitsInTalkgroup(talkgroupId);

        // Transform unit data for response
        const unitStatus = units.map(unit => ({
            unit: unit.unit,
            sys_name: unit.sys_name,
            unit_alpha_tag: unit.unit_alpha_tag,
            status: {
                online: unit.status.online,
                last_seen: unit.status.last_seen,
                last_activity_type: unit.status.last_activity_type
            }
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            talkgroup: talkgroupId,
            count: unitStatus.length,
            units: unitStatus
        });
    } catch (err) {
        logger.error('Error getting units in talkgroup:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve units in talkgroup'
        });
    }
});

module.exports = router;
