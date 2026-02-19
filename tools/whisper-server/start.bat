@echo off
REM Start the faster-whisper API server
REM Usage: start.bat [model] [device] [compute_type] [port]
REM
REM Examples:
REM   start.bat                                    (large-v3, auto, float16, port 8000)
REM   start.bat large-v3 cuda float16 8000         (explicit)
REM   start.bat distil-large-v3 cpu int8 9000      (CPU mode)

setlocal

set WHISPER_MODEL=%~1
if "%WHISPER_MODEL%"=="" set WHISPER_MODEL=large-v3

set DEVICE=%~2
if "%DEVICE%"=="" set DEVICE=auto

set COMPUTE_TYPE=%~3
if "%COMPUTE_TYPE%"=="" set COMPUTE_TYPE=float16

set PORT=%~4
if "%PORT%"=="" set PORT=8000

echo ============================================
echo  faster-whisper API server
echo  Model:   %WHISPER_MODEL%
echo  Device:  %DEVICE%
echo  Compute: %COMPUTE_TYPE%
echo  Port:    %PORT%
echo ============================================

REM Check Python
python --version >nul 2>&1
if errorlevel 1 (
    echo ERROR: Python not found in PATH
    exit /b 1
)

REM Check dependencies
python -c "import faster_whisper" >nul 2>&1
if errorlevel 1 (
    echo Installing dependencies...
    pip install -r "%~dp0requirements.txt"
)

REM Start server
python "%~dp0server.py"
