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
        
        // Track recording failures
        this.failureTracking = new Map();
        
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

    checkStorageStatus(usage) {
        const storageUsed = usage.used / usage.total * 100;
        
        if (storageUsed >= this.storageThresholds.critical) {
            this.emit('storage.critical', { 
                usage: storageUsed,
                available: usage.available
            });
            return 'critical';
        } 
        
        if (storageUsed >= this.storageThresholds.warning) {
            this.emit('storage.warning', {
                usage: storageUsed,
                available: usage.available
            });
            return 'warning';
        }
        
        return 'normal';
    }
}

module.exports = RecordingMonitor;
