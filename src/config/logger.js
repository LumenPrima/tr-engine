const winston = require('winston');
const path = require('path');
require('dotenv').config();

// Define log levels
const levels = {
    error: 0,
    warn: 1,
    info: 2,
    http: 3,
    debug: 4,
};

// Define log colors
const colors = {
    error: 'red',
    warn: 'yellow',
    info: 'green',
    http: 'magenta',
    debug: 'blue',
};

winston.format.printf(
    (info) => `${info.timestamp} [${info.level}] cLogger: ${info.message}`
)

// Tell winston about the colors
winston.addColors(colors);

// Configure log format
const logFormat = winston.format.combine(
    winston.format.timestamp({ format: 'YYYY-MM-DD HH:mm:ss' }),
    winston.format.colorize({ all: true }),
    winston.format.printf(
        (info) => `${info.timestamp} [${info.level}] : ${info.message}`
    )
);

// Create logger
const logger = winston.createLogger({
    level: process.env.LOG_LEVEL || 'info',
    levels,
    format: logFormat,
    transports: [
        // Console transport
        new winston.transports.Console(),
        
        // File transport for errors
        new winston.transports.File({
            filename: path.join(__dirname, '../../logs/error.log'),
            level: 'error',
            handleExceptions: true,
        }),
        
        // File transport for combined logs
        new winston.transports.File({
            filename: path.join(__dirname, '../../logs/combined.log'),
            handleExceptions: true,
        }),
    ],
    exitOnError: false,
});

// Create log directory if it doesn't exist
const fs = require('fs');
const logDir = path.join(__dirname, '../../logs');
if (!fs.existsSync(logDir)) {
    fs.mkdirSync(logDir);
}

module.exports = logger;
