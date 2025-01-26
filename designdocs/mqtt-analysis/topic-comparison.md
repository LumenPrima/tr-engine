# MQTT Topic Data Handling Comparison

This document compares how different MQTT topics handle data in the trunk-recorder system, specifically analyzing `tr-mqtt_main_audio`, `tr-mqtt_main_call_end`, and `tr-mqtt_units_butco2_end`.

## Common Elements

All topics share these core fields:

- Basic message metadata:
  - `type` (different values per topic)
  - `timestamp`
  - `instance_id`

- Call information:
  - Frequency
  - Talkgroup details (ID, tag, description, group)
  - Unit information
  - Start/stop times
  - Error and spike counts
  - Emergency and encryption status

## Metadata and Data References

### Call Identification
- `tr-mqtt_main_audio`: Uses combination of talkgroup + timestamp in filename (e.g., "9131-1737430014_851350000.0-call_1571.wav")
- `tr-mqtt_main_call_end`: Uses structured ID "1_9131_1737430015" (format: sys_num_talkgroup_timestamp)
- `tr-mqtt_units_butco2_end`: References call through call_num and basic identifiers (no structured ID)

### File References
- `tr-mqtt_main_audio`: Points to final .wav file in call directory
- `tr-mqtt_main_call_end`: Points to processed .m4a file in date-structured directory
- `tr-mqtt_units_butco2_end`: Points to temporary .wav file in /dev/shm

### Talkgroup and Unit Identification
- `tr-mqtt_main_audio`:
  * Uses `talkgroup_tag` for short form (e.g., "Security")
  * Uses `talkgroup_description` for full name (e.g., "UC Health West Chester Hospital - Security")
  * Uses `talkgroup_group` for category (e.g., "Butler County (09) Fire/EMS/Hospitals")
  * Uses `talkgroup_group_tag` for category shorthand
  * No unit alpha tag field

- `tr-mqtt_main_call_end` and `tr-mqtt_units_butco2_end`:
  * Use `talkgroup_alpha_tag` instead of `talkgroup_tag` (same content, different field name)
  * Use same `talkgroup_description` and `talkgroup_group` fields
  * Include `unit_alpha_tag` field for unit identification
  * Include `talkgroup_patches` field (not present in audio topic)

### Data Relationships
- `tr-mqtt_main_audio`: 
  * Contains nested metadata structure with detailed signal and source information
  * Tracks multiple frequencies and sources over time through arrays
  * References same call through filename pattern
- `tr-mqtt_main_call_end`:
  * Uses call_id to uniquely identify the transmission
  * Links to final processed audio file
  * Contains state information about recording and monitoring
- `tr-mqtt_units_butco2_end`:
  * References transmission through basic identifiers (call_num, unit, talkgroup)
  * Points to temporary audio file during processing
  * Contains position and sample count for specific transmission segment

## Topic-Specific Handling

### tr-mqtt_main_audio

Primary purpose: Delivers audio data and comprehensive signal information

Unique features:
- Contains actual audio data (`audio_wav_base64`)
- Most detailed signal metadata:
  - Frequency error
  - Signal strength
  - Noise levels
- Tracks multiple frequencies and sources over time:
  - `freqList` array with timing and error data
  - `srcList` array with source changes
- Additional radio parameters:
  - Mode
  - Duplex settings
  - TDMA configuration

### tr-mqtt_main_call_end

Primary purpose: Provides comprehensive call completion status

Unique features:
- Extensive state tracking:
  - `call_state` and `call_state_type`
  - `mon_state` and `mon_state_type`
  - `rec_state` and `rec_state_type`
- Call processing metadata:
  - Unique call ID
  - Processing timestamp
  - Retry attempts
- Recording details:
  - Permanent call filename
  - Recording device numbers
  - Conventional/analog flags

### tr-mqtt_units_butco2_end

Primary purpose: Provides basic transmission completion information

Unique features:
- Simplified structure focused on essential data
- Transmission-specific fields:
  - Position in transmission
  - Sample count
  - Transmission length
- Uses temporary transmission filename
- Minimal overhead compared to other topics

## Missing Elements by Topic

### tr-mqtt_units_butco2_end missing:
- Signal quality metrics (signal, noise)
- State tracking fields
- Processing metadata
- Audio data
- Frequency and source lists

### tr-mqtt_main_call_end missing:
- Audio data
- Detailed frequency tracking
- Source tracking
- Sample count information
- Position in transmission

### tr-mqtt_main_audio missing:
- Call state tracking
- Processing retry information
- Sample count metrics
- Permanent vs temporary filename handling
- Recording state information

## Implications

1. The topics appear to serve different purposes in the system:
   - `tr-mqtt_main_audio`: Primary audio and signal data delivery
   - `tr-mqtt_main_call_end`: Call completion and recording status
   - `tr-mqtt_units_butco2_end`: Basic transmission completion notification

2. Data granularity varies significantly:
   - Most detailed: tr-mqtt_main_audio
   - Moderate detail: tr-mqtt_main_call_end
   - Minimal detail: tr-mqtt_units_butco2_end

3. The differences suggest a system design that separates concerns:
   - Audio handling
   - Call state management
   - Basic transmission tracking

## Recommended Standardization

The tr-mqtt_main_audio topic is the outlier in metadata field naming and structure. To standardize metadata handling across all topics, the following changes should be made to tr-mqtt_main_audio:

### Field Renames
- Change `talkgroup_tag` to `talkgroup_alpha_tag` to match other topics
- Remove `talkgroup_group_tag` as it's not used in other topics

### Missing Fields to Add
- Add `unit_alpha_tag` field for consistent unit identification
- Add `talkgroup_patches` field for patch tracking

### Benefits of Standardization
1. Consistent field naming across all topics
2. Simplified client-side processing (no need to check topic type for field names)
3. Better support for future features like talkgroup patches
4. More complete unit identification with alpha tags

### Implementation Notes
- Field additions should maintain backward compatibility
- Empty strings can be used as default values for new fields
- Existing consumers of talkgroup_tag will need to be updated to use talkgroup_alpha_tag
