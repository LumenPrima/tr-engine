const mongoose = require('mongoose');
const _ = require('lodash');

// Real-time performance metrics - high frequency updates
const SystemMetricsSchema = new mongoose.Schema({
    timestamp: { type: Date, required: true },
    sys_name: { type: String, required: true },
    
    // Decoder performance
    decode_rate: Number,         // Messages per second
    decode_errors: Number,       // Error count
    control_channel: Number,     // Current control channel frequency
    
    // Channel utilization
    active_channels: [{
        freq: Number,
        usage_percent: Number,
        error_rate: Number
    }],
    
    // Resource usage
    recorder_utilization: {
        total: Number,
        active: Number,
        available: Number
    },
    
    // Message processing
    message_stats: {
        received: Number,
        processed: Number,
        errors: Number,
        latency: Number          // Average processing time in ms
    }
}, {
    timeseries: {
        timeField: 'timestamp',
        metaField: 'sys_name',
        granularity: 'seconds'
    }
});

// Hourly aggregated metrics for longer-term analysis
const SystemMetricsHourlySchema = new mongoose.Schema({
    timestamp: { type: Date, required: true },
    sys_name: { type: String, required: true },
    hour: { type: Number, required: true },  // 0-23
    
    // Aggregated performance metrics
    decode_rate: {
        avg: Number,
        min: Number,
        max: Number,
        std_dev: Number
    },
    
    error_rates: {
        decode_errors: Number,
        message_errors: Number,
        audio_errors: Number
    },
    
    channel_usage: [{
        freq: Number,
        total_time: Number,      // Seconds in use
        call_count: Number,
        error_count: Number
    }],
    
    resource_usage: {
        avg_recorder_utilization: Number,
        peak_recorder_utilization: Number,
        recorder_saturation_time: Number  // Seconds at max capacity
    },
    
    message_processing: {
        total_received: Number,
        total_processed: Number,
        avg_latency: Number,
        max_latency: Number
    }
}, {
    timeseries: {
        timeField: 'timestamp',
        metaField: 'sys_name',
        granularity: 'hours'
    }
});

// System health events for tracking issues and maintenance
const SystemHealthEventSchema = new mongoose.Schema({
    timestamp: { type: Date, required: true },
    sys_name: { type: String, required: true },
    event_type: {
        type: String,
        enum: [
            'error',            // System errors
            'warning',          // Warning conditions
            'maintenance',      // Scheduled maintenance
            'recovery',         // System recovery
            'configuration'     // Configuration changes
        ]
    },
    severity: {
        type: String,
        enum: ['critical', 'high', 'medium', 'low'],
        required: true
    },
    
    // Event details
    component: String,          // Affected system component
    message: String,           // Human-readable description
    details: mongoose.Schema.Types.Mixed,
    
    // Resolution tracking
    resolved: { type: Boolean, default: false },
    resolved_at: Date,
    resolution_details: String
});

// Performance thresholds and alerts configuration
const SystemThresholdSchema = new mongoose.Schema({
    sys_name: { type: String, required: true },
    
    // Threshold configurations
    thresholds: {
        decode_rate: {
            warning: Number,    // Minimum acceptable decode rate
            critical: Number    // Critical minimum decode rate
        },
        error_rate: {
            warning: Number,    // Maximum acceptable error percentage
            critical: Number    // Critical error percentage
        },
        recorder_utilization: {
            warning: Number,    // High utilization threshold
            critical: Number    // Critical utilization threshold
        },
        message_latency: {
            warning: Number,    // Maximum acceptable latency (ms)
            critical: Number    // Critical latency threshold (ms)
        }
    },
    
    // Alert settings
    alerts: {
        enabled: { type: Boolean, default: true },
        notification_endpoints: [{
            type: String,      // Email, webhook, etc.
            config: mongoose.Schema.Types.Mixed
        }],
        cooldown_period: { type: Number, default: 300 }  // Seconds between repeated alerts
    },
    
    // Auto-recovery settings
    auto_recovery: {
        enabled: { type: Boolean, default: false },
        actions: [{
            condition: String,  // Condition triggering recovery
            action: String,    // Action to take
            config: mongoose.Schema.Types.Mixed
        }]
    }
});

