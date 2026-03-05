// AudioWorklet processor with jitter buffer for live radio audio.
// Receives PCM int16 samples via port.postMessage, outputs at AudioContext sample rate.

class RadioAudioProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.buffer = new Float32Array(16384); // ~2s ring buffer at 8kHz
    this.writePos = 0;
    this.readPos = 0;
    this.buffered = 0;
    this.inputSampleRate = 8000;
    this.active = true;

    this.port.onmessage = (e) => {
      if (e.data.type === 'audio') {
        this.enqueueSamples(e.data.samples, e.data.sampleRate);
      } else if (e.data.type === 'stop') {
        this.active = false;
      }
    };
  }

  enqueueSamples(int16Array, sr) {
    if (sr && sr !== this.inputSampleRate) {
      this.inputSampleRate = sr;
    }

    for (let i = 0; i < int16Array.length; i++) {
      this.buffer[this.writePos] = int16Array[i] / 32768.0;
      this.writePos = (this.writePos + 1) % this.buffer.length;
      this.buffered = Math.min(this.buffered + 1, this.buffer.length);
    }

    // Overflow protection: if buffered > 150ms worth, skip ahead to 80ms
    const maxSamples = Math.floor(this.inputSampleRate * 0.15);
    const targetSamples = Math.floor(this.inputSampleRate * 0.08);
    if (this.buffered > maxSamples) {
      const skip = this.buffered - targetSamples;
      this.readPos = (this.readPos + skip) % this.buffer.length;
      this.buffered -= skip;
    }
  }

  process(inputs, outputs, parameters) {
    if (!this.active) return false;

    const output = outputs[0][0]; // mono
    if (!output) return true;

    const ratio = this.inputSampleRate / sampleRate; // sampleRate is global AudioWorklet var (48000 typically)

    for (let i = 0; i < output.length; i++) {
      if (this.buffered > 0) {
        output[i] = this.buffer[this.readPos];
        // Advance read position by ratio to resample
        const advance = Math.max(1, Math.round(ratio));
        for (let j = 0; j < advance && this.buffered > 0; j++) {
          this.readPos = (this.readPos + 1) % this.buffer.length;
          this.buffered--;
        }
      } else {
        output[i] = 0; // silence on underrun
      }
    }

    return true;
  }
}

registerProcessor('radio-audio-processor', RadioAudioProcessor);
