// Template management
const messageTemplates = {};

// Helper functions
const getCurrentTimestamp = () => Math.floor(Date.now() / 1000);

const updateMessageTimestamps = (message) => {
  const timestamp = getCurrentTimestamp();
  const result = { ...message };
  result.timestamp = timestamp;

  if (result.call) {
    result.call.start_time = timestamp;
    result.call.stop_time = timestamp;
    if (result.type === 'call_end') {
      result.call.process_call_time = timestamp;
    }
  }

  return result;
};

const updateMessageIds = (message, values) => {
  const result = { ...message };
  if (result.call) {
    result.call.sys_num = values.sys_num || 0;
    result.call.sys_name = values.sys_name || 'butco';
    result.call.unit = values.unit || 909248;
    result.call.talkgroup = values.talkgroup || 9179;
    result.call.id = `${result.call.sys_num}_${result.call.talkgroup}_${result.timestamp}`;
  } else if (result.on) {
    result.on.sys_num = values.sys_num || 0;
    result.on.sys_name = values.sys_name || 'butco';
    result.on.unit = values.unit || 909248;
  } else if (result.off) {
    result.off.sys_num = values.sys_num || 0;
    result.off.sys_name = values.sys_name || 'butco';
    result.off.unit = values.unit || 909248;
  }
  return result;
};

// Import all message templates
const importTemplates = async () => {
  try {
    const messages = import.meta.glob('../../mqtt_messages/*.txt', { query: '?raw', import: 'default' });
    const loadPromises = Object.entries(messages).map(async ([path, loader]) => {
      try {
        const content = await loader();
        const filename = path.split('/').pop().replace('.txt', '');
        // Get first message from file as template
        const firstMessage = content.split('\n')[0];
        messageTemplates[filename] = JSON.parse(firstMessage);
        console.log('Loaded template:', filename);
      } catch (error) {
        console.error(`Error loading template ${path}:`, error);
      }
    });

    await Promise.all(loadPromises);
    console.log('All templates loaded:', Object.keys(messageTemplates));
  } catch (error) {
    console.error('Error loading message templates:', error);
  }
};

// Initialize templates
(async () => {
  await importTemplates();
  console.log('Template initialization complete:', Object.keys(messageTemplates));
})();

export const getMessageTemplate = (templateName, values = {}) => {
  const template = messageTemplates[templateName];
  if (!template) return null;

  let message = JSON.parse(JSON.stringify(template)); // Deep clone
  message = updateMessageTimestamps(message);
  message = updateMessageIds(message, values);

  return JSON.stringify(message, null, 2);
};

export const getTemplateNames = () => {
  return Object.keys(messageTemplates);
};

export const validateMessage = (message) => {
  try {
    JSON.parse(message);
    return true;
  } catch (error) {
    return false;
  }
};

export const generateTopicFromTemplate = (templateName) => {
  const parts = templateName.split('_');
  if (parts.length < 2) return '';

import { MQTT_CONFIG } from '../config';

  // Example: tr-mqtt_main_call_start.txt -> {topicPrefix}/calls
  // Example: tr-mqtt_units_butco_on.txt -> {topicPrefix}/units
  if (parts[1] === 'main') {
    return `${MQTT_CONFIG.topicPrefix}/calls`;
  } else if (parts[1] === 'units') {
    return `${MQTT_CONFIG.topicPrefix}/units`;
  }
  return MQTT_CONFIG.topicPrefix;
};
