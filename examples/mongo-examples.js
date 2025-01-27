// Example of a CallMessage document with transcription
const exampleCallMessage = {
  "type": "audio",
  "call": {
    "audio_wav_base64": "base64 audio goes here",
    "metadata": {
      "freq": 851350000,
      "freq_error": 176,
      "signal": 999,
      "noise": 999,
      "source_num": 0,
      "recorder_num": 0,
      "tdma_slot": 0,
      "phase2_tdma": 0,
      "start_time": 1737430015,
      "stop_time": 1737430023,
      "emergency": 0,
      "priority": 4,
      "mode": 0,
      "duplex": 0,
      "encrypted": 0,
      "call_length": 6,
      "talkgroup": 9131,
      "talkgroup_tag": "09 WC HOSP SEC",
      "talkgroup_description": "UC Health West Chester Hospital - Security",
      "talkgroup_group_tag": "Security",
      "talkgroup_group": "Butler County (09) Fire/EMS/Hospitals",
      "audio_type": "digital",
      "short_name": "butco2",
      "freqList": [
        {
          "freq": 851350000,
          "time": 1737430015,
          "pos": 0,
          "len": 2.7,
          "error_count": 1,
          "spike_count": 0
        },
        {
          "freq": 851350000,
          "time": 1737430020,
          "pos": 2.7,
          "len": 3.42,
          "error_count": 0,
          "spike_count": 0
        }
      ],
      "srcList": [
        {
          "src": 976109,
          "time": 1737430015,
          "pos": 0,
          "emergency": 0,
          "signal_system": "",
          "tag": ""
        },
        {
          "src": 976001,
          "time": 1737430020,
          "pos": 2.7,
          "emergency": 0,
          "signal_system": "",
          "tag": ""
        }
      ],
      "filename": "9131-1737430014_851350000.0-call_1571.wav",
      // Added transcription field
      "transcription": {
        "text": "Security dispatch to west entrance. Copy that, proceeding to west entrance.",
        "segments": [
          {
            "start_time": 0.0,
            "end_time": 2.4,
            "text": "Security dispatch to west entrance.",
            "confidence": 0.92,
            "source": {
              "unit": 976109,
              "emergency": false,
              "signal_system": "",
              "tag": ""
            }
          },
          {
            "start_time": 2.7,
            "end_time": 5.8,
            "text": "Copy that, proceeding to west entrance.",
            "confidence": 0.95,
            "source": {
              "unit": 976001,
              "emergency": false,
              "signal_system": "",
              "tag": ""
            }
          }
        ],
        "metadata": {
          "model": "guillaumekln/faster-whisper-base.en",
          "processing_time": 1.23,
          "audio_duration": 6.12,
          "timestamp": "2024-01-27T15:45:00.123Z"
        }
      }
    }
  },
  "timestamp": 1737430027,
  "instance_id": "trunk-recorder"
};

// Example of a longer conversation with multiple speakers
const exampleLongCallMessage = {
  "type": "audio",
  "call": {
    "audio_wav_base64": "base64 audio goes here",
    "metadata": {
      // ... same metadata fields as above ...
      "talkgroup": 9468,
      "talkgroup_tag": "09 TA 02",
      "talkgroup_description": "County Regional Transit 2",
      "srcList": [
        {
          "src": 984766,
          "time": 1737429986,
          "pos": 0,
          "emergency": 0,
          "signal_system": "",
          "tag": ""
        },
        {
          "src": 984844,
          "time": 1737429992,
          "pos": 5.04,
          "emergency": 0,
          "signal_system": "",
          "tag": ""
        },
        // ... additional sources ...
      ],
      "transcription": {
        "text": "Bus 12 requesting status check. Dispatch copies, all clear on last run. Confirming next pickup at transit center. Copy that, ETA 10 minutes.",
        "segments": [
          {
            "start_time": 0.0,
            "end_time": 4.8,
            "text": "Bus 12 requesting status check.",
            "confidence": 0.94,
            "source": {
              "unit": 984766,
              "emergency": false,
              "signal_system": "",
              "tag": ""
            }
          },
          {
            "start_time": 5.04,
            "end_time": 9.4,
            "text": "Dispatch copies, all clear on last run.",
            "confidence": 0.91,
            "source": {
              "unit": 984844,
              "emergency": false,
              "signal_system": "",
              "tag": ""
            }
          },
          // ... additional segments ...
        ],
        "metadata": {
          "model": "guillaumekln/faster-whisper-base.en",
          "processing_time": 4.56,
          "audio_duration": 30.0,
          "timestamp": "2024-01-27T15:45:30.456Z"
        }
      }
    }
  },
  "timestamp": 1737430031,
  "instance_id": "trunk-recorder"
};

// Example MongoDB queries for transcriptions
const exampleQueries = {
  // Find all transcriptions for a specific talkgroup
  findByTalkgroup: {
    'call.metadata.talkgroup': 9131,
    'call.metadata.transcription': { $exists: true }
  },
  
  // Find transcriptions containing specific text
  findByText: {
    'call.metadata.transcription.text': /west entrance/i
  },
  
  // Find transcriptions by unit
  findByUnit: {
    'call.metadata.transcription.segments.source.unit': 976109
  },
  
  // Find emergency transmissions with transcriptions
  findEmergencies: {
    'call.metadata.emergency': 1,
    'call.metadata.transcription': { $exists: true }
  }
};
