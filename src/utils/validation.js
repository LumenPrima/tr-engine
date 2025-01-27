const { 
    systemMessageSchema,
    ratesMessageSchema,
    recorderMessageSchema,
    unitMessageSchema
} = require('../models/validation/messageSchemas');

class MessageValidator {
    constructor() {
        // Map message types to their corresponding schemas
        this.schemaMap = new Map([
            ['systems', systemMessageSchema],
            ['rates', ratesMessageSchema],
            ['recorder', recorderMessageSchema],
            ['unit', unitMessageSchema],
            ['audio', audioMessageSchema],
            ['call_start', callStartSchema],
            ['call_end', callEndSchema],
            ['calls_active', callsActiveSchema]
        ]);

        // Cache for validation results
        this.validationCache = new Map();
        this.maxCacheSize = 1000;
        
        // Performance metrics
        this.validationTimes = [];
        this.maxTimingSamples = 100;
    }

    /**
     * Validate a message against its schema
     * @param {string} messageType - The type of message
     * @param {Object} message - The message to validate
     * @returns {Object} - { isValid: boolean, errors: Array }
     */
    validateMessage(messageType, message) {
        // Skip validation in production unless explicitly enabled
        if (process.env.NODE_ENV === 'production' && !process.env.ENABLE_VALIDATION) {
            return { isValid: true, errors: [] };
        }

        const schema = this.schemaMap.get(messageType);
        if (!schema) {
            return {
                isValid: false,
                errors: [`No validation schema found for message type: ${messageType}`]
            };
        }

        // Generate cache key from message structure
        const cacheKey = this.getCacheKey(messageType, message);
        
        // Check cache first
        if (this.validationCache.has(cacheKey)) {
            return this.validationCache.get(cacheKey);
        }

        // Measure validation time
        const startTime = process.hrtime();
        
        const validation = schema.validate(message, { abortEarly: false });
        const result = validation.error ? {
            isValid: false,
            errors: validation.error.details.map(detail => detail.message)
        } : {
            isValid: true,
            errors: []
        };

        // Update performance metrics
        const [seconds, nanoseconds] = process.hrtime(startTime);
        const milliseconds = seconds * 1000 + nanoseconds / 1000000;
        this.updateMetrics(milliseconds);

        // Cache the result
        this.validationCache.set(cacheKey, result);
        
        // Maintain cache size limit
        if (this.validationCache.size > this.maxCacheSize) {
            const firstKey = this.validationCache.keys().next().value;
            this.validationCache.delete(firstKey);
        }

        return result;
    }

    getCacheKey(messageType, message) {
        // Create a structural hash of the message
        const structure = JSON.stringify(
            Object.keys(message).sort().map(key => {
                return typeof message[key];
            })
        );
        return `${messageType}:${structure}`;
    }

    updateMetrics(milliseconds) {
        this.validationTimes.push(milliseconds);
        if (this.validationTimes.length > this.maxTimingSamples) {
            this.validationTimes.shift();
        }
    }

    getPerformanceMetrics() {
        if (this.validationTimes.length === 0) return null;
        
        return {
            averageTime: this.validationTimes.reduce((a, b) => a + b) / this.validationTimes.length,
            maxTime: Math.max(...this.validationTimes),
            minTime: Math.min(...this.validationTimes),
            cacheSize: this.validationCache.size,
            cacheHitRate: this.validationTimes.length > 0 ? 
                (this.validationCache.size / this.validationTimes.length) : 0
        };
    }

    /**
     * Normalize message data
     * @param {Object} message - The message to normalize
     * @returns {Object} - Normalized message
     */
    normalizeMessage(message) {
        // Ensure timestamp is a Date object
        if (message.timestamp && !(message.timestamp instanceof Date)) {
            message.timestamp = new Date(message.timestamp);
        }

        // Ensure numeric fields are actually numbers
        if (message.unit?.unit) {
            message.unit.unit = Number(message.unit.unit);
        }
        if (message.unit?.talkgroup) {
            message.unit.talkgroup = Number(message.unit.talkgroup);
        }

        return message;
    }
}

module.exports = new MessageValidator();
