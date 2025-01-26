const mongoose = require('mongoose');
const _ = require('lodash');

// Schema for detailed error tracking
const ErrorEventSchema = new mongoose.Schema({
    // Error identification
    error_id: { type: String, required: true, unique: true },
    timestamp: { type: Date, required: true },
    
    // Error categorization
    component: {
        type: String,
        required: true,
        enum: ['mqtt', 'call_processing', 'audio', 'database', 'system']
    },
    severity: {
        type: String,
        required: true,
        enum: ['critical', 'error', 'warning', 'info']
    },
    error_type: String,  // Specific error classification
    
    // Error context
    message: String,
    details: mongoose.Schema.Types.Mixed,
    stack_trace: String,
    
    // Related entities
    sys_name: String,
    talkgroup: Number,
    unit: Number,
    call_id: String,
    
    // Resolution tracking
    resolved: { type: Boolean, default: false },
    resolution: {
        timestamp: Date,
        action_taken: String,
        notes: String
    },
    
    // Impact assessment
    impact: {
        affected_calls: Number,
        affected_units: Number,
        data_loss: Boolean,
        service_disruption: Boolean
    }
});

// Create indexes for efficient querying
ErrorEventSchema.index({ timestamp: -1 });
ErrorEventSchema.index({ component: 1, severity: 1 });
ErrorEventSchema.index({ sys_name: 1, timestamp: -1 });
ErrorEventSchema.index({ resolved: 1, severity: 1 });

class ErrorTrackingManager {
    constructor() {
        this.ErrorEvent = mongoose.model('ErrorEvent', ErrorEventSchema);
        
        // Cache recent errors for quick access and pattern detection
        this.recentErrors = new Map();
        this.errorPatterns = new Map();
        
        // Initialize error pattern detection
        this.initializePatternDetection();
    }
    
