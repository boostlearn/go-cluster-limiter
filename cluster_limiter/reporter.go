package cluster_limiter

type ReporterI interface {
	Update(name string, metrics map[string]float64)
}
