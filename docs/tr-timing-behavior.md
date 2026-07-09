# trunk-recorder Timing Behavior

trunk-recorder generates events in real time from multiple subsystems (recorder, control channel parser, audio pipeline). Those subsystems do not always agree on timestamps or ordering. Frontends that build timelines, track active calls, or associate unit activity with calls need to account for these behaviors.

tr-engine has built-in mitigations for all known timing issues. This document describes the behaviors and the mitigations so frontend authors can understand why Event Horizon and other dashboards handle events the way they do.

## Call ID Stability

trunk-recorder encodes its internal call ID as:

```
{sys_num}_{tgid}_{start_time}
```

where `start_time` is a Unix epoch timestamp in seconds.

### The Problem

The `start_time` field can shift by **1-2 seconds** between the `call_start` and `call_end` messages for the same call. Because the call ID embeds `start_time`, the ID itself changes. On analog systems the shift can be larger (up to ~6 seconds).

### Why It Happens

trunk-recorder generates the call ID independently at the start and end of a call. The `start_time` field is not perfectly stable across those two moments.

### Impact

Exact call-ID-based lookup fails. The `call_end` handler cannot find the active call if it searches by the exact TR call ID received in the `call_end` message.

### Mitigation in tr-engine

The active-calls map uses fuzzy matching by `(tgid, start_time ± 10 seconds)` instead of exact call-ID lookup. When multiple calls match, it prefers the call whose start time is at or before the reported time, breaking ties by closest time difference. This prevents accidentally matching a newer back-to-back call on the same talkgroup.

**Code reference:** `internal/ingest/handler_calls.go` — `FindByTgidAndTime` with a `10*time.Second` tolerance; `internal/ingest/pipeline.go` — `activeCallMap.FindByTgidAndTime` implementation.

## unit_event:end Lag

The `unit_event:end` message arrives **3-4 seconds** after the `call_end` message for the same call.

### Why It Happens

- `call_end` fires from the **recorder** when voice frames stop. This is immediate.
- `unit_event:end` fires from the **control channel parser** when it sees the deaffiliation message. This is delayed by the P25 trunking update cycle.

The P25 channel stays allocated during hang time, but the actual voice traffic stops immediately.

### Impact

Frontends that associate unit activity with calls see a gap: the call is already over, but the unit still appears active. Without handling, a unit can look "stuck" on a call for several seconds after it ended.

### Mitigation in tr-engine

Event Horizon uses a **6-second coalesce window** to pair unit events with calls. When a `unit_event:call` or `unit_event:end` arrives within 6 seconds of a `call_start` or `call_end` on the same talkgroup, the frontend treats them as part of the same call and merges them visually instead of showing separate timeline entries.

**Code reference:** `web/timeline.html` — `COALESCE_WINDOW = 6000` and the `shouldCoalesce` function.

## Active Call Matching

### How tr-engine matches `call_start` to active calls

When a `call_start` message arrives, tr-engine resolves the system and site identity, then checks whether an active call already exists for the same `(system_id, tgid, start_time)`. If a call with a fuzzy-matching start time is already in the active map, the existing call is reused rather than creating a duplicate.

### How `call_end` matches

As described above, `call_end` first tries an exact TR call ID lookup in the active map. If that fails, it falls back to fuzzy lookup by `(tgid, start_time ± 10s)`. Only if both lookups fail does it query the database (for calls that started before the current process was running).

### Identity Resolution

Systems are identified by:

- **P25 / smartnet:** `(sysid, wacn)`
- **Conventional:** `(instance_id, sys_name)`

Sites are identified by `(system_id, instance_id, sys_name)`. `sys_num` is never used for identity because it is a positional index that can shift across restarts.

### Warmup Gate

On a fresh start, tr-engine buffers non-identity MQTT messages until system registration establishes the real P25 `(sysid, wacn)`. This prevents duplicate system creation from early calls that arrive before the first `system_info` or `config` message. Conventional systems release the gate immediately when their type is detected (there is no `sysid` to wait for). A 5-second timeout acts as a fallback if no system info arrives.

On restarts, the gate is skipped because the identity cache is loaded from the database.

**Code reference:** `internal/ingest/identity.go` — `IdentityResolver.Resolve` and warmup logic in the pipeline startup.

## Summary Table

| Behavior | Window | Affected Components | Mitigation |
|----------|--------|---------------------|------------|
| Call ID shift | 1-2s (up to ~6s analog) | `call_end` lookup | Fuzzy matching by `(tgid, start_time ± 10s)` |
| `unit_event:end` lag | 3-4s | Unit-call association | 6s coalesce window (Event Horizon timeline) |

## Frontend Recommendations

1. **Do not trust exact call IDs** for correlating `call_start` and `call_end` events. Use `(system_id, tgid, start_time)` with a tolerance window.
2. **Coalesce unit events with call events** on the same talkgroup within ~6 seconds. This prevents units from appearing active after their call has ended.
3. **Expect out-of-order events** within small windows. The SSE stream may deliver a `call_end` slightly before its matching `unit_event:end`.
4. **Use server-side timestamps cautiously.** The `start_time` in TR messages is the source of truth for call boundaries, but it can drift. tr-engine normalizes and stores its own `start_time` values in the database.
