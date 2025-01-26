const mongoose = require('mongoose');

const TalkgroupSchema = new mongoose.Schema({
  sys_name: { type: String, required: true, index: true },
  sys_num: { type: Number, required: true, index: true },
  talkgroup: { type: Number, required: true, index: true },
  alpha_tag: { type: String, index: true },
  description: String,
  category: String,
  group: String,
  emergency: { type: Boolean, default: false, index: true },
  last_heard: { type: Date, index: true },
  first_heard: { type: Date, default: Date.now },
  config: {  // For user-overridable settings
    alias: String,
    priority: { type: Number, default: 1 }
  }
}, {
  timestamps: true
});

// Compound index for unique talkgroup per system
TalkgroupSchema.index({ sys_num: 1, talkgroup: 1 }, { unique: true });

module.exports = TalkgroupSchema;
