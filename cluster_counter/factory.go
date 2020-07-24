package cluster_counter

import (
	"errors"
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

type ClusterCounterFactory struct {
	clusterCounterVectors sync.Map
	clusterCounters       sync.Map

	keyPrefix string

	Store DataStoreI

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
		keyPrefix:                opts.KeyPrefix,
		Store:                    store,
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
) (*ClusterCounterVec, error) {
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
		return counter.(*ClusterCounterVec), nil
	}

	if opts.DefaultLocalTrafficRatio == 0 {
		opts.DefaultLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	clusterCounterVec := &ClusterCounterVec{
		factory:                  factory,
		resetInterval:            opts.ResetInterval,
		loadDataInterval:         opts.LoadDataInterval,
		storeDataInterval:        opts.StoreDataInterval,
		name:                     opts.Name,
		labelNames:               append([]string{}, labelNames...),
		defaultLocalTrafficRatio: opts.DefaultLocalTrafficRatio,
		discardPreviousData:      opts.DiscardPreviousData,
	}

	factory.clusterCounterVectors.Store(opts.Name, clusterCounterVec)

	return factory.NewClusterCounterVec(opts, labelNames)
}

func (factory *ClusterCounterFactory) NewClusterCounter(opts *ClusterCounterOpts,
) (*ClusterCounter, error) {
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
		return counter.(*ClusterCounter), nil
	}

	if opts.DefaultLocalTrafficRatio == 0 {
		opts.DefaultLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	clusterCounter := &ClusterCounter{
		factory:             factory,
		resetInterval:       opts.ResetInterval,
		loadDataInterval:    opts.LoadDataInterval,
		storeDataInterval:   opts.StoreDataInterval,
		name:                opts.Name,
		defaultTrafficRatio: opts.DefaultLocalTrafficRatio,
		discardPreviousData: opts.DiscardPreviousData,
	}
	clusterCounter.Init()

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

func (factory *ClusterCounterFactory) Delete(name string) {
	factory.clusterCounters.Delete(name)
}

func (factory *ClusterCounterFactory) DeleteVec(name string) {
	factory.clusterCounterVectors.Delete(name)
}
