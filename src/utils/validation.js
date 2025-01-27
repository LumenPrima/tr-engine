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
            ['unit', unitMessageSchema]
        ]);
    }

    /**
     * Validate a message against its schema
     * @param {string} messageType - The type of message
     * @param {Object} message - The message to validate
     * @returns {Object} - { isValid: boolean, errors: Array }
     */
    validateMessage(messageType, message) {
        const schema = this.schemaMap.get(messageType);
        
        if (!schema) {
            return {
                isValid: false,
                errors: [`No validation schema found for message type: ${messageType}`]
            };
        }

        const validation = schema.validate(message, { abortEarly: false });
        
        if (validation.error) {
            return {
                isValid: false,
                errors: validation.error.details.map(detail => detail.message)
            };
        }

        return {
            isValid: true,
            errors: []
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
