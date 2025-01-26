const messageProcessor = require('../../../../src/services/mqtt/processor');
const { SystemMessage } = require('../../../../src/models/raw/MessageCollections');
const mongoose = require('mongoose');

describe('Message Processor Tests', () => {
  beforeEach(async () => {
    await SystemMessage.deleteMany({});
  });

  it('processes valid system message', async () => {
    // Ensure MongoDB is connected
    expect(mongoose.connection.readyState).toBe(1);

    const topic = 'tr-mqtt/main/systems';
    const message = {
      type: 'systems',
      timestamp: Date.now(),
      instance_id: 'test',
      systems: [{
        sys_name: 'Test System',
        type: 'P25'
      }]
    };

    await messageProcessor.handleMessage(
      topic,
      Buffer.from(JSON.stringify(message))
    );

    // Wait for message to be processed
    await new Promise(resolve => setTimeout(resolve, 100));

    const stored = await SystemMessage.findOne({ topic });
    expect(stored).toBeTruthy();
    expect(stored.type).toBe('systems');
    expect(stored.topic).toBe(topic);
    expect(stored.payload).toBeTruthy();
    
    const payload = JSON.parse(stored.payload);
    expect(payload.systems[0].sys_name).toBe('Test System');
  });

  it('handles invalid message format', async () => {
    await messageProcessor.handleMessage(
      'tr-mqtt/main/systems',
      Buffer.from('invalid json')
    );

    const count = await SystemMessage.countDocuments();
    expect(count).toBe(0);
  });

  it('handles empty message', async () => {
    await messageProcessor.handleMessage(
      'tr-mqtt/main/systems',
      Buffer.from('{}')
    );

    const count = await SystemMessage.countDocuments();
    expect(count).toBe(0);
  });

  it('gets messages in correct order', async () => {
    const topic = 'tr-mqtt/main/systems';
    const baseTime = Date.now();
    
    const messages = [
      {
        type: 'systems',
        topic,
        timestamp: baseTime - 1000,
        instance_id: 'test1',
        payload: JSON.stringify({
          type: 'systems',
          systems: [{ sys_name: 'System 1' }]
        })
      },
      {
        type: 'systems',
        topic,
        timestamp: baseTime,
        instance_id: 'test2',
        payload: JSON.stringify({
          type: 'systems',
          systems: [{ sys_name: 'System 2' }]
        })
      }
    ];

    await SystemMessage.insertMany(messages);

    const result = await messageProcessor.getMessages('systems');
    expect(result).toHaveLength(2);
    expect(result[0].instance_id).toBe('test2');
    expect(result[1].instance_id).toBe('test1');
  });

  it('handles unknown message type', async () => {
    await expect(
      messageProcessor.getMessages('unknown')
    ).rejects.toThrow('Unknown collection');
  });
});
