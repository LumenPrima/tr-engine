const mongoose = require('mongoose');
const mqtt = require('mqtt');
const _ = require('lodash');

// Schema for raw MQTT messages
// This stores every message exactly as received
const RawMessageSchema = new mongoose.Schema({
  topic: { type: String, required: true, index: true },
  payload: { type: mongoose.Schema.Types.Mixed, required: true },
  timestamp: { type: Date, required: true, index: true },
  instance_id: String
}, {
  timeseries: {
    timeField: 'timestamp',
    metaField: 'topic',
    granularity: 'seconds'
  }
});

// Schema for processed call events
// This gives us quick access to active and recent calls
const CallEventSchema = new mongoose.Schema({
  call_id: { type: String, required: true, unique: true },
  sys_num: Number,
  sys_name: { type: String, required: true, index: true },
  talkgroup: { type: Number, required: true, index: true },
  talkgroup_tag: String,
  talkgroup_description: String,
  talkgroup_group: String,
  unit: { type: Number, index: true },
  start_time: { type: Date, required: true, index: true },
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

// MQTT message processor
class MessageProcessor {
  constructor(mongoUrl, mqttUrl) {
    this.mongoUrl = mongoUrl;
    this.mqttUrl = mqttUrl;
    this.models = {};
    this.activeCalls = new Map();
  }

  async initialize() {
    // Connect to MongoDB
    await mongoose.connect(this.mongoUrl, {
      useNewUrlParser: true,
      useUnifiedTopology: true
    });

    // Initialize models
    this.models.RawMessage = mongoose.model('RawMessage', RawMessageSchema);
    this.models.CallEvent = mongoose.model('CallEvent', CallEventSchema);

    // Connect to MQTT
    this.mqttClient = mqtt.connect(this.mqttUrl);
    
    this.mqttClient.on('connect', () => {
      console.log('Connected to MQTT broker');
      // Subscribe to all tr-mqtt topics
      this.mqttClient.subscribe('tr-mqtt/#');
    });

    this.mqttClient.on('message', (topic, payload) => {
      this.handleMessage(topic, payload);
    });
  }

  async handleMessage(topic, payload) {
    try {
      const timestamp = new Date();
      const message = JSON.parse(payload.toString());

      // Store raw message
      const rawMessage = new this.models.RawMessage({
        topic,
        payload: message,
        timestamp,
        instance_id: message.instance_id
      });
      await rawMessage.save();

      // Process based on topic
      const topicParts = topic.split('/');
      const mainTopic = topicParts[1];
      const subTopic = topicParts[2];

      // Map message to consistent format based on topic structure
      if (mainTopic === 'main') {
        switch (subTopic) {
          case 'call_start':
          case 'call_end':
          case 'calls_active':
            await this.handleCallMessage(message, subTopic, rawMessage._id);
            break;
          case 'audio':
            await this.handleCallAudio(message, rawMessage._id);
            break;
          case 'systems':
          case 'rates':
            await this.handleSystemMessage(message, subTopic, rawMessage._id);
            break;
        }
      } else if (mainTopic === 'units') {
        const unitAction = topicParts[3]; // call, location, data, join
        await this.handleUnitMessage(message, unitAction, rawMessage._id);
      }
    } catch (err) {
      console.error('Error processing message:', err);
      // Here we could emit an error event or store in an error collection
    }
  }

  async handleCallMessage(message, messageType, rawMessageId) {
    const call = message.call || message.calls?.[0];
    if (!call) return;

    // Map all fields to consistent format
    const baseFields = this.mapBaseFields(call);
    const talkgroupFields = this.mapTalkgroupFields(call);
    const unitFields = this.mapUnitFields(call);
    
    const callId = `${baseFields.sys_num}_${talkgroupFields.talkgroup}_${call.start_time}`;

    switch (messageType) {
      case 'call_start': {
        const callEvent = new this.models.CallEvent({
          ...baseFields,
          ...talkgroupFields,
          ...unitFields,
          call_id: callId,
          start_time: new Date(call.start_time * 1000),
          freq: call.freq,
          emergency: call.emergency || false,
          encrypted: call.encrypted || false,
          audio_type: call.audio_type,
          state: 'STARTING',
          raw_messages: [{
            topic: `tr-mqtt/main/${messageType}`,
            timestamp: new Date(),
            message_id: rawMessageId
          }]
        });

        await callEvent.save();
        this.activeCalls.set(callId, callEvent);
        break;
      }

      case 'call_end': {
        const callEvent = await this.models.CallEvent.findOne({ call_id: callId });
        if (callEvent) {
          // Update with latest field mappings
          Object.assign(callEvent, baseFields, talkgroupFields, unitFields);
          
          callEvent.end_time = new Date(call.stop_time * 1000);
          callEvent.call_length = call.length;
          callEvent.state = 'COMPLETED';
          callEvent.raw_messages.push({
            topic: `tr-mqtt/main/${messageType}`,
            timestamp: new Date(),
            message_id: rawMessageId
          });
          
          await callEvent.save();
          this.activeCalls.delete(callId);
        }
        break;
      }

      case 'calls_active': {
        const callEvent = await this.models.CallEvent.findOne({ call_id: callId });
        if (callEvent) {
          // Update with latest field mappings and state
          Object.assign(callEvent, baseFields, talkgroupFields, unitFields);
          callEvent.state = 'RECORDING';
          callEvent.raw_messages.push({
            topic: `tr-mqtt/main/${messageType}`,
            timestamp: new Date(),
            message_id: rawMessageId
          });
          
          await callEvent.save();
        }
        break;
      }
    }
  }

  async handleSystemMessage(message, messageType, rawMessageId) {
    // Handle system-level messages (systems, rates)
    const baseFields = this.mapBaseFields(message);
    
    switch (messageType) {
      case 'systems': {
        const systems = message.systems.map(system => ({
          ...this.mapBaseFields(system),
          type: system.type,
          sysid: system.sysid,
          wacn: system.wacn,
          nac: system.nac,
          rfss: system.rfss,
          site_id: system.site_id
        }));
        
        // Update systems collection or state as needed
        break;
      }
      
      case 'rates': {
        const rates = message.rates.map(rate => ({
          ...this.mapBaseFields(rate),
          decoderate: rate.decoderate,
          decoderate_interval: rate.decoderate_interval,
          control_channel: rate.control_channel
        }));
        
        // Update rates collection or state as needed
        break;
      }
    }
  }

  async handleUnitMessage(message, unitAction, rawMessageId) {
    const baseFields = this.mapBaseFields(message);
    const unitFields = this.mapUnitFields(message);
    const talkgroupFields = this.mapTalkgroupFields(message);
    
    switch (unitAction) {
      case 'call': {
        // Unit initiating or participating in a call
        const unitCall = {
          ...baseFields,
          ...unitFields,
          ...talkgroupFields,
          timestamp: new Date(),
          action: 'call'
        };
        // Update unit state/history as needed
        break;
      }
      
      case 'location': {
        // Unit location update
        const unitLocation = {
          ...baseFields,
          ...unitFields,
          ...talkgroupFields,
          timestamp: new Date(),
          action: 'location'
        };
        // Update unit location state/history as needed
        break;
      }
      
      case 'join': {
        // Unit joining a talkgroup
        const unitJoin = {
          ...baseFields,
          ...unitFields,
          ...talkgroupFields,
          timestamp: new Date(),
          action: 'join'
        };
        // Update unit affiliations as needed
        break;
      }
    }
  }

  // Field mapping helpers
  
  // Base field mapping that enforces consistent field names across all message types
  mapBaseFields(message) {
    return {
      sys_name: message.sys_name || message.short_name || message.metadata?.short_name,
      sys_num: message.sys_num || message.metadata?.source_num,
      instance_id: message.instance_id
    };
  }

  // Maps talkgroup-related fields to a consistent format
  mapTalkgroupFields(message) {
    const tg = message.talkgroup || {};
    return {
      talkgroup: message.talkgroup || tg.id,
      talkgroup_alpha_tag: message.talkgroup_alpha_tag || tg.alpha_tag || message.talkgroup_tag,
      talkgroup_tag: message.talkgroup_tag || tg.category || message.talkgroup_group_tag,
      talkgroup_description: message.talkgroup_description || tg.description,
      talkgroup_group: message.talkgroup_group || tg.group
    };
  }

  // Maps unit-related fields to a consistent format
  mapUnitFields(message) {
    const unit = message.unit || {};
    return {
      unit: message.unit || unit.id,
      unit_alpha_tag: message.unit_alpha_tag || unit.tag,
      unit_type: message.unit_type || unit.type,
      unit_last_status: message.last_status || unit.status
    };
  }

  // Maps audio-specific fields
  mapAudioFields(metadata) {
    return {
      // Map audio metadata fields to match standard field names
      talkgroup_alpha_tag: metadata.talkgroup_tag, // Audio messages use talkgroup_tag for alpha_tag
      talkgroup_tag: metadata.talkgroup_group_tag, // Audio messages use group_tag for category
      talkgroup_description: metadata.talkgroup_description,
      talkgroup_group: metadata.talkgroup_group,
      sys_name: metadata.short_name, // Audio messages use short_name instead of sys_name
      // Preserve additional audio-specific fields
      freq: metadata.freq,
      freq_error: metadata.freq_error,
      emergency: Boolean(metadata.emergency),
      encrypted: Boolean(metadata.encrypted),
      phase2_tdma: Boolean(metadata.phase2_tdma),
      audio_type: metadata.audio_type
    };
  }

  async handleCallAudio(message, rawMessageId) {
    const metadata = message.call.metadata;
    // Extract system number from audio filename pattern or default to parsing from call id
    const sysNum = metadata.filename.split('/').pop().split('-')[0];
    const callId = `${sysNum}_${metadata.talkgroup}_${metadata.start_time}`;
    
    const callEvent = await this.models.CallEvent.findOne({ call_id: callId });
    if (callEvent) {
      // Map the fields to match our standard format
      const mappedFields = this.mapAudioFields(metadata);
      
      // Update the call record with normalized fields
      Object.assign(callEvent, mappedFields);
      
      // Store audio data in GridFS
      if (message.call.audio_wav_base64) {
        const audioBuffer = Buffer.from(message.call.audio_wav_base64, 'base64');
        const gridFSBucket = new mongoose.mongo.GridFSBucket(mongoose.connection.db, {
          bucketName: 'audioFiles'
        });
        
        // Create a unique filename for GridFS
        const audioFilename = `${callId}_${metadata.start_time}.wav`;
        
        // Store the audio file
        const uploadStream = gridFSBucket.openUploadStream(audioFilename, {
          metadata: {
            callId,
            talkgroup: metadata.talkgroup,
            start_time: metadata.start_time,
            audio_type: metadata.audio_type,
            filename: metadata.filename
          }
        });
        
        await new Promise((resolve, reject) => {
          uploadStream.once('finish', resolve);
          uploadStream.once('error', reject);
          uploadStream.end(audioBuffer);
        });
        
        callEvent.audio_file = audioFilename;
      }
      
      // Update message history
      callEvent.raw_messages.push({
        topic: 'tr-mqtt/main/audio',
        timestamp: new Date(),
        message_id: rawMessageId
      });
      
      await callEvent.save();
    }
  }
}

module.exports = MessageProcessor;
