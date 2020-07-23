package cluster_counter

import (
	"errors"
	"sync"
	"time"
)

const DEFAULT_KEY_PREFIX = "clct:"

// 计数器生产工厂
type ClusterCounterFactoryI interface {
	// 构建计数器
	NewClusterCounter(opts *ClusterCounterOpts) (ClusterCounterI, error)

	NewClusterCounterVec(opts *ClusterCounterOpts, labelNames []string) (ClusterCounterVecI, error)

	// 向集群推送数据
	PushData(name string, measure string, key string, value int64) error

	// 向集群推送数据
	PullData(name string, measure string, key string) (int64, error)

	Delete(name string)
}

type ClusterCounterRedisFactory struct {
	clusterCounterVectors sync.Map
	clusterCounters       sync.Map

	redisKeyPrefix string
	store StoreI

	syncStatus     bool

	DefaultClusterAmpFactor float64

	UpdateInterval time.Duration
}

type ClusterCounterRedisFactoryOpts struct {
	KeyPrefix               string
	DefaultClusterAmpFactor float64
	UpdateInterval          time.Duration
}

func NewFactory(opts *ClusterCounterRedisFactoryOpts, store StoreI) *ClusterCounterRedisFactory {
	if len(opts.KeyPrefix) == 0 {
		opts.KeyPrefix = DEFAULT_KEY_PREFIX
	}
	if opts.DefaultClusterAmpFactor < 1.0 {
		opts.DefaultClusterAmpFactor = 1.0
	}

	if opts.UpdateInterval.Seconds() < 1 {
		opts.UpdateInterval = time.Duration(1) * time.Second
	}

	factory := &ClusterCounterRedisFactory{
		redisKeyPrefix:          opts.KeyPrefix,
		store:             store,
		syncStatus:              false,
		DefaultClusterAmpFactor: opts.DefaultClusterAmpFactor,
		UpdateInterval:          opts.UpdateInterval,
	}
	factory.Start()
	return factory
}

// 新建计数器
func (factory *ClusterCounterRedisFactory) NewClusterCounterVec(opts *ClusterCounterOpts,
	labelNames []string,
) (ClusterCounterVecI, error) {
	if opts == nil || len(opts.Name) == 0 {
		return nil, errors.New("name error")
	}

	if len(labelNames) == 0 {
		return nil, errors.New("need label names")
	}

	if opts.DefaultClusterAmpFactor == 0.0 {
		opts.DefaultClusterAmpFactor = factory.DefaultClusterAmpFactor
	}

	if opts.PushInterval == 0 {
		opts.PushInterval = time.Duration(5) * time.Second
	}

	if opts.PullInterval == 0 {
		opts.PullInterval = time.Duration(10) * time.Second
	}

	if counter, ok := factory.clusterCounterVectors.Load(opts.Name); ok {
		return counter.(ClusterCounterVecI), nil
	}

	if opts.DefaultClusterAmpFactor == 0 {
		opts.DefaultClusterAmpFactor = factory.DefaultClusterAmpFactor
	}

	clusterCounterVec := &ClusterCounterVec{
		Factory:                 factory,
		ResetInterval:           opts.ResetInterval,
		PullInterval:            opts.PullInterval,
		PushInterval:            opts.PushInterval,
		Name:                    opts.Name,
		LabelNames:              append([]string{}, labelNames...),
		DefaultClusterAmpFactor: opts.DefaultClusterAmpFactor,
	}
	clusterCounterVec.Update()

	factory.clusterCounterVectors.Store(opts.Name, clusterCounterVec)

	return factory.NewClusterCounterVec(opts, labelNames)
}

func (factory *ClusterCounterRedisFactory) NewClusterCounter(opts *ClusterCounterOpts,
) (ClusterCounterI, error) {
	if opts == nil || len(opts.Name) == 0 {
		return nil, errors.New("name error")
	}

	if opts.DefaultClusterAmpFactor == 0.0 {
		opts.DefaultClusterAmpFactor = factory.DefaultClusterAmpFactor
	}

	if opts.PushInterval == 0 {
		opts.PushInterval = time.Duration(5) * time.Second
	}

	if opts.PullInterval == 0 {
		opts.PullInterval = time.Duration(10) * time.Second
	}

	if counter, ok := factory.clusterCounters.Load(opts.Name); ok {
		return counter.(ClusterCounterI), nil
	}

	if opts.DefaultClusterAmpFactor == 0 {
		opts.DefaultClusterAmpFactor = factory.DefaultClusterAmpFactor
	}

	clusterCounter := &ClusterCounter{
		Factory:                 factory,
		ResetInterval:           opts.ResetInterval,
		PullInterval:            opts.PullInterval,
		PushInterval:            opts.PushInterval,
		Name:                    opts.Name,
		DefaultClusterAmpFactor: opts.DefaultClusterAmpFactor,
	}
	clusterCounter.Update()

	factory.clusterCounters.Store(opts.Name, clusterCounter)

	return factory.NewClusterCounter(opts)
}

func (factory *ClusterCounterRedisFactory) Start() {
	factory.syncStatus = true
	go factory.WatchAndSync()
}

func (factory *ClusterCounterRedisFactory) Stop() {
	factory.syncStatus = false
}

func (factory *ClusterCounterRedisFactory) WatchAndSync() {
	for {
		factory.clusterCounterVectors.Range(func(k interface{}, v interface{}) bool {
			if counter, ok := v.(*ClusterCounterVec); ok {
				counter.Update()
			}
			return true
		})

		factory.clusterCounters.Range(func(k interface{}, v interface{}) bool {
			if counter, ok := v.(*ClusterCounter); ok {
				counter.Update()
			}
			return true
		})

		time.Sleep(factory.UpdateInterval)
	}
}

func (factory *ClusterCounterRedisFactory) PushData(name string, measure string, key string, value int64) error {
	return factory.store.Store(factory.redisKeyPrefix+measure+SPACE_SEP+key, value)
}

func (factory *ClusterCounterRedisFactory) PullData(name string, measure string, key string) (int64, error) {
	return factory.store.Load(factory.redisKeyPrefix + measure + SPACE_SEP + key)
}

func (factory *ClusterCounterRedisFactory) Delete(name string) {
	factory.clusterCounterVectors.Delete(name)
}
