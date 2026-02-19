"""
Minimal OpenAI-compatible faster-whisper API server.

Exposes ALL faster-whisper transcribe() parameters as form fields,
unlike speaches-ai which only passes 10 of 32.

Usage:
    pip install faster-whisper fastapi uvicorn python-multipart
    python server.py

    # Or with env vars:
    WHISPER_MODEL=large-v3 DEVICE=cuda COMPUTE_TYPE=float16 python server.py

Endpoints:
    POST /v1/audio/transcriptions  — OpenAI-compatible transcription
    GET  /v1/models                — List loaded model
    GET  /health                   — Health check
"""

import io
import os
import tempfile
import time
from typing import Optional

import uvicorn
from fastapi import FastAPI, File, Form, UploadFile
from fastapi.responses import JSONResponse, PlainTextResponse
from faster_whisper import WhisperModel

# ---------------------------------------------------------------------------
# Configuration (env vars)
# ---------------------------------------------------------------------------
MODEL_ID = os.environ.get("WHISPER_MODEL", "large-v3")
DEVICE = os.environ.get("DEVICE", "auto")
COMPUTE_TYPE = os.environ.get("COMPUTE_TYPE", "float16")
HOST = os.environ.get("HOST", "0.0.0.0")
PORT = int(os.environ.get("PORT", "8000"))

# ---------------------------------------------------------------------------
# App + model
# ---------------------------------------------------------------------------
app = FastAPI(title="faster-whisper-server", version="1.0.0")
whisper_model: Optional[WhisperModel] = None


@app.on_event("startup")
def load_model():
    global whisper_model
    print(f"Loading model: {MODEL_ID} (device={DEVICE}, compute_type={COMPUTE_TYPE})")
    whisper_model = WhisperModel(MODEL_ID, device=DEVICE, compute_type=COMPUTE_TYPE)
    print("Model loaded.")


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
def parse_temperature(raw: str):
    """Parse temperature as float or comma-separated fallback list."""
    if "," in raw:
        return [float(t.strip()) for t in raw.split(",") if t.strip()]
    val = float(raw)
    # Single 0.0 → use faster-whisper default fallback behavior
    if val == 0.0:
        return 0.0
    return val


