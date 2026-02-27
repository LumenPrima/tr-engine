package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// IngestStats provides the metrics collector access to pipeline state.
type IngestStats interface {
	MsgCount() int64
	HandlerCounts() map[string]int64
	ActiveCallCount() int
	SSESubscriberCount() int
}

// Collector implements prometheus.Collector to read live gauges at scrape time.
type Collector struct {
	pool  *pgxpool.Pool
	stats IngestStats

	// Descriptors for scrape-time gauges.
	activeCalls    *prometheus.Desc
	sseSubscribers *prometheus.Desc
	dbTotalConns   *prometheus.Desc
	dbAcquiredConns *prometheus.Desc
	dbIdleConns    *prometheus.Desc
}

// NewCollector creates a collector that reads live state at scrape time.
// pool may be nil (metrics will report 0). stats may be nil if no pipeline is running.
func NewCollector(pool *pgxpool.Pool, stats IngestStats) *Collector {
	return &Collector{
		pool:  pool,
		stats: stats,
		activeCalls: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "active_calls"),
			"Current number of in-progress calls.",
			nil, nil,
		),
		sseSubscribers: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "sse_subscribers_active"),
			"Current number of SSE subscribers.",
			nil, nil,
		),
		dbTotalConns: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "total_conns"),
			"Total database pool connections.",
			nil, nil,
		),
		dbAcquiredConns: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "acquired_conns"),
			"Database pool connections currently in use.",
			nil, nil,
		),
		dbIdleConns: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "idle_conns"),
			"Database pool idle connections.",
			nil, nil,
		),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.activeCalls
	ch <- c.sseSubscribers
	ch <- c.dbTotalConns
	ch <- c.dbAcquiredConns
	ch <- c.dbIdleConns
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	// Ingest stats
	if c.stats != nil {
		ch <- prometheus.MustNewConstMetric(c.activeCalls, prometheus.GaugeValue, float64(c.stats.ActiveCallCount()))
		ch <- prometheus.MustNewConstMetric(c.sseSubscribers, prometheus.GaugeValue, float64(c.stats.SSESubscriberCount()))
	} else {
		ch <- prometheus.MustNewConstMetric(c.activeCalls, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.sseSubscribers, prometheus.GaugeValue, 0)
	}

	// Database pool stats
	if c.pool != nil {
		stat := c.pool.Stat()
		ch <- prometheus.MustNewConstMetric(c.dbTotalConns, prometheus.GaugeValue, float64(stat.TotalConns()))
		ch <- prometheus.MustNewConstMetric(c.dbAcquiredConns, prometheus.GaugeValue, float64(stat.AcquiredConns()))
		ch <- prometheus.MustNewConstMetric(c.dbIdleConns, prometheus.GaugeValue, float64(stat.IdleConns()))
	} else {
		ch <- prometheus.MustNewConstMetric(c.dbTotalConns, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.dbAcquiredConns, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.dbIdleConns, prometheus.GaugeValue, 0)
	}
}
