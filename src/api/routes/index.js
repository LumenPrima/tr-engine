const express = require('express');
const router = express.Router();
const fs = require('fs');
const path = require('path');

// Test endpoint (keep this as a health check)
router.get('/hello', (req, res) => {
    res.json({
        status: 'success',
        message: 'Hello World',
        timestamp: new Date().toISOString()
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
