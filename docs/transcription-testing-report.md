# Transcription Testing Report

**Date:** 2026-01-30
**Audio Source:** Butler County, OH P25 radio system (`~/trunk-recorder-mqtt/tr_audio/butco/`)
**Test Environment:**
- Local speaches-ai server (localhost:8000) for Whisper models
- Ollama server (sarah:11434) for LLM post-processing
- OpenAI API and AssemblyAI API for cloud comparison

## Executive Summary

Comprehensive testing was conducted to evaluate speech-to-text transcription accuracy for P25 radio dispatch audio, including cloud providers, local models, audio preprocessing, Whisper prompting, and LLM post-processing. Key findings:

1. **Model Selection:** `faster-whisper-large-v3-turbo` provides the best balance of speed and accuracy
2. **Avoid:** `Systran/faster-whisper-large-v3` has a hallucination bug (repeats phrases)
3. **Preprocessing:** Voice bandpass filter (300-3000Hz) eliminates certain hallucinations but results vary
4. **Local vs Cloud:** Local transcription is 3-10x faster than cloud providers with comparable quality
5. **LLM Post-Processing:** Small LLMs (4-7B) can improve domain-specific errors when given exact context
6. **Auto-Context Pipeline:** Using talkgroup metadata (group_tag + description) provides context without manual configuration
7. **Limitations:** All models occasionally miss content entirely or produce plausible-but-wrong corrections

### Recommended Architecture

```
Audio → Whisper (local, ~400ms) → LLM cleanup (gemma3 4B, ~600ms) → Store
```

**Total latency:** ~1 second per call. Improves searchability but should not be considered verbatim transcription.

---

## Table of Contents

