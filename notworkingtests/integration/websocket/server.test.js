const WebSocket = require('ws');
const { app } = require('../../setup');

describe('WebSocket Tests', () => {
  let wsServer;
  let wsClient;

  beforeAll(async () => {
    wsServer = new WebSocket.Server({ port: 3002 });
    wsServer.on('connection', ws => {
      ws.send(JSON.stringify({ type: 'initial.state', data: { calls: [] } }));
      ws.on('message', data => {
        try {
          const msg = JSON.parse(data);
          if (msg.type === 'get.active_calls') {
            ws.send(JSON.stringify({ type: 'active_calls', data: [] }));
          }
        } catch {
          ws.send(JSON.stringify({ type: 'error' }));
        }
      });
    });
  });

  afterAll(async () => {
    if (wsServer) {
      await new Promise(resolve => wsServer.close(resolve));
    }
  });

  beforeEach(async () => {
    wsClient = new WebSocket('ws://localhost:3002');
    await new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error('WebSocket connection timeout'));
      }, 1000);
      wsClient.on('open', () => {
        clearTimeout(timeout);
        resolve();
      });
    });
  });

  afterEach(() => {
    if (wsClient?.readyState === WebSocket.OPEN) {
      wsClient.close();
    }
  });

  it('handles client requests', async () => {
    wsClient.send(JSON.stringify({ type: 'get.active_calls' }));
    const message = await new Promise(resolve => {
      wsClient.once('message', data => resolve(JSON.parse(data.toString())));
    });
    expect(message.type).toBe('active_calls');
    expect(Array.isArray(message.data)).toBe(true);
  });

  it('handles invalid messages', async () => {
    wsClient.send('invalid json');
    const message = await new Promise(resolve => {
      wsClient.once('message', data => resolve(JSON.parse(data.toString())));
    });
    expect(message.type).toBe('error');
  });
});
