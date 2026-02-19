# faster-whisper API Server

A minimal OpenAI-compatible API server for [faster-whisper](https://github.com/SYSTRAN/faster-whisper) that exposes all 32 transcribe() parameters. Built for P25 radio transcription in tr-engine.

## Why not speaches-ai?

[speaches-ai](https://github.com/speaches-ai/speaches) only passes 10 of 32 faster-whisper parameters to `model.transcribe()`. Critical parameters like `repetition_penalty`, `condition_on_previous_text`, `beam_size`, `hallucination_silence_threshold`, and `no_repeat_ngram_size` are silently ignored. It also hardcodes `temperature=0.0` instead of supporting the fallback list (see [issue #134](https://github.com/speaches-ai/speaches/issues/134), [#583](https://github.com/speaches-ai/speaches/issues/583)).

This server is a single 300-line Python file that passes everything through.

## Prerequisites

### NVIDIA GPU (recommended)

The server runs on CPU but is impractical without a GPU. Any CUDA-capable NVIDIA GPU works. VRAM requirements by model:

| Model | VRAM (float16) | VRAM (int8) | Quality | Speed |
|-------|---------------|-------------|---------|-------|
| large-v3 | ~3 GB | ~2 GB | Best for radio | Baseline |
| large-v3-turbo | ~2 GB | ~1.5 GB | Worse on vocoder audio | 3-4x faster |
| distil-large-v3 | ~2 GB | ~1.5 GB | Untested on radio | 2-3x faster |
| medium | ~1.5 GB | ~1 GB | Untested on radio | 2x faster |

**Recommended: `large-v3` with float16.** See [TUNING.md](TUNING.md) for why turbo/distil models struggle with P25 IMBE vocoder audio.

### NVIDIA Driver + CUDA

The NVIDIA driver includes CUDA runtime. You do NOT need to install the CUDA Toolkit separately -- faster-whisper bundles its own CTranslate2 CUDA libraries.

**Check your driver:**
```bash
nvidia-smi
```

If this shows your GPU and a CUDA version, you're good. If not, install the driver from [nvidia.com/drivers](https://www.nvidia.com/drivers/).

Minimum driver versions:
- CUDA 12.x: Driver 525+ (Linux), 528+ (Windows)
- CUDA 11.x: Driver 450+ (any recent driver)

### Python 3.10+

**Windows:**
```
winget install Python.Python.3.12
```
Or download from [python.org](https://www.python.org/downloads/). Check "Add to PATH" during install.

**Ubuntu/Debian:**
```bash
sudo apt update && sudo apt install python3 python3-pip python3-venv
```

**Verify:**
```bash
python --version   # or python3 --version
```

## Installation

### Option 1: Native (simplest)

```bash
cd tools/whisper-server

# (Optional) Create a virtual environment
python -m venv .venv
# Windows: .venv\Scripts\activate
# Linux/Mac: source .venv/bin/activate

# Install dependencies
pip install -r requirements.txt
```

This installs:
- **faster-whisper** -- CTranslate2-based Whisper inference (includes cuDNN, cuBLAS)
- **fastapi** -- HTTP framework
- **uvicorn** -- ASGI server
- **python-multipart** -- Form/file upload parsing

The first run downloads the model from HuggingFace (~3 GB for large-v3). It's cached in `~/.cache/huggingface/` and reused on subsequent starts.

### Option 2: Docker (GPU passthrough)

Requires [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html):

```bash
# Ubuntu/Debian
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
sudo apt update && sudo apt install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

Then:
```bash
cd tools/whisper-server
docker compose up -d
```

The compose file handles GPU passthrough and persists the HuggingFace model cache in a named volume.

## Starting the Server

### Windows
```
start.bat
start.bat large-v3 cuda float16 8000
```

### Linux/Mac
```bash
chmod +x start.sh
./start.sh
./start.sh large-v3 cuda float16 8000
```

### Direct
```bash
# Defaults: large-v3, auto device, float16, port 8000
python server.py

# Custom via env vars
WHISPER_MODEL=large-v3 DEVICE=cuda COMPUTE_TYPE=float16 PORT=8000 python server.py
```

### Docker
```bash
docker compose up -d

# Custom model
WHISPER_MODEL=large-v3 docker compose up -d

# View logs
docker compose logs -f
```

## Verifying

```bash
# Health check
curl http://localhost:8000/health

# List models
curl http://localhost:8000/v1/models

# Transcribe a file
curl -X POST http://localhost:8000/v1/audio/transcriptions \
  -F "file=@test.m4a" \
  -F "language=en" \
  -F "response_format=verbose_json" \
  -F "timestamp_granularities[]=word" \
  -F "condition_on_previous_text=false"
```

## Configuration for tr-engine

Add to your `.env`:

```env
WHISPER_URL=http://localhost:8000/v1/audio/transcriptions
WHISPER_MODEL=large-v3
WHISPER_CONDITION_ON_PREV=false
```

See [TUNING.md](TUNING.md) for the full parameter sweep results and recommended settings.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `WHISPER_MODEL` | `large-v3` | HuggingFace model ID or local path |
| `DEVICE` | `auto` | `auto`, `cuda`, or `cpu` |
| `COMPUTE_TYPE` | `float16` | `float16`, `int8`, `int8_float16`, `float32` |
| `HOST` | `0.0.0.0` | Listen address |
| `PORT` | `8000` | Listen port |

## API Reference

### POST /v1/audio/transcriptions

OpenAI-compatible endpoint. Accepts multipart form data.

**Standard OpenAI fields:**
- `file` (required) -- Audio file (m4a, wav, mp3, etc.)
- `model` -- Model name (ignored, uses loaded model)
- `language` -- ISO 639-1 language code (default: `en`)
- `prompt` -- Initial prompt for domain vocabulary
- `response_format` -- `json`, `verbose_json`, `text`, `srt`, `vtt`
- `temperature` -- Float or comma-separated fallback list (default: `0.0,0.2,0.4,0.6,0.8,1.0`)
- `timestamp_granularities[]` -- `word` and/or `segment`

**Extended faster-whisper fields (all optional):**
- `beam_size` -- Beam search width (default: 5)
- `best_of` -- Number of candidates (default: 5)
- `patience` -- Beam search patience (default: 1.0)
- `length_penalty` -- Length penalty (default: 1.0)
- `repetition_penalty` -- Repetition penalty (default: 1.0, don't increase for radio)
- `no_repeat_ngram_size` -- Block n-gram repetition (default: 0)
- `compression_ratio_threshold` -- Compression ratio filter (default: 2.4)
- `log_prob_threshold` -- Log probability filter (default: -1.0)
- `no_speech_threshold` -- No-speech detection (default: 0.6)
- `condition_on_previous_text` -- Use previous segment as context (default: true, **set false for radio**)
- `prompt_reset_on_temperature` -- Reset prompt on temp fallback (default: 0.5)
- `suppress_blank` -- Suppress blank outputs (default: true)
- `suppress_tokens` -- Token IDs to suppress (default: -1)
- `max_new_tokens` -- Max output tokens per segment (default: unlimited)
- `max_initial_timestamp` -- Max initial timestamp (default: 1.0)
- `hallucination_silence_threshold` -- Skip hallucinations in silence (default: disabled)
- `hotwords` -- Vocabulary boost terms (default: none)
- `word_timestamps` -- Enable word-level timestamps (default: auto from granularities)
- `without_timestamps` -- Disable timestamps (default: false)

**VAD fields:**
- `vad_filter` -- Enable Silero VAD preprocessing (default: false, **don't use for radio**)
- `vad_threshold` -- VAD detection threshold (default: 0.5)
- `vad_min_speech_duration_ms` -- Min speech duration (default: 250)
- `vad_min_silence_duration_ms` -- Min silence duration (default: 2000)
- `vad_max_speech_duration_s` -- Max speech duration (default: inf)
- `vad_speech_pad_ms` -- Speech padding (default: 400)

### GET /v1/models

Returns the loaded model in OpenAI format.

### GET /health

Returns server status, model name, device, and compute type.

## Troubleshooting

**"No module named 'faster_whisper'"**
```bash
pip install -r requirements.txt
```

**CTranslate2 CUDA error / "Could not load library libcudnn"**
Your NVIDIA driver may be too old. Update from [nvidia.com/drivers](https://www.nvidia.com/drivers/). faster-whisper bundles its own CUDA libraries but needs a compatible driver.

**Model download is slow**
Install the HuggingFace Xet extension for faster downloads:
```bash
pip install huggingface_hub[hf_xet]
```

**"symlinks not supported" warning on Windows**
Cosmetic only. Enable Developer Mode in Windows Settings > For Developers to suppress it.

**Out of VRAM**
Switch to int8 quantization:
```bash
COMPUTE_TYPE=int8 python server.py
```
Or use a smaller model (`medium`, `small`).

**Server starts but transcription is slow**
Check that `nvidia-smi` shows the GPU. If `DEVICE=auto` falls back to CPU, set `DEVICE=cuda` explicitly. CPU inference on large-v3 is roughly 100x slower than GPU.
