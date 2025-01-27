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

// Audio message validation - only for unencrypted calls
const audioMessageSchema = baseMessageSchema.keys({
    call: Joi.object({
        metadata: Joi.object({
            filename: Joi.string().required(),
            talkgroup: Joi.number().required(),
            talkgroup_tag: Joi.string(),
            talkgroup_description: Joi.string(),
            talkgroup_group: Joi.string(),
            start_time: Joi.date().iso().required(),
            freq: Joi.number(),
            freq_error: Joi.number(),
            emergency: Joi.boolean(),
            encrypted: Joi.boolean().valid(false), // Audio messages should never be for encrypted calls
            phase2_tdma: Joi.boolean(),
            audio_type: Joi.string()
        }).required(),
        audio_wav_base64: Joi.string().required(), // Audio data must be present for unencrypted calls
        audio_m4a_base64: Joi.string()
    }).required()
});

// Call event validation schemas
const callStartSchema = baseMessageSchema.keys({
    call: Joi.object({
        sys_name: Joi.string().required(),
        sys_num: Joi.number().required(),
        talkgroup: Joi.number().required(),
        talkgroup_tag: Joi.string(),
        talkgroup_description: Joi.string(),
        start_time: Joi.number().required(),
        emergency: Joi.boolean(),
        encrypted: Joi.boolean().required(), // Encryption status must be specified
        freq: Joi.number(),
        audio_type: Joi.when('encrypted', {
            is: false,
            then: Joi.string().required(), // Audio type required for unencrypted calls
            otherwise: Joi.string().optional() // Optional for encrypted calls
        })
    }).required()
});

const callEndSchema = baseMessageSchema.keys({
    call: Joi.object({
        sys_name: Joi.string().required(),
        sys_num: Joi.number().required(),
        talkgroup: Joi.number().required(),
        stop_time: Joi.number().required(),
        length: Joi.number()
    }).required()
});

const callsActiveSchema = baseMessageSchema.keys({
    calls: Joi.array().items(Joi.object({
        sys_name: Joi.string().required(),
        sys_num: Joi.number().required(),
        talkgroup: Joi.number().required(),
        start_time: Joi.number().required(),
        emergency: Joi.boolean(),
        encrypted: Joi.boolean().required(), // Encryption status must be specified
        freq: Joi.number()
    })).required()
});

module.exports = {
    systemMessageSchema,
    ratesMessageSchema,
    recorderMessageSchema,
    unitMessageSchema,
    audioMessageSchema,
    callStartSchema,
    callEndSchema,
    callsActiveSchema
};