class SystemMetricsManager {
    constructor() {
        this.SystemMetrics = mongoose.model('SystemMetrics', SystemMetricsSchema);
        this.SystemMetricsHourly = mongoose.model('SystemMetricsHourly', SystemMetricsHourlySchema);
        this.SystemHealthEvent = mongoose.model('SystemHealthEvent', SystemHealthEventSchema);
        this.SystemThreshold = mongoose.model('SystemThreshold', SystemThresholdSchema);
        
        // Cache for current metrics and thresholds
        this.metricsCache = new Map();
        this.thresholdCache = new Map();
        
        // Initialize hourly aggregation
        this.initializeHourlyAggregation();
    }
    
    async processMetrics(message) {
        const timestamp = new Date();
        const sysName = message.sys_name;
        
        // Calculate metrics from message
        const metrics = {
            timestamp,
            sys_name: sysName,
            decode_rate: message.decoderate,
            decode_errors: message.error_count || 0,
            control_channel: message.control_channel,
            
            // Calculate recorder utilization
            recorder_utilization: this.calculateRecorderUtilization(message),
            
            // Process message statistics
            message_stats: this.calculateMessageStats(message)
        };
        
        // Store metrics
        const storedMetrics = new this.SystemMetrics(metrics);
        await storedMetrics.save();
        
        // Update cache
        this.metricsCache.set(sysName, metrics);
        
        // Check thresholds and generate health events if needed
        await this.checkThresholds(sysName, metrics);
    }
    
    calculateRecorderUtilization(message) {
        const recorders = message.recorders || [];
        const total = recorders.length;
        const active = recorders.filter(r => r.rec_state_type === 'RECORDING').length;
        
        return {
            total,
            active,
            available: total - active
        };
    }
    
    calculateMessageStats(message) {
        // Calculate message processing statistics
        // This would be enhanced with actual message processing metrics
        return {
            received: 1,
            processed: 1,
            errors: 0,
            latency: Date.now() - (message.timestamp || Date.now())
        };
    }
    
    async checkThresholds(sysName, metrics) {
        const thresholds = await this.getThresholds(sysName);
        if (!thresholds) return;
        
        // Check decode rate
        if (metrics.decode_rate < thresholds.thresholds.decode_rate.critical) {
            await this.createHealthEvent(sysName, {
                event_type: 'error',
                severity: 'critical',
                component: 'decoder',
                message: `Critical decode rate: ${metrics.decode_rate}`,
                details: { threshold: thresholds.thresholds.decode_rate.critical }
            });
        }
        
        // Check recorder utilization
        const utilizationPercent = (metrics.recorder_utilization.active / 
                                  metrics.recorder_utilization.total) * 100;
        
        if (utilizationPercent > thresholds.thresholds.recorder_utilization.warning) {
            await this.createHealthEvent(sysName, {
                event_type: 'warning',
                severity: 'high',
                component: 'recorder',
                message: `High recorder utilization: ${utilizationPercent.toFixed(1)}%`,
                details: { 
                    active: metrics.recorder_utilization.active,
                    total: metrics.recorder_utilization.total
                }
            });
        }
    }
    
    async createHealthEvent(sysName, eventData) {
        const event = new this.SystemHealthEvent({
            timestamp: new Date(),
            sys_name: sysName,
            ...eventData
        });
        
        await event.save();
        
        // Trigger alerts if configured
        const thresholds = await this.getThresholds(sysName);
        if (thresholds?.alerts.enabled) {
            await this.triggerAlerts(sysName, event);
        }
    }
    
    async getThresholds(sysName) {
        // Check cache first
        if (this.thresholdCache.has(sysName)) {
            return this.thresholdCache.get(sysName);
        }
        
        // Load from database
        const thresholds = await this.SystemThreshold.findOne({ sys_name: sysName });
        if (thresholds) {
            this.thresholdCache.set(sysName, thresholds);
        }
        
        return thresholds;
    }
    
    async triggerAlerts(sysName, event) {
        const thresholds = await this.getThresholds(sysName);
        if (!thresholds?.alerts.enabled) return;
        
        // Check cooldown period
        const lastAlert = await this.SystemHealthEvent.findOne({
            sys_name: sysName,
            event_type: event.event_type,
            component: event.component,
            timestamp: {
                $gte: new Date(Date.now() - thresholds.alerts.cooldown_period * 1000)
            }
        });
        
        if (lastAlert) return;
        
        // Process each configured notification endpoint
        for (const endpoint of thresholds.alerts.notification_endpoints) {
            try {
                // Implementation would vary based on notification type
                // This is where you'd implement email, webhook, etc.
                console.log(`Alert triggered for ${sysName}: ${event.message}`);
            } catch (error) {
                console.error(`Failed to send alert to endpoint: ${endpoint.type}`, error);
            }
        }
    }
    
