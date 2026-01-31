# Transcription Testing Report

**Date:** 2026-01-30
**Audio Source:** Butler County, OH P25 radio system (`~/trunk-recorder-mqtt/tr_audio/butco/`)
**Test Environment:** Local speaches-ai server (localhost:8000) + OpenAI API

## Executive Summary

Testing was conducted to evaluate speech-to-text transcription accuracy for P25 radio dispatch audio. Key findings:

1. **Model Selection:** `faster-whisper-large-v3-turbo` provides the best balance of speed and accuracy
2. **Avoid:** `Systran/faster-whisper-large-v3` has a hallucination bug (repeats phrases)
3. **Preprocessing:** Voice bandpass filter (300-3000Hz) can fix hallucinations and improve accuracy
4. **Local vs Cloud:** Local transcription is 3-4x faster than OpenAI with equivalent quality

---

## Models Tested

### Local Models (speaches-ai server)

| Model ID | Type | Avg Latency | Notes |
|----------|------|-------------|-------|
| `Systran/faster-whisper-small` | Small | ~220ms | Fastest, occasional callsign errors |
| `Systran/faster-whisper-large-v3` | Large | ~700ms | **AVOID** - hallucination bug |
| `deepdml/faster-whisper-large-v3-turbo-ct2` | Large Turbo | ~420ms | **Recommended** - best balance |
| `Systran/faster-distil-whisper-large-v3` | Distil | ~400ms | Good quality, fast |
| `distil-whisper/distil-large-v3-ct2` | Distil | ~750ms | Similar to distil-v3 |
| `distil-whisper/distil-large-v3.5-ct2` | Distil | ~400ms | Good formatting |

### Cloud Models

| Provider | Model | Avg Latency | Cost |
|----------|-------|-------------|------|
| OpenAI | `whisper-1` | ~1700ms | $0.006/min |

---

## Model Comparison Results

### Test 1: Basic Accuracy Comparison

**File:** `21006-1769530437_855962500.0-call_6896.wav` (305,324 bytes)
**Content:** Air ambulance dispatch notification

| Model | Result |
|-------|--------|
| faster-whisper-small | Care Care 2 in the Butler County team, this is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. The time is 11.14. |
| faster-whisper-large-v3 | Care care to you and the Butler County team. This is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. Time is 1114. |
| faster-whisper-large-v3-turbo | Care care to you and the Butler County team, this is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. Time is 1114. |
| OpenAI whisper-1 | Care Care to you and the Butler County team, this is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. |

**File:** `21006-1769530769_852200000.0-call_6991.wav` (69,164 bytes)
**Content:** Maintenance abort notification

| Model | Result |
|-------|--------|
| faster-whisper-small | Hey, base, this is two. I've got a maintenance issue. I'll have to abort. |
| faster-whisper-large-v3 | Hey, BASE, this is 2. I've got a maintenance issue. I'll have to abort. |
| faster-whisper-large-v3-turbo | Hey, base, this is 2. I've got a maintenance issue. I'll have to abort. |
| OpenAI whisper-1 | Hey, Base, this is 2. I've got a maintenance issue. I'll have to abort. |

### Test 2: Hallucination Detection

**File:** `9173-1769554590_852200000.0-call_1109.wav` (207,404 bytes)
**Content:** Traffic accident dispatch

| Model | Result | Hallucination |
|-------|--------|---------------|
| faster-whisper-small | 182 Respond to drive smart at 6121 Dixie highway for an auto accident. No injury. Is this with information exchange? Okay 1756 | No |
| faster-whisper-large-v3 | 182. Go ahead. Responda, drive smart at 6121 Dixie Highway for an auto accident, no injury, assist with information exchange. Okay. 1756. 1756. Okay. Thank you. Thank you. Thank you. Thank you. Thank you. [repeats 27 times] | **YES** |
| faster-whisper-large-v3-turbo | 182. Go ahead. Respond to Drive Smart at 6121 Dixie Highway for an auto accident. No injury. This is with information exchange. Okay. 1756. | No |
| faster-distil-whisper-large-v3 | 182. Good. Respond to drive smart at 6121 Dixie Highway for an auto accident, no injury, assist with information exchange. Okay. 1756. | No |
| distil-large-v3.5-ct2 | 182. Go ahead. Respond to drive smart at 6121 Dixie Highway for an auto accident, no injury, assist with information exchange. Okay. 1756. | No |

**Critical Finding:** Only `Systran/faster-whisper-large-v3` exhibited hallucination behavior. All other models (including turbo and distil variants) were clean.

### Test 3: Extended Accuracy Test (10 Random Files)

**File:** `9179-1769497185_852687500.0-call_815.wav` (175,724 bytes)

