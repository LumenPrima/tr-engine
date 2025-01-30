const logger = require('../../../utils/logger');
const fileStorage = require('../message-processor/file-storage');
const WebSocketServer = require('../../../api/websocket/server');

class AudioHandler {
  constructor() {
    this.wsServer = null;
  }

  setWebSocketServer(server) {
    this.wsServer = server;
  }

  /**
   * Process an audio message
   * @param {string} topic - MQTT topic
   * @param {Object} originalMessage - Original message
   * @param {Object} transformedMessage - Transformed message
   */
  async processAudioMessage(topic, originalMessage, transformedMessage) {
    logger.debug('Processing audio message:', {
      topic,
      hasAudioWav: !!originalMessage.call?.audio_wav_base64,
      hasAudioM4a: !!originalMessage.call?.audio_m4a_base64,
      metadata: originalMessage.call?.metadata
    });

    if (!originalMessage.call || (!originalMessage.call.audio_wav_base64 && !originalMessage.call.audio_m4a_base64)) {
      logger.debug('No audio data to process in message');
      return;
    }

    const fileStorageTasks = [];
    
    // Resolve filename from various possible sources
    const baseFilename = transformedMessage.filename || 
      originalMessage.call?.metadata?.filename || 
      `unknown_${Date.now()}`;

    // Store WAV file if present
    if (originalMessage.call?.audio_wav_base64) {
      const wavFilename = baseFilename.endsWith('.wav') ? baseFilename : `${baseFilename}.wav`;
      logger.debug('Preparing to store WAV file:', {
        baseFilename,
        wavFilename,
        audioDataLength: originalMessage.call.audio_wav_base64.length
      });
      
      fileStorageTasks.push(
        fileStorage.storeFile(
          originalMessage.call.audio_wav_base64, 
          originalMessage, 
          wavFilename
        )
      );
    }

    // Store M4A file if present
    if (originalMessage.call?.audio_m4a_base64) {
      const m4aFilename = baseFilename.replace('.wav', '.m4a');
      logger.debug('Preparing to store M4A file:', {
        baseFilename,
        m4aFilename,
        audioDataLength: originalMessage.call.audio_m4a_base64.length
      });
      
      fileStorageTasks.push(
        fileStorage.storeFile(
          originalMessage.call.audio_m4a_base64, 
          originalMessage, 
          m4aFilename
        )
      );
    }

    // Process all file storage tasks
    if (fileStorageTasks.length > 0) {
      try {
        logger.debug('Starting file storage tasks', {
          numberOfTasks: fileStorageTasks.length,
          baseFilename
        });
        
        await Promise.all(fileStorageTasks);
        
        logger.debug('Completed file storage tasks successfully', {
          baseFilename
        });

        // Broadcast audio to subscribed WebSocket clients
        if (this.wsServer) {
          const metadata = {
            call_id: transformedMessage.call_id || (
                transformedMessage.short_name ? 
                `${transformedMessage.short_name.replace('sys', '')}_${transformedMessage.talkgroup}_${transformedMessage.start_time}` :
                `${transformedMessage.talkgroup}_${transformedMessage.start_time}`
            ),
            talkgroup: transformedMessage.talkgroup,
            talkgroup_tag: transformedMessage.talkgroup_tag,
            talkgroup_description: transformedMessage.talkgroup_description,
            start_time: transformedMessage.start_time,
            stop_time: transformedMessage.stop_time,
            call_length: transformedMessage.call_length,
            emergency: transformedMessage.emergency === 1,
            encrypted: transformedMessage.encrypted === 1,
            freq: transformedMessage.freq,
            audio_type: transformedMessage.audio_type,
            short_name: transformedMessage.short_name,
            srcList: transformedMessage.srcList || [],
            filename: baseFilename
          };

          const audioData = {
            audio_wav_base64: originalMessage.call?.audio_wav_base64,
            audio_m4a_base64: originalMessage.call?.audio_m4a_base64
          };

          this.wsServer.broadcastAudio(audioData, metadata);
          logger.debug('Broadcasted audio to WebSocket clients', {
            talkgroup: metadata.talkgroup,
            filename: metadata.filename
          });
        }
      } catch (error) {
        logger.error('Error storing audio files:', {
          error: error.message,
          errorStack: error.stack,
          baseFilename
        });
        throw error;
      }
    }
  }
}

module.exports = new AudioHandler();
