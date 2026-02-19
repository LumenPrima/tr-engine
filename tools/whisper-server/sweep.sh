#!/bin/bash
# Parameter sweep for faster-whisper on P25 radio audio
# Tests key parameters systematically and logs results

URL="http://localhost:8000/v1/audio/transcriptions"
OUTDIR="D:/Downloads/whisper-sweep"
mkdir -p "$OUTDIR"

# Test files
FILES=(
  "D:/Downloads/test_audio/11501-1732852092-101.m4a"
  "D:/Downloads/test_audio/11501-1732890340-4771.m4a"
  "D:/Downloads/test_audio/11501-1732896070-5814.m4a"
  "D:/Downloads/test_audio/11531-1732910648-858.m4a"
  "D:/Downloads/test_audio/17508-1726293137_851762500.0-call_91989.m4a"
  "D:/Downloads/test_audio/21029-1726738591_852687500.0-call_35128.m4a"
)

run_test() {
  local label="$1"
  shift
  local extra_params=("$@")

  echo "=============================================="
  echo "TEST: $label"
  echo "=============================================="

  for f in "${FILES[@]}"; do
    fname=$(basename "$f")
    result=$(curl -s -X POST "$URL" \
      -F "file=@$f" \
      -F "language=en" \
      -F "response_format=verbose_json" \
      -F "timestamp_granularities[]=word" \
      "${extra_params[@]}" 2>&1)

    text=$(echo "$result" | python -c "import sys,json; d=json.load(sys.stdin); print(d.get('text','ERROR'))" 2>/dev/null)
    dur=$(echo "$result" | python -c "import sys,json; d=json.load(sys.stdin); print(d.get('duration',0))" 2>/dev/null)
    ptime=$(echo "$result" | python -c "import sys,json; d=json.load(sys.stdin); print(d.get('processing_time',0))" 2>/dev/null)
    nsegs=$(echo "$result" | python -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('segments',[])))" 2>/dev/null)
    # Average word probability
    avgprob=$(echo "$result" | python -c "
import sys,json
d=json.load(sys.stdin)
words=d.get('words',[])
if words:
    avg=sum(w['probability'] for w in words)/len(words)
    print(f'{avg:.3f}')
else:
    print('N/A')
" 2>/dev/null)
    # Min word probability (weakest word)
    minprob=$(echo "$result" | python -c "
import sys,json
d=json.load(sys.stdin)
words=d.get('words',[])
if words:
    mn=min(w['probability'] for w in words)
    print(f'{mn:.3f}')
else:
    print('N/A')
" 2>/dev/null)

    printf "  %-50s dur=%-6s proc=%-6s segs=%-3s avgP=%-6s minP=%-6s\n" "$fname" "$dur" "$ptime" "$nsegs" "$avgprob" "$minprob"
    echo "    TEXT: $text"
  done
  echo ""
}

# =====================================================================
# BASELINE: defaults only
# =====================================================================
run_test "BASELINE (defaults)" \
  -F "temperature=0.0"

# =====================================================================
# TEMPERATURE: single 0.0 vs fallback list
# =====================================================================
run_test "TEMP: fallback list 0.0,0.2,0.4,0.6,0.8,1.0" \
  -F "temperature=0.0,0.2,0.4,0.6,0.8,1.0"

run_test "TEMP: 0.0 only (no fallback)" \
  -F "temperature=0.0"

# =====================================================================
# BEAM SIZE
# =====================================================================
run_test "BEAM: 1 (greedy)" \
  -F "temperature=0.0" \
  -F "beam_size=1"

run_test "BEAM: 3" \
  -F "temperature=0.0" \
  -F "beam_size=3"

run_test "BEAM: 5 (default)" \
  -F "temperature=0.0" \
  -F "beam_size=5"

run_test "BEAM: 10" \
  -F "temperature=0.0" \
  -F "beam_size=10"

# =====================================================================
# REPETITION PENALTY
# =====================================================================
run_test "REP_PENALTY: 1.0 (none)" \
  -F "temperature=0.0" \
  -F "repetition_penalty=1.0"

run_test "REP_PENALTY: 1.1" \
  -F "temperature=0.0" \
  -F "repetition_penalty=1.1"

run_test "REP_PENALTY: 1.2" \
  -F "temperature=0.0" \
  -F "repetition_penalty=1.2"

run_test "REP_PENALTY: 1.5" \
  -F "temperature=0.0" \
  -F "repetition_penalty=1.5"

run_test "REP_PENALTY: 2.0" \
  -F "temperature=0.0" \
  -F "repetition_penalty=2.0"

# =====================================================================
# NO REPEAT NGRAM
# =====================================================================
run_test "NGRAM: 0 (disabled)" \
  -F "temperature=0.0" \
  -F "no_repeat_ngram_size=0"

run_test "NGRAM: 2" \
  -F "temperature=0.0" \
  -F "no_repeat_ngram_size=2"

run_test "NGRAM: 3" \
  -F "temperature=0.0" \
  -F "no_repeat_ngram_size=3"

run_test "NGRAM: 4" \
  -F "temperature=0.0" \
  -F "no_repeat_ngram_size=4"

# =====================================================================
# CONDITION ON PREVIOUS TEXT
# =====================================================================
run_test "COND_PREV: true (default)" \
  -F "temperature=0.0" \
  -F "condition_on_previous_text=true"

run_test "COND_PREV: false" \
  -F "temperature=0.0" \
  -F "condition_on_previous_text=false"

# =====================================================================
# HALLUCINATION SILENCE THRESHOLD
# =====================================================================
run_test "HALLUC_THRESH: 0 (disabled)" \
  -F "temperature=0.0"

run_test "HALLUC_THRESH: 1.0" \
  -F "temperature=0.0" \
  -F "hallucination_silence_threshold=1.0"

run_test "HALLUC_THRESH: 2.0" \
  -F "temperature=0.0" \
  -F "hallucination_silence_threshold=2.0"

run_test "HALLUC_THRESH: 3.0" \
  -F "temperature=0.0" \
  -F "hallucination_silence_threshold=3.0"

# =====================================================================
# NO SPEECH THRESHOLD
# =====================================================================
run_test "NO_SPEECH: 0.3 (aggressive)" \
  -F "temperature=0.0" \
  -F "no_speech_threshold=0.3"

run_test "NO_SPEECH: 0.6 (default)" \
  -F "temperature=0.0" \
  -F "no_speech_threshold=0.6"

run_test "NO_SPEECH: 0.8 (permissive)" \
  -F "temperature=0.0" \
  -F "no_speech_threshold=0.8"

# =====================================================================
# PROMPT (domain-specific)
# =====================================================================
run_test "PROMPT: none" \
  -F "temperature=0.0"

run_test "PROMPT: radio dispatch" \
  -F "temperature=0.0" \
  -F "prompt=Butler County dispatch radio. Medic 23, Engine 7, Rescue 52, District 1 Life Squad. 10-4, copy, en route, responding, on scene. State Route 134, Hamilton-Mason Road."

run_test "PROMPT: radio short" \
  -F "temperature=0.0" \
  -F "prompt=Police and fire dispatch radio communications."

# =====================================================================
# HOTWORDS
# =====================================================================
run_test "HOTWORDS: radio terms" \
  -F "temperature=0.0" \
  -F "hotwords=Medic,Engine,Ladder,Rescue,Squad,District,dispatch,responding,en route,10-4,copy,State Route,Hamilton"

# =====================================================================
# COMPRESSION RATIO THRESHOLD
# =====================================================================
run_test "COMPRESS: 1.8 (strict)" \
  -F "temperature=0.0" \
  -F "compression_ratio_threshold=1.8"

run_test "COMPRESS: 2.4 (default)" \
  -F "temperature=0.0" \
  -F "compression_ratio_threshold=2.4"

run_test "COMPRESS: 3.0 (permissive)" \
  -F "temperature=0.0" \
  -F "compression_ratio_threshold=3.0"

# =====================================================================
# VAD FILTER
# =====================================================================
run_test "VAD: disabled (default)" \
  -F "temperature=0.0" \
  -F "vad_filter=false"

run_test "VAD: enabled (defaults)" \
  -F "temperature=0.0" \
  -F "vad_filter=true"

run_test "VAD: aggressive (low threshold)" \
  -F "temperature=0.0" \
  -F "vad_filter=true" \
  -F "vad_threshold=0.3" \
  -F "vad_min_speech_duration_ms=100"

# =====================================================================
# COMBO: best anti-hallucination stack
# =====================================================================
run_test "COMBO: anti-halluc stack A (rep1.2 + ngram3 + noprev + halluc2.0)" \
  -F "temperature=0.0" \
  -F "repetition_penalty=1.2" \
  -F "no_repeat_ngram_size=3" \
  -F "condition_on_previous_text=false" \
  -F "hallucination_silence_threshold=2.0"

run_test "COMBO: anti-halluc stack B (rep1.1 + ngram3 + noprev + prompt)" \
  -F "temperature=0.0" \
  -F "repetition_penalty=1.1" \
  -F "no_repeat_ngram_size=3" \
  -F "condition_on_previous_text=false" \
  -F "prompt=Butler County dispatch radio. Medic 23, Engine 7, Rescue 52, District 1 Life Squad."

run_test "COMBO: kitchen sink" \
  -F "temperature=0.0,0.2,0.4,0.6,0.8,1.0" \
  -F "beam_size=5" \
  -F "repetition_penalty=1.2" \
  -F "no_repeat_ngram_size=3" \
  -F "condition_on_previous_text=false" \
  -F "hallucination_silence_threshold=2.0" \
  -F "no_speech_threshold=0.6" \
  -F "prompt=Butler County dispatch radio. Medic 23, Engine 7, Rescue 52, District 1 Life Squad. 10-4, copy, en route." \
  -F "hotwords=Medic,Engine,Ladder,Rescue,Squad,District,dispatch,responding,en route,10-4"

echo "SWEEP COMPLETE"
