# Whisper Parameter Tuning for P25 Radio Audio

Results from a systematic parameter sweep using faster-whisper `large-v3` on an RTX 4090 (float16) against 6 real P25 radio recordings (3s to 127s duration, IMBE vocoder, Butler/Warren County dispatch and fireground comms).

## Test Methodology

- 38 parameter configurations tested across 6 audio files (228 total transcriptions)
- Metrics: average word probability (AvgProb), minimum word probability (MinProb), low-confidence word count (words with probability < 0.3), and processing time
- Baseline: `temperature=0.0`, all other params at faster-whisper defaults

## Model Selection: large-v3 vs large-v3-turbo

| Model | File (31s) | Proc Time | Output |
|-------|-----------|-----------|--------|
| **large-v3** | Life Squad dispatch | 2.84s | "Deputy Ongstein's advising female is responsive..." |
| large-v3-turbo | Life Squad dispatch | 0.75s | "Staffing on scenes of vaccine female is responsive..." |

Turbo is 3-4x faster but significantly worse on vocoder-distorted P25 audio. It hallucinated on short files ("me, Mitch" became "the image") and garbled key phrases. **Use large-v3 for radio.**

## Summary Table

| Config | AvgProb | MinProb | LowConf | AvgProc | Verdict |
|--------|---------|---------|---------|---------|---------|
| BASELINE (temp=0.0) | 0.817 | 0.006 | 23 | 5.86s | Good baseline |
| TEMP: fallback list | 0.815 | 0.006 | 23 | 4.91s | No change (insurance) |
| BEAM=1 (greedy) | 0.749 | 0.000 | 28 | 2.90s | Bad |
| BEAM=3 | 0.807 | 0.004 | 29 | 3.71s | Slightly worse |
| **BEAM=5 (default)** | **0.817** | **0.006** | **23** | 5.42s | **Sweet spot** |
| BEAM=10 | 0.811 | 0.000 | 26 | 7.13s | Slower, no gain |
| REP=1.0 (none) | 0.817 | 0.006 | 23 | 5.39s | Default, fine |
| REP=1.1 | 0.797 | 0.004 | 29 | 4.08s | Slightly worse |
| REP=1.2 | 0.758 | 0.001 | 55 | 3.79s | Harmful |
| REP=1.5 | 0.676 | 0.000 | 125 | 3.52s | Very harmful |
| REP=2.0 | 0.601 | 0.000 | 172 | 4.09s | Catastrophic |
| NGRAM=0 (off) | 0.817 | 0.006 | 23 | 5.51s | Default, fine |
| NGRAM=2 | 0.744 | 0.000 | 56 | 3.99s | Harmful |
| NGRAM=3 | 0.781 | 0.000 | 41 | 4.08s | Harmful |
| NGRAM=4 | 0.787 | 0.000 | 38 | 4.34s | Slightly harmful |
| COND_PREV=true | 0.817 | 0.006 | 23 | 5.69s | Default |
| **COND_PREV=false** | **0.805** | **0.042** | **20** | **3.85s** | **Best change** |
| HALLUC=0 (off) | 0.817 | 0.006 | 23 | 5.68s | Default |
| HALLUC=1.0 | 0.801 | 0.000 | 32 | 6.54s | Slightly worse |
| HALLUC=2.0 | 0.801 | 0.000 | 28 | 4.74s | Neutral |
| HALLUC=3.0 | 0.801 | 0.000 | 28 | 4.69s | Neutral |
| NOSPEECH=0.3 | 0.817 | 0.006 | 23 | 5.33s | No change |
| NOSPEECH=0.6 (default) | 0.817 | 0.006 | 23 | 5.33s | No change |
| NOSPEECH=0.8 | 0.817 | 0.006 | 23 | 5.37s | No change |
| PROMPT: none | 0.817 | 0.006 | 23 | 5.52s | Default |
| PROMPT: detailed dispatch | 0.797 | 0.000 | 30 | 3.99s | Harmful |
| PROMPT: short generic | 0.805 | 0.000 | 35 | 5.34s | Slightly harmful |
| HOTWORDS: radio terms | 0.726 | 0.001 | 51 | 6.50s | Harmful |
| COMPRESS=1.8 (strict) | 0.817 | 0.006 | 23 | 5.33s | No change |
| COMPRESS=2.4 (default) | 0.817 | 0.006 | 23 | 5.49s | No change |
| COMPRESS=3.0 (permissive) | 0.817 | 0.006 | 23 | 5.30s | No change |
| VAD=off | 0.817 | 0.006 | 23 | 5.33s | Default |
| VAD=on (defaults) | 0.825 | 0.000 | 22 | 4.15s | Rephrases text |
| VAD=aggressive | 0.826 | 0.000 | 27 | 3.95s | Rephrases text |
| COMBO-A (full stack) | 0.753 | 0.000 | 56 | 3.99s | Harmful |
| COMBO-B (rep+ngram+prompt) | 0.765 | 0.000 | 38 | 4.02s | Harmful |
| COMBO-C (kitchen sink) | 0.750 | 0.000 | 52 | 4.15s | Harmful |
| COMBO-D (prompt+hotwords+rep) | 0.732 | 0.009 | 48 | 4.18s | Harmful |

## Detailed Findings

### condition_on_previous_text=false (the only winner)

The single most impactful change. When enabled (default), Whisper uses the previous segment's text as context for the next segment. On radio audio with silence gaps between transmissions, this causes hallucination cascading -- the model fills silence with repetitions of what it just heard.

