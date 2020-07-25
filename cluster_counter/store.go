package cluster_counter

import "time"

type DataStoreI interface {
	Store(name string, startTime time.Time, endTime time.Time, lbs map[string]string, value float64) error
	Load(name string, startTime time.Time, endTime time.Time, lbs map[string]string) (float64, error)
}
