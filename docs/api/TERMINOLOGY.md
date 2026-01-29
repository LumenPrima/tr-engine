# Radio System Terminology

This document explains the terminology used in tr-engine and trunk-recorder for developers unfamiliar with radio systems.

## Core Concepts

### System

A **system** is a radio network infrastructure. Examples include:

- A county's public safety radio system
- A city's municipal radio network
- A regional interoperability system

Systems have a **short_name** (e.g., "warco" for Warren County) used throughout the API.

**System Types:**
| Type | Description |
|------|-------------|
| `p25` | Project 25 digital trunked system |
| `smartnet` | Motorola SmartNet/SmartZone |
| `conventional` | Non-trunked system |

### Talkgroup

A **talkgroup** is a virtual channel that groups related users. Think of it like a chat room - everyone affiliated with a talkgroup hears each other.

**Examples:**
- "09-8L Main" (Middletown Police Dispatch)
- "Fire Dispatch"
- "EMS Operations"

**Key Fields:**
| Field | Description |
|-------|-------------|
| `tgid` | Numeric identifier (e.g., 9178) |
| `alpha_tag` | Human-readable name |
| `group` | Category (e.g., "Butler County Law") |
| `tag` | Type (e.g., "Law Dispatch", "Fire-Tac") |

### Unit

A **unit** is a radio device - typically a mobile (vehicle-mounted) or portable (handheld) radio. Units are identified by a numeric **Radio ID** (RID).

**Examples:**
- Unit 9001234 = "Engine 1"
- Unit 9002001 = "09-8L Unit 1" (Police car)

**Key Fields:**
| Field | Description |
|-------|-------------|
| `unit_id` / `unit_rid` | Radio ID number |
| `alpha_tag` | Human-readable name |

### Call

A **call** is a recorded audio session on a talkgroup. When someone presses their PTT (Push-To-Talk) button, a call is created. The call may contain multiple **transmissions** from different units.

**Important:** A "call" in tr-engine is NOT a telephone call. It's a recording session that captures radio traffic on a talkgroup.

**Key Fields:**
| Field | Description |
|-------|-------------|
| `tr_call_id` | Trunk-recorder's unique identifier |
| `start_time` | When recording started |
| `stop_time` | When recording ended (null if active) |
| `duration` | Length in seconds |
| `audio_path` | Path to audio file |

### Transmission

A **transmission** is a single unit keying up (pressing PTT) during a call. A call typically contains multiple transmissions from different units having a conversation.

**Example Call with 3 Transmissions:**
```
[0.0s]  Unit 9001234 (Dispatch): "Unit 123, respond to..."
[5.2s]  Unit 9001235 (Unit 123): "10-4, en route"
[8.5s]  Unit 9001234 (Dispatch): "Copy, Unit 123"
```

In the API, transmissions are returned with a `position` field indicating where in the audio they occur.

## Recording Concepts

### Recorder

A **recorder** is a virtual component that captures audio from a frequency. Trunk-recorder can run multiple recorders simultaneously to capture parallel conversations.

**Recorder States:**
| State | Description |
|-------|-------------|
| `available` | Ready to record new calls |
| `recording` | Currently capturing audio |
| `idle` | Monitoring but not recording |

### Source

A **source** is an SDR (Software Defined Radio) hardware device. Each source can monitor a range of frequencies.

### Instance

An **instance** is a running trunk-recorder process. Multiple instances can feed into a single tr-engine database.

## P25 Concepts

### TGID (Talkgroup ID)

The numeric identifier for a talkgroup. In P25 systems, TGIDs are typically 5-6 digits.

### WACN (Wide Area Communications Network)

A P25 identifier for a regional network. Multiple systems can share a WACN.

### NAC (Network Access Code)

A P25 identifier used for access control and system identification.

### RFSS (RF Sub-System)

A P25 concept for dividing large systems into manageable sections.

### Phase 2 TDMA

