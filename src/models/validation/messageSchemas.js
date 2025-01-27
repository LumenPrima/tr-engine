const Joi = require('joi');

// Base fields that should be present in all messages
const baseMessageSchema = Joi.object({
    instance_id: Joi.string().required(),
    timestamp: Joi.date().iso(),
    type: Joi.string().required()
});

// System message validation
const systemMessageSchema = baseMessageSchema.keys({
    systems: Joi.array().items(Joi.object({
        sys_name: Joi.string().required(),
        sys_num: Joi.number().required(),
        type: Joi.string().required(),
        sysid: Joi.number().required(),
        wacn: Joi.string(),
        nac: Joi.string()
    })).required()
});

// Rates message validation
const ratesMessageSchema = baseMessageSchema.keys({
    rates: Joi.array().items(Joi.object({
        sys_name: Joi.string().required(),
        control_channel: Joi.string().required(),
        decoderate: Joi.number().required(),
        decoderate_interval: Joi.number().required()
    })).required()
});

// Recorder message validation
const recorderMessageSchema = baseMessageSchema.keys({
    recorder: Joi.object({
        id: Joi.string().required(),
        status: Joi.string().valid('idle', 'recording', 'error').required(),
        current_call: Joi.object({
            talkgroup: Joi.number(),
            start_time: Joi.date().iso(),
            sys_name: Joi.string()
        }).optional()
    }).required()
});

// Unit message validation
const unitMessageSchema = baseMessageSchema.keys({
    unit: Joi.object({
        unit: Joi.number().required(),
        sys_name: Joi.string().required(),
        talkgroup: Joi.number().required(),
        unit_alpha_tag: Joi.string().allow(''),
        location: Joi.object({
            lat: Joi.number(),
            lon: Joi.number()
        }).optional()
    }).required()
});

module.exports = {
    systemMessageSchema,
    ratesMessageSchema,
    recorderMessageSchema,
    unitMessageSchema
};
