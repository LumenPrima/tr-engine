const logger = require('../../../utils/logger');
const timestamps = require('../../../utils/timestamps');

class MessageTransformer {
  transformMessage(message) {
    const transformed = {
      ...this.flattenObject(message),
      _processed_at: timestamps.getCurrentTimeISO(),
      _mqtt_received_at: timestamps.getCurrentTimeISO()
    };

    // Ensure all timestamps are proper Unix timestamps (seconds since epoch)
    // MQTT messages already provide Unix timestamps, so we just need to ensure they're valid
    if (transformed.timestamp && !timestamps.isValidUnix(transformed.timestamp)) {
      logger.warn('Invalid Unix timestamp received:', transformed.timestamp);
    }
    if (transformed.start_time && !timestamps.isValidUnix(transformed.start_time)) {
      logger.warn('Invalid start_time timestamp received:', transformed.start_time);
    }
    if (transformed.end_time && !timestamps.isValidUnix(transformed.end_time)) {
      logger.warn('Invalid end_time timestamp received:', transformed.end_time);
    }

    logger.debug('Transformed message:', {
      original: message,
      transformed: transformed
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
