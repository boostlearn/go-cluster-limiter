package cluster_counter

type StoreI interface {
	Store(key string, value int64) error
	Load(key string) (int64, error)
}