    async initializeHourlyAggregation() {
        // Run hourly aggregation every hour
        setInterval(async () => {
            const hourAgo = new Date();
            hourAgo.setHours(hourAgo.getHours() - 1);
            
            // Aggregate metrics for the past hour
            const systems = await this.SystemMetrics.distinct('sys_name');
            
            for (const sysName of systems) {
                await this.aggregateHourlyMetrics(sysName, hourAgo);
            }
        }, 60 * 60 * 1000);  // Run every hour
    }
    
    async aggregateHourlyMetrics(sysName, timestamp) {
        const hourStart = new Date(timestamp);
        hourStart.setMinutes(0, 0, 0);
        
        const hourEnd = new Date(hourStart);
        hourEnd.setHours(hourEnd.getHours() + 1);
        
        // Aggregate metrics for the hour
        const metrics = await this.SystemMetrics.aggregate([
            {
                $match: {
                    sys_name: sysName,
                    timestamp: {
                        $gte: hourStart,
                        $lt: hourEnd
                    }
                }
            },
            {
                $group: {
                    _id: null,
                    avg_decode_rate: { $avg: '$decode_rate' },
                    min_decode_rate: { $min: '$decode_rate' },
                    max_decode_rate: { $max: '$decode_rate' },
                    total_errors: { $sum: '$decode_errors' },
                    avg_latency: { $avg: '$message_stats.latency' },
                    max_latency: { $max: '$message_stats.latency' }
                }
            }
        ]);
        
        if (metrics.length === 0) return;
        
        // Store hourly aggregation
        const hourlyMetrics = new this.SystemMetricsHourly({
            timestamp: hourStart,
            sys_name: sysName,
            hour: hourStart.getHours(),
            
            decode_rate: {
                avg: metrics[0].avg_decode_rate,
                min: metrics[0].min_decode_rate,
                max: metrics[0].max_decode_rate
            },
            
            error_rates: {
                decode_errors: metrics[0].total_errors
            },
            
            message_processing: {
                avg_latency: metrics[0].avg_latency,
                max_latency: metrics[0].max_latency
            }
        });
        
        await hourlyMetrics.save();
    }
    
    // API support methods
    
    async getSystemStats(sysName, timeframe = '1h') {
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
        
        // Get current metrics
        const currentMetrics = this.metricsCache.get(sysName);
        
        // Get historical metrics
        const historicalMetrics = await this.SystemMetricsHourly.find({
            sys_name: sysName,
            timestamp: {
                $gte: startTime,
                $lte: endTime
            }
        }).sort({ timestamp: 1 });
        
        // Get active health events
        const activeEvents = await this.SystemHealthEvent.find({
            sys_name: sysName,
            resolved: false
        }).sort({ severity: 1 });
        
        return {
            current: currentMetrics,
            historical: this.formatHistoricalMetrics(historicalMetrics),
            health: {
                status: this.calculateHealthStatus(currentMetrics, activeEvents),
                active_events: activeEvents
            }
        };
    }
    
    formatHistoricalMetrics(metrics) {
        return {
            timestamps: metrics.map(m => m.timestamp),
            decode_rates: metrics.map(m => m.decode_rate.avg),
            error_rates: metrics.map(m => m.error_rates.decode_errors),
            latencies: metrics.map(m => m.message_processing.avg_latency)
        };
    }
    
    calculateHealthStatus(currentMetrics, activeEvents) {
        if (!currentMetrics) return 'unknown';
        
        // Check for critical events
        const criticalEvents = activeEvents.filter(e => e.severity === 'critical');
        if (criticalEvents.length > 0) return 'critical';
        
        // Check for high severity events
        const highSeverityEvents = activeEvents.filter(e => e.severity === 'high');
        if (highSeverityEvents.length > 0) return 'degraded';
        
        // Check current metrics against standard thresholds
        if (currentMetrics.decode_rate < 10 || 
            currentMetrics.recorder_utilization.available === 0 ||
            currentMetrics.message_stats.errors > 100) {
            return 'warning';
        }
        
        return 'healthy';
    }
    
