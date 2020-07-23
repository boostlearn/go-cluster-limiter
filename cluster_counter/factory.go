package cluster_counter

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type ClusterCounterOpts struct {
	Name             string
	ResetInterval    time.Duration
	LoadDataInterval time.Duration

	StoreDataInterval        time.Duration
	DefaultLocalTrafficRatio float64

	DiscardPreviousData bool
}

type ClusterCounterFactoryI interface {
	// 构建计数器
	NewClusterCounter(opts *ClusterCounterOpts) (ClusterCounterI, error)

	NewClusterCounterVec(opts *ClusterCounterOpts, labelNames []string) (ClusterCounterVecI, error)

	// 向集群推送数据
	StoreData(name string, measure string, key string, value int64) error

	// 向集群推送数据
	LoadData(name string, measure string, key string) (int64, error)

	Delete(name string)
}

type ClusterCounterFactory struct {
	clusterCounterVectors sync.Map
	clusterCounters       sync.Map

	keyPrefix string
	store          DataStoreI

	syncStatus bool

	defaultLocalTrafficRatio float64

	updateInterval time.Duration
}

type ClusterCounterFactoryOpts struct {
	KeyPrefix                string
	DefaultLocalTrafficRatio float64
	UpdateInterval           time.Duration
}

func NewFactory(opts *ClusterCounterFactoryOpts, store DataStoreI) *ClusterCounterFactory {
	if len(opts.KeyPrefix) == 0 {
		opts.KeyPrefix = "CLCT:"
	}
	if opts.DefaultLocalTrafficRatio < 1.0 {
		opts.DefaultLocalTrafficRatio = 1.0
	}

	if opts.UpdateInterval.Seconds() < 1 {
		opts.UpdateInterval = time.Duration(1) * time.Second
	}

	factory := &ClusterCounterFactory{
		keyPrefix:           opts.KeyPrefix,
		store:                    store,
		syncStatus:               false,
		defaultLocalTrafficRatio: opts.DefaultLocalTrafficRatio,
		updateInterval:           opts.UpdateInterval,
	}
	factory.Start()
	return factory
}

// 新建计数器
func (factory *ClusterCounterFactory) NewClusterCounterVec(opts *ClusterCounterOpts,
	labelNames []string,
) (ClusterCounterVecI, error) {
	if opts == nil || len(opts.Name) == 0 {
		return nil, errors.New("name error")
	}

	if len(labelNames) == 0 {
		return nil, errors.New("need label names")
	}

	if opts.DefaultLocalTrafficRatio == 0.0 {
		opts.DefaultLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	if opts.StoreDataInterval == 0 {
		opts.StoreDataInterval = time.Duration(5) * time.Second
	}

	if opts.LoadDataInterval == 0 {
		opts.LoadDataInterval = time.Duration(10) * time.Second
	}

	if counter, ok := factory.clusterCounterVectors.Load(opts.Name); ok {
		return counter.(ClusterCounterVecI), nil
	}

	if opts.DefaultLocalTrafficRatio == 0 {
		opts.DefaultLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	clusterCounterVec := &ClusterCounterVec{
		Factory:                  factory,
		resetInterval:            opts.ResetInterval,
		loadDataInterval:         opts.LoadDataInterval,
		storeDataInterval:        opts.StoreDataInterval,
		name:                     opts.Name,
		labelNames:               append([]string{}, labelNames...),
		defaultLocalTrafficRatio: opts.DefaultLocalTrafficRatio,
		discardPreviousData:opts.DiscardPreviousData,
	}
	clusterCounterVec.Update()

	factory.clusterCounterVectors.Store(opts.Name, clusterCounterVec)

	return factory.NewClusterCounterVec(opts, labelNames)
}

func (factory *ClusterCounterFactory) NewClusterCounter(opts *ClusterCounterOpts,
) (ClusterCounterI, error) {
	if opts == nil || len(opts.Name) == 0 {
		return nil, errors.New("name error")
	}

	if opts.DefaultLocalTrafficRatio == 0.0 {
		opts.DefaultLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	if opts.StoreDataInterval == 0 {
		opts.StoreDataInterval = time.Duration(5) * time.Second
	}

	if opts.LoadDataInterval == 0 {
		opts.LoadDataInterval = time.Duration(10) * time.Second
	}

	if counter, ok := factory.clusterCounters.Load(opts.Name); ok {
		return counter.(ClusterCounterI), nil
	}

	if opts.DefaultLocalTrafficRatio == 0 {
		opts.DefaultLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	clusterCounter := &ClusterCounter{
		Factory:             factory,
		resetInterval:       opts.ResetInterval,
		loadDataInterval:    opts.LoadDataInterval,
		storeDataInterval:   opts.StoreDataInterval,
		name:                opts.Name,
		defaultTrafficRatio: opts.DefaultLocalTrafficRatio,
		discardPreviousData:opts.DiscardPreviousData,
	}
	clusterCounter.Update()

	factory.clusterCounters.Store(opts.Name, clusterCounter)

	return factory.NewClusterCounter(opts)
}

func (factory *ClusterCounterFactory) Start() {
	factory.syncStatus = true
	go factory.WatchAndSync()
}

func (factory *ClusterCounterFactory) Stop() {
	factory.syncStatus = false
}

func (factory *ClusterCounterFactory) WatchAndSync() {
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

		time.Sleep(factory.updateInterval)
	}
}

func (factory *ClusterCounterFactory) StoreData(name string, measure string, key string, value int64) error {
	storeKey := fmt.Sprintf("%v%v%v%v%v%v", factory.keyPrefix, name, SEP, measure, SEP, key)
	return factory.store.Store(storeKey, value)
}

func (factory *ClusterCounterFactory) LoadData(name string, measure string, key string) (int64, error) {
	storeKey := fmt.Sprintf("%v%v%v%v%v%v", factory.keyPrefix, name, SEP, measure, SEP, key)
	return factory.store.Load(storeKey)
}

func (factory *ClusterCounterFactory) Delete(name string) {
	factory.clusterCounterVectors.Delete(name)
}
