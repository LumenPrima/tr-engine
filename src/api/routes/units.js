const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const unitManager = require('../../services/state/UnitManager');

// GET /units
// Get all currently active units
router.get('/units', async (req, res) => {
    try {
        const options = {
            timeWindow: parseInt(req.query.window) * 60 * 1000 || 5 * 60 * 1000 // Default 5 minutes
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

// GET /:unit_id/status
// Get current status and recent activity for a specific unit
router.get('/:unit_id/status', async (req, res) => {
    try {
        const unitId = parseInt(req.params.unit_id);
        const sysName = req.query.sys_name;

        if (!sysName) {
            return res.status(400).json({
                status: 'error',
                message: 'sys_name query parameter is required'
            });
        }

        const unitState = await unitManager.getUnitState(sysName, unitId);
        
        if (!unitState) {
            return res.status(404).json({
                status: 'error',
                message: 'Unit not found'
            });
        }

        // Transform unit data for detailed response
        const detailedStatus = {
            unit: unitState.unit,
            sys_name: unitState.sys_name,
            unit_alpha_tag: unitState.unit_alpha_tag,
            status: unitState.status,
            activity_summary: unitState.activity_summary,
            recent_activity: unitState.recent_activity
        };

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            unit: detailedStatus
        });
    } catch (err) {
        logger.error('Error getting unit status:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve unit status'
        });
    }
});

// GET /unit/{unit_id}
// Get complete history for a specific unit
router.get('/unit/:unit_id', async (req, res) => {
    try {
        const unitId = parseInt(req.params.unit_id);
        const sysName = req.query.sys_name;

        if (!sysName) {
            return res.status(400).json({
                status: 'error',
                message: 'sys_name query parameter is required'
            });
        }

        const options = {
            limit: parseInt(req.query.limit) || 100,
            startTime: req.query.start ? new Date(req.query.start) : new Date(Date.now() - 24 * 60 * 60 * 1000), // Default to last 24 hours
            endTime: req.query.end ? new Date(req.query.end) : new Date(),
            activityTypes: req.query.types ? req.query.types.split(',') : ['calls', 'status', 'affiliations'] // Default to all types
        };

        const history = await unitManager.getUnitHistory(sysName, unitId, options);

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
                unit: {
                    id: unitId,
                    sys_name: sysName
                },
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
