const express = require('express');
const router = express.Router();
const logger = require('../../utils/logger');
const systemManager = require('../../services/state/SystemManager');

// GET /status
// Get current status of all systems
router.get('/status', async (req, res) => {
    try {
        const activeSystems = await systemManager.getActiveSystems();
        
        // Transform system data for response
        const systemStatus = activeSystems.map(system => ({
            sys_name: system.sys_name,
            sys_num: system.sys_num,
            type: system.type,
            status: {
                connected: system.status.connected,
                last_seen: system.status.last_seen,
                current_control_channel: system.current_control_channel,
                current_decoderate: system.current_decoderate
            },
            config: {
                system_type: system.config.system_type,
                control_channels: system.config.control_channels,
                voice_channels: system.config.voice_channels
            },
            active_recorders: system.active_recorders
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

// GET /stats
// Get system performance statistics
router.get('/stats', async (req, res) => {
    try {
        const activeSystems = await systemManager.getActiveSystems();
        
        // Calculate system-wide statistics
        const stats = {
            total_systems: activeSystems.length,
            active_systems: activeSystems.filter(sys => sys.status.connected).length,
            system_stats: activeSystems.map(system => ({
                sys_name: system.sys_name,
                performance: {
                    decoderate: system.current_decoderate,
                    decoderate_interval: system.decoderate_interval,
                    recent_rates: system.recent_rates.slice(-10) // Last 10 readings
                },
                recorders: {
                    total: system.active_recorders.length,
                    active: system.active_recorders.filter(rec => rec.state === 'RECORDING').length
                }
            }))
        };

        // Add aggregate statistics
        stats.aggregate = {
            average_decoderate: stats.system_stats.reduce((sum, sys) => 
                sum + (sys.performance.decoderate || 0), 0) / stats.total_systems,
            total_recorders: stats.system_stats.reduce((sum, sys) => 
                sum + sys.recorders.total, 0),
            active_recorders: stats.system_stats.reduce((sum, sys) => 
                sum + sys.recorders.active, 0)
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

// GET /:sys_name/status
// Get detailed status for a specific system
router.get('/:sys_name/status', async (req, res) => {
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
                connected: systemState.status.connected,
                last_seen: systemState.status.last_seen,
                last_config_update: systemState.status.last_config_update,
                last_rate_update: systemState.status.last_rate_update
            },
            performance: {
                current_control_channel: systemState.current_control_channel,
                current_decoderate: systemState.current_decoderate,
                decoderate_interval: systemState.decoderate_interval,
                recent_rates: systemState.recent_rates
            },
            config: systemState.config,
            active_recorders: systemState.active_recorders
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
