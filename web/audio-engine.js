// Main thread audio coordinator for live radio streaming.
// Manages WebSocket connection, per-TG audio nodes, mixing, and compression.
// Usage: const engine = new AudioEngine(); await engine.start(); engine.subscribe({tgids: [1234]});

class AudioEngine {
  constructor(wsPath, options) {
    options = options || {};
    this.wsPath = wsPath || '/api/v1/audio/live';
    this.options = {
      reconnectMaxMs: options.reconnectMaxMs || 30000,
    };
    this.ws = null;
    this.audioCtx = null;
    this.masterGain = null;
    this.masterCompressor = null;
    this.tgNodes = new Map(); // tgid -> { worklet, gain, compressor, compressorEnabled, muted, lastActivity }
    this.reconnectDelay = 1000;
    this.lastSubscription = null;
    this.listeners = {};
    this._intentionalClose = false;
    this._serverAudioFormat = null; // set by server 'config' message; null = auto-detect
  }

  // Event emitter
  on(event, fn) {
    if (!this.listeners[event]) this.listeners[event] = [];
    this.listeners[event].push(fn);
    return this;
  }

  off(event, fn) {
    if (!this.listeners[event]) return;
    this.listeners[event] = this.listeners[event].filter(function (f) { return f !== fn; });
  }

  emit(event, data) {
    var fns = this.listeners[event] || [];
    for (var i = 0; i < fns.length; i++) {
      fns[i](data);
    }
  }

  async start() {
    this.audioCtx = new AudioContext({ sampleRate: 48000 });
    await this.audioCtx.audioWorklet.addModule('audio-worklet.js');

    // Master chain: compressor -> gain -> destination
    this.masterCompressor = this.audioCtx.createDynamicsCompressor();
    this.masterCompressor.threshold.value = -24;
    this.masterCompressor.knee.value = 12;
    this.masterCompressor.ratio.value = 4;
    this.masterCompressor.attack.value = 0.003;
    this.masterCompressor.release.value = 0.25;

    this.masterGain = this.audioCtx.createGain();
    this.masterCompressor.connect(this.masterGain);
    this.masterGain.connect(this.audioCtx.destination);

    this._loadSettings();
    this._intentionalClose = false;
    this._connect();
  }

  stop() {
    this._intentionalClose = true;
    if (this.ws) {
      this.ws.close(1000);
      this.ws = null;
    }
    var self = this;
    this.tgNodes.forEach(function (nodes, tgid) {
      self._removeTG(tgid);
    });
    this.tgNodes.clear();
    if (this.audioCtx) {
      this.audioCtx.close();
      this.audioCtx = null;
    }
  }

