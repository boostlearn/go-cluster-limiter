package cluster_counter

type ReporterI interface {
	Update(name string, lbs map[string]string, metrics map[string]float64)
}
