const logger = require('../../../utils/logger');

class MessageTransformer {
  transformMessage(message) {
    const transformed = {
      ...this.flattenObject(message),
      _processed_at: new Date(),
      _mqtt_received_at: new Date()
    };

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