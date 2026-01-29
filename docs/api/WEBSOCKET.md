# WebSocket API Reference

Endpoint: `ws://localhost:8080/api/ws`

The WebSocket API provides real-time updates for calls, units, decode rates, and recorder status.

## Connection

```javascript
const ws = new WebSocket('ws://localhost:8080/api/ws');

ws.onopen = () => {
  console.log('Connected');
  // Subscribe to channels
};

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  console.log(msg.event, msg.data);
};

ws.onclose = () => {
  console.log('Disconnected');
};
```

## Configuration

| Setting | Value |
|---------|-------|
| Max message size | 512 KB |
| Ping interval | 54 seconds |
| Pong timeout | 60 seconds |
| Send buffer | 256 messages |

## Message Format

All messages are JSON with this structure:

```json
{
  "event": "event_type",
  "timestamp": 1705312200,
  "data": { ... }
}
```

## Subscribing to Channels

Send a subscription message after connecting:

```json
{
  "action": "subscribe",
  "channels": ["calls", "units"],
  "systems": ["warco"],
  "talkgroups": [9178, 9180],
  "units": [9001234, 9001235]
}
```

### Subscription Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| action | string | yes | `subscribe` or `unsubscribe` |
| channels | string[] | yes | Channels to subscribe to |
| systems | string[] | no | Filter by system short_name |
| talkgroups | int[] | no | Filter by talkgroup ID |
| units | int[] | no | Filter by unit ID |

### Available Channels

| Channel | Events | Description |
|---------|--------|-------------|
| `calls` | call_start, call_end, call_active, audio_available | Call lifecycle events |
| `units` | unit_event | Unit activity events |
| `rates` | rate_update | System decode rates |
| `recorders` | recorder_update | Recorder state changes |
| `*` | all | Subscribe to all events |

### Subscription Confirmation

Server responds with:

```json
{
  "event": "subscribed",
  "data": {
    "action": "subscribe",
    "channels": ["calls", "units"],
    "systems": ["warco"],
    "talkgroups": [9178],
    "units": []
  }
}
```

## Unsubscribing

```json
{
  "action": "unsubscribe",
  "channels": ["rates"]
}
```

## Event Types

### call_start

Fired when a new call begins.

```json
{
  "event": "call_start",
  "timestamp": 1705312200,
  "data": {
    "system": "warco",
    "system_id": 1,
    "talkgroup": 9178,
    "tg_alpha_tag": "09-8L Main",
    "unit": 9001234,
    "freq": 850387500,
    "encrypted": false,
    "emergency": false
  }
}
```

### call_end

Fired when a call ends.

```json
{
  "event": "call_end",
  "timestamp": 1705312245,
  "data": {
    "system": "warco",
    "system_id": 1,
    "talkgroup": 9178,
    "tg_alpha_tag": "09-8L Main",
    "start_time": "2024-01-15T10:30:00Z",
    "stop_time": "2024-01-15T10:30:45Z",
    "duration": 45.2,
    "freq": 850387500,
    "encrypted": false,
    "emergency": false
  }
}
```

### call_active

Periodic update for active calls.

```json
{
  "event": "call_active",
  "timestamp": 1705312220,
  "data": {
    "system": "warco",
    "system_id": 1,
    "talkgroup": 9178,
    "tg_alpha_tag": "09-8L Main",
    "unit": 9001234,
    "freq": 850387500,
    "elapsed": 20.5,
    "encrypted": false,
    "emergency": false
  }
}
```

### audio_available

Fired when audio file is ready for a completed call.

```json
{
  "event": "audio_available",
  "timestamp": 1705312250,
  "data": {
    "system": "warco",
    "system_id": 1,
    "call_id": 12345,
    "tr_call_id": "1705312200_850387500_9178",
    "talkgroup": 9178,
    "tg_alpha_tag": "09-8L Main",
    "audio_path": "warco/2024/1/15/9178-1705312200.m4a",
    "duration": 45.2
  }
}
```

### unit_event

Fired for unit activity (affiliations, registrations, etc.)

```json
{
  "event": "unit_event",
  "timestamp": 1705312200,
  "data": {
    "system": "warco",
    "system_id": 1,
    "unit": 9001234,
    "unit_alpha_tag": "Unit 123",
    "event_type": "call",
    "talkgroup": 9178,
    "tg_alpha_tag": "09-8L Main"
  }
}
```

**Event Types:**
| Type | Description |
|------|-------------|
| `on` | Unit registered on system |
| `off` | Unit deregistered |
| `join` | Unit affiliated with talkgroup |
| `call` | Unit transmitted on talkgroup |
| `ackresp` | Unit acknowledged |
| `end` | Unit transmission ended |
| `leave` | Unit left talkgroup |
| `data` | Data transmission |
| `status_update` | Status change |

