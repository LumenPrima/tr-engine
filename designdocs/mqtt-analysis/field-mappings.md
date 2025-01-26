# MQTT Topic Field Mappings

## Message Categories

### System Messages
- tr-mqtt/main/systems: System configuration and status
- tr-mqtt/main/rates: System performance metrics
- tr-mqtt/main/trunk_recorder/status: Connection status
- tr-mqtt/main/recorder: Individual recorder status

### Call Messages
- tr-mqtt/main/calls_active: Full call information
- tr-mqtt/main/audio: Call audio with metadata
- tr-mqtt/main/call_start/end: Call lifecycle events

### Unit Messages
- tr-mqtt/units/{system}/call: Call information
- tr-mqtt/units/{system}/location: Unit location
- tr-mqtt/units/{system}/data: Basic unit info
- tr-mqtt/units/{system}/join: Talkgroup joins

## System-Level Messages

### tr-mqtt/main/systems
```json
{
  "systems": [
    {
      "sys_num": 0,                // System identifier
      "sys_name": "warco2",        // Canonical system name
      "type": "p25",
      "sysid": "348",
      "wacn": "BEE00",
      "nac": "34D",
      "rfss": 1,
      "site_id": 13
    }
  ]
}
```

### tr-mqtt/main/rates
```json
{
  "rates": [
    {
      "sys_num": 0,                // Matches systems message
      "sys_name": "warco2",        // Matches systems message
      "decoderate": 40,
      "decoderate_interval": 3,
      "control_channel": 859762500
    }
  ]
}
```

### tr-mqtt/main/recorder
```json
{
  "recorder": {
    "id": "0_0",
    "src_num": 0,
    "rec_num": 0,
    "type": "P25",
    "duration": 3243.06,
    "freq": 851350000,
    "count": 462,
    "rec_state": 4,
    "rec_state_type": "IDLE",      // Uses _type suffix pattern
    "squelched": true
  }
}
```

### tr-mqtt/main/trunk_recorder/status
```json
{
  "client_id": "tr-status-45048230",
  "instance_id": "trunk-recorder",
  "status": "connected"
}
```

## Call-Related Messages

### tr-mqtt/main/calls_active
```json
{
  "talkgroup_alpha_tag": "09 TA 02",        // Display name
  "talkgroup_tag": "Transportation",         // Category
  "talkgroup_description": "County Regional Transit 2",
  "talkgroup_group": "Butler County (09) Local Government/Public Works",
  "talkgroup_patches": "",
  "unit_alpha_tag": "",
  "call_state_type": "RECORDING",           // Uses _type suffix pattern
  "mon_state_type": "UNSPECIFIED",          // Uses _type suffix pattern
  "rec_state_type": "RECORDING",            // Uses _type suffix pattern
  "sys_name": "butco2"                      // Should match systems message
}
```

### tr-mqtt/main/audio
```json
{
  "metadata": {
    "talkgroup_tag": "09 WC HOSP SEC",      // Maps to talkgroup_alpha_tag
    "talkgroup_group_tag": "Security",       // Maps to talkgroup_tag
    "talkgroup_description": "UC Health West Chester Hospital - Security",
    "talkgroup_group": "Butler County (09) Fire/EMS/Hospitals",
    "short_name": "butco2"                   // Should be sys_name
  }
}
```

## Unit Messages

### tr-mqtt/units/{system}/call
```json
{
  "call": {
    "sys_name": "butco2",                   // Matches systems message
    "unit_alpha_tag": "",
    "talkgroup_alpha_tag": "09 WC HOSP SEC",
    "talkgroup_tag": "Security",
    "talkgroup_description": "UC Health West Chester Hospital - Security",
    "talkgroup_group": "Butler County (09) Fire/EMS/Hospitals"
  }
}
```

## Field Patterns

1. System Identification:
   - `sys_name`: Canonical system name (used in systems, rates, unit messages)
   - `sys_num`: Numeric system identifier
   - `short_name`: Legacy field in some messages (should be replaced with sys_name)

2. Message Structure:
   - Most messages follow type-based nesting pattern:
     ```json
     {
       "type": "message_type",
       "message_type": { ... }
     }
     ```
   - Audio message uses "metadata" instead of type-based nesting

3. State Fields:
   - Consistent use of `_type` suffix for state fields:
     - `rec_state_type`
     - `call_state_type`
     - `mon_state_type`

4. Talkgroup Fields:
   - Display name: `talkgroup_alpha_tag` or `talkgroup_tag`
   - Category: `talkgroup_tag` or `talkgroup_group_tag`
   - Description: Always `talkgroup_description`
   - Group: Always `talkgroup_group`

## Recommendations

1. System Name Standardization:
   - Use `sys_name` everywhere (as shown in systems/rates messages)
   - Remove `short_name` usage
   - Keep `sys_num` for system identification
   - Validate against systems list

2. Message Structure:
   - Standardize on type-based nesting for all messages
   - Convert audio message to use type nesting:
     ```json
     {
       "type": "audio",
       "audio": {
         // current metadata contents
       }
     }
     ```

3. Field Naming:
   - Use `talkgroup_alpha_tag` for display names
   - Use `talkgroup_tag` for categories
   - Keep `_type` suffix for state fields
   - Use consistent field names across all messages

## Implementation Notes

1. System Validation:
```javascript
// Get valid systems from systems message
const validSystems = new Set(systems.map(sys => sys.sys_name));

// Validate system name
const systemName = message.sys_name || message.short_name || message.metadata?.short_name;
if (!validSystems.has(systemName)) {
    console.warn(`Unknown system: ${systemName}`);
}
```

2. Field Mapping:
```javascript
// Map system name
const systemName = message.sys_name || message.short_name || message.metadata?.short_name;

// Map talkgroup display name
const displayName = message.talkgroup_alpha_tag || 
                   message.talkgroup_tag ||
                   message.metadata?.talkgroup_tag;

// Map talkgroup category
const category = message.talkgroup_tag ||
                message.talkgroup_group_tag ||
                message.metadata?.talkgroup_group_tag;

// Map state types
const recorderState = message.rec_state_type || 
                     message.recorder?.rec_state_type;
```

3. Message Processing:
- Extract message type from topic
- Handle appropriate nesting level
- Map fields to consistent names
- Validate system names against systems list
- Preserve original message for debugging