    async logError(component, severity, errorType, message, context = {}) {
        const errorId = `${component}_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
        
        const errorEvent = new this.ErrorEvent({
            error_id: errorId,
            timestamp: new Date(),
            component,
            severity,
            error_type: errorType,
            message,
            
            // Extract context information
            sys_name: context.sys_name,
            talkgroup: context.talkgroup,
            unit: context.unit,
            call_id: context.call_id,
            
            // Store error details
            details: context.details || {},
            stack_trace: context.error?.stack,
            
            // Assess impact
            impact: this.assessImpact(component, context)
        });
        
        await errorEvent.save();
        
        // Update recent errors cache
        this.recentErrors.set(errorId, errorEvent);
        if (this.recentErrors.size > 1000) {
            const oldestKey = this.recentErrors.keys().next().value;
            this.recentErrors.delete(oldestKey);
        }
        
        // Check for error patterns
        await this.detectPatterns(errorEvent);
        
        // Trigger alerts for critical errors
        if (severity === 'critical') {
            await this.handleCriticalError(errorEvent);
        }
        
        return errorEvent;
    }
    
    assessImpact(component, context) {
        const impact = {
            affected_calls: 0,
            affected_units: 0,
            data_loss: false,
            service_disruption: false
        };
        
        switch (component) {
            case 'mqtt':
                impact.service_disruption = true;
                break;
                
            case 'call_processing':
                impact.affected_calls = 1;
                impact.affected_units = context.units?.length || 1;
                break;
                
            case 'audio':
                impact.data_loss = true;
                impact.affected_calls = 1;
                break;
                
            case 'database':
                impact.data_loss = true;
                impact.service_disruption = true;
                break;
        }
        
        return impact;
    }
    
    async detectPatterns(newError) {
        const timeWindow = 5 * 60 * 1000; // 5 minutes
        const minOccurrences = 3;
        
        // Get recent errors of the same type
        const recentSimilarErrors = Array.from(this.recentErrors.values())
            .filter(error => 
                error.component === newError.component &&
                error.error_type === newError.error_type &&
                error.timestamp > new Date(Date.now() - timeWindow)
            );
        
        if (recentSimilarErrors.length >= minOccurrences) {
            const patternKey = `${newError.component}_${newError.error_type}`;
            
            if (!this.errorPatterns.has(patternKey)) {
                // New pattern detected
                await this.handleErrorPattern(patternKey, recentSimilarErrors);
            }
            
            this.errorPatterns.set(patternKey, {
                count: recentSimilarErrors.length,
                lastSeen: new Date(),
                errors: recentSimilarErrors.map(e => e.error_id)
            });
        }
    }
    
    async handleErrorPattern(patternKey, errors) {
        // Create a system error event for the pattern
        await this.logError(
            'system',
            'error',
            'error_pattern_detected',
            `Error pattern detected: ${patternKey}`,
            {
                details: {
                    pattern_key: patternKey,
                    occurrence_count: errors.length,
                    error_ids: errors.map(e => e.error_id),
                    time_window: '5 minutes'
                }
            }
        );
    }
    
    async handleCriticalError(error) {
        // Log system event for critical error
        await this.logError(
            'system',
            'critical',
            'critical_error_detected',
            `Critical error in ${error.component}: ${error.message}`,
            {
                details: {
                    original_error_id: error.error_id,
                    impact: error.impact
                }
            }
        );
        
        // Additional handling could be implemented here
        // For example, sending notifications or triggering recovery procedures
    }
    
    initializePatternDetection() {
        // Periodically clean up old patterns
        setInterval(() => {
            const patternTimeout = 15 * 60 * 1000; // 15 minutes
            
            for (const [key, pattern] of this.errorPatterns) {
                if (pattern.lastSeen < new Date(Date.now() - patternTimeout)) {
                    this.errorPatterns.delete(key);
                }
            }
        }, 60 * 1000); // Check every minute
    }
    
    // Query methods for error analysis
    
    async getActiveErrorPatterns() {
        return Array.from(this.errorPatterns.entries()).map(([key, pattern]) => ({
            pattern_key: key,
            ...pattern,
            active: pattern.lastSeen > new Date(Date.now() - 5 * 60 * 1000)
        }));
    }
    
    async getErrorStatistics(timeframe = '1h') {
        const endTime = new Date();
        const startTime = new Date(endTime);
        
        switch (timeframe) {
            case '1h':
                startTime.setHours(endTime.getHours() - 1);
                break;
            case '24h':
                startTime.setHours(endTime.getHours() - 24);
                break;
            case '7d':
                startTime.setDate(endTime.getDate() - 7);
                break;
        }
        
        const stats = await this.ErrorEvent.aggregate([
            {
                $match: {
                    timestamp: { $gte: startTime, $lte: endTime }
                }
            },
            {
                $group: {
                    _id: {
                        component: '$component',
                        severity: '$severity'
                    },
                    count: { $sum: 1 },
                    affected_calls: { $sum: '$impact.affected_calls' },
                    affected_units: { $sum: '$impact.affected_units' },
                    data_loss_count: {
                        $sum: { $cond: ['$impact.data_loss', 1, 0] }
                    },
                    service_disruptions: {
                        $sum: { $cond: ['$impact.service_disruption', 1, 0] }
                    },
                    resolution_time_avg: {
                        $avg: {
                            $cond: [
                                '$resolved',
                                { $subtract: ['$resolution.timestamp', '$timestamp'] },
                                null
                            ]
                        }
                    }
                }
            },
            {
                $sort: {
                    '_id.severity': -1,
                    '_id.component': 1
                }
            }
        ]);
        
        return {
            timeframe,
            stats,
            patterns: await this.getActiveErrorPatterns()
        };
    }
    
    async searchErrors(query = {}) {
        const filter = {};
        
        if (query.component) filter.component = query.component;
        if (query.severity) filter.severity = query.severity;
        if (query.error_type) filter.error_type = query.error_type;
        if (query.sys_name) filter.sys_name = query.sys_name;
        if (query.resolved !== undefined) filter.resolved = query.resolved;
        
        if (query.start_time) {
            filter.timestamp = { $gte: new Date(query.start_time) };
        }
        if (query.end_time) {
            filter.timestamp = {
                ...filter.timestamp,
                $lte: new Date(query.end_time)
            };
        }
        
        return this.ErrorEvent.find(filter)
            .sort({ timestamp: -1 })
            .limit(query.limit || 100);
    }
}

module.exports = ErrorTrackingManager;