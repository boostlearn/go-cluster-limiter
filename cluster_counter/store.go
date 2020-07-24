package cluster_counter

import "time"

type DataStoreI interface {
	Store(name string, startTime time.Time, endTime time.Time, lbs map[string]string, value int64) error
	Load(name string, startTime time.Time, endTime time.Time, lbs map[string]string) (int64, error)
}
