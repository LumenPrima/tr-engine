# Data Models Reference

Complete reference for all data structures returned by the tr-engine API.

## Core Entities

### System

Represents a radio system (P25, SmartNet, etc.) being monitored.

```typescript
interface System {
  id: number;              // Database ID
  instance_id: number;     // Trunk-recorder instance
  sys_num: number;         // System number in TR config
  short_name: string;      // Short identifier (e.g., "warco")
  system_type?: string;    // "p25", "smartnet", "conventional"
  sysid?: string;          // P25 system ID (hex)
  wacn?: string;           // P25 WACN (hex)
  nac?: string;            // P25 NAC (hex)
  rfss?: number;           // P25 RFSS
  site_id?: number;        // Site ID
  config_json?: object;    // Full TR config
}
```

### Talkgroup

Represents a radio talkgroup (channel/group).

```typescript
interface Talkgroup {
  id: number;              // Database ID
  system_id: number;       // Parent system
  tgid: number;            // Talkgroup ID (e.g., 9178)
  alpha_tag?: string;      // Human name (e.g., "09-8L Main")
  description?: string;    // Description
  group?: string;          // Category (e.g., "Butler County Law")
  tag?: string;            // Type tag (e.g., "Law Dispatch")
  priority: number;        // Priority level
  mode?: string;           // "D" = digital, "A" = analog
  first_seen: string;      // ISO 8601 timestamp
  last_seen: string;       // ISO 8601 timestamp
}
```

### Unit

Represents a radio unit (mobile/portable radio).

```typescript
interface Unit {
  id: number;              // Database ID
  system_id: number;       // Parent system
  unit_id: number;         // Radio ID (e.g., 9001234)
  alpha_tag?: string;      // Human name (e.g., "Unit 123")
  alpha_tag_source?: string; // Source of tag (e.g., "radioreference")
  first_seen: string;      // ISO 8601 timestamp
  last_seen: string;       // ISO 8601 timestamp

  // Extended fields (from /units/active)
  last_event_type?: string;    // "call", "join", "on", etc.
  last_event_tgid?: number;    // Talkgroup of last event
  last_event_tg_tag?: string;  // Talkgroup name
  last_event_time?: string;    // ISO 8601 timestamp
}
```

### Call

Represents a recorded radio call (audio recording session).

```typescript
interface Call {
  id: number;              // Database ID
  call_group_id?: number;  // Deduplication group
  instance_id: number;     // TR instance
  system_id: number;       // Parent system
  talkgroup_id?: number;   // FK to talkgroups table
  recorder_id?: number;    // Recorder that captured

  tr_call_id?: string;     // TR's call ID (e.g., "1705312200_850387500_9178")
  call_num: number;        // Call number

  start_time: string;      // ISO 8601 timestamp
  stop_time?: string;      // ISO 8601 timestamp (null if active)
  duration: number;        // Seconds

  call_state: number;      // Internal state
  mon_state: number;       // Monitor state

  encrypted: boolean;      // Is encrypted
  emergency: boolean;      // Emergency flag
  phase2_tdma: boolean;    // P25 Phase 2
  tdma_slot: number;       // TDMA slot
  conventional: boolean;   // Conventional system
  analog: boolean;         // Analog audio

  audio_type: string;      // "digital", "analog"
  freq: number;            // Frequency in Hz
  freq_error: number;      // Frequency error
  error_count: number;     // Decode errors
  spike_count: number;     // Audio spikes
  signal_db: number;       // Signal strength
  noise_db: number;        // Noise level

  audio_path?: string;     // Relative path to audio file
  audio_size: number;      // File size in bytes

  patched_tgids?: number[]; // Patched talkgroups
  metadata_json?: object;  // Additional metadata

  // Joined fields (from list endpoints)
  tgid?: number;           // Actual talkgroup ID
  tg_alpha_tag?: string;   // Talkgroup name
  units?: CallUnit[];      // Units in this call
}
```

### CallUnit

Unit information joined to a call.

```typescript
interface CallUnit {
  unit_rid: number;        // Radio ID
  alpha_tag: string;       // Unit name
}
```

### Transmission

Individual unit transmission within a call (from srcList).

```typescript
interface Transmission {
  id: number;              // Database ID
  call_id: number;         // Parent call
  unit_id?: number;        // FK to units table
  unit_rid: number;        // Radio ID
  start_time: string;      // ISO 8601 timestamp
  stop_time?: string;      // ISO 8601 timestamp
  duration: number;        // Seconds
  position: number;        // Position in audio (seconds)
  emergency: boolean;      // Emergency flag
  error_count: number;     // Decode errors
  spike_count: number;     // Audio spikes
}
```

### CallFrequency

Frequency usage during a call (from freqList).

```typescript
interface CallFrequency {
  id: number;              // Database ID
  call_id: number;         // Parent call
  freq: number;            // Frequency in Hz
  time: string;            // ISO 8601 timestamp
  position: number;        // Position in audio (seconds)
  duration: number;        // Duration on this frequency
  error_count: number;     // Decode errors
  spike_count: number;     // Audio spikes
}
```

### UnitEvent

Unit activity event (affiliation, registration, etc.).

```typescript
interface UnitEvent {
  id: number;              // Database ID
  instance_id: number;     // TR instance
  system_id: number;       // Parent system
  unit_id?: number;        // FK to units table
  unit_rid: number;        // Radio ID
  event_type: string;      // Event type (see below)
  talkgroup_id?: number;   // FK to talkgroups table
  tgid: number;            // Actual talkgroup ID
  time: string;            // ISO 8601 timestamp
  metadata_json?: object;  // Additional data
}

type EventType =
  | 'on'           // Unit registered on system
  | 'off'          // Unit deregistered
  | 'join'         // Unit affiliated with talkgroup
  | 'call'         // Unit transmitted
  | 'ackresp'      // Unit acknowledged
  | 'end'          // Transmission ended
  | 'leave'        // Unit left talkgroup
  | 'data'         // Data transmission
  | 'status_update'; // Status change
```

