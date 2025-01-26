const activeCallManager = require('../../../../src/services/state/ActiveCallManager');

describe('Active Call Manager Tests', () => {
  beforeEach(async () => {
    // Clear any active calls
    const calls = await activeCallManager.getActiveCalls();
    for (const call of calls) {
      await activeCallManager.processMessage('tr-mqtt/main/call_end', {
        type: 'call_end',
        call: {
          sys_name: call.sys_name,
          talkgroup: call.talkgroup,
          start_time: call.start_time
        }
      });
    }
  });

  it('manages call lifecycle', async () => {
    // Start call
    const startTime = Date.now();
    await activeCallManager.processMessage('tr-mqtt/main/call_start', {
      type: 'call_start',
      call: {
        sys_name: 'Test System',
        talkgroup: 101,
        start_time: startTime,
        unit: 1234
      }
    });

    // Verify call started
    let calls = await activeCallManager.getActiveCalls();
    expect(calls).toHaveLength(1);
    expect(calls[0].talkgroup).toBe(101);
    expect(calls[0].units).toHaveLength(1);
    expect(calls[0].units[0].unit).toBe(1234);

    // Add unit
    await activeCallManager.processMessage('tr-mqtt/units/test/join', {
      type: 'join',
      join: {
        sys_name: 'Test System',
        talkgroup: 101,
        unit: 5678
      }
    });

    // Verify unit added
    calls = await activeCallManager.getActiveCalls();
    expect(calls[0].units).toHaveLength(2);
    expect(calls[0].units[1].unit).toBe(5678);

    // End call
    await activeCallManager.processMessage('tr-mqtt/main/call_end', {
      type: 'call_end',
      call: {
        sys_name: 'Test System',
        talkgroup: 101,
        start_time: startTime
      }
    });

    // Verify call ended
    calls = await activeCallManager.getActiveCalls();
    expect(calls).toHaveLength(0);
  });

  it('filters active calls', async () => {
    // Create test calls
    const calls = [
      {
        type: 'call_start',
        call: {
          sys_name: 'System 1',
          talkgroup: 101,
          start_time: Date.now(),
          unit: 1234,
          emergency: true
        }
      },
      {
        type: 'call_start',
        call: {
          sys_name: 'System 2', 
          talkgroup: 102,
          start_time: Date.now(),
          unit: 5678
        }
      }
    ];

    for (const call of calls) {
      await activeCallManager.processMessage('tr-mqtt/main/call_start', call);
    }

    // Test filters
    let filtered = await activeCallManager.getActiveCalls({ emergency: true });
    expect(filtered).toHaveLength(1);
    expect(filtered[0].talkgroup).toBe(101);

    filtered = await activeCallManager.getActiveCalls({ sys_name: 'System 2' });
    expect(filtered).toHaveLength(1);
    expect(filtered[0].talkgroup).toBe(102);
  });
});
