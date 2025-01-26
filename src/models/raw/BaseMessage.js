const mongoose = require('mongoose');

// Base schema for all MQTT messages
const BaseMessageSchema = new mongoose.Schema({
  type: { 
    type: String, 
    required: true,
    index: true 
  },
  timestamp: { 
    type: Number,
    required: true,
    index: true 
  },
  instance_id: { 
    type: String,
    required: true,
    index: true 
  },
  // The raw message payload stored as Mixed type
  payload: mongoose.Schema.Types.Mixed,
  // Original MQTT topic
  topic: {
    type: String,
    required: true,
    index: true
  }
}, {
  timestamps: true, // Adds createdAt and updatedAt
  strict: false // Allow dynamic fields from MQTT messages
});

// Compound indexes for common queries
BaseMessageSchema.index({ type: 1, timestamp: -1 });
BaseMessageSchema.index({ instance_id: 1, timestamp: -1 });
BaseMessageSchema.index({ topic: 1, timestamp: -1 });

// Helper function to create a collection for a specific message type
const createMessageCollection = (name, additionalIndexes = []) => {
  // Check if model already exists to prevent overwrite error
  let model;
  try {
    model = mongoose.model(name);
  } catch (error) {
    // Model doesn't exist yet, create it
    model = mongoose.model(name, BaseMessageSchema, name.toLowerCase());
  }

  // Add any additional indexes specific to this message type
  additionalIndexes.forEach(index => {
    model.collection.createIndex(index.fields, index.options || {});
  });

  return model;
};

module.exports = {
  BaseMessageSchema,
  createMessageCollection
};
