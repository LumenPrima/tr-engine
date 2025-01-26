const request = require('supertest');
const { TREngine } = require('../../../src/app');
const systemManager = require('../../../src/services/state/SystemManager');
const mongoose = require('mongoose');

describe('Systems API Tests', () => {
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
    // Clear any existing systems
    await systemManager.clearSystems();
  });

  describe('System Status', () => {
    it('gets all systems status', async () => {
      // Add test system
      await systemManager.processMessage('tr-mqtt/main/systems', {
        type: 'systems',
        instance_id: 'test',
        systems: [{
          sys_name: 'Test System',
          sys_num: 1,
          type: 'P25',
          sysid: '123',
          wacn: '456',
          nac: '789',
          rfss: 1,
          site_id: 1
        }]
      });

      // Add system rates
      await systemManager.processMessage('tr-mqtt/main/rates', {
        type: 'rates',
        instance_id: 'test',
        rates: [{
          sys_name: 'Test System',
          sys_num: 1,
          decoderate: 45.5,
          decoderate_interval: 60,
          control_channel: 851.0125
        }]
      });

      // Add system config
      await systemManager.processMessage('tr-mqtt/main/config', {
        type: 'config',
        instance_id: 'test',
        config: {
          systems: [{
            sys_name: 'Test System',
            system_type: 'p25',
            control_channels: [851.0125, 851.0375],
            voice_channels: [851.2125, 851.2375]
          }]
        }
      });

      const res = await request(app)
        .get('/api/v1/systems/status')
        .expect('Content-Type', /json/)
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.systems).toHaveLength(1);
      expect(res.body.data.count).toBe(1);
      expect(res.body.data.systems[0].sys_name).toBe('Test System');
      expect(res.body.data.systems[0].status.connected).toBe(true);
    });

    it('gets system performance stats', async () => {
      // Add test system with performance data
      await systemManager.processMessage('tr-mqtt/main/systems', {
        type: 'systems',
        instance_id: 'test',
        systems: [{
          sys_name: 'Test System',
          sys_num: 1,
          type: 'P25'
        }]
      });

      // Add performance data
      await systemManager.processMessage('tr-mqtt/main/rates', {
        type: 'rates',
        instance_id: 'test',
        rates: [{
          sys_name: 'Test System',
          sys_num: 1,
          decoderate: 45.5,
          decoderate_interval: 60,
          control_channel: 851.0125
        }]
      });

      // Add recorder status
      await systemManager.processMessage('tr-mqtt/main/recorder', {
        type: 'recorder',
        instance_id: 'test',
        recorder: {
          sys_name: 'Test System',
          id: 'rec1',
          state: 'RECORDING'
        }
      });

      const res = await request(app)
        .get('/api/v1/systems/stats')
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.stats.system_stats).toHaveLength(1);
      expect(res.body.data.stats.aggregate).toBeDefined();
      expect(res.body.data.stats.aggregate.average_decoderate).toBeCloseTo(45.5);
    });

    it('gets detailed status for specific system', async () => {
      // Add test system
      await systemManager.processMessage('tr-mqtt/main/systems', {
        type: 'systems',
        instance_id: 'test',
        systems: [{
          sys_name: 'Test System',
          sys_num: 1,
          type: 'P25',
          sysid: '123',
          wacn: '456',
          nac: '789',
          rfss: 1,
          site_id: 1
        }]
      });

      // Add system rates
      await systemManager.processMessage('tr-mqtt/main/rates', {
        type: 'rates',
        instance_id: 'test',
        rates: [{
          sys_name: 'Test System',
          sys_num: 1,
          decoderate: 45.5,
          decoderate_interval: 60,
          control_channel: 851.0125
        }]
      });

      // Add system config
      await systemManager.processMessage('tr-mqtt/main/config', {
        type: 'config',
        instance_id: 'test',
        config: {
          systems: [{
            sys_name: 'Test System',
            system_type: 'p25',
            control_channels: [851.0125, 851.0375]
          }]
        }
      });

      const res = await request(app)
        .get('/api/v1/systems/Test System/status')
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.system.sys_name).toBe('Test System');
      expect(res.body.data.system.sysid).toBe('123');
      expect(res.body.data.system.status.connected).toBe(true);
      expect(res.body.data.system.performance).toBeDefined();
      expect(res.body.data.system.config).toBeDefined();
    });

    it('returns 404 for non-existent system', async () => {
      const res = await request(app)
        .get('/api/v1/systems/NonExistent/status')
        .expect(404);

      expect(res.body.status).toBe('error');
      expect(res.body.message).toBe('System not found');
    });
  });
});
