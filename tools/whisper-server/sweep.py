"""Parameter sweep for faster-whisper on P25 radio audio."""

import json
import os
import subprocess
import sys
import time

URL = "http://localhost:8000/v1/audio/transcriptions"

FILES = [
    r"D:\Downloads\test_audio\11501-1732852092-101.m4a",
    r"D:\Downloads\test_audio\11501-1732890340-4771.m4a",
    r"D:\Downloads\test_audio\11501-1732896070-5814.m4a",
    r"D:\Downloads\test_audio\11531-1732910648-858.m4a",
    r"D:\Downloads\test_audio\17508-1726293137_851762500.0-call_91989.m4a",
    r"D:\Downloads\test_audio\21029-1726738591_852687500.0-call_35128.m4a",
]

OUTFILE = os.path.join(os.path.dirname(__file__), "sweep-results.txt")


def run_test(label: str, params: dict) -> list[dict]:
    """Run one test config across all files, return results."""
    results = []
    for f in FILES:
        if not os.path.exists(f):
            print(f"  SKIP (missing): {f}")
            continue

        cmd = ["curl", "-s", "-X", "POST", URL, "-F", f"file=@{f}"]
        # Always include these
        cmd += ["-F", "language=en", "-F", "response_format=verbose_json",
                "-F", "timestamp_granularities[]=word"]
        for k, v in params.items():
            cmd += ["-F", f"{k}={v}"]

        try:
            out = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
            data = json.loads(out.stdout)
        except Exception as e:
            print(f"  ERROR on {os.path.basename(f)}: {e}")
            results.append({"file": os.path.basename(f), "error": str(e)})
            continue

        text = data.get("text", "")
        dur = data.get("duration", 0)
        ptime = data.get("processing_time", 0)
        words = data.get("words", [])
        segs = data.get("segments", [])

        avg_prob = sum(w["probability"] for w in words) / len(words) if words else 0
        min_prob = min((w["probability"] for w in words), default=0)
        low_words = [w for w in words if w["probability"] < 0.3]

        r = {
            "file": os.path.basename(f),
            "duration": dur,
            "proc_time": ptime,
            "segments": len(segs),
            "words": len(words),
            "avg_prob": round(avg_prob, 3),
            "min_prob": round(min_prob, 3),
            "low_conf_words": len(low_words),
            "text": text,
        }
        results.append(r)

        fname = os.path.basename(f)[:45]
        print(f"  {fname:<45} dur={dur:<6} proc={ptime:<6} words={len(words):<3} "
              f"avgP={avg_prob:.3f} minP={min_prob:.3f} low={len(low_words)}")
        print(f"    > {text}")

    return results