P25 Phase 2 uses Time Division Multiple Access (TDMA) to double channel capacity. Two calls can share one frequency by alternating time slots.

## Signal Quality

### Decode Rate

Percentage of control channel messages successfully decoded (0-100%). Higher is better.

```
98.5% = Excellent reception
90-98% = Good reception
<90% = Poor reception, may miss calls
```

### Error Count

Number of decode errors during a transmission. High error counts indicate poor signal quality.

### Spike Count

Number of audio spikes (pops/clicks) in the recording.

### Signal/Noise (dB)

- `signal_db`: Signal strength in decibels
- `noise_db`: Noise floor in decibels

## Event Types

### Unit Events

| Event | Description |
|-------|-------------|
| `on` | Unit powered on / registered with system |
| `off` | Unit powered off / deregistered |
| `join` | Unit affiliated with a talkgroup |
| `leave` | Unit left a talkgroup |
| `call` | Unit transmitted on a talkgroup |
| `end` | Unit's transmission ended |
| `ackresp` | Unit acknowledged a message |
| `data` | Unit sent data (not voice) |
| `status_update` | Unit sent status update |

### Call States

| State | Description |
|-------|-------------|
| Active | Call in progress (no `stop_time`) |
| Completed | Call ended with audio |
| Encrypted | Call was encrypted (no audio) |

## Audio Concepts

### Audio Types

| Type | Description |
|------|-------------|
| `digital` | Digitally encoded (P25, DMR) |
| `analog` | Traditional analog FM |

### srcList (Source List)

The list of transmissions in a call, including:
- Which unit transmitted
- When they started/stopped
- Position in the audio file

### freqList (Frequency List)

The list of frequencies used during a call. Calls may hop between frequencies.

## Deduplication

### Call Group

When multiple recorders capture the same transmission, tr-engine groups them into a **call group**. The best quality recording is marked as the **primary call**.

This prevents duplicate audio in the UI while preserving all captured data.

## ID Conventions

### Database IDs vs Radio IDs

tr-engine uses two types of identifiers:

| Type | Field Name | Description |
|------|------------|-------------|
| Database ID | `id` | Auto-incrementing integer, stable |
| Radio ID | `tgid`, `unit_id`, `unit_rid` | From the radio system |

**Example:**
```json
{
  "id": 1729,         // Database ID (use in URLs)
  "unit_id": 9001234  // Radio ID (what dispatchers see)
}
```

### tr_call_id Format

Trunk-recorder generates call IDs in this format:
```
{unix_timestamp}_{frequency}_{tgid}
```

Example: `1705312200_850387500_9178`
- `1705312200` = Unix timestamp (Jan 15, 2024 10:30:00)
- `850387500` = Frequency in Hz (850.3875 MHz)
- `9178` = Talkgroup ID

## Common Patterns

### Finding Active Calls

```javascript
// Active calls have no stop_time
const activeCalls = calls.filter(c => !c.stop_time);
```

### Identifying Emergency Traffic

```javascript
// Emergency flag indicates priority traffic
if (call.emergency) {
  highlightEmergency(call);
}
```

### Handling Encrypted Calls

```javascript
// Encrypted calls have no usable audio
if (call.encrypted) {
  showEncryptedIndicator();
} else {
  playAudio(call.id);
}
```

### Building Audio Player Timeline

```javascript
// Use transmissions to show who spoke when
const transmissions = await fetch(`/api/v1/calls/${id}/transmissions`);
transmissions.forEach(tx => {
  addTimelineMarker(tx.position, tx.unit_rid, tx.duration);
});
```

## Common Abbreviations

| Abbrev | Meaning |
|--------|---------|
| TG | Talkgroup |
| TGID | Talkgroup ID |
| RID | Radio ID |
| PTT | Push-To-Talk |
| SDR | Software Defined Radio |
| TR | Trunk-Recorder |
| P25 | Project 25 (digital radio standard) |
| TDMA | Time Division Multiple Access |
| NAC | Network Access Code |
| WACN | Wide Area Communications Network |
