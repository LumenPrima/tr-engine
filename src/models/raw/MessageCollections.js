const { createMessageCollection } = require('./BaseMessage');

// System-related collections
const SystemMessage = createMessageCollection('SystemMessage', [
  { fields: { 'payload.systems.sys_num': 1 } },
  { fields: { 'payload.systems.sys_name': 1 } }
]);

const RatesMessage = createMessageCollection('RatesMessage', [
  { fields: { 'payload.rates.sys_num': 1 } },
  { fields: { 'payload.rates.sys_name': 1 } }
]);

const RecorderMessage = createMessageCollection('RecorderMessage', [
  { fields: { 'payload.recorder.id': 1 } },
  { fields: { 'payload.recorder.rec_state_type': 1 } }
]);

// Call-related collections
const CallStartMessage = createMessageCollection('CallStartMessage', [
  { fields: { 'payload.call.id': 1 } },
  { fields: { 'payload.call.sys_name': 1 } },
  { fields: { 'payload.call.talkgroup': 1 } },
  { fields: { 'payload.call.unit': 1 } },
  { fields: { 'payload.call.start_time': -1 } },
  { fields: { 'payload.call.emergency': 1 } }
]);

const CallEndMessage = createMessageCollection('CallEndMessage', [
  { fields: { 'payload.call.id': 1 } },
  { fields: { 'payload.call.sys_name': 1 } },
  { fields: { 'payload.call.talkgroup': 1 } },
  { fields: { 'payload.call.stop_time': -1 } }
]);

const CallsActiveMessage = createMessageCollection('CallsActiveMessage', [
  { fields: { 'payload.calls.id': 1 } },
  { fields: { 'payload.calls.sys_name': 1 } },
  { fields: { 'payload.calls.talkgroup': 1 } }
]);

const AudioMessage = createMessageCollection('AudioMessage', [
  // Basic metadata indexes
  { fields: { 'payload.call.metadata.filename': 1 } },
  { fields: { 'payload.call.metadata.talkgroup': 1 } },
  { fields: { 'payload.call.metadata.start_time': -1 } },
  { fields: { 'payload.call.metadata.stop_time': -1 } },
  { fields: { 'payload.call.metadata.short_name': 1 } },
  { fields: { 'payload.call.metadata.call_length': 1 } },
  { fields: { 'payload.call.metadata.audio_type': 1 } },
  
  // Frequency and signal quality indexes
  { fields: { 'payload.call.metadata.freq': 1 } },
  { fields: { 'payload.call.metadata.freq_error': 1 } },
  { fields: { 'payload.call.metadata.signal': 1 } },
  { fields: { 'payload.call.metadata.noise': 1 } },
  
  // Source tracking indexes
  { fields: { 'payload.call.metadata.source_num': 1 } },
  { fields: { 'payload.call.metadata.recorder_num': 1 } },
  
  // Status flags indexes
  { fields: { 'payload.call.metadata.emergency': 1 } },
  { fields: { 'payload.call.metadata.priority': 1 } },
  { fields: { 'payload.call.metadata.encrypted': 1 } },
  { fields: { 'payload.call.metadata.phase2_tdma': 1 } },
  
  // Compound indexes for common queries
  { fields: { 
    'payload.call.metadata.short_name': 1, 
    'payload.call.metadata.talkgroup': 1, 
    'payload.call.metadata.start_time': -1 
  }},
  { fields: { 
    'payload.call.metadata.freq': 1, 
    'payload.call.metadata.start_time': -1 
  }},
  { fields: {
    'payload.call.metadata.emergency': 1,
    'payload.call.metadata.priority': 1,
    'payload.call.metadata.start_time': -1
  }},
  
  // Array element indexes
  { fields: { 'payload.call.metadata.freqList.freq': 1 } },
  { fields: { 'payload.call.metadata.freqList.error_count': 1 } },
  { fields: { 'payload.call.metadata.freqList.spike_count': 1 } },
  { fields: { 'payload.call.metadata.srcList.src': 1 } },
  { fields: { 'payload.call.metadata.srcList.emergency': 1 } },
  { fields: { 'payload.call.metadata.srcList.signal_system': 1 } }
]);

// Unit-related collections
const UnitCallMessage = createMessageCollection('UnitCallMessage', [
  { fields: { 'payload.call.sys_name': 1 } },
  { fields: { 'payload.call.unit': 1 } },
  { fields: { 'payload.call.talkgroup': 1 } }
]);

const UnitLocationMessage = createMessageCollection('UnitLocationMessage', [
  { fields: { 'payload.location.sys_name': 1 } },
  { fields: { 'payload.location.unit': 1 } }
]);

const UnitDataMessage = createMessageCollection('UnitDataMessage', [
  { fields: { 'payload.data.sys_name': 1 } },
  { fields: { 'payload.data.unit': 1 } }
]);

const UnitJoinMessage = createMessageCollection('UnitJoinMessage', [
  { fields: { 'payload.join.sys_name': 1 } },
  { fields: { 'payload.join.unit': 1 } },
  { fields: { 'payload.join.talkgroup': 1 } }
]);

const UnitEndMessage = createMessageCollection('UnitEndMessage', [
  { fields: { 'payload.end.sys_name': 1 } },
  { fields: { 'payload.end.unit': 1 } }
]);

const UnitOnMessage = createMessageCollection('UnitOnMessage', [
  { fields: { 'payload.on.sys_name': 1 } },
  { fields: { 'payload.on.unit': 1 } }
]);

const UnitOffMessage = createMessageCollection('UnitOffMessage', [
  { fields: { 'payload.off.sys_name': 1 } },
  { fields: { 'payload.off.unit': 1 } }
]);

const UnitAckRespMessage = createMessageCollection('UnitAckRespMessage', [
  { fields: { 'payload.ackresp.sys_name': 1 } },
  { fields: { 'payload.ackresp.unit': 1 } }
]);

module.exports = {
  // System messages
  SystemMessage,
  RatesMessage,
  RecorderMessage,
  
  // Call messages
  CallStartMessage,
  CallEndMessage,
  CallsActiveMessage,
  AudioMessage,
  
  // Unit messages
  UnitCallMessage,
  UnitLocationMessage,
  UnitDataMessage,
  UnitJoinMessage,
  UnitEndMessage,
  UnitOnMessage,
  UnitOffMessage,
  UnitAckRespMessage
};
