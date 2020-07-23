package cluster_counter

type DataStoreI interface {
	Store(key string, value int64) error
	Load(key string) (int64, error)
}
