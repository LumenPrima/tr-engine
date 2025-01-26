const express = require('express');
const app = express();

// Method 1: Direct route definition (works)
app.get('/direct', (req, res) => {
    res.json({
        status: 'success',
        message: 'Direct route works',
        timestamp: new Date().toISOString()
    });
});

// Method 2: Loading routes via index.js (current working method)
const indexRoutes = require('./api/routes');
app.use('/via-index', indexRoutes);

// Method 3: Attempting to load external route file directly
const testRoute = require('./api/routes/test-route');
app.use('/external', testRoute);

const port = 3001; // Different port to avoid conflicts
app.listen(port, '0.0.0.0', () => {
    console.log(`Test API server listening on 0.0.0.0:${port}`);
    console.log('Try these endpoints:');
    console.log('1. http://localhost:3001/direct');
    console.log('2. http://localhost:3001/via-index/hello');
    console.log('3. http://localhost:3001/external/test');
});
