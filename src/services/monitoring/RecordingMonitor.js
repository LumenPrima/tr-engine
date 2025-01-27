const EventEmitter = require('events');
const logger = require('../../utils/logger');

/**
 * RecordingMonitor
 * Monitors local storage resources and database for the recording system.
 * Note: Remote trunk-recorder monitoring should be handled by separate systems.
 */
class RecordingMonitor extends EventEmitter {
    constructor() {
        super();
        this.recorderStates = new Map();
        this.storageThresholds = {
            warning: 85,  // Percentage
            critical: 95
        };
        this.storageInterval = null;
        
        // Storage paths to monitor
        this.storagePaths = {
            recordings: process.env.RECORDINGS_PATH || '/var/lib/tr/recordings',
            temp: process.env.TEMP_RECORDINGS_PATH || '/var/lib/tr/temp'
        };
        
        // MongoDB connection for stats
        this.mongoose = require('mongoose');
        
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
                // Check filesystem storage
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
                    await this.checkStorageThresholds(type, stats);
                }

                // Check MongoDB storage if connected
                if (this.mongoose.connection.readyState === 1) {
                    try {
                        const dbStats = await this.mongoose.connection.db.stats();
                        const dataSize = dbStats.dataSize;
                        const storageSize = dbStats.storageSize;
                        const usedPercent = (dataSize / storageSize) * 100;
                        
                        const stats = {
                            total: storageSize,
                            used: dataSize,
                            available: storageSize - dataSize,
                            usedPercent,
                            timestamp: new Date(),
                            indexSize: dbStats.indexSize,
                            avgObjSize: dbStats.avgObjSize
                        };
                        
                        this.storageStats.set('database', stats);
                        await this.checkStorageThresholds('database', stats);
                    } catch (error) {
                        logger.error('MongoDB stats check failed:', error);
                        this.emit('storage.error', {
                            type: 'database',
                            error
                        });
                    }
                }
                    
                }
            } catch (error) {
                logger.error('Storage monitoring error:', error);
                this.emit('storage.error', error);
            }
        }, 5 * 60 * 1000); // Every 5 minutes

        // Return the interval handle for cleanup
        return interval;
    }

    async checkStorageThresholds(type, stats) {
        if (stats.usedPercent >= this.storageThresholds.critical) {
            this.emit('storage.critical', {
                type,
                stats,
                message: `Critical storage level for ${type}: ${stats.usedPercent.toFixed(1)}%`
            });
        } else if (stats.usedPercent >= this.storageThresholds.warning) {
            this.emit('storage.warning', {
                type,
                stats,
                message: `High storage usage for ${type}: ${stats.usedPercent.toFixed(1)}%`
            });
        }
    }

    getStorageStats(type = null) {
        if (type) {
            return this.storageStats.get(type);
        }
        return Object.fromEntries(this.storageStats);
    }

    cleanup() {
        if (this.storageInterval) {
            clearInterval(this.storageInterval);
            this.storageInterval = null;
        }
    }
}

module.exports = RecordingMonitor;
