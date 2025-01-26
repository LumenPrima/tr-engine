const request = require('supertest');
const { TREngine } = require('../../../src/app');
const unitManager = require('../../../src/services/state/UnitManager');
const mongoose = require('mongoose');

describe('Units API Tests', () => {
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
    // Clear any existing units
    await unitManager.clearUnits();
  });

  describe('Real-time Monitoring', () => {
    it('gets active units', async () => {
      // Add test unit - first comes online
      await unitManager.processMessage('tr-mqtt/units/Test System/on', {
        type: 'on',
        unit: 1234,
        unit_alpha_tag: 'UNIT1'
      });

      // Then joins a talkgroup
      await unitManager.processMessage('tr-mqtt/units/Test System/join', {
        type: 'join',
        unit: 1234,
        unit_alpha_tag: 'UNIT1',
        talkgroup: 101,
        talkgroup_tag: 'DISPATCH'
      });

      const res = await request(app)
        .get('/api/v1/active/units')
        .expect('Content-Type', /json/)
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.units).toHaveLength(1);
      expect(res.body.data.count).toBe(1);
      expect(res.body.data.units[0].id).toBe(1234);
      expect(res.body.data.units[0].unit_tag).toBe('UNIT1');
      expect(res.body.data.window).toBeDefined();
    });

    it('filters active units by time window', async () => {
      // Add test unit that came online 10 minutes ago
      const oldTime = new Date(Date.now() - 10 * 60 * 1000);
      await unitManager.processMessage('tr-mqtt/units/Test System/on', {
        type: 'on',
        unit: 1234,
        unit_alpha_tag: 'UNIT1',
        timestamp: oldTime
      });

      const res = await request(app)
        .get('/api/v1/active/units')
        .query({ window: 5 }) // 5 minute window
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.units).toHaveLength(0); // Unit should be outside window
      expect(res.body.data.window).toBe(300); // 5 minutes in seconds
    });
  });

  describe('Historical Data', () => {
    it('gets unit history', async () => {
      const unitId = 1234;
      const sysName = 'Test System';

      // Unit comes online
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/on`, {
        type: 'on',
        unit: unitId,
        unit_alpha_tag: 'UNIT1'
      });

      // Unit makes a call
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/call`, {
        type: 'call',
        unit: unitId,
        unit_alpha_tag: 'UNIT1',
        talkgroup: 101,
        emergency: false
      });

      // Unit goes offline
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/off`, {
        type: 'off',
        unit: unitId,
        unit_alpha_tag: 'UNIT1'
      });

      const res = await request(app)
        .get(`/api/v1/history/unit/${unitId}`)
        .query({ sys_name: sysName })
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.unit.id).toBe(unitId);
      expect(res.body.data.history).toBeDefined();
      expect(res.body.data.time_range).toBeDefined();
    });

    it('filters unit history by type', async () => {
      const unitId = 1234;
      const sysName = 'Test System';

      // Unit comes online
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/on`, {
        type: 'on',
        unit: unitId,
        unit_alpha_tag: 'UNIT1'
      });

      // Unit makes a call
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/call`, {
        type: 'call',
        unit: unitId,
        unit_alpha_tag: 'UNIT1',
        talkgroup: 101
      });

      // Unit affiliates with talkgroup
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/join`, {
        type: 'join',
        unit: unitId,
        unit_alpha_tag: 'UNIT1',
        talkgroup: 101
      });

      const res = await request(app)
        .get(`/api/v1/history/unit/${unitId}`)
        .query({
          sys_name: sysName,
          types: 'call,status'
        })
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.history.every(event => 
        ['call', 'status'].includes(event.type)
      )).toBe(true);
    });
  });

  describe('Talkgroup Association', () => {
    it('gets units in talkgroup', async () => {
      const talkgroupId = 101;
      const sysName = 'Test System';

      // First unit comes online and joins talkgroup
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/on`, {
        type: 'on',
        unit: 1234,
        unit_alpha_tag: 'UNIT1'
      });
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/join`, {
        type: 'join',
        unit: 1234,
        unit_alpha_tag: 'UNIT1',
        talkgroup: talkgroupId,
        talkgroup_tag: 'DISPATCH'
      });

      // Second unit comes online and joins talkgroup
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/on`, {
        type: 'on',
        unit: 5678,
        unit_alpha_tag: 'UNIT2'
      });
      await unitManager.processMessage(`tr-mqtt/units/${sysName}/join`, {
        type: 'join',
        unit: 5678,
        unit_alpha_tag: 'UNIT2',
        talkgroup: talkgroupId,
        talkgroup_tag: 'DISPATCH'
      });

      const res = await request(app)
        .get(`/api/v1/units/talkgroup/${talkgroupId}`)
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.units).toHaveLength(2);
      expect(res.body.data.talkgroup).toBe(talkgroupId);
    });

    it('handles empty talkgroup', async () => {
      const res = await request(app)
        .get('/api/v1/units/talkgroup/999')
        .expect(200);

      expect(res.body.status).toBe('success');
      expect(res.body.data.units).toHaveLength(0);
      expect(res.body.data.count).toBe(0);
    });
  });
});
