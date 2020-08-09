package cluster_counter

import "time"

type DataStoreI interface {
	Store(name string, beginTime time.Time, endTime time.Time, lbs map[string]string, value CounterValue, force bool) error
	Load(name string, beginTime time.Time, endTime time.Time, lbs map[string]string) (CounterValue, error)
}
