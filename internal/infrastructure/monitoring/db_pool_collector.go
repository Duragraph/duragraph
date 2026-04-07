package monitoring

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type DBPoolCollector struct {
	pool      *pgxpool.Pool
	poolName  string
	namespace string

	acquireCount    *prometheus.Desc
	acquireDuration *prometheus.Desc
	acquiredConns   *prometheus.Desc
	idleConns       *prometheus.Desc
	totalConns      *prometheus.Desc
	maxConns        *prometheus.Desc
	emptyAcquire    *prometheus.Desc
	canceledAcquire *prometheus.Desc
}

func NewDBPoolCollector(pool *pgxpool.Pool, namespace, poolName string) *DBPoolCollector {
	labels := prometheus.Labels{"pool": poolName}
	return &DBPoolCollector{
		pool:      pool,
		poolName:  poolName,
		namespace: namespace,
		acquireCount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "acquire_count_total"),
			"Total number of pool connection acquisitions",
			nil, labels,
		),
		acquireDuration: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "acquire_duration_seconds_total"),
			"Total time spent acquiring connections",
			nil, labels,
		),
		acquiredConns: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "acquired_conns"),
			"Number of currently acquired connections",
			nil, labels,
		),
		idleConns: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "idle_conns"),
			"Number of idle connections in the pool",
			nil, labels,
		),
		totalConns: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "total_conns"),
			"Total number of connections in the pool",
			nil, labels,
		),
		maxConns: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "max_conns"),
			"Maximum number of connections",
			nil, labels,
		),
		emptyAcquire: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "empty_acquire_count_total"),
			"Total acquisitions when pool was empty",
			nil, labels,
		),
		canceledAcquire: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "db_pool", "canceled_acquire_count_total"),
			"Total canceled acquisitions",
			nil, labels,
		),
	}
}

func (c *DBPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquireCount
	ch <- c.acquireDuration
	ch <- c.acquiredConns
	ch <- c.idleConns
	ch <- c.totalConns
	ch <- c.maxConns
	ch <- c.emptyAcquire
	ch <- c.canceledAcquire
}

func (c *DBPoolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquireCount, prometheus.CounterValue, float64(stat.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.acquireDuration, prometheus.CounterValue, stat.AcquireDuration().Seconds())
	ch <- prometheus.MustNewConstMetric(c.acquiredConns, prometheus.GaugeValue, float64(stat.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(stat.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.totalConns, prometheus.GaugeValue, float64(stat.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.maxConns, prometheus.GaugeValue, float64(stat.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.emptyAcquire, prometheus.CounterValue, float64(stat.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.canceledAcquire, prometheus.CounterValue, float64(stat.CanceledAcquireCount()))
}
