package cluster_counter

type DataStoreI interface {
	Store(name string, intervalKey string, lbs map[string]string, value int64) error
	Load(name string, intervalKey string, lbs map[string]string) (int64, error)
}