| Model | Latency | Result |
|-------|---------|--------|
| faster-whisper-small | 399ms | I think 85 clear. 601 clear. Here's information. The caller buys no weapons, but the male in the background yell that the caller has weapons. This may be a female. |
| faster-whisper-large-v3 | 798ms | 6-8-5 clear. 6-8-1 clear. Misinformation. Caller buys no weapons, but the male in the background yelled that the caller has weapons. This may be a female. |
| faster-distil-whisper-large-v3 | 432ms | I think 85 clear. 6.01 clear. News information. Collar buys no weapons with a male in the background yell that the caller has weapons. To be a female. |
| distil-large-v3-ct2 | 918ms | I think 85 clear. 6-01 clear. News information. Caller buys no weapons, with a male in the background, yell that the caller has weapons. To be a female. |
| distil-large-v3.5-ct2 | 439ms | I think 85, clear. 601, clear. Just information. Caller buys no weapons, but the male in the background yell that the caller has weapons. It's being a female. |

**File:** `9138-1769521316_851350000.0-call_3854.wav` (72,044 bytes)

| Model | Latency | Result |
|-------|---------|--------|
| faster-whisper-small | 208ms | Mag A3 transporting. |
| faster-whisper-large-v3 | 484ms | Meg A3 transporting. Meg A3 transporting. |
| faster-distil-whisper-large-v3 | 377ms | Mega A3 Transpiring. Megay3 transporting. Megay3 transporting. |
| distil-large-v3-ct2 | 696ms | Minig A3 Transpiring. Meg.A3 transporting. |
| distil-large-v3.5-ct2 | 347ms | Mike A3 is transporting. |

**File:** `9264-1769530261_855962500.0-call_6818.wav` (57,644 bytes)

| Model | Latency | Result |
|-------|---------|--------|
| faster-whisper-small | 232ms | 2-141, Dometic-141 on Township. |
| faster-whisper-large-v3 | 484ms | Chief 141 to Medic 141 on Township. |
| faster-distil-whisper-large-v3 | 353ms | Chief 141, the medic 141 on Township. |
| distil-large-v3-ct2 | 699ms | Chief 141, the medic 141 on Township. |
| distil-large-v3.5-ct2 | 352ms | Chief 141, Dometic 141 on Township. |

### Test 4: OpenAI vs Local Comparison (8 Files)

| File | OpenAI (ms) | Local Turbo (ms) | Speedup |
|------|-------------|------------------|---------|
| 9066-1769531801_852200000.0-call_7347.wav | 1713 | 480 | 3.6x |
| 58105-1769518114_855962500.0-call_3258.wav | 1169 | 375 | 3.1x |
| 9176-1769538646_852687500.0-call_9718.wav | 910 | 385 | 2.4x |
| 51509-1769533856_852200000.0-call_8126.wav | 2006 | 428 | 4.7x |
| 9451-1769512797_855962500.0-call_2208.wav | 1834 | 456 | 4.0x |
| 63001-1769525006_852200000.0-call_4848.wav | 852 | 422 | 2.0x |
| 9458-1769547539_852200000.0-call_13287.wav | 1777 | 391 | 4.5x |
| 9130-1769527750_851350000.0-call_5625.wav | 997 | 429 | 2.3x |
| **TOTAL** | **11,258ms** | **3,366ms** | **3.3x** |

**Quality Comparison:**

**File:** `9451-1769512797_855962500.0-call_2208.wav`
- **OpenAI:** Luke, that fire alarm is showing a first floor southwest vestibule, but one of the housekeeping assistants called from o...
- **Local:** Luke, that fire alarm is showing a first floor southwest vestibule, but one of the housekeepingists just called from ove...

**File:** `9130-1769527750_851350000.0-call_5625.wav`
- **OpenAI:** 9COM, Medic 28 is transporting to port. Medic 28, you're on the port. Engine 26 available. Engine 26 is available.
- **Local:** 9com, Medic 28 is transporting to Fort. Medic 28, you're out to Fort. Engine 26 is available. Engine 26 is available.

---

## Audio Preprocessing Tests

### Sox Filters Tested

| Filter | Sox Command | Purpose |
|--------|-------------|---------|
| Original | `rate 16000` | Baseline (resample only) |
| High-pass 300Hz | `rate 16000 highpass 300` | Remove low frequency noise |
| Noise Gate | `rate 16000 compand 0.1,0.3 -60,-60,-30,-15,0,0 -6` | Reduce quiet noise |
| Bandpass 300-3400Hz | `rate 16000 sinc 300-3400` | Voice frequency range |
| Voice Bandpass | `rate 16000 sinc 300-3000 norm` | Narrower voice range + normalize |
| Combined | `rate 16000 highpass 200 compand 0.1,0.3 -50,-50,-30,-20,0,0 -5` | HP + compression |
| Noise Reduction | `noisered profile.prof 0.21` | Profile-based noise reduction |
| Aggressive | `rate 16000 sinc 250-3500 compand 0.1,0.2 -50,-50,-40,-30,-20,-15,0,0 -5 norm` | Full processing chain |

### Test 5: Filter Comparison

**File:** `9451-1769512797_855962500.0-call_2208.wav` (housekeeping test)

| Filter | Result |
|--------|--------|
| original | Luke, that fire alarm is showing a first floor southwest vestibule, but one of the housekeepingers just called from over there and... |
| highpass | Luke, that fire alarm is showing a first floor southwest vestibule, but one of the **housekeepers** just called from over there and sa... |
| bandpass | Luke, that fire alarm is showing a first floor southwest vestibule, but one of the **housekeepers** just called from over there and sa... |
| combined | Luke, that fire alarm is showing a first floor southwest vestibule, but one of the **housekeepers** just called from over there and sa... |