### rate_update

Fired periodically with system decode rates.

```json
{
  "event": "rate_update",
  "timestamp": 1705312200,
  "data": {
    "system": "warco",
    "system_id": 1,
    "decode_rate": 98.5,
    "control_channel": 851012500
  }
}
```

### recorder_update

Fired when recorder state changes.

```json
{
  "event": "recorder_update",
  "timestamp": 1705312200,
  "data": {
    "system": "warco",
    "rec_num": 0,
    "state": 1,
    "state_name": "recording",
    "freq": 850387500,
    "talkgroup": 9178,
    "tg_alpha_tag": "09-8L Main",
    "unit": 9001234
  }
}
```

**Recorder States:**
| State | Name | Description |
|-------|------|-------------|
| 0 | available | Ready to record |
| 1 | recording | Currently recording |
| 2 | idle | Idle/monitoring |

## Filtering Logic

Filters are applied as follows:

1. **No channels subscribed** → No events sent
2. **Channel not subscribed** → Event filtered out
3. **Channels subscribed, no filters** → All events for those channels
4. **Filters present** → Event must match at least one value in each filter category

### Filter Examples

**All call events:**
```json
{
  "action": "subscribe",
  "channels": ["calls"]
}
```

**Calls from specific system:**
```json
{
  "action": "subscribe",
  "channels": ["calls"],
  "systems": ["warco"]
}
```

**Calls from specific talkgroups:**
```json
{
  "action": "subscribe",
  "channels": ["calls"],
  "talkgroups": [9178, 9180, 9182]
}
```

**Events for specific units:**
```json
{
  "action": "subscribe",
  "channels": ["calls", "units"],
  "units": [9001234, 9001235]
}
```

**Combined filters (system AND talkgroup):**
```json
{
  "action": "subscribe",
  "channels": ["calls"],
  "systems": ["warco"],
  "talkgroups": [9178]
}
```

## Complete Example

```javascript
class RadioWebSocket {
  constructor(url) {
    this.url = url;
    this.ws = null;
    this.handlers = {};
  }

  connect() {
    this.ws = new WebSocket(this.url);

    this.ws.onopen = () => {
      console.log('Connected to tr-engine');
      this.subscribe(['calls', 'units', 'rates']);
    };

    this.ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      this.dispatch(msg.event, msg.data, msg.timestamp);
    };

    this.ws.onclose = () => {
      console.log('Disconnected, reconnecting...');
      setTimeout(() => this.connect(), 5000);
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  }

  subscribe(channels, filters = {}) {
    this.ws.send(JSON.stringify({
      action: 'subscribe',
      channels,
      ...filters
    }));
  }

  unsubscribe(channels) {
    this.ws.send(JSON.stringify({
      action: 'unsubscribe',
      channels
    }));
  }

  on(event, handler) {
    if (!this.handlers[event]) {
      this.handlers[event] = [];
    }
    this.handlers[event].push(handler);
  }

  dispatch(event, data, timestamp) {
    const handlers = this.handlers[event] || [];
    handlers.forEach(h => h(data, timestamp));

    // Also dispatch to wildcard handlers
    const wildcardHandlers = this.handlers['*'] || [];
    wildcardHandlers.forEach(h => h(event, data, timestamp));
  }
}

// Usage
const radio = new RadioWebSocket('ws://localhost:8080/api/ws');

radio.on('call_start', (data) => {
  console.log(`New call on ${data.tg_alpha_tag}`);
});

radio.on('call_end', (data) => {
  console.log(`Call ended: ${data.duration}s`);
});

radio.on('audio_available', (data) => {
  // Audio ready for playback
  const audio = new Audio(`/api/v1/calls/${data.call_id}/audio`);
  audio.play();
});

radio.on('unit_event', (data) => {
  console.log(`Unit ${data.unit}: ${data.event_type}`);
});

radio.connect();
```

## Monitoring Multiple Talkgroups

```javascript
// Subscribe to specific talkgroups for monitoring
radio.subscribe(['calls'], {
  talkgroups: [9178, 9180, 9182, 9184]
});

radio.on('audio_available', (data) => {
  if (isMonitored(data.talkgroup)) {
    playAudio(data.call_id);
  }
});
```

## Heartbeat / Keep-Alive

The server sends WebSocket ping frames every 54 seconds. The browser handles pong responses automatically. If no pong is received within 60 seconds, the connection is closed.

For application-level keep-alive, you can track the last message time:

```javascript
let lastMessage = Date.now();

ws.onmessage = (event) => {
  lastMessage = Date.now();
  // ... handle message
};

// Check connection health
setInterval(() => {
  if (Date.now() - lastMessage > 120000) {
    console.log('No messages for 2 minutes, reconnecting...');
    ws.close();
  }
}, 30000);
```
