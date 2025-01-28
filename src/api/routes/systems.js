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
            systems: activeSystems.map(system => ({
                sys_name: system.sys_name,
                current_decoderate: system.current_decoderate || 0,
                active_recorders: system.active_recorders || [],
                config: {
                    control_channels: system.config?.control_channels || []
                }
            })),
            aggregate: {
                decoderate: activeSystems.reduce((sum, sys) => 
                    sum + (sys.current_decoderate || 0), 0) / activeSystems.length,
                active_recorders: activeSystems.reduce((sum, sys) => 
                    sum + (sys.active_recorders?.length || 0), 0),
                total_recorders: activeSystems.reduce((sum, sys) => 
                    sum + (sys.config?.control_channels?.length || 0), 0)
            }
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
