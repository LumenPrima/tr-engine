package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "trengine"

var (
	// MQTTMessagesReceived counts MQTT messages received by type and system
	MQTTMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "mqtt_messages_total",
			Help:      "Total number of MQTT messages received",
		},
		[]string{"type", "system"},
	)

	// MQTTParseErrors counts JSON parse failures by message type
	MQTTParseErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "mqtt_parse_errors_total",
			Help:      "Total number of MQTT message parse errors",
		},
		[]string{"type"},
	)

	// CallsProcessed counts calls processed by state and system
	CallsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "calls_total",
			Help:      "Total number of calls processed",
		},
		[]string{"state", "system"},
	)

	// AudioFilesProcessed counts audio files saved by system
	AudioFilesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "audio_files_total",
			Help:      "Total number of audio files processed",
		},
		[]string{"system"},
	)

	// UnitEventsProcessed counts unit events by type and system
	UnitEventsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "unit_events_total",
			Help:      "Total number of unit events processed",
		},
		[]string{"event_type", "system"},
	)

	// TransmissionsRecorded counts individual transmissions by system
	TransmissionsRecorded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "transmissions_total",
			Help:      "Total number of transmissions recorded",
		},
		[]string{"system"},
	)

	// DBOperations counts database operations by type and success
	DBOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "db_operations_total",
			Help:      "Total number of database operations",
		},
		[]string{"operation", "success"},
	)

	// DedupCallsProcessed counts calls processed by deduplication engine
	DedupCallsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dedup_calls_total",
			Help:      "Total number of calls processed by deduplication",
		},
		[]string{"system"},
	)

	// DedupGroupsCreated counts new call groups created
	DedupGroupsCreated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dedup_groups_created_total",
			Help:      "Total number of call groups created",
		},
		[]string{"system"},
	)

	// DedupGroupsLinked counts calls linked to existing groups
	DedupGroupsLinked = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dedup_groups_linked_total",
			Help:      "Total number of calls linked to existing groups",
		},
		[]string{"system"},
	)

	// MessageProcessingDuration measures message handler latency
	MessageProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "message_duration_seconds",
			Help:      "Message processing duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"type"},
	)

	// CallDuration measures call durations
	CallDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "call_duration_seconds",
			Help:      "Call duration in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800},
		},
	)

	// AudioFileSize measures audio file sizes
	AudioFileSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "audio_bytes",
			Help:      "Audio file size in bytes",
			Buckets:   []float64{1024, 10240, 102400, 512000, 1048576, 5242880, 10485760},
		},
	)

	// DBOperationDuration measures database operation latency
	DBOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "db_operation_duration_seconds",
			Help:      "Database operation duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
		},
		[]string{"operation"},
	)

	// DedupScoreDistribution measures deduplication match scores
	DedupScoreDistribution = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "dedup_score",
			Help:      "Deduplication match score distribution",
			Buckets:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
		},
	)

	// ActiveWebSocketClients tracks connected WebSocket clients
	ActiveWebSocketClients = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "websocket_clients",
			Help:      "Number of active WebSocket clients",
		},
	)

	// SystemsRegistered tracks registered systems
	SystemsRegistered = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "systems",
			Help:      "Number of registered systems",
		},
	)

	// TalkgroupsRegistered tracks known talkgroups
	TalkgroupsRegistered = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "talkgroups",
			Help:      "Number of known talkgroups",
		},
	)

	// UnitsTracked tracks known units
	UnitsTracked = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "units",
			Help:      "Number of tracked units",
		},
	)

	// HTTPRequestDuration measures HTTP request latency
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestsTotal counts HTTP requests
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// TranscriptionQueueDepth tracks pending transcription jobs
	TranscriptionQueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "transcription_queue_depth",
			Help:      "Number of pending transcription jobs",
		},
	)

	// TranscriptionsCompleted counts completed transcriptions by provider
	TranscriptionsCompleted = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "transcriptions_total",
			Help:      "Total number of completed transcriptions",
		},
		[]string{"provider"},
	)

	// TranscriptionErrors counts transcription errors by provider
	TranscriptionErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "transcription_errors_total",
			Help:      "Total number of transcription errors",
		},
		[]string{"provider"},
	)

	// TranscriptionDuration measures transcription processing time
	TranscriptionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "transcription_duration_seconds",
			Help:      "Transcription processing duration in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
	)
)