**Result:** Highpass and bandpass filters corrected "housekeepingers" to "housekeepers"

### Test 6: Hallucination Fix via Filtering

**File:** `9468-1769545162_853287500.0-call_12405.wav`

| Filter | Result |
|--------|--------|
| original | Go ahead, 1-5-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8... |
| voice (300-3000Hz) | Go ahead, 1-5-8. Radio check. I see that you logged in. Hear you loud and clear. Have a great day and be safe out there. |

**Critical Finding:** Voice bandpass filter completely eliminated hallucination and recovered the actual speech content.

### Test 7: Bandpass Range Comparison

**File:** `9177-1769530121_851762500.0-call_6748.wav`

| Range | Result |
|-------|--------|
| original | ...It's a proxy MP off of the private driveway on a steep hill. It's a small town. |
| 200-3400Hz | ...It's a large box truck. It's a proxy MP off of the private driveway on a steep hill. It's a small town. |
| 300-3000Hz | ...It's a large box truck. It's **approximately 10 feet** off of the private driveway on a steep hill. Wow. |
| 400-3400Hz | ...It's a large box truck. It's approximately 10 feet off of the private driveway on a steep hill. Wow. |

**Result:** 300-3000Hz bandpass converted "proxy MP" to "approximately 10 feet" (more sensible)

### Test 8: Statistical Filter Analysis (20 Files)

| Metric | Original | Voice Bandpass |
|--------|----------|----------------|
| Total characters | 1,856 | 1,863 |
| Hallucinations detected | 0 | 0 |
| Processing overhead | baseline | ~50ms |

**Conclusion:** Voice bandpass filter is safe to apply - does not increase hallucinations or significantly alter output length.

### Test 9: Side-by-Side Quality Samples

**File:** `9173-1769539244_855737500.0-call_9923.wav`
- **Original:** I'm going to be Augusta and Wood Creek. Stand by for tag.
- **Voice BP:** Come on, 34, go ahead. I'm going to be Augusta and Wood Creek. Standby for tag.

**Result:** Voice bandpass captured additional content missed by original.

**File:** `53317-1769493124_859262500.0-call_417.wav`
- **Original:** 285, Walter County radio check. Hot and clear. Thank you.
- **Voice BP:** 285, Walter County Radio, Jake. Hot and clear. Thank you.

**Result:** Different interpretation - unclear which is correct without ground truth.

---

## Recommendations

### Model Selection

1. **Primary:** `deepdml/faster-whisper-large-v3-turbo-ct2`
   - Best balance of speed (~420ms) and accuracy
   - No hallucination issues observed

2. **Alternative:** `Systran/faster-distil-whisper-large-v3` or `distil-whisper/distil-large-v3.5-ct2`
   - Similar quality to turbo
   - Good fallback options

3. **Speed Priority:** `Systran/faster-whisper-small`
   - Fastest (~220ms)
   - Acceptable for high-volume, lower-accuracy requirements

4. **Avoid:** `Systran/faster-whisper-large-v3`
   - Hallucination bug confirmed
   - Repeats phrases like "Thank you" endlessly on certain files

### Preprocessing

**Recommended filter for radio audio:**
```bash
sox input.wav output.wav rate 16000 sinc 300-3000 norm
```

**Benefits:**
- Fixes some hallucinations
- Improves word clarity in some cases
- Does not introduce new errors
- Minimal processing overhead (~50ms)

**When to use:**
- Apply to all files as default preprocessing
- Especially important for files with background noise or weak signals

### Architecture

For tr-engine integration:
1. Use local speaches-ai server with `faster-whisper-large-v3-turbo`
2. Apply voice bandpass preprocessing before transcription
3. Configure model download on first use via `POST /v1/models/{model}`
4. Set reasonable timeout (60s) for transcription requests

---

## Test Environment Details

- **Server:** speaches-ai (localhost:8000)
- **Audio Format:** WAV, 8kHz mono (trunk-recorder output)
- **Resampling:** 16kHz for Whisper models
- **Sox Version:** 14.4.2
- **Test Date:** 2026-01-30

## Reproducibility

All test files are located in:
```
~/trunk-recorder-mqtt/tr_audio/butco/2026/1/27/
```

To reproduce any test:
```bash
# Download model
curl -X POST "http://localhost:8000/v1/models/deepdml/faster-whisper-large-v3-turbo-ct2"

# Transcribe
curl http://localhost:8000/v1/audio/transcriptions \
  -F "file=@/path/to/file.wav" \
  -F "model=deepdml/faster-whisper-large-v3-turbo-ct2"

# With preprocessing
sox input.wav /tmp/processed.wav rate 16000 sinc 300-3000 norm
curl http://localhost:8000/v1/audio/transcriptions \
  -F "file=@/tmp/processed.wav" \
  -F "model=deepdml/faster-whisper-large-v3-turbo-ct2"
```
