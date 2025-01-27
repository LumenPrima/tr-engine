const express = require('express');
const logger = require('../../utils/logger');

// Security middleware
const helmet = require('helmet');
const cors = require('cors');
const rateLimit = require('express-rate-limit');

// Create rate limiter
const limiter = rateLimit({
    windowMs: 15 * 60 * 1000, // 15 minutes
    max: 100, // Limit each IP to 100 requests per windowMs
    message: {
        status: 'error',
        message: 'Too many requests, please try again later',
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

    // Rate limiting - only apply to /api/v1 routes
    app.use('/api/v1', limiter);
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