def format_timestamp(seconds: float) -> str:
    """Format seconds as SRT/VTT timestamp."""
    h = int(seconds // 3600)
    m = int((seconds % 3600) // 60)
    s = int(seconds % 60)
    ms = int((seconds % 1) * 1000)
    return f"{h:02d}:{m:02d}:{s:02d},{ms:03d}"


def format_vtt_timestamp(seconds: float) -> str:
    h = int(seconds // 3600)
    m = int((seconds % 3600) // 60)
    s = int(seconds % 60)
    ms = int((seconds % 1) * 1000)
    return f"{h:02d}:{m:02d}:{s:02d}.{ms:03d}"


def segments_to_srt(segments):
    lines = []
    for i, seg in enumerate(segments, 1):
        lines.append(str(i))
        lines.append(f"{format_timestamp(seg['start'])} --> {format_timestamp(seg['end'])}")
        lines.append(seg["text"].strip())
        lines.append("")
    return "\n".join(lines)


def segments_to_vtt(segments):
    lines = ["WEBVTT", ""]
    for seg in segments:
        lines.append(f"{format_vtt_timestamp(seg['start'])} --> {format_vtt_timestamp(seg['end'])}")
        lines.append(seg["text"].strip())
        lines.append("")
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# POST /v1/audio/transcriptions
# ---------------------------------------------------------------------------
@app.post("/v1/audio/transcriptions")
async def transcribe(
    file: UploadFile = File(...),
    # --- Standard OpenAI params ---
    model: str = Form("whisper-1"),
    language: Optional[str] = Form(None),
    prompt: Optional[str] = Form(None),
    response_format: str = Form("json"),
    temperature: str = Form("0.0,0.2,0.4,0.6,0.8,1.0"),
    # timestamp_granularities[] comes as repeated form field
    timestamp_granularities: Optional[list[str]] = Form(None, alias="timestamp_granularities[]"),
    # --- faster-whisper extended params ---
    beam_size: int = Form(5),
    best_of: int = Form(5),
    patience: float = Form(1.0),
    length_penalty: float = Form(1.0),
    repetition_penalty: float = Form(1.0),
    no_repeat_ngram_size: int = Form(0),
    compression_ratio_threshold: float = Form(2.4),
    log_prob_threshold: float = Form(-1.0),
    no_speech_threshold: float = Form(0.6),
    condition_on_previous_text: bool = Form(True),
    prompt_reset_on_temperature: float = Form(0.5),
    suppress_blank: bool = Form(True),
    suppress_tokens: str = Form("-1"),
    max_new_tokens: Optional[int] = Form(None),
    max_initial_timestamp: float = Form(1.0),
    hallucination_silence_threshold: Optional[float] = Form(None),
    hotwords: Optional[str] = Form(None),
    word_timestamps: Optional[bool] = Form(None),
    without_timestamps: bool = Form(False),
    # --- VAD params ---
    vad_filter: bool = Form(False),
    vad_threshold: float = Form(0.5),
    vad_min_speech_duration_ms: int = Form(250),
    vad_min_silence_duration_ms: int = Form(2000),
    vad_max_speech_duration_s: float = Form(float("inf")),
    vad_speech_pad_ms: int = Form(400),
):
    t0 = time.time()

    # Determine word_timestamps from either explicit param or timestamp_granularities
    want_words = word_timestamps
    if want_words is None:
        want_words = bool(
            timestamp_granularities and "word" in timestamp_granularities
        )

    # Parse temperature (single float or fallback list)
    temp = parse_temperature(temperature)

    # Parse suppress_tokens
    sup_tokens = [int(t.strip()) for t in suppress_tokens.split(",") if t.strip()]

    # Build VAD parameters dict (only if vad_filter enabled)
    vad_params = None
    if vad_filter:
        vad_params = dict(
            threshold=vad_threshold,
            min_speech_duration_ms=vad_min_speech_duration_ms,
            min_silence_duration_ms=vad_min_silence_duration_ms,
            max_speech_duration_s=vad_max_speech_duration_s,
            speech_pad_ms=vad_speech_pad_ms,
        )

    # Write upload to temp file (faster-whisper needs a file path)
    data = await file.read()
    suffix = os.path.splitext(file.filename or "audio.wav")[1] or ".wav"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp.write(data)
        tmp_path = tmp.name

    try:
        segments_gen, info = whisper_model.transcribe(
            tmp_path,
            language=language,
            task="transcribe",
            beam_size=beam_size,
            best_of=best_of,
            patience=patience,
            length_penalty=length_penalty,
            repetition_penalty=repetition_penalty,
            no_repeat_ngram_size=no_repeat_ngram_size,
            temperature=temp,
            compression_ratio_threshold=compression_ratio_threshold,
            log_prob_threshold=log_prob_threshold,
            no_speech_threshold=no_speech_threshold,
            condition_on_previous_text=condition_on_previous_text,
            prompt_reset_on_temperature=prompt_reset_on_temperature,
            initial_prompt=prompt,
            suppress_blank=suppress_blank,
            suppress_tokens=sup_tokens,
            max_new_tokens=max_new_tokens,
            max_initial_timestamp=max_initial_timestamp,
            word_timestamps=want_words,
            without_timestamps=without_timestamps,
            hallucination_silence_threshold=hallucination_silence_threshold,
            hotwords=hotwords,
            vad_filter=vad_filter,
            vad_parameters=vad_params,
        )

        # Consume the generator
        result_segments = []
        all_words = []
        full_text_parts = []

        for seg in segments_gen:
            seg_dict = {
                "id": seg.id,
                "start": round(seg.start, 3),
                "end": round(seg.end, 3),
                "text": seg.text,
                "avg_logprob": round(seg.avg_logprob, 4),
                "compression_ratio": round(seg.compression_ratio, 4),
                "no_speech_prob": round(seg.no_speech_prob, 4),
                "temperature": seg.temperature,
            }
            if want_words and seg.words:
                seg_dict["words"] = [
                    {
                        "word": w.word,
                        "start": round(w.start, 3),
                        "end": round(w.end, 3),
                        "probability": round(w.probability, 4),
                    }
                    for w in seg.words
                ]
                all_words.extend(seg_dict["words"])
            result_segments.append(seg_dict)
            full_text_parts.append(seg.text)

        full_text = "".join(full_text_parts).strip()
        duration = round(info.duration, 3)
        processing_time = round(time.time() - t0, 3)

    finally:
        os.unlink(tmp_path)

    # --- Format response ---
    if response_format == "text":
        return PlainTextResponse(full_text)

    if response_format == "srt":
        return PlainTextResponse(segments_to_srt(result_segments), media_type="text/plain")

    if response_format == "vtt":
        return PlainTextResponse(segments_to_vtt(result_segments), media_type="text/plain")

    if response_format == "verbose_json":
        return JSONResponse({
            "task": "transcribe",
            "language": info.language,
            "duration": duration,
            "text": full_text,
            "segments": result_segments,
            "words": all_words if want_words else [],
            "processing_time": processing_time,
        })

    # Default: json
    return JSONResponse({
        "text": full_text,
    })


# ---------------------------------------------------------------------------
# GET /v1/models
# ---------------------------------------------------------------------------
@app.get("/v1/models")
def list_models():
    return {
        "object": "list",
        "data": [
            {
                "id": MODEL_ID,
                "object": "model",
                "owned_by": "local",
            }
        ],
    }


# ---------------------------------------------------------------------------
# GET /health
# ---------------------------------------------------------------------------
@app.get("/health")
def health():
    return {
        "status": "ok",
        "model": MODEL_ID,
        "device": DEVICE,
        "compute_type": COMPUTE_TYPE,
    }


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    uvicorn.run(app, host=HOST, port=PORT)
