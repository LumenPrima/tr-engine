#!/usr/bin/env bash
# Start the faster-whisper API server
# Usage: ./start.sh [model] [device] [compute_type] [port]
#
# Examples:
#   ./start.sh                                    (large-v3, auto, float16, port 8000)
#   ./start.sh large-v3 cuda float16 8000         (explicit)
#   ./start.sh distil-large-v3 cpu int8 9000      (CPU mode)

set -e

export WHISPER_MODEL="${1:-large-v3}"
export DEVICE="${2:-auto}"
export COMPUTE_TYPE="${3:-float16}"
export PORT="${4:-8000}"

echo "============================================"
echo " faster-whisper API server"
echo " Model:   $WHISPER_MODEL"
echo " Device:  $DEVICE"
echo " Compute: $COMPUTE_TYPE"
echo " Port:    $PORT"
echo "============================================"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Check Python
if ! command -v python3 &>/dev/null && ! command -v python &>/dev/null; then
    echo "ERROR: Python not found in PATH"
    exit 1
fi
PYTHON=$(command -v python3 || command -v python)

# Check dependencies
if ! "$PYTHON" -c "import faster_whisper" &>/dev/null; then
    echo "Installing dependencies..."
    "$PYTHON" -m pip install -r "$SCRIPT_DIR/requirements.txt"
fi

exec "$PYTHON" "$SCRIPT_DIR/server.py"
