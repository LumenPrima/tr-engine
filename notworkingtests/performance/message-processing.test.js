const messageProcessor = require('../../src/services/mqtt/processor');
const { SystemMessage } = require('../../src/models/raw/MessageCollections');
const mongoose = require('mongoose');

describe('Message Processing Performance', () => {
  beforeEach(async () => {
    await SystemMessage.deleteMany({});
  });

  it('processes messages efficiently', async () => {
    // Ensure MongoDB is connected
    expect(mongoose.connection.readyState).toBe(1);

    const topic = 'tr-mqtt/main/systems';
    const baseTime = Date.now();
    const messages = Array(100).fill().map((_, i) => ({
      type: 'systems',
      timestamp: baseTime + i,
      instance_id: `test-${i}`,
      systems: [{
        sys_name: `System ${i}`,
        type: 'P25'
      }]
    }));

    const start = Date.now();

    // Process messages sequentially to ensure reliable storage
    for (const msg of messages) {
      await messageProcessor.handleMessage(
        topic,
        Buffer.from(JSON.stringify(msg))
      );
    }

    const duration = Date.now() - start;
    console.log(`Processed ${messages.length} messages in ${duration}ms`);

    // Wait for messages to be processed
    await new Promise(resolve => setTimeout(resolve, 100));

    // Verify message count
    const stored = await SystemMessage.countDocuments();
    expect(stored).toBe(messages.length);

    // Verify message order and content
    const storedMessages = await messageProcessor.getMessages('systems', {}, { limit: 1 });
    expect(storedMessages).toHaveLength(1);
    
    const payload = JSON.parse(storedMessages[0].payload);
    expect(payload.topic).toBe(topic);
    expect(payload.systems[0].sys_name).toBe('System 99');
  });
});
