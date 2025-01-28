const express = require('express');
const router = express.Router();
const fs = require('fs');
const path = require('path');
const os = require('os');

// Config endpoint to expose necessary frontend configuration
router.get('/config', (req, res) => {
    const port = process.env.PORT || 3000;
    res.json({
        status: 'success',
        timestamp: new Date().toISOString(),
        config: {
            api: {
                port: port,
                base_url: `/api/v1`
            }
        }
    });
});

// Health check endpoint with system stats
router.get('/hello', (req, res) => {
    const totalMem = os.totalmem();
    const freeMem = os.freemem();
    const usedMem = totalMem - freeMem;

    res.json({
        status: 'success',
        timestamp: new Date().toISOString(),
        system: {
            hostname: os.hostname(),
            platform: os.platform(),
            arch: os.arch(),
            cpus: os.cpus().length,
            uptime_hours: Math.floor(os.uptime() / 3600),
            memory: {
                total_gb: Math.round(totalMem / 1024 / 1024 / 1024 * 100) / 100,
                used_gb: Math.round(usedMem / 1024 / 1024 / 1024 * 100) / 100,
                free_gb: Math.round(freeMem / 1024 / 1024 / 1024 * 100) / 100,
                usage_percent: Math.round((usedMem / totalMem) * 100)
            }
        },
        process: {
            pid: process.pid,
            node_version: process.version,
            uptime_hours: Math.floor(process.uptime() / 3600),
            memory_mb: Math.round(process.memoryUsage().heapUsed / 1024 / 1024 * 100) / 100
        }
    });
});

// Automatically load and mount all route files in this directory
fs.readdirSync(__dirname)
    .filter(file => 
        file !== 'index.js' && // Skip this file
        (file.endsWith('.js') || file.endsWith('.ts')) // Only load JS/TS files
    )
    .forEach(file => {
        const route = require(path.join(__dirname, file));
        // Mount each route file with a prefix based on its name
        const routePrefix = '/' + file.replace(/\.(js|ts)$/, '');
        router.use(routePrefix, route);
        console.log(`Loaded route file: ${file} at ${routePrefix}`);
    });

module.exports = router;
