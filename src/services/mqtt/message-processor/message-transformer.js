const logger = require('../../../utils/logger');
const timestamps = require('../../../utils/timestamps');

class MessageTransformer {
  transformMessage(message) {
    // Validate incoming Unix timestamps
    if (message.timestamp && !timestamps.isValidUnix(message.timestamp)) {
      logger.warn('Invalid Unix timestamp received:', message.timestamp);
    }
    if (message.start_time && !timestamps.isValidUnix(message.start_time)) {
      logger.warn('Invalid start_time timestamp received:', message.start_time);
    }
    if (message.end_time && !timestamps.isValidUnix(message.end_time)) {
      logger.warn('Invalid end_time timestamp received:', message.end_time);
    }

    // Keep all timestamps as Unix (seconds since epoch)
    const transformed = {
      ...this.flattenObject(message),
      _processed_at: timestamps.getCurrentUnix(),
      _mqtt_received_at: timestamps.getCurrentUnix()
    };

    logger.debug('Transformed message:', {
      original: JSON.stringify(message),
      transformed: JSON.stringify(transformed),
      topic: message.topic
    });

    return transformed;
  }

  flattenObject(obj) {
    if (!obj || typeof obj !== 'object') {
      return obj;
    }

    if (Array.isArray(obj)) {
      return obj;
    }

    return Object.entries(obj).reduce((acc, [key, value]) => {
      if (Array.isArray(value)) {
        acc[key] = value;
      } else if (value && typeof value === 'object') {
        Object.assign(acc, this.flattenObject(value));
      } else {
        acc[key] = value;
      }
      return acc;
    }, {});
  }
}

module.exports = new MessageTransformer();