# Define all test configs
TESTS = [
    ("BASELINE (temp=0.0 only)", {"temperature": "0.0"}),

    ("TEMP: fallback list", {"temperature": "0.0,0.2,0.4,0.6,0.8,1.0"}),

    # Beam size
    ("BEAM=1 (greedy)", {"temperature": "0.0", "beam_size": "1"}),
    ("BEAM=3", {"temperature": "0.0", "beam_size": "3"}),
    ("BEAM=5 (default)", {"temperature": "0.0", "beam_size": "5"}),
    ("BEAM=10", {"temperature": "0.0", "beam_size": "10"}),

    # Repetition penalty
    ("REP=1.0 (none)", {"temperature": "0.0", "repetition_penalty": "1.0"}),
    ("REP=1.1", {"temperature": "0.0", "repetition_penalty": "1.1"}),
    ("REP=1.2", {"temperature": "0.0", "repetition_penalty": "1.2"}),
    ("REP=1.5", {"temperature": "0.0", "repetition_penalty": "1.5"}),
    ("REP=2.0", {"temperature": "0.0", "repetition_penalty": "2.0"}),

    # No repeat ngram
    ("NGRAM=0 (off)", {"temperature": "0.0", "no_repeat_ngram_size": "0"}),
    ("NGRAM=2", {"temperature": "0.0", "no_repeat_ngram_size": "2"}),
    ("NGRAM=3", {"temperature": "0.0", "no_repeat_ngram_size": "3"}),
    ("NGRAM=4", {"temperature": "0.0", "no_repeat_ngram_size": "4"}),

    # Condition on previous
    ("COND_PREV=true", {"temperature": "0.0", "condition_on_previous_text": "true"}),
    ("COND_PREV=false", {"temperature": "0.0", "condition_on_previous_text": "false"}),

    # Hallucination silence threshold
    ("HALLUC=0 (off)", {"temperature": "0.0"}),
    ("HALLUC=1.0", {"temperature": "0.0", "hallucination_silence_threshold": "1.0"}),
    ("HALLUC=2.0", {"temperature": "0.0", "hallucination_silence_threshold": "2.0"}),
    ("HALLUC=3.0", {"temperature": "0.0", "hallucination_silence_threshold": "3.0"}),

    # No speech threshold
    ("NOSPEECH=0.3", {"temperature": "0.0", "no_speech_threshold": "0.3"}),
    ("NOSPEECH=0.6 (default)", {"temperature": "0.0", "no_speech_threshold": "0.6"}),
    ("NOSPEECH=0.8", {"temperature": "0.0", "no_speech_threshold": "0.8"}),

    # Prompt
    ("PROMPT: none", {"temperature": "0.0"}),
    ("PROMPT: detailed dispatch",
     {"temperature": "0.0",
      "prompt": "Butler County dispatch radio. Medic 23, Engine 7, Rescue 52, District 1 Life Squad. 10-4, copy, en route, responding, on scene. State Route 134, Hamilton-Mason Road."}),
    ("PROMPT: short generic",
     {"temperature": "0.0",
      "prompt": "Police and fire dispatch radio communications."}),

    # Hotwords
    ("HOTWORDS: radio terms",
     {"temperature": "0.0",
      "hotwords": "Medic,Engine,Ladder,Rescue,Squad,District,dispatch,responding,en route,10-4,copy,State Route,Hamilton"}),

    # Compression ratio
    ("COMPRESS=1.8 (strict)", {"temperature": "0.0", "compression_ratio_threshold": "1.8"}),
    ("COMPRESS=2.4 (default)", {"temperature": "0.0", "compression_ratio_threshold": "2.4"}),
    ("COMPRESS=3.0 (permissive)", {"temperature": "0.0", "compression_ratio_threshold": "3.0"}),

    # VAD
    ("VAD=off", {"temperature": "0.0", "vad_filter": "false"}),
    ("VAD=on (defaults)", {"temperature": "0.0", "vad_filter": "true"}),
    ("VAD=aggressive",
     {"temperature": "0.0", "vad_filter": "true",
      "vad_threshold": "0.3", "vad_min_speech_duration_ms": "100"}),

    # Combos
    ("COMBO-A: rep1.2+ngram3+noprev+halluc2.0",
     {"temperature": "0.0",
      "repetition_penalty": "1.2", "no_repeat_ngram_size": "3",
      "condition_on_previous_text": "false",
      "hallucination_silence_threshold": "2.0"}),

    ("COMBO-B: rep1.1+ngram3+noprev+prompt",
     {"temperature": "0.0",
      "repetition_penalty": "1.1", "no_repeat_ngram_size": "3",
      "condition_on_previous_text": "false",
      "prompt": "Butler County dispatch radio. Medic 23, Engine 7, Rescue 52, District 1 Life Squad."}),

    ("COMBO-C: kitchen sink",
     {"temperature": "0.0,0.2,0.4,0.6,0.8,1.0",
      "beam_size": "5",
      "repetition_penalty": "1.2", "no_repeat_ngram_size": "3",
      "condition_on_previous_text": "false",
      "hallucination_silence_threshold": "2.0",
      "no_speech_threshold": "0.6",
      "prompt": "Butler County dispatch radio. Medic 23, Engine 7, Rescue 52, District 1 Life Squad. 10-4, copy, en route.",
      "hotwords": "Medic,Engine,Ladder,Rescue,Squad,District,dispatch,responding,en route,10-4"}),

    ("COMBO-D: prompt+hotwords+rep1.2+halluc2.0 (no ngram)",
     {"temperature": "0.0",
      "repetition_penalty": "1.2",
      "condition_on_previous_text": "false",
      "hallucination_silence_threshold": "2.0",
      "prompt": "Butler County dispatch radio. Medic 23, Engine 7, Rescue 52, District 1 Life Squad.",
      "hotwords": "Medic,Engine,Ladder,Rescue,Squad,District,dispatch,responding,en route,10-4"}),
]

all_results = {}
total = len(TESTS)

with open(OUTFILE, "w", encoding="utf-8") as out:
    for i, (label, params) in enumerate(TESTS, 1):
        header = f"\n{'='*70}\n[{i}/{total}] {label}\n  params: {params}\n{'='*70}"
        print(header)
        out.write(header + "\n")
        out.flush()

        results = run_test(label, params)
        all_results[label] = results

        for r in results:
            line = (f"  {r.get('file','?'):<45} dur={r.get('duration',0):<6} "
                    f"proc={r.get('proc_time',0):<6} avgP={r.get('avg_prob',0):.3f} "
                    f"minP={r.get('min_prob',0):.3f} low={r.get('low_conf_words',0)}")
            out.write(line + "\n")
            out.write(f"    > {r.get('text','')}\n")
        out.flush()

    # Summary table
    out.write(f"\n\n{'='*70}\nSUMMARY TABLE\n{'='*70}\n")
    out.write(f"{'Config':<55} {'AvgProb':>8} {'MinProb':>8} {'LowConf':>8} {'AvgProc':>8}\n")
    out.write("-" * 90 + "\n")

    for label, results in all_results.items():
        valid = [r for r in results if "error" not in r]
        if not valid:
            continue
        avg_p = sum(r["avg_prob"] for r in valid) / len(valid)
        min_p = min(r["min_prob"] for r in valid)
        low_c = sum(r["low_conf_words"] for r in valid)
        avg_t = sum(r["proc_time"] for r in valid) / len(valid)
        line = f"{label:<55} {avg_p:>8.3f} {min_p:>8.3f} {low_c:>8d} {avg_t:>8.3f}"
        print(line)
        out.write(line + "\n")

print(f"\nResults saved to {OUTFILE}")
print("SWEEP COMPLETE")