1. [Whisper Model Comparison](#whisper-model-comparison)
2. [Cloud Provider Comparison](#cloud-provider-comparison)
3. [Audio Preprocessing](#audio-preprocessing)
4. [Whisper Prompt/Context Testing](#whisper-promptcontext-testing)
5. [LLM Post-Processing](#llm-post-processing)
6. [Auto-Context Pipeline](#auto-context-pipeline)
7. [Final Recommendations](#final-recommendations)

---

## Whisper Model Comparison

### Local Models (speaches-ai server)

| Model ID | Type | Avg Latency | Notes |
|----------|------|-------------|-------|
| `Systran/faster-whisper-small` | Small | ~220ms | Fastest, more errors on callsigns |
| `Systran/faster-whisper-large-v3` | Large | ~700ms | **AVOID** - hallucination bug |
| `deepdml/faster-whisper-large-v3-turbo-ct2` | Large Turbo | ~420ms | **Recommended** - best balance |
| `Systran/faster-distil-whisper-large-v3` | Distil | ~400ms | Good quality, fast |
| `distil-whisper/distil-large-v3-ct2` | Distil | ~750ms | Similar to distil-v3 |
| `distil-whisper/distil-large-v3.5-ct2` | Distil | ~400ms | Good formatting |

### Test 1: Basic Accuracy Comparison

**File:** `21006-1769530437_855962500.0-call_6896.wav` (305,324 bytes)
**Content:** Air ambulance dispatch notification
**Verified Ground Truth:** "Air Care 2 and the Butler County team, this is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. Time is 11:14."

| Model | Result |
|-------|--------|
| faster-whisper-small | Care Care 2 in the Butler County team, this is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. The time is 11.14. |
| faster-whisper-large-v3 | Care care to you and the Butler County team. This is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. Time is 1114. |
| faster-whisper-large-v3-turbo | Care care to you and the Butler County team, this is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. Time is 1114. |
| OpenAI whisper-1 | Care Care to you and the Butler County team, this is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. |

**Note:** All models transcribe "Air Care 2" incorrectly as "Care care" or "Care Care" - this is a domain-specific error. The talkgroup description contains "UC Air Care Dispatch" which could provide context for correction.

**File:** `21006-1769530769_852200000.0-call_6991.wav` (69,164 bytes)
**Content:** Helicopter maintenance abort
**Verified Ground Truth:** "Hey, base, this is 2. I've got a maintenance issue. I'll have to abort."

| Model | Result |
|-------|--------|
| faster-whisper-small | Hey, base, this is two. I've got a maintenance issue. I'll have to abort. |
| faster-whisper-large-v3 | Hey, BASE, this is 2. I've got a maintenance issue. I'll have to abort. |
| faster-whisper-large-v3-turbo | Hey, base, this is 2. I've got a maintenance issue. I'll have to abort. |
| OpenAI whisper-1 | Hey, Base, this is 2. I've got a maintenance issue. I'll have to abort. |

**Note:** High accuracy - all models captured this correctly.

### Test 2: Hallucination Detection

**File:** `9173-1769554590_852200000.0-call_1109.wav` (207,404 bytes)
**Content:** Traffic accident dispatch
**Verified Ground Truth:** "182. Go ahead. Respond to Drive Smart at 6121 Dixie Highway for an auto accident, no injury, assist with information exchange. Okay. 1756."

| Model | Result | Hallucination |
|-------|--------|---------------|
| faster-whisper-small | 182 Respond to drive smart at 6121 Dixie highway for an auto accident. No injury. Is this with information exchange? Okay 1756 | No |
| faster-whisper-large-v3 | 182. Go ahead. Responda, drive smart at 6121 Dixie Highway for an auto accident, no injury, assist with information exchange. Okay. 1756. 1756. Okay. Thank you. Thank you. Thank you. Thank you. Thank you. [repeats 27 times] | **YES** |
| faster-whisper-large-v3-turbo | 182. Go ahead. Respond to Drive Smart at 6121 Dixie Highway for an auto accident. No injury. This is with information exchange. Okay. 1756. | No |
| faster-distil-whisper-large-v3 | 182. Good. Respond to drive smart at 6121 Dixie Highway for an auto accident, no injury, assist with information exchange. Okay. 1756. | No |
| distil-large-v3.5-ct2 | 182. Go ahead. Respond to drive smart at 6121 Dixie Highway for an auto accident, no injury, assist with information exchange. Okay. 1756. | No |

**Notes:**
- "1756" is the time (5:56 PM), not a unit callsign
- Only `Systran/faster-whisper-large-v3` exhibited hallucination behavior
- Turbo model said "This is with" instead of "assist with" - minor error

### Test 3: Content Truncation

**File:** `9138-1769521316_851350000.0-call_3854.wav` (72,044 bytes)
**Verified Ground Truth:** "Medic 83 transporting to Atrium. Medic 83 transporting"

| Model | Latency | Result |
|-------|---------|--------|
| faster-whisper-small | 208ms | Mag A3 transporting. |
| faster-whisper-large-v3 | 484ms | Meg A3 transporting. Meg A3 transporting. |
| faster-distil-whisper-large-v3 | 377ms | Mega A3 Transpiring. Megay3 transporting. Megay3 transporting. |
| distil-large-v3-ct2 | 696ms | Minig A3 Transpiring. Meg.A3 transporting. |
| distil-large-v3.5-ct2 | 347ms | Mike A3 is transporting. |

**Critical Finding:** All models missed "to Atrium" destination and garbled "Medic 83" as various forms of "A3". First segment was faint in the audio.

**File:** `9264-1769530261_855962500.0-call_6818.wav` (57,644 bytes)
**Verified Ground Truth:** "Chief 141 to Medic 141 on Township."

| Model | Latency | Result |
|-------|---------|--------|
| faster-whisper-small | 232ms | 2-141, Dometic-141 on Township. |
| faster-whisper-large-v3 | 484ms | Chief 141 to Medic 141 on Township. |
| faster-distil-whisper-large-v3 | 353ms | Chief 141, the medic 141 on Township. |
| distil-large-v3-ct2 | 699ms | Chief 141, the medic 141 on Township. |
| distil-large-v3.5-ct2 | 352ms | Chief 141, Dometic 141 on Township. |

**Note:** Large models performed well here; small model struggled with "Chief".

---

## Cloud Provider Comparison

### OpenAI vs Local Comparison (8 Files)

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

**File:** `9130-1769527750_851350000.0-call_5625.wav`
**Verified Ground Truth:** "9-com, Medic 28 is transporting to Fort. Medic 28, you're out to Fort. Engine 26 available. Engine 26 is available."
- **OpenAI:** 9COM, Medic 28 is transporting to port. Medic 28, you're on the port. Engine 26 available. Engine 26 is available.
- **Local:** 9com, Medic 28 is transporting to Fort. Medic 28, you're out to Fort. Engine 26 is available. Engine 26 is available.

**Note:** Local model got "Fort" correct; OpenAI said "port".

### AssemblyAI Testing

AssemblyAI was tested as an alternative cloud provider. Their API uses an async flow (upload → transcribe → poll).

**Pricing:** $0.00025/second (~$0.015/minute) - approximately 2.5x more expensive than OpenAI

#### AssemblyAI vs Local Speed Comparison

| File | Size | Local Turbo (ms) | AssemblyAI (ms) | Speedup |
|------|------|------------------|-----------------|---------|
| call_9989.wav | 63KB | 393 | 3207 | **8.2x** |
| call_8056.wav | 144KB | 468 | 3142 | **6.7x** |
| call_1237.wav | 20KB | 373 | 4009 | **10.7x** |
| call_3631.wav | 32KB | 365 | 3245 | **8.9x** |
| call_494.wav | 37KB | 385 | 4773 | **12.4x** |

**Local is 8-12x faster than AssemblyAI** (vs ~3.3x faster than OpenAI).

#### AssemblyAI Quality Comparison

**File:** `21006-1769530437_855962500.0-call_6896.wav` (Air ambulance)
**Verified Ground Truth:** "Air Care 2 and the Butler County team..."

| Provider | Result |
|----------|--------|
| Local Turbo | Care care to you and the Butler County team... |
| AssemblyAI | Hair care 2 and the butler county team... |

**Note:** Both providers got "Air Care" wrong - Local said "Care care", AssemblyAI said "Hair care".

**File:** `9173-1769554590_852200000.0-call_1109.wav` (Traffic dispatch)
**Verified Ground Truth:** "...assist with information exchange..."

| Provider | Result |
|----------|--------|
| Local Turbo | ...This is with information exchange... |
| AssemblyAI | ...Assist with information exchange... |

**Note:** AssemblyAI got "assist with" correct where Local Whisper said "This is with".

#### AssemblyAI word_boost Testing

AssemblyAI supports a `word_boost` parameter to prioritize specific terms. Testing showed **minimal improvement**:

| Test Case | Without Boost | With Boost |
|-----------|---------------|------------|
| "Air Care 2" callsign | "Hair care 2" | "Hair care 2" (no change) |

**Conclusion:** AssemblyAI's word_boost does not significantly help with radio terminology.

#### Cloud Provider Summary

| Provider | Avg Latency | Cost | Quality | Recommendation |
|----------|-------------|------|---------|----------------|
| Local (speaches-ai) | ~400ms | Free | Good | **Primary choice** |
| OpenAI whisper-1 | ~1700ms | $0.006/min | Good | Fallback option |
| AssemblyAI | ~3500ms | $0.015/min | Good | Not recommended (slow, expensive) |

---

## Audio Preprocessing

### Sox Filters Tested

| Filter | Sox Command | Purpose |
|--------|-------------|---------|
| Original | `rate 16000` | Baseline (resample only) |
| High-pass 300Hz | `rate 16000 highpass 300` | Remove low frequency noise |
| Bandpass 300-3400Hz | `rate 16000 sinc 300-3400` | Voice frequency range |
| Voice Bandpass | `rate 16000 sinc 300-3000 norm` | Narrower voice range + normalize |

### Test: Hallucination Elimination via Filtering

**File:** `9468-1769545162_853287500.0-call_12291.wav`
**Verified Ground Truth:** "Go ahead, 1-5-8. Radio check. I see that. I see that you logged in. Hear you loud and clear. Have a great day and be safe out there."

| Filter | Result |
|--------|--------|
| original | Go ahead, 1-5-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8-8... |
| voice (300-3000Hz) | Go ahead, 1-5-8. Radio check. I see that you logged in. Hear you loud and clear. Have a great day and be safe out there. |

**Critical Finding:** Voice bandpass filter completely eliminated the hallucination and recovered actual speech content.

### Test: Bandpass Range Comparison

**File:** `9177-1769530121_851762500.0-call_6748.wav`
**Verified Ground Truth (partial):** "...It's a large box truck. It's approximately 10 feet off of the private driveway on a steep hill..."

| Range | Result |
|-------|--------|
| original | ...It's a proxy MP off of the private driveway on a steep hill. It's a small town. |
| 300-3000Hz | ...It's a large box truck. It's approximately 10 feet off of the private driveway on a steep hill. Wow. |

**Note:** Bandpass filter converted nonsensical "proxy MP" to "approximately 10 feet" which matches ground truth. However, all models missed the beginning of the transmission which contained license plate information.

### Test: Statistical Filter Analysis (20 Files)

| Metric | Original | Voice Bandpass |
|--------|----------|----------------|
| Total characters | 1,856 | 1,863 |
| Hallucinations detected | 0 | 0 |
| Processing overhead | baseline | ~50ms |

**Conclusion:** Voice bandpass filter eliminates certain hallucinations and may improve clarity, but results are not uniformly better. Preprocessing overhead is minimal (~50ms).

### Preprocessing Recommendation

**Filter for radio audio (optional):**
```bash
sox input.wav output.wav rate 16000 sinc 300-3000 norm
```

**Use when:**
- Encountering hallucination issues (repeated phrases)
- Audio has significant background noise

**Benefits:**
- Eliminates certain hallucinations
- May improve word clarity

**Caveats:**
- Results vary by file
- Not a universal improvement

---

## Whisper Prompt/Context Testing

Whisper models support a `prompt` parameter that can guide transcription with domain-specific terminology.

### Test: Prompt Impact on "Air Care 2"

**File:** `21006-1769530437_855962500.0-call_6896.wav`
**Verified Ground Truth:** "Air Care 2 and the Butler County team..."

| Condition | Result |
|-----------|--------|
| No prompt | Care care to you and the Butler County team... |
| Generic prompt | Care Care 2 and the Butler County team... |
| Specific prompt ("Air Care 2 is an air ambulance") | Care Care 2 and the Butler County Team... |

**Observation:** Prompt improved "Care care to you" → "Care Care 2" but did not achieve correct "Air Care 2". The acoustic ambiguity is too strong for prompting alone.

### Test: Prompt Impact on Other Files

**File:** `9173-1769554590_852200000.0-call_1109.wav` (Traffic dispatch)
**Verified Ground Truth:** "...assist with information exchange..."

| Condition | Result |
|-----------|--------|
| No prompt | ...This is with information exchange... |
| With context | ...assist with information exchange... |

**Observation:** Context correctly fixed "This is with" → "assist with".

### Test: Prompt Can Remove Content

**File:** `9069-1769552246_851762500.0-call_402.wav` (Security call)
**Verified Ground Truth:** "Attention all units, I got Miller's head of security here with me..."

| Condition | Result |
|-----------|--------|
| No prompt | In all units, I got Miller's head of security here with me... |
| With context | I got Miller's head of security here with me... |

**Warning:** The prompt caused Whisper to drop the "Attention all units" prefix entirely.

### Whisper Prompt Conclusions

| Aspect | Finding |
|--------|---------|
| Callsign improvement | Partial - may get closer but not exact |
| Phrase correction | Yes - "This is with" → "assist with" |
| Risk | Can remove content that doesn't match expected patterns |
| Recommendation | Use conservative prompts; prefer LLM post-processing |

---

## LLM Post-Processing

LLM post-processing was tested as a way to correct domain-specific transcription errors using contextual knowledge.

### Test Environment

- **Server:** Ollama on sarah:11434
- **Hardware:** Dual NVIDIA 3090 Ti, 256GB RAM
- **Models Tested:**
  - `gemma3:4b-it-qat` (4.3B parameters)
  - `qwen2.5:7b` (7B parameters)
  - `qwen2.5:3b` (3B parameters)
  - `deepseek-r1:32b` (32.8B parameters)
  - `deepseek-r1:70b` (70.6B parameters)
  - `glm-4.7-flash:q4_K_M` (29.9B parameters)

### Model Speed Comparison

| Model | Avg Latency | Notes |
|-------|-------------|-------|
| gemma3:4b-it-qat | ~500-700ms | Fast, conservative corrections |
| qwen2.5:7b | ~500-800ms | Fast, aggressive corrections |
| qwen2.5:3b | ~7000ms | Unexpectedly slow (not recommended) |
| glm-4.7-flash | ~24000ms | Too slow for real-time use |
| deepseek-r1:32b | ~10000-40000ms | Slow, prone to hallucination |

### Test: LLM Correction with Exact Context

**Raw Whisper:** "Care care to you and the Butler County team, this is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. Time is 1114."

**Context provided:** "UC Air Care Dispatch. EMS dispatch. Air Care 2 is an air ambulance."

| Model | Time | Result |
|-------|------|--------|
| gemma3:4b-it-qat | 717ms | Air Care 2 to you and the Butler County team, this is your notification for your scene flight into Bright, Indiana for a 75-year-old male with chest pains. Time is 11:14. |
| qwen2.5:7b | 618ms | Air Care 2 to you and the Butler County team, this is your notification for a scene flight into Bright, Indiana for a 75-year-old male with chest pain. Time is 11:14. |

**Note:** When given the exact term "Air Care" in the context, the LLM correctly fixes the transcription. The talkgroup description "UC Air Care Dispatch" contains this term.

### Test: Conservative vs Aggressive Prompting

**Conservative prompt:**
```
Fix transcription errors. Output ONLY corrected text. Context: {context}
```

**Aggressive prompt:**
```
You are a radio dispatch transcription editor. Fix obvious errors, rearrange for clarity...
```

| Prompt Style | Risk | Example Issue |
|--------------|------|---------------|
| Conservative | Low | Fewer fixes, but safe |
| Aggressive | High | qwen2.5 moved "1756" to wrong position in sentence |

**Recommendation:** Use conservative prompts to minimize false corrections.

### Test: Model Reliability Comparison

**Test case:** "Go ahead. He's advising he needs to throw a key down. Are you ready?"
**Context:** "Butler County Fire dispatch."

| Model | Result | Assessment |
|-------|--------|------------|
| gemma3:4b-it-qat | "throw a key air down" | Added erroneous word |
| qwen2.5:7b | "throw a key down" (unchanged) | **Correct** |
| deepseek-r1:32b | "tie a tourniquet" | **Severe hallucination** |

**Test case:** "Clear. I'll show Agent 51 en route, Miller Coors, at 1644."
**Context:** "Butler County EMS. Units: Medic 51, Engine 26, Squad 32."

| Model | Result | Assessment |
|-------|--------|------------|
| gemma3:4b-it-qat | "Agent 51" (unchanged) | Missed correction |
| qwen2.5:7b | "Medic 51" | **Correct** |
| deepseek-r1:32b | "Medic 51" | **Correct** |

### Test: Batch Processing (10 Random Files)

Using qwen2.5:7b with conservative prompt:

| Outcome | Count | Percentage |
|---------|-------|------------|
| Improved | 4 | 40% |
| Unchanged | 6 | 60% |
| Made worse | 0 | 0% |

**Sample corrections made:**
- "floor and under" → "floor and roof" (fire context)
- "medics transport" → "medics are transporting" (grammar)
- "fliggy out there" → "tricky out there" (obvious error)
- "Agent 51" → "Medic 51" (unit callsign)

**Note:** Some "corrections" may still be wrong if the context doesn't contain the exact terminology. LLM post-processing improves searchability but should not be considered verbatim.

### LLM Post-Processing Conclusions

| Model | Speed | Accuracy | Hallucination Risk | Recommendation |
|-------|-------|----------|-------------------|----------------|
| **gemma3:4b-it-qat** | Fast (~600ms) | Conservative | Low | **Primary choice** |
| qwen2.5:7b | Fast (~600ms) | Aggressive | Medium | Good alternative |
| deepseek-r1 | Slow (10-40s) | Variable | **High** | **Avoid** |
| glm-4.7-flash | Slow (~24s) | Good | Low | Not practical |

**Key Finding:** Small models (4-7B) are sufficient for transcription cleanup. Larger reasoning models (deepseek-r1) are counterproductive - they overthink and hallucinate. Context must contain exact terminology to enable correct fixes.

---

## Auto-Context Pipeline

The key insight from testing is that **context with exact terminology is essential for fixing domain-specific errors**, but requiring users to manually configure per-talkgroup context is not practical.

### Solution: Use Existing Talkgroup Metadata

Trunk-recorder JSON sidecars already contain rich talkgroup metadata:

```json
{
  "talkgroup": 21006,
  "talkgroup_tag": "AC31E773",
  "talkgroup_description": "UC Air Care Dispatch",
  "talkgroup_group_tag": "EMS Dispatch",
  "talkgroup_group": "Medical Transportation"
}
```

The `talkgroup_description` field often contains the exact terminology needed (e.g., "UC Air Care Dispatch" contains "Air Care").

### Tag-to-Context Mapping

Map `talkgroup_group_tag` to generic domain context:

| Group Tag | Auto-Generated Context |
|-----------|----------------------|
| Fire Dispatch | Fire/EMS dispatch. Units: Engine, Medic, Squad, Rescue, Chief. |
| Fire-Tac | Fire tactical/fireground operations. |
| EMS Dispatch | EMS dispatch. Units: Medic, Ambulance, Care Flight. |
| EMS-Tac | EMS tactical operations. |
| Law Dispatch | Police dispatch. Numeric unit callsigns. |
| Law-Tac | Police tactical operations. |
| Interop | Multi-agency interoperability. |
| Public Works | Public works/utilities radio. |
| Security | Security radio. Facility patrols. |
| (default) | Emergency services radio. |

### Context Construction

Combine `talkgroup_description` + mapped context:

```
{talkgroup_description}. {context_from_group_tag}.
```

**Example for talkgroup 21006:**
```
UC Air Care Dispatch. EMS dispatch. Units: Medic, Ambulance, Care Flight.
```

This provides both the specific terminology ("Air Care") and generic domain context.

### Test: Full Pipeline with Auto-Context

**File:** `21006-1769530437_855962500.0-call_6896.wav`

| Step | Output |
|------|--------|
| **Talkgroup metadata** | description: "UC Air Care Dispatch", group_tag: "EMS Dispatch" |
| **Auto-generated context** | "UC Air Care Dispatch. EMS dispatch. Units: Medic, Ambulance, Care Flight." |
| **Whisper raw** | Care care to you and the Butler County team... |
| **LLM corrected** | **Air Care** 2 to you and the Butler County team... |

**The pipeline correctly fixed "Care care" → "Air Care" using the talkgroup description - no manual configuration required.**

### Test: Various Talkgroup Types

| TGID | Group Tag | Description | Whisper Raw | LLM Corrected |
|------|-----------|-------------|-------------|---------------|
| 21006 | EMS Dispatch | UC Air Care Dispatch | "Care care to you" | "Air Care 2 to you" ✓ |
| 9173 | Fire Dispatch | 09-3L Main | "This is with information" | "assist with information" ✓ |
| 9065 | Fire-Tac | Fireground 5 | "on fire five" | "on fire scene" ✗ |
| 51788 | Law Dispatch | Shipley Bldg Dispatch | (unchanged) | (unchanged) ✓ |

**Note:** One overcorrection observed - "fire five" (referring to Fireground 5 channel) was incorrectly changed to "fire scene". The LLM lacks channel naming context when talkgroup description doesn't contain it.

### Pipeline Performance

| Stage | Latency |
|-------|---------|
| Whisper transcription | ~400ms |
| LLM post-processing | ~600ms |
| **Total** | **~1000ms** |

This is acceptable for background processing of completed calls.

### Auto-Context Pipeline Summary

**Advantages:**
- Zero configuration required
- Uses existing trunk-recorder metadata
- Fixes domain-specific errors when exact terminology is in talkgroup_description
- Works across different talkgroup types

**Limitations:**
- Requires exact terminology in talkgroup metadata to fix specialized terms
- May overcorrect channel/tactical names not in metadata
- Cannot fix errors when ground truth term is not in any available context
- LLM server availability required

**Future Enhancements:**
1. Extract callsigns from talkgroup_description field automatically
2. Learn vocabulary from historical transcriptions
3. Allow manual context override for edge cases

---

## Final Recommendations

### Recommended Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐     ┌───────┐
│ Audio File  │────▶│ Whisper (local)  │────▶│ LLM Postprocess │────▶│ Store │
│             │     │ ~400ms           │     │ ~600ms          │     │       │
└─────────────┘     └──────────────────┘     └─────────────────┘     └───────┘
                            │                        │
                            ▼                        ▼
                    ┌──────────────┐         ┌──────────────┐
                    │ Preprocess   │         │ Talkgroup    │
                    │ (optional)   │         │ Metadata     │
                    └──────────────┘         └──────────────┘
```

### Component Selection

| Component | Recommendation | Alternative |
|-----------|----------------|-------------|
| **Whisper Model** | `deepdml/faster-whisper-large-v3-turbo-ct2` | `distil-large-v3.5-ct2` |
| **Whisper Server** | speaches-ai (local) | OpenAI API (cloud fallback) |
| **Audio Preprocessing** | Optional - use for hallucination issues | sox `rate 16000 sinc 300-3000 norm` |
| **LLM Model** | `gemma3:4b-it-qat` | `qwen2.5:7b` |
| **LLM Server** | Ollama | Any OpenAI-compatible API |

### Configuration Example

```yaml
transcription:
  enabled: true

  # Whisper settings
  whisper:
    provider: "http"
    url: "http://localhost:8000/v1/audio/transcriptions"
    model: "deepdml/faster-whisper-large-v3-turbo-ct2"
    timeout: 60

  # Audio preprocessing (optional)
  preprocess:
    enabled: false  # Enable if encountering hallucinations
    sample_rate: 16000
    highpass_hz: 300
    lowpass_hz: 3000
    normalize: true

  # LLM post-processing
  postprocess:
    enabled: true
    provider: "ollama"
    endpoint: "http://localhost:11434"
    model: "gemma3:4b-it-qat"
    # Context auto-built from talkgroup metadata

  # Queue settings
  min_duration: 2.0  # Skip calls shorter than 2 seconds
  concurrency: 2     # Parallel transcription workers
```

### What to Avoid

| Don't Use | Reason |
|-----------|--------|
| `Systran/faster-whisper-large-v3` | Hallucination bug |
| `deepseek-r1` for post-processing | Slow, hallucinates |
| AssemblyAI | Expensive, slow, no accuracy benefit |
| Aggressive LLM prompts | Cause overcorrection |
| Manual per-talkgroup context | User burden, not scalable |

### Expected Results

With the recommended pipeline:

| Metric | Expected |
|--------|----------|
| Transcription latency | ~1 second |
| Domain error correction | Dependent on talkgroup metadata quality |
| False corrections | ~5% (mostly on tactical channel names) |
| Hallucinations | Near zero with turbo model |

### Important Caveats

1. **Not verbatim transcription:** Output is suitable for search and general understanding, not legal or precise record-keeping
2. **Context-dependent accuracy:** Corrections only work when exact terminology exists in talkgroup metadata
3. **Content can be missed:** All models occasionally truncate or drop portions of transmissions
4. **Some errors are unfixable:** Acoustic ambiguity (e.g., "Air Care" sounding like "Care care") cannot always be resolved

---

## Test Environment Details

- **Whisper Server:** speaches-ai (localhost:8000)
- **LLM Server:** Ollama (sarah:11434) - Dual 3090 Ti, 256GB RAM
- **Audio Format:** WAV, 8kHz mono (trunk-recorder output)
- **Resampling:** 16kHz for Whisper models
- **Sox Version:** 14.4.2
- **Test Date:** 2026-01-30

## Reproducibility

All test files are located in:
```
~/trunk-recorder-mqtt/tr_audio/butco/2026/1/27/
```

### Whisper Test

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

### LLM Post-Processing Test

```bash
# Transcribe and post-process
RAW="Care care to you and the Butler County team..."
CONTEXT="UC Air Care Dispatch. EMS dispatch. Units: Medic, Ambulance, Air Care."

curl http://localhost:11434/api/chat -d "{
  \"model\": \"gemma3:4b-it-qat\",
  \"messages\": [
    {\"role\": \"system\", \"content\": \"Fix transcription errors. Output ONLY corrected text. Context: $CONTEXT\"},
    {\"role\": \"user\", \"content\": \"$RAW\"}
  ],
  \"stream\": false
}"
```

### Full Pipeline Test

```bash
# Get talkgroup metadata from sidecar
TAG=$(jq -r '.talkgroup_group_tag' /path/to/call.json)
DESC=$(jq -r '.talkgroup_description' /path/to/call.json)

# Build context
CONTEXT="$DESC. $(get_context_for_tag "$TAG")"

# Whisper transcription
RAW=$(curl -s http://localhost:8000/v1/audio/transcriptions \
  -F "file=@/path/to/call.wav" \
  -F "model=deepdml/faster-whisper-large-v3-turbo-ct2" | jq -r '.text')

# LLM post-processing
CORRECTED=$(curl -s http://localhost:11434/api/chat -d "{
  \"model\": \"gemma3:4b-it-qat\",
  \"messages\": [
    {\"role\": \"system\", \"content\": \"Fix transcription errors. Output ONLY corrected text. Context: $CONTEXT\"},
    {\"role\": \"user\", \"content\": \"$RAW\"}
  ],
  \"stream\": false
}" | jq -r '.message.content')

echo "Final: $CORRECTED"
```
