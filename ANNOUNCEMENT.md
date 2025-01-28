**🎉 Announcing TR-Engine: A Trunk Radio Monitoring & Analysis Server**

TR-Engine is now available at <https://github.com/LumenPrima/tr-engine>! This high-throughput server ingests MQTT message streams from trunk-recorder systems, with features like:

- AI-powered voice transcription (local Whisper instance preferred, OpenAI API compatible)
- Advanced search with multi-field and time-range queries
- Comprehensive monitoring dashboard
- MQTT message validation and error tracking
- Audio quality assessment

**Key Documentation:**
- API Usage Guide: <https://github.com/LumenPrima/tr-engine/blob/main/designdocs/api-usage.md>
- AI Features Overview: <https://github.com/LumenPrima/tr-engine/blob/main/designdocs/ai-features.md>
- System Architecture: <https://github.com/LumenPrima/tr-engine/blob/main/designdocs/app-structure.md>
- Audio File Handling: <https://github.com/LumenPrima/tr-engine/blob/main/designdocs/audio-file-handling.md>

> ⚠️ **Note:** This is an early release with some features still in development:
> - WebSocket implementation planned
> - Authentication not yet implemented
> - Some features may be unstable

**Required Dependencies:**
- [trunk-recorder](https://github.com/robotastic/trunk-recorder) with [MQTT Status plugin](https://github.com/taclane/trunk-recorder-mqtt-status) compiled in

The documentation is AI-assisted and may contain inaccuracies. Please verify all information in the docs against the actual code.

Check out the Implementation Report for details: <https://github.com/LumenPrima/tr-engine/blob/main/designdocs/system-implementation-report.md>
