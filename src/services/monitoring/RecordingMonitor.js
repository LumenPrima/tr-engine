const EventEmitter = require('events');
const logger = require('../../utils/logger');

/**
 * RecordingMonitor
 * Monitors recording system health and resources:
 * 
 * Storage Monitoring:
 * - Recording files directory space usage
 * - Temporary recording files space usage
 * - MongoDB database storage utilization
 * 
 * Recording State:
 * - Recorder status changes (active/inactive/error)
 * - Error detection and tracking
 * - Failure pattern detection (3+ failures/24h)
 * 
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
        
        // Track file counts and sizes
        this.fileStats = new Map();
        
        // Initialize monitoring
        this.monitorStorage();
        this.monitorFiles();
        
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

    async monitorFiles() {
        const { readdir, stat } = require('fs/promises');
        const path = require('path');

        // Monitor every 15 minutes
        const interval = setInterval(async () => {
            try {
                for (const [type, dirPath] of Object.entries(this.storagePaths)) {
                    const files = await readdir(dirPath);
                    let totalSize = 0;
                    let fileTypes = new Map();

                    for (const file of files) {
                        const filePath = path.join(dirPath, file);
                        const stats = await stat(filePath);
                        totalSize += stats.size;

                        // Track by extension
                        const ext = path.extname(file);
                        fileTypes.set(ext, (fileTypes.get(ext) || 0) + 1);
                    }

                    const fileStats = {
                        totalFiles: files.length,
                        totalSize,
                        byType: Object.fromEntries(fileTypes),
                        timestamp: new Date()
                    };

                    this.fileStats.set(type, fileStats);

                    // Alert if too many temp files
                    if (type === 'temp' && files.length > 100) {
                        this.emit('files.warning', {
                            type,
                            message: `High number of temp files: ${files.length}`,
                            stats: fileStats
                        });
                    }
                }
            } catch (error) {
                logger.error('File monitoring error:', error);
                this.emit('files.error', error);
            }
        }, 15 * 60 * 1000); // Every 15 minutes

        return interval;
    }

    getFileStats(type = null) {
        if (type) {
            return this.fileStats.get(type);
        }
        return Object.fromEntries(this.fileStats);
    }

    cleanup() {
        if (this.storageInterval) {
            clearInterval(this.storageInterval);
            this.storageInterval = null;
        }
        if (this.fileInterval) {
            clearInterval(this.fileInterval);
            this.fileInterval = null;
        }
    }
}

module.exports = RecordingMonitor;
