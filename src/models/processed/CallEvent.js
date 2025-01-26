const mongoose = require('mongoose');

const CallEventSchema = new mongoose.Schema({
  call_id: { 
    type: String, 
    required: true, 
    unique: true 
  },
  sys_num: Number,
  sys_name: { 
    type: String, 
    required: true, 
    index: true 
  },
  talkgroup: { 
    type: Number, 
    required: true, 
    index: true 
  },
  talkgroup_tag: String,
  talkgroup_description: String,
  talkgroup_group: String,
  unit: { 
    type: Number, 
    index: true 
  },
  start_time: { 
    type: Date, 
    required: true, 
    index: true 
  },
  end_time: Date,
  freq: Number,
  emergency: Boolean,
  encrypted: Boolean,
  call_length: Number,
  audio_type: String,
  audio_file: String,
  state: {
    type: String,
    enum: ['STARTING', 'RECORDING', 'COMPLETED', 'ERROR'],
    default: 'STARTING'
  },
  raw_messages: [{
    topic: String,
    timestamp: Date,
    message_id: mongoose.Schema.Types.ObjectId
  }]
});

// Create indexes for common query patterns
CallEventSchema.index({ sys_name: 1, start_time: -1 });
CallEventSchema.index({ talkgroup: 1, start_time: -1 });
CallEventSchema.index({ state: 1, start_time: -1 });
CallEventSchema.index({ end_time: 1 }, { sparse: true });

module.exports = mongoose.model('CallEvent', CallEventSchema);