**Example (Life Squad dispatch, 31s):**
- `cond_prev=true` (default): Hallucinates a 3rd repetition: "...Time 1504. Dispatch District 1 Life Squad, I need you to respond to 2907 State Route 134 South," (runs off the end)
- `cond_prev=false`: Stops at the 2 real readbacks, ends cleanly with "Time is 1504."

Effect on metrics: Highest MinProb (0.042 vs 0.006), lowest LowConf count (20 vs 23), 34% faster processing.

### beam_size=5 (keep default)

Beam=1 (greedy decoding) produced garbage on short files: "101.7 reach. 2048." instead of "101, that's me, Mitch. 2248." and "Defense Estrogen" instead of "defense, SG." Beam=3 was marginally worse than 5. Beam=10 was 2x slower with no quality improvement and actually hallucinated more on the 127s fireground file.

### repetition_penalty (don't touch)

Every value above 1.0 made things worse. This parameter penalizes repeated tokens, but radio dispatch intentionally repeats:
- Dispatchers read addresses twice (readback protocol)
- Units repeat back commands for confirmation
- Talkgroup IDs and unit numbers repeat naturally

At REP=1.2, low-confidence words more than doubled (23 to 55). At REP=1.5, it was catastrophic (125 low-confidence words). The model fights against producing legitimate repeated content and outputs garbled alternatives instead.

### no_repeat_ngram_size (don't touch)

Same problem as repetition_penalty. Blocking n-gram repetition prevents the model from producing repeated phrases that are actually present in the audio. NGRAM=2 was worst (56 LowConf); NGRAM=3 and 4 were less bad but still worse than disabled.

### prompt / initial_prompt (skip for now)

The detailed dispatch prompt ("Butler County dispatch radio. Medic 23, Engine 7...") introduced phantom content on short files -- "255 County. Go ahead." appeared as hallucinated dialogue that wasn't in the audio. On the Life Squad file, the readback degraded. The short generic prompt ("Police and fire dispatch radio communications.") was slightly less harmful but still worse than no prompt.

This may become useful with fine-tuning or per-talkgroup prompts, but blanket prompts hurt more than they help on varied radio audio.

### hotwords (harmful)

Boosting radio vocabulary terms ("Medic,Engine,Ladder,Rescue,Squad,District,dispatch") made overall quality significantly worse. AvgProb dropped from 0.817 to 0.726, and LowConf more than doubled (23 to 51). The probability boosting distorted the model's natural decoding across the board.

### VAD filter (don't use for radio)

Silero VAD preprocessing produced the highest average probability (0.825-0.826) but at the cost of accuracy. It rearranged and rephrased content: "Dispatch District 1 Life Squad, I need you to respond to..." became "I'm making dispatch to District 1 Life Squad. I need your response to..." It also dropped content from the 63s helipad conversation. P25 audio is already PTT-gated (no open-mic noise), so VAD is unnecessary and actively harmful.

### hallucination_silence_threshold (neutral)

Values of 2.0 and 3.0 had minimal effect on these test files. The parameter is designed to catch hallucinations during long silence gaps, but `condition_on_previous_text=false` already handles the primary hallucination vector. May provide marginal benefit on files with very long dead-air segments.

### no_speech_threshold (no effect)

Values 0.3, 0.6, and 0.8 produced identical results. P25 audio has very little true silence (PTT-gated), so this threshold rarely triggers.

### compression_ratio_threshold (no effect)

Values 1.8, 2.4, and 3.0 produced identical results on these test files.

### temperature fallback list (no effect, keep as insurance)

`temperature=0.0` vs `temperature=0.0,0.2,0.4,0.6,0.8,1.0` produced identical output on all test files. No segments triggered fallback decoding. Keep the fallback list as insurance against edge cases, but it doesn't change typical results.

## Recommended Configuration

```env
# Model
WHISPER_MODEL=large-v3

# The only parameter worth changing from defaults
WHISPER_CONDITION_ON_PREV=false

# Keep defaults for everything else
WHISPER_TEMPERATURE=0.0
WHISPER_BEAM_SIZE=5
WHISPER_REPETITION_PENALTY=0
WHISPER_NO_REPEAT_NGRAM=0
WHISPER_NO_SPEECH_THRESHOLD=0
WHISPER_HALLUCINATION_THRESHOLD=0
WHISPER_MAX_TOKENS=0
WHISPER_VAD_FILTER=false
```

## Performance

On RTX 4090 with large-v3 (float16):

| Audio Duration | Processing Time | Real-time Factor |
|---------------|----------------|-----------------|
| 3.3s | 0.5s | 0.15x |
| 10.8s | 1.0s | 0.09x |
| 17.1s | 1.1s | 0.06x |
| 31.0s | 2.4s | 0.08x |
| 63.2s | 6.8s | 0.11x |
| 126.7s | 11.9s | 0.09x |

Average real-time factor: ~0.10x (10x faster than real-time).

## Test Files

| File | Duration | Content |
|------|----------|---------|
| 11501-1732852092-101.m4a | 3.3s | Short unit check-in |
| 11501-1732890340-4771.m4a | 10.8s | District 5 dispatch, deputy on scene |
| 11501-1732896070-5814.m4a | 17.1s | Squad 55 medical dispatch |
| 11531-1732910648-858.m4a | 31.0s | District 1 Life Squad dispatch (with readback) |
| 17508-...call_91989.m4a | 63.2s | Helicopter/helipad coordination |
| 21029-...call_35128.m4a | 126.7s | Multi-unit fireground operations |

## Reproducing

```bash
# Start the server
python tools/whisper-server/server.py

# Run the sweep
python tools/whisper-server/sweep.py
```

Raw results in `tools/whisper-server/sweep-results.txt`.