### CallGroup

Deduplicated group of calls from multiple recorders.

```typescript
interface CallGroup {
  id: number;              // Database ID
  system_id: number;       // Parent system
  talkgroup_id?: number;   // FK to talkgroups
  tgid: number;            // Actual talkgroup ID
  start_time: string;      // ISO 8601 timestamp
  end_time?: string;       // ISO 8601 timestamp
  primary_call_id?: number; // Best quality call
  call_count: number;      // Number of duplicate calls
  encrypted: boolean;      // Is encrypted
  emergency: boolean;      // Emergency flag
}
```

## Infrastructure Entities

### Instance

A trunk-recorder instance sending data.

```typescript
interface Instance {
  id: number;              // Database ID
  instance_id: string;     // TR instance identifier
  instance_key?: string;   // Authentication key
  first_seen: string;      // ISO 8601 timestamp
  last_seen: string;       // ISO 8601 timestamp
  config_json?: object;    // TR configuration
}
```

### Source

An SDR hardware source.

```typescript
interface Source {
  id: number;              // Database ID
  instance_id: number;     // Parent instance
  source_num: number;      // Source number
  center_freq: number;     // Center frequency (Hz)
  rate: number;            // Sample rate
  driver: string;          // "osmosdr", etc.
  device: string;          // Device identifier
  antenna: string;         // Antenna
  gain: number;            // Gain (dB)
  config_json?: object;    // Full config
}
```

### Recorder

A virtual recorder associated with a source.

```typescript
interface Recorder {
  id: number;              // Database ID
  instance_id: number;     // Parent instance
  source_id?: number;      // Parent source
  rec_num: number;         // Recorder number
  rec_type: string;        // "p25", "dmr", "nxdn", etc.
}
```

### RecorderStatus

Recorder state at a point in time.

```typescript
interface RecorderStatus {
  id: number;              // Database ID
  recorder_id: number;     // Parent recorder
  time: string;            // ISO 8601 timestamp
  state: number;           // State code (see below)
  freq?: number;           // Current frequency (Hz)
  call_count: number;      // Calls recorded
  duration: number;        // Recording duration
  squelched: boolean;      // Is squelched
}

// Recorder state codes
const RecorderState = {
  AVAILABLE: 0,   // Ready to record
  RECORDING: 1,   // Currently recording
  IDLE: 2         // Idle/monitoring
};
```

### SystemRate

System decode rate measurement.

```typescript
interface SystemRate {
  id: number;              // Database ID
  system_id: number;       // Parent system
  time: string;            // ISO 8601 timestamp
  decode_rate: number;     // Percentage (0-100)
  control_channel: number; // Control channel freq (Hz)
}
```

## Response Wrappers

### List Response

Most list endpoints return data in this format:

```typescript
interface ListResponse<T> {
  [key: string]: T[];      // e.g., "calls", "units", "talkgroups"
  count: number;           // Items in this response
  limit: number;           // Requested limit
  offset: number;          // Requested offset
}

// Example
interface CallListResponse {
  calls: Call[];
  count: number;
  limit: number;
  offset: number;
}
```

### Stats Response

```typescript
interface StatsResponse {
  total_systems: number;
  total_talkgroups: number;
  total_units: number;
  total_calls: number;
  active_calls: number;
  calls_last_hour: number;
  calls_last_24h: number;
  audio_files: number;
  audio_bytes: number;
}
```

### Activity Response

```typescript
interface ActivityResponse {
  systems: number;
  talkgroups: number;
  units: number;
  calls_24h: number;
  system_activity: Array<{
    system: string;
    call_count: number;
  }>;
}
```

### Encryption Stats Response

```typescript
interface EncryptionStatsResponse {
  stats: {
    [tgid: string]: {
      encrypted: number;
      clear: number;
    };
  };
  hours: number;
}
```

### Recent Call Response

Extended call format from `/calls/recent`:

```typescript
interface RecentCallInfo {
  id: number;
  call_id: number;
  tr_call_id?: string;
  call_num: number;
  start_time: string;
  stop_time?: string;
  duration: number;
  system: string;          // System short_name
  tgid: number;
  tg_alpha_tag?: string;
  freq: number;
  encrypted: boolean;
  emergency: boolean;
  audio_path?: string;
  has_audio: boolean;
  units: Array<{
    unit_id: number;       // Radio ID
    unit_tag: string;      // Unit name
  }>;
}
```

## Null Handling

- String fields that may be null: `alpha_tag`, `description`, `group`, `tag`, `mode`, `audio_path`, `tr_call_id`
- Timestamp fields that may be null: `stop_time`, `last_event_time`
- Number fields that may be null are typically represented as `0` or omitted

Always check for null/undefined when accessing optional fields:

```javascript
const name = talkgroup.alpha_tag || `TG ${talkgroup.tgid}`;
const isActive = call.stop_time === null;
```

## Frequency Values

Frequencies are stored in Hz (not MHz):

```javascript
// Convert Hz to MHz for display
const freqMHz = call.freq / 1000000;
console.log(`${freqMHz.toFixed(4)} MHz`); // "850.3875 MHz"
```

## Timestamps

All timestamps are ISO 8601 format in UTC:

```javascript
const date = new Date(call.start_time);
console.log(date.toLocaleString()); // Local time
```
