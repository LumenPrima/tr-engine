const mongoose = require('mongoose');

const TalkgroupSchema = new mongoose.Schema({
  sys_name: { type: String, required: true },
  sys_num: { type: Number, required: true },
  talkgroup: { type: Number, required: true },
  alpha_tag: String,
  description: String,
  category: String,
  group: String,
  emergency: { type: Boolean, default: false },
  last_heard: Date,
  first_heard: { type: Date, default: Date.now },
  config: {  // For user-overridable settings
    alias: String,
    priority: { type: Number, default: 1 }
  }
}, {
  timestamps: true
});

// Indexes
TalkgroupSchema.index({ sys_num: 1, talkgroup: 1 }, { unique: true });
TalkgroupSchema.index({ sys_name: 1 });  // Keep sys_name index for filtering
TalkgroupSchema.index({ emergency: 1 });
TalkgroupSchema.index({ last_heard: 1 });

const Talkgroup = mongoose.model('Talkgroup', TalkgroupSchema);

module.exports = Talkgroup;
