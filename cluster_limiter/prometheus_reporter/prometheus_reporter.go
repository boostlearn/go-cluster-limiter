package prometheus_reporter

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
	"github.com/prometheus/client_golang/prometheus"
)

type PromReporter struct {
	metricVec *prometheus.GaugeVec
}

func NewLimiterReporter(reporterName string) cluster_limiter.ReporterI {
	if len(reporterName) == 0 {
		reporterName = "boostlearn"
	}
	reporter := &PromReporter{}
	reporter.metricVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: reporterName,
		Subsystem: "",
		Name:      "cluster_limiter",
	}, []string{"name", "metric"})

	prometheus.MustRegister(reporter.metricVec)

	return reporter
}

func (reporter *PromReporter) Update(name string, metrics map[string]float64) {
	for metricName, metricValue := range metrics {
		metric := reporter.metricVec.WithLabelValues(name, metricName)
		if metric != nil {
			metric.Set(metricValue)
		}
	}
}