    async getSystemHealth(sysName) {
        // Get current metrics and events
        const currentMetrics = this.metricsCache.get(sysName);
        const activeEvents = await this.SystemHealthEvent.find({
            sys_name: sysName,
            resolved: false
        }).sort({ timestamp: -1 });
        
        // Calculate resource utilization
        const resourceUtilization = {
            cpu: this.calculateCPUUtilization(),
            memory: this.calculateMemoryUtilization(),
            storage: await this.calculateStorageUtilization(),
            network: this.calculateNetworkUtilization()
        };
        
        // Get component status
        const componentStatus = await this.getComponentStatus(sysName);
        
        return {
            status: this.calculateHealthStatus(currentMetrics, activeEvents),
            timestamp: new Date(),
            uptime: process.uptime(),
            
            components: componentStatus,
            resources: resourceUtilization,
            
            active_events: activeEvents.map(event => ({
                type: event.event_type,
                severity: event.severity,
                component: event.component,
                message: event.message,
                timestamp: event.timestamp
            })),
            
            metrics: {
                current: currentMetrics,
                thresholds: await this.getThresholds(sysName)
            }
        };
    }
    
    async getComponentStatus(sysName) {
        return {
            mqtt: {
                status: 'connected',  // Would be determined by MQTT client status
                lastMessage: this.getLastMessageTimestamp(sysName)
            },
            database: {
                status: mongoose.connection.readyState === 1 ? 'connected' : 'disconnected',
                latency: await this.measureDatabaseLatency()
            },
            recorders: await this.getRecorderStatus(sysName),
            decoders: await this.getDecoderStatus(sysName)
        };
    }
    
    async measureDatabaseLatency() {
        const start = Date.now();
        try {
            await this.SystemMetrics.findOne({}, { _id: 1 }).limit(1);
            return Date.now() - start;
        } catch (error) {
            return -1;
        }
    }
    
    async getRecorderStatus(sysName) {
        const metrics = this.metricsCache.get(sysName);
        if (!metrics) return { status: 'unknown' };
        
        const utilization = metrics.recorder_utilization;
        const status = utilization.available === 0 ? 'saturated' :
                      utilization.active / utilization.total > 0.8 ? 'high' : 'normal';
                      
        return {
            status,
            active: utilization.active,
            total: utilization.total,
            utilization: (utilization.active / utilization.total * 100).toFixed(1) + '%'
        };
    }
    
    async getDecoderStatus(sysName) {
        const metrics = this.metricsCache.get(sysName);
        if (!metrics) return { status: 'unknown' };
        
        const status = metrics.decode_rate < 10 ? 'degraded' :
                      metrics.decode_rate < 20 ? 'warning' : 'normal';
                      
        return {
            status,
            decode_rate: metrics.decode_rate,
            errors: metrics.decode_errors,
            control_channel: metrics.control_channel
        };
    }
    
    calculateCPUUtilization() {
        // This would be implemented based on your system's CPU monitoring
        // Could use OS-level metrics or process metrics
        return {
            current: 0,  // Placeholder
            average: 0   // Placeholder
        };
    }
    
    calculateMemoryUtilization() {
        const used = process.memoryUsage();
        return {
            heapUsed: used.heapUsed,
            heapTotal: used.heapTotal,
            rss: used.rss,
            percentage: (used.heapUsed / used.heapTotal * 100).toFixed(1) + '%'
        };
    }
    
    async calculateStorageUtilization() {
        // This would check database storage and audio file storage
        // Implementation would depend on your storage setup
        return {
            database: {
                used: 0,    // Placeholder
                total: 0    // Placeholder
            },
            audio: {
                used: 0,    // Placeholder
                total: 0    // Placeholder
            }
        };
    }
    
    calculateNetworkUtilization() {
        // This would track MQTT message rates and network usage
        // Implementation would depend on your network monitoring setup
        return {
            messageRate: 0,     // Placeholder
            bandwidth: 0        // Placeholder
        };
    }
    
    getLastMessageTimestamp(sysName) {
        const metrics = this.metricsCache.get(sysName);
        return metrics ? metrics.timestamp : null;
    }
    
