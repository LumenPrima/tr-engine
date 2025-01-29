const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const unitManager = require('../../services/state/UnitManager');
const timestamps = require('../../utils/timestamps');

// GET /active - Get all currently active units
router.get('/active', async (req, res) => {
    try {
        const window = parseInt(req.query.window);
        const options = {
            limit: parseInt(req.query.limit) || 100,
            offset: parseInt(req.query.offset) || 0,
            timeWindow: !isNaN(window) ? window * 60 : 5 * 60 // Default 5 minutes if window is invalid, in seconds
        };

        const activeUnits = await unitManager.getActiveUnits(options);
        
        // Transform and paginate unit data
        const formattedUnits = activeUnits
            .sort((a, b) => b.status.last_seen - a.status.last_seen) // Sort by last seen
            .slice(options.offset, options.offset + options.limit)
            .map(unit => ({
                unit: unit.unit,
                sys_name: unit.sys_name,
                unit_alpha_tag: unit.unit_alpha_tag,
                status: {
                    online: unit.status.online,
                    last_seen: unit.status.last_seen,
                    current_talkgroup: unit.status.current_talkgroup,
                    current_talkgroup_tag: unit.status.current_talkgroup_tag
                }
            }));

        res.json({
            status: 'success',
            timestamp: timestamps.getCurrentTimeISO(),
            data: {
                pagination: {
                    total: activeUnits.length,
                    limit: options.limit,
                    offset: options.offset,
                    has_more: activeUnits.length > (options.offset + options.limit)
                },
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
        const options = {
            limit: parseInt(req.query.limit) || 100,
            offset: parseInt(req.query.offset) || 0,
            timeWindow: 24 * 60 * 60 // Last 24 hours in seconds
        };

        const units = await unitManager.getActiveUnits({ timeWindow: options.timeWindow });
        
        // Transform unit data for response
        const formattedUnits = units
            .sort((a, b) => b.status.last_seen - a.status.last_seen) // Sort by last seen
            .slice(options.offset, options.offset + options.limit)
            .map(unit => ({
                unit: unit.unit,
                sys_name: unit.sys_name,
                unit_alpha_tag: unit.unit_alpha_tag,
                status: {
                    online: unit.status.online,
                    last_seen: unit.status.last_seen,
                    current_talkgroup: unit.status.current_talkgroup,
                    current_talkgroup_tag: unit.status.current_talkgroup_tag
                }
            }));

        res.json({
            status: 'success',
            timestamp: timestamps.getCurrentTimeISO(),
            data: {
                pagination: {
                    total: units.length,
                    limit: options.limit,
                    offset: options.offset,
                    has_more: units.length > (options.offset + options.limit)
                },
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
            timestamp: timestamps.getCurrentTimeISO(),
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

        const unitState = await unitManager.getUnitState(unitId);
        if (!unitState) {
            return res.status(404).json({
                status: 'error',
                message: 'Unit not found'
            });
        }

        const options = {
            limit: parseInt(req.query.limit) || 100,
            offset: parseInt(req.query.offset) || 0
        };

        // Sort by timestamp and paginate
        const history = unitState.recent_activity
            .sort((a, b) => b.timestamp - a.timestamp)
            .slice(options.offset, options.offset + options.limit)
            .map(activity => ({
                timestamp: activity.timestamp,
                activity_type: activity.activity_type,
                talkgroup: activity.talkgroup,
                talkgroup_tag: activity.talkgroup_tag,
                details: activity.details
            }));

        res.json({
            status: 'success',
            timestamp: timestamps.getCurrentTimeISO(),
            data: {
                unit: unitId,
                pagination: {
                    total: unitState.recent_activity.length,
                    limit: options.limit,
                    offset: options.offset,
                    has_more: unitState.recent_activity.length > (options.offset + options.limit)
                },
                count: history.length,
                history: history
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
        const options = {
            limit: parseInt(req.query.limit) || 100,
            offset: parseInt(req.query.offset) || 0
        };

        const units = await unitManager.getUnitsInTalkgroup(talkgroupId);

        // Transform and paginate unit data
        const unitStatus = units
            .sort((a, b) => b.status.last_seen - a.status.last_seen) // Sort by last seen
            .slice(options.offset, options.offset + options.limit)
            .map(unit => ({
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
            timestamp: timestamps.getCurrentTimeISO(),
            data: {
                talkgroup: talkgroupId,
                pagination: {
                    total: units.length,
                    limit: options.limit,
                    offset: options.offset,
                    has_more: units.length > (options.offset + options.limit)
                },
                count: unitStatus.length,
                units: unitStatus
            }
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
