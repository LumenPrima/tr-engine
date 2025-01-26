// Set up environment variables for testing
process.env.NODE_ENV = 'test';
process.env.MONGODB_URI = 'mongodb://localhost:27017/tr-engine-test';
process.env.MQTT_BROKER_URL = 'mqtt://localhost:1883';
process.env.PORT = '3001';
process.env.WS_PORT = '3002';

module.exports = async () => {
  // This function runs once before all tests
  console.log('Setting up test environment...');
  
  // You can add any one-time setup here
  // For example, creating test directories, setting up mock services, etc.
  
  console.log('Test environment ready');
};