    async getPerformanceReport(sysName, timeframe = '24h') {
        const endTime = new Date();
        const startTime = new Date(endTime);
        
        switch (timeframe) {
            case '24h':
                startTime.setHours(endTime.getHours() - 24);
                break;
            case '7d':
                startTime.setDate(endTime.getDate() - 7);
                break;
            case '30d':
                startTime.setDate(endTime.getDate() - 30);
                break;
        }
        
        // Get hourly metrics
        const hourlyMetrics = await this.SystemMetricsHourly.find({
            sys_name: sysName,
            timestamp: {
                $gte: startTime,
                $lte: endTime
            }
        }).sort({ timestamp: 1 });
        
        // Get health events
        const healthEvents = await this.SystemHealthEvent.find({
            sys_name: sysName,
            timestamp: {
                $gte: startTime,
                $lte: endTime
            }
        }).sort({ timestamp: 1 });
        
        // Calculate performance statistics
        const decodedMessages = _.sumBy(hourlyMetrics, 
            m => m.decode_rate.avg * 3600);  // Convert hourly rate to message count
        
        const errorRate = _.sumBy(healthEvents, 
            e => e.event_type === 'error' ? 1 : 0) / healthEvents.length;
        
        const avgDecodeRate = _.meanBy(hourlyMetrics, m => m.decode_rate.avg);
        const peakDecodeRate = _.maxBy(hourlyMetrics, m => m.decode_rate.max)?.decode_rate.max;
        
        return {
            timeframe,
            summary: {
                total_messages: decodedMessages,
                error_rate: errorRate,
                avg_decode_rate: avgDecodeRate,
                peak_decode_rate: peakDecodeRate,
                health_events: healthEvents.length
            },
            hourly_metrics: this.formatHistoricalMetrics(hourlyMetrics),
            significant_events: healthEvents.filter(e => 
                e.severity === 'critical' || e.severity === 'high'
            ),
            recommendations: this.generateRecommendations(
                hourlyMetrics, 
                healthEvents, 
                await this.getThresholds(sysName)
            )
        };
    }
    
    generateRecommendations(metrics, events, thresholds) {
        const recommendations = [];
        
        // Check decode rate stability
        const decodeRates = metrics.map(m => m.decode_rate.avg);
        const decodeRateStdDev = this.calculateStandardDeviation(decodeRates);
        
        if (decodeRateStdDev > 5) {
            recommendations.push({
                type: 'performance',
                priority: 'medium',
                message: 'Decode rate shows high variability. Consider investigating signal quality and interference.',
                metric: 'decode_rate',
                current_stddev: decodeRateStdDev
            });
        }
        
        // Check error patterns
        const errorPatterns = this.analyzeErrorPatterns(events);
        if (errorPatterns.recurring.length > 0) {
            recommendations.push({
                type: 'reliability',
                priority: 'high',
                message: 'Recurring error patterns detected. Consider implementing automated recovery for these scenarios.',
                patterns: errorPatterns.recurring
            });
        }
        
        // Resource utilization recommendations
        const peakUtilization = _.maxBy(metrics, 
            m => m.resource_usage.peak_recorder_utilization)?.resource_usage.peak_recorder_utilization;
            
        if (peakUtilization > 90) {
            recommendations.push({
                type: 'capacity',
                priority: 'high',
                message: 'System approaching maximum recorder capacity. Consider adding additional resources.',
                current_peak: peakUtilization
            });
        }
        
        return recommendations;
    }
    
    calculateStandardDeviation(values) {
        const avg = _.mean(values);
        const squareDiffs = values.map(value => Math.pow(value - avg, 2));
        return Math.sqrt(_.mean(squareDiffs));
    }
    
    analyzeErrorPatterns(events) {
        // Group similar errors
        const patterns = _.groupBy(events, e => 
            `${e.component}_${e.event_type}_${e.severity}`
        );
        
        // Find recurring patterns
        const recurring = Object.entries(patterns)
            .filter(([_, events]) => events.length >= 3)
            .map(([pattern, events]) => ({
                pattern,
                count: events.length,
                first_seen: _.minBy(events, 'timestamp').timestamp,
                last_seen: _.maxBy(events, 'timestamp').timestamp
            }));
            
        return {
            recurring,
            total_patterns: Object.keys(patterns).length
        };
    }
}