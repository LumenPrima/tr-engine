const express = require('express');
const logger = require('../../utils/logger');

// Security middleware
const helmet = require('helmet');
const cors = require('cors');
const rateLimit = require('express-rate-limit');

// Development rate limiter with extremely high limits
const devLimiter = rateLimit({
    windowMs: 60 * 1000, // 1 minute
    max: 100000, // Effectively unlimited
    message: {
        status: 'error',
        message: 'Rate limit exceeded',
        timestamp: new Date().toISOString()
    }
});

// Request logging middleware
const requestLogger = (req, res, next) => {
    const start = Date.now();
    res.on('finish', () => {
        const duration = Date.now() - start;
        logger.debug(`${req.method} ${req.originalUrl} ${res.statusCode} ${duration}ms`);
    });
    next();
};

// Error handling middleware
const errorHandler = (err, req, res, next) => {
    logger.error('API Error:', err);
    res.status(err.status || 500).json({
        status: 'error',
        message: process.env.NODE_ENV === 'development' ? err.message : 'Internal server error',
        timestamp: new Date().toISOString()
    });
};

// Not found middleware
const notFoundHandler = (req, res) => {
    res.status(404).json({
        status: 'error',
        message: 'Resource not found',
        timestamp: new Date().toISOString()
    });
};

// Setup middleware for Express app
const setupMiddleware = (app) => {
    // Security middleware
    app.use(cors()); // Enable CORS first
    app.use(helmet({
        contentSecurityPolicy: false,
        crossOriginEmbedderPolicy: false
    })); // Less restrictive Helmet config for development

    // Request logging
    app.use(requestLogger);

    // Apply extremely high rate limit for development
    if (process.env.NODE_ENV === 'production') {
        logger.warn('Running in production without proper rate limiting!');
    }
    app.use('/api/v1', devLimiter);
};

// Setup error handling middleware (should be called after routes are mounted)
const setupErrorHandling = (app) => {
    // Error handler for /api/v1 routes
    app.use('/api/v1', (err, req, res, next) => {
        if (err) {
            errorHandler(err, req, res, next);
        } else {
            next();
        }
    });

    // 404 handler for unmatched /api/v1 routes
    app.use('/api/v1/*', (req, res) => {
        notFoundHandler(req, res);
    });
};

module.exports = {
    setupMiddleware,
    setupErrorHandling,
    requestLogger,
    errorHandler,
    notFoundHandler
};
