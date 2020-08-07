package prometheus_reporter

import (
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
	"github.com/prometheus/client_golang/prometheus"
	"strings"
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
	}, []string{"name", "labels", "metric"})

	prometheus.MustRegister(reporter.metricVec)

	return reporter
}

func NewCounterReporter(reporterName string) cluster_counter.ReporterI {
	if len(reporterName) == 0 {
		reporterName = "boostlearn"
	}
	reporter := &PromReporter{}
	reporter.metricVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: reporterName,
		Subsystem: "",
		Name:      "cluster_counter",
	}, []string{"name", "labels", "metric"})

	prometheus.MustRegister(reporter.metricVec)

	return reporter
}

func (reporter *PromReporter) Update(name string, lbs map[string]string, metrics map[string]float64) {
	var labels []string
	for k, v := range lbs {
		labels = append(labels, fmt.Sprintf("%s=%s", k, v))
	}
	for metricName, metricValue := range metrics {
		metric := reporter.metricVec.WithLabelValues(name, strings.Join(labels, "::"), metricName)
		if metric != nil {
			metric.Set(metricValue)
		}
	}
}
