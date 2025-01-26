const request = require('supertest');
const { TREngine } = require('../../../src/app');
const activeCallManager = require('../../../src/services/state/ActiveCallManager');
const mongoose = require('mongoose');

describe('Call API Tests', () => {
  let app;
  let engine;

  beforeAll(async () => {
    engine = new TREngine();
    await engine.initialize();
    app = engine.app;
  });

  afterAll(async () => {
    await engine.shutdown();
  });

  beforeEach(async () => {
    // Clear any existing calls
    await activeCallManager.clearActiveCalls();
  });

  describe('Real-time Monitoring', () => {
    it('gets active calls', async () => {
      // Ensure MongoDB is connected
      expect(mongoose.connection.readyState).toBe(1);

      // Create test call
      await activeCallManager.processMessage('tr-mqtt/main/call_start', {
        type: 'call_start',
        call: {
          sys_name: 'Test System',
          talkgroup: 101,
          start_time: Date.now(),
          unit: 1234,
          emergency: true
        }
      });

      // Wait for call to be processed
      await new Promise(resolve => setTimeout(resolve, 100));

      const res = await request(app)
        .get('/api/v1/active/calls')
        .expect('Content-Type', /json/)
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.calls).toHaveLength(1);
      expect(res.body.data.count).toBe(1);
      expect(res.body.data.calls[0].talkgroup).toBe(101);
      expect(res.body.data.calls[0].emergency).toBe(true);
    });

    it('filters active calls', async () => {
      // Create test calls
      await activeCallManager.processMessage('tr-mqtt/main/call_start', {
        type: 'call_start', 
        call: {
          sys_name: 'System 1',
          talkgroup: 101,
          start_time: Date.now(),
          unit: 1234,
          emergency: true
        }
      });

      await activeCallManager.processMessage('tr-mqtt/main/call_start', {
        type: 'call_start',
        call: {
          sys_name: 'System 2',
          talkgroup: 102,
          start_time: Date.now(),
          unit: 5678
        }
      });

      // Wait for calls to be processed
      await new Promise(resolve => setTimeout(resolve, 100));

      // Test emergency filter
      let res = await request(app)
        .get('/api/v1/active/calls?emergency=true')
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.calls).toHaveLength(1);
      expect(res.body.data.calls[0].emergency).toBe(true);

      // Test system filter
      res = await request(app)
        .get('/api/v1/active/calls?sys_name=System 2')
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.calls).toHaveLength(1);
      expect(res.body.data.calls[0].talkgroup).toBe(102);
    });

    it('gets active events', async () => {
      // Create test emergency call
      await activeCallManager.processMessage('tr-mqtt/main/call_start', {
        type: 'call_start',
        call: {
          sys_name: 'System 1',
          talkgroup: 101,
          start_time: Date.now(),
          unit: 1234,
          emergency: true
        }
      });

      const res = await request(app)
        .get('/api/v1/active/events')
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.events.emergencies).toHaveLength(1);
      expect(res.body.data.events.calls).toHaveLength(1);
      expect(res.body.data.count).toBe(2);
    });
  });

  describe('Historical Data', () => {
    it('gets call history', async () => {
      // Create historical call
      const startTime = new Date(Date.now() - 60 * 60 * 1000); // 1 hour ago
      await activeCallManager.processMessage('tr-mqtt/main/call_start', {
        type: 'call_start',
        call: {
          sys_name: 'System 1',
          talkgroup: 101,
          start_time: startTime,
          unit: 1234,
          emergency: true
        }
      });

      const res = await request(app)
        .get('/api/v1/history/calls')
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.calls).toHaveLength(1);
      expect(res.body.data.pagination).toBeDefined();
      expect(res.body.data.time_range).toBeDefined();
    });

    it('gets talkgroup history', async () => {
      // Create historical call for talkgroup
      const startTime = new Date(Date.now() - 60 * 60 * 1000); // 1 hour ago
      await activeCallManager.processMessage('tr-mqtt/main/call_start', {
        type: 'call_start',
        call: {
          sys_name: 'System 1',
          talkgroup: 101,
          start_time: startTime,
          unit: 1234
        }
      });

      const res = await request(app)
        .get('/api/v1/history/talkgroup/101')
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.events).toBeDefined();
      expect(res.body.data.talkgroup.id).toBe(101);
      expect(res.body.data.time_range).toBeDefined();
    });
  });
});
