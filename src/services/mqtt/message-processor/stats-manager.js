class StatsManager {
  constructor() {
    this.stats = {
      processed: 0,
      errors: 0,
      processingTimes: [],
      messageTypes: new Map(),
      lastError: null
    };
  }

  /**
   * Update processing statistics
   * @param {string} messageType - Type of processed message
   * @param {[number, number]} startTime - Process start time from hrtime
   */
  updateStats(messageType, startTime) {
    const diff = process.hrtime(startTime);
    const processingTime = (diff[0] * 1e9 + diff[1]) / 1e6;

    this.stats.processed++;
    this.stats.processingTimes.push(processingTime);
    if (this.stats.processingTimes.length > 100) {
      this.stats.processingTimes.shift();
    }

    const typeCount = this.stats.messageTypes.get(messageType) || 0;
    this.stats.messageTypes.set(messageType, typeCount + 1);
  }

  /**
   * Handle and log processing errors
   * @param {Error} err - Error object
   * @param {string} topic - MQTT topic where error occurred
   */
  handleError(err, topic) {
    this.stats.errors++;
    this.stats.lastError = {
      time: new Date(),
      message: err.message,
      topic,
      stack: err.stack
    };
  }

  /**
   * Get current processing statistics
   * @returns {Object} Processing stats
   */
  getStats() {
    const times = this.stats.processingTimes;
    const avgTime = times.length ? 
      times.reduce((a, b) => a + b, 0) / times.length : 0;

    return {
      processed: this.stats.processed,
      errors: this.stats.errors,
      averageProcessingTime: avgTime,
      messageTypes: Object.fromEntries(this.stats.messageTypes),
      uptime: process.uptime()
    };
  }
}

module.exports = new StatsManager();