  subscribe(filter) {
    this.lastSubscription = filter;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'subscribe', ...filter }));
    }
  }

  unsubscribe() {
    this.lastSubscription = null;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'unsubscribe' }));
    }
  }

  setVolume(tgid, value) {
    var nodes = this.tgNodes.get(tgid);
    if (nodes) nodes.gain.gain.value = value;
    this._saveSetting('vol_' + tgid, value);
  }

  getVolume(tgid) {
    var nodes = this.tgNodes.get(tgid);
    return nodes ? nodes.gain.gain.value : 1.0;
  }

  setMute(tgid, muted) {
    var nodes = this.tgNodes.get(tgid);
    if (nodes) {
      nodes.muted = muted;
      nodes.gain.gain.value = muted ? 0 : (this._loadSetting('vol_' + tgid) ?? 1.0);
    }
  }

  setMasterVolume(value) {
    if (this.masterGain) this.masterGain.gain.value = value;
    this._saveSetting('master_vol', value);
  }

  getMasterVolume() {
    return this.masterGain ? this.masterGain.gain.value : 1.0;
  }

  setMasterCompressorEnabled(enabled) {
    if (this.masterCompressor) {
      this.masterCompressor.ratio.value = enabled ? 4 : 1;
    }
    this._saveSetting('master_comp', enabled);
  }

  setTGCompressorEnabled(tgid, enabled) {
    var nodes = this.tgNodes.get(tgid);
    if (!nodes) return;
    nodes.compressorEnabled = enabled;
    nodes.compressor.ratio.value = enabled ? 3 : 1;
    this._saveSetting('comp_' + tgid, enabled);
  }

  getActiveTGs() {
    var result = [];
    this.tgNodes.forEach(function (nodes, tgid) {
      result.push({
        tgid: tgid,
        volume: nodes.gain.gain.value,
        muted: !!nodes.muted,
        compressorEnabled: nodes.compressorEnabled,
        lastActivity: nodes.lastActivity,
      });
    });
    return result;
  }

  isConnected() {
    return this.ws && this.ws.readyState === WebSocket.OPEN;
  }

  // --- Internal ---

  _connect() {
    var self = this;
    var protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    var token = window._authToken || '';
    var url = protocol + '//' + location.host + this.wsPath + '?token=' + encodeURIComponent(token);

    this.ws = new WebSocket(url);
    this.ws.binaryType = 'arraybuffer';

    this.ws.onopen = function () {
      self.reconnectDelay = 1000;
      self.emit('status', { connected: true });
      if (self.lastSubscription) {
        self.subscribe(self.lastSubscription);
      }
    };

    this.ws.onmessage = function (event) {
      if (typeof event.data === 'string') {
        try {
          self._handleTextMessage(JSON.parse(event.data));
        } catch (e) {
          // ignore bad JSON
        }
      } else {
        self._handleBinaryFrame(event.data);
      }
    };

    this.ws.onclose = function (event) {
      self.emit('status', { connected: false, code: event.code });
      if (!self._intentionalClose && event.code !== 1000) {
        setTimeout(function () { self._connect(); }, self.reconnectDelay);
        self.reconnectDelay = Math.min(self.reconnectDelay * 2, self.options.reconnectMaxMs);
      }
    };

    this.ws.onerror = function () {
      self.emit('error', { message: 'WebSocket error' });
    };
  }

  _handleTextMessage(msg) {
    switch (msg.type) {
      case 'call_start':
        this.emit('call_start', msg);
        break;
      case 'call_end':
        this.emit('call_end', msg);
        break;
      case 'keepalive':
        this.emit('status', { connected: true, active_streams: msg.active_streams });
        break;
      case 'config':
        if (msg.audio_format) {
          this._serverAudioFormat = msg.audio_format;
        }
        this.emit('config', msg);
        break;
    }
  }

  _handleBinaryFrame(buffer) {
    if (buffer.byteLength < 12) return;

    var view = new DataView(buffer);
    var systemId = view.getUint16(0);
    var tgid = view.getUint32(2);
    // timestamp at offset 6 (4 bytes) - available for latency measurement
    // seq at offset 10 (2 bytes) - available for gap detection

    var audioData = buffer.slice(12);
    var audioLen = audioData.byteLength;

    if (!this.tgNodes.has(tgid)) {
      this._createTG(tgid);
    }

    // Determine format: use server-sent config if available, otherwise auto-detect.
    // PCM frames are 320+ bytes (160+ int16 samples at 8kHz/20ms),
    // Opus frames are typically 10-80 bytes after compression.
    var format = this._serverAudioFormat;
    if (!format) {
      format = (audioLen >= 160) ? 'pcm' : 'opus';
    }

    if (format === 'pcm') {
      var pcmData = new Int16Array(audioData);
      this._feedPCM(tgid, pcmData, 8000);
    } else if (audioLen > 0) {
      this._decodeOpus(tgid, new Uint8Array(audioData));
    }
  }

  _feedPCM(tgid, int16Samples, sampleRate) {
    var nodes = this.tgNodes.get(tgid);
    if (!nodes) return;
    nodes.worklet.port.postMessage({
      type: 'audio',
      samples: int16Samples,
      sampleRate: sampleRate,
    });
    nodes.lastActivity = Date.now();
  }

  async _decodeOpus(tgid, opusData) {
    var nodes = this.tgNodes.get(tgid);
    if (!nodes) return;

    // Lazy-init Opus decoder for this TG
    if (!nodes.opusDecoder) {
      if (typeof AudioDecoder === 'undefined') {
        // Browser doesn't support WebCodecs (e.g. Firefox) — drop Opus frames
        console.warn('AudioDecoder not available; Opus frames will be dropped');
        return;
      }

      try {
        var self = this;
        var currentTgid = tgid;
        nodes.opusDecoder = new AudioDecoder({
          output: function(audioData) {
            // Convert decoded AudioData to Float32Array, then to Int16
            var float32 = new Float32Array(audioData.numberOfFrames);
            audioData.copyTo(float32, { planeIndex: 0 });
            var int16 = new Int16Array(float32.length);
            for (var i = 0; i < float32.length; i++) {
              int16[i] = Math.max(-32768, Math.min(32767, Math.round(float32[i] * 32768)));
            }
            self._feedPCM(currentTgid, int16, audioData.sampleRate);
            audioData.close();
          },
          error: function(e) {
            console.error('Opus decode error:', e);
          }
        });

        nodes.opusDecoder.configure({
          codec: 'opus',
          sampleRate: 8000,
          numberOfChannels: 1,
        });
      } catch (e) {
        console.error('Failed to create Opus decoder:', e);
        return;
      }
    }

    try {
      nodes.opusDecoder.decode(new EncodedAudioChunk({
        type: 'key',
        timestamp: 0,
        data: opusData,
      }));
    } catch (e) {
      // Ignore decode errors for individual frames
    }
  }

  _createTG(tgid) {
    var worklet = new AudioWorkletNode(this.audioCtx, 'radio-audio-processor', {
      outputChannelCount: [1],
    });

    var compressor = this.audioCtx.createDynamicsCompressor();
    compressor.threshold.value = -20;
    compressor.knee.value = 10;
    compressor.ratio.value = 1; // disabled by default
    compressor.attack.value = 0.003;
    compressor.release.value = 0.15;

    var gain = this.audioCtx.createGain();

    // Load persisted settings
    var savedVol = this._loadSetting('vol_' + tgid);
    if (savedVol !== null) gain.gain.value = savedVol;

    var savedComp = this._loadSetting('comp_' + tgid);
    var compEnabled = savedComp === true;
    if (compEnabled) compressor.ratio.value = 3;

    // Chain: worklet -> compressor -> gain -> masterCompressor
    worklet.connect(compressor);
    compressor.connect(gain);
    gain.connect(this.masterCompressor);

    this.tgNodes.set(tgid, {
      worklet: worklet,
      compressor: compressor,
      gain: gain,
      compressorEnabled: compEnabled,
      muted: false,
      lastActivity: Date.now(),
    });

    this.emit('tg_created', { tgid: tgid });
  }

  _removeTG(tgid) {
    var nodes = this.tgNodes.get(tgid);
    if (!nodes) return;
    nodes.worklet.port.postMessage({ type: 'stop' });
    nodes.worklet.disconnect();
    nodes.compressor.disconnect();
    nodes.gain.disconnect();
    if (nodes.opusDecoder) {
      try { nodes.opusDecoder.close(); } catch (e) { /* ignore */ }
    }
    this.tgNodes.delete(tgid);
    this.emit('tg_removed', { tgid: tgid });
  }

  _saveSetting(key, value) {
    try {
      var settings = JSON.parse(localStorage.getItem('audio-engine') || '{}');
      settings[key] = value;
      localStorage.setItem('audio-engine', JSON.stringify(settings));
    } catch (e) {
      // ignore storage errors
    }
  }

  _loadSetting(key) {
    try {
      var settings = JSON.parse(localStorage.getItem('audio-engine') || '{}');
      return settings[key] ?? null;
    } catch (e) {
      return null;
    }
  }

  _loadSettings() {
    var masterVol = this._loadSetting('master_vol');
    if (masterVol !== null && this.masterGain) this.masterGain.gain.value = masterVol;

    var masterComp = this._loadSetting('master_comp');
    if (masterComp === false && this.masterCompressor) this.masterCompressor.ratio.value = 1;
  }
}

// Export for use by pages
window.AudioEngine = AudioEngine;
