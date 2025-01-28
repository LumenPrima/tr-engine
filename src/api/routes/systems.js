const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const systemManager = require('../../services/state/SystemManager');

// GET /
// Get current status of all systems
router.get('/', async (req, res) => {
    try {
        const activeSystems = await systemManager.getActiveSystems();
        
        // Transform system data for response
        const systemStatus = activeSystems.map(system => ({
            name: system.name || system.sys_name,
            sys_name: system.sys_name,
            sys_num: system.sys_num,
            type: system.type,
            status: {
                connected: system.status?.connected || true,
                last_seen: system.status?.last_seen || new Date(),
                current_control_channel: system.current_control_channel,
                current_decoderate: system.current_decoderate
            },
            config: system.config || {}
        }));

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            count: systemStatus.length,
            systems: systemStatus
        });
    } catch (err) {
        logger.error('Error getting system status:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve system status'
        });
    }
});

// GET /performance
// Get system performance statistics
router.get('/performance', async (req, res) => {
    try {
        const activeSystems = await systemManager.getActiveSystems();
        
        // Calculate system-wide statistics
        const stats = {
            total_systems: activeSystems.length,
            active_systems: activeSystems.filter(sys => sys.status?.connected !== false).length,
            system_stats: activeSystems.map(system => ({
                name: system.sys_name,
                sys_name: system.sys_name,
                sys_num: system.sys_num,
                type: system.type,
                control_channel: system.current_control_channel,
                decoderate: system.current_decoderate || 0,
                decoderate_interval: system.decoderate_interval || 3,
                recent_rates: (system.recent_rates || []).slice(-10), // Last 10 readings
            }))
        };

        // Add aggregate statistics
        stats.aggregate = {
            average_decoderate: stats.system_stats.reduce((sum, sys) => 
                sum + (sys.decoderate || 0), 0) / stats.total_systems
        };

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            stats: stats
        });
    } catch (err) {
        logger.error('Error getting system stats:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve system statistics'
        });
    }
});

// GET /:sys_name
// Get detailed status for a specific system
router.get('/:sys_name', async (req, res) => {
    try {
        const systemState = await systemManager.getSystemState(req.params.sys_name);
        
        if (!systemState) {
            return res.status(404).json({
                status: 'error',
                message: 'System not found'
            });
        }

        // Transform system data for detailed response
        const detailedStatus = {
            sys_name: systemState.sys_name,
            sys_num: systemState.sys_num,
            type: systemState.type,
            sysid: systemState.sysid,
            wacn: systemState.wacn,
            nac: systemState.nac,
            rfss: systemState.rfss,
            site_id: systemState.site_id,
            status: {
                connected: systemState.status?.connected || true,
                last_seen: systemState.status?.last_seen || new Date(),
                last_config_update: systemState.status?.last_config_update,
                last_rate_update: systemState.status?.last_rate_update
            },
            performance: {
                current_control_channel: systemState.current_control_channel || 0,
                current_decoderate: systemState.current_decoderate || 0,
                decoderate_interval: systemState.decoderate_interval || 3,
                recent_rates: systemState.recent_rates || []
            },
            config: systemState.config || {}
        };

        res.json({
            status: 'success',
            timestamp: new Date().toISOString(),
            system: detailedStatus
        });
    } catch (err) {
        logger.error('Error getting detailed system status:', err);
        res.status(500).json({
            status: 'error',
            message: 'Failed to retrieve system details'
        });
    }
});

module.exports = router;
