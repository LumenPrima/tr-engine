const EventEmitter = require('events');
const logger = require('../../utils/logger');

class RecordingMonitor extends EventEmitter {
    constructor() {
        super();
        this.recorderStates = new Map();
        this.storageThresholds = {
            warning: 85,  // Percentage
            critical: 95
        };
        
        // Storage paths to monitor
        this.storagePaths = {
            recordings: process.env.RECORDINGS_PATH || '/var/lib/tr/recordings',
            temp: process.env.TEMP_RECORDINGS_PATH || '/var/lib/tr/temp',
            database: process.env.DB_PATH || '/var/lib/mongodb'
        };
        
        // Track recording failures
        this.failureTracking = new Map();
        
        // Track storage stats
        this.storageStats = new Map();
        
        // Initialize storage monitoring
        this.monitorStorage();
        
        logger.info('RecordingMonitor initialized');
    }

    updateRecorderState(recorderId, state) {
        const previousState = this.recorderStates.get(recorderId);
        const timestamp = new Date();

        // Update state
        this.recorderStates.set(recorderId, {
            ...state,
            lastUpdate: timestamp
        });

        // Check for state changes
        if (previousState?.status !== state.status) {
            this.emit('recorder.statusChange', {
                recorderId,
                previousStatus: previousState?.status,
                newStatus: state.status,
                timestamp
            });
        }

        // Track failures
        if (state.status === 'error') {
            this.trackFailure(recorderId, state.error);
        }
    }

    trackFailure(recorderId, error) {
        const failures = this.failureTracking.get(recorderId) || [];
        failures.push({
            timestamp: new Date(),
            error
        });

        // Keep last 24 hours of failures
        const dayAgo = new Date(Date.now() - 24 * 60 * 60 * 1000);
        const recentFailures = failures.filter(f => f.timestamp > dayAgo);
        
        this.failureTracking.set(recorderId, recentFailures);

        // Emit alert if too many recent failures
        if (recentFailures.length >= 3) {
            this.emit('recorder.failurePattern', {
                recorderId,
                failures: recentFailures
            });
        }
    }

    getRecorderState(recorderId) {
        return this.recorderStates.get(recorderId);
    }

    getAllRecorderStates() {
        return Array.from(this.recorderStates.entries()).map(([id, state]) => ({
            recorderId: id,
            ...state
        }));
    }

    getFailureHistory(recorderId) {
        return this.failureTracking.get(recorderId) || [];
    }

    async monitorStorage() {
        const { execFile } = require('child_process');
        const util = require('util');
        const execFileAsync = util.promisify(execFile);

        // Monitor every 5 minutes
        setInterval(async () => {
            try {
                for (const [type, path] of Object.entries(this.storagePaths)) {
                    const { stdout } = await execFileAsync('df', ['-B1', path]);
                    const [, , used, available] = stdout.split('\n')[1].split(/\s+/);
                    
                    const total = parseInt(used) + parseInt(available);
                    const usedPercent = (parseInt(used) / total) * 100;
                    
                    const stats = {
                        total,
                        used: parseInt(used),
                        available: parseInt(available),
                        usedPercent,
                        timestamp: new Date()
                    };
                    
                    this.storageStats.set(type, stats);
                    
                    // Check thresholds
                    if (usedPercent >= this.storageThresholds.critical) {
                        this.emit('storage.critical', {
                            type,
                            stats,
                            message: `Critical storage level for ${type}: ${usedPercent.toFixed(1)}%`
                        });
                    } else if (usedPercent >= this.storageThresholds.warning) {
                        this.emit('storage.warning', {
                            type,
                            stats,
                            message: `High storage usage for ${type}: ${usedPercent.toFixed(1)}%`
                        });
                    }
                }
            } catch (error) {
                logger.error('Storage monitoring error:', error);
                this.emit('storage.error', error);
            }
        }, 5 * 60 * 1000); // Every 5 minutes
    }

    getStorageStats(type = null) {
        if (type) {
            return this.storageStats.get(type);
        }
        return Object.fromEntries(this.storageStats);
    }
}

module.exports = RecordingMonitor;
