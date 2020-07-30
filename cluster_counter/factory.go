package cluster_counter

import (
	"errors"
	"sync"
	"time"
)

const DEFAULT_HEARTBEAT_INTERVAL_MILLISECONDS = 100
const DEFAULT_FACTORY_NAME = "clct"

type ClusterCounterOpts struct {
	Name           string
	BeginTime      time.Time
	EndTime        time.Time
	PeriodInterval time.Duration

	DiscardPreviousData bool
	StoreDataInterval   time.Duration

	InitLocalTrafficRatio float64
	DeclineExpRatio       float64
}

type ClusterCounterFactory struct {
	name   string
	Store  DataStoreI
	ticker *time.Ticker

	defaultLocalTrafficRatio float64
	heartbeatInterval        time.Duration

	clusterCounterVectors sync.Map
	clusterCounters       sync.Map
}

type ClusterCounterFactoryOpts struct {
	Name                     string
	DefaultLocalTrafficRatio float64
	HeartbeatInterval        time.Duration
}

func NewFactory(opts *ClusterCounterFactoryOpts, store DataStoreI) *ClusterCounterFactory {
	if len(opts.Name) == 0 {
		opts.Name = DEFAULT_FACTORY_NAME
	}
	if opts.DefaultLocalTrafficRatio > 1.0 {
		opts.DefaultLocalTrafficRatio = DEFAULT_TRAFFIC_RATIO
	}

	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = time.Duration(DEFAULT_HEARTBEAT_INTERVAL_MILLISECONDS) * time.Millisecond
	}

	factory := &ClusterCounterFactory{
		name:  opts.Name,
		Store: store,

		defaultLocalTrafficRatio: opts.DefaultLocalTrafficRatio,
		heartbeatInterval:        opts.HeartbeatInterval,
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

	if opts.InitLocalTrafficRatio == 0.0 {
		opts.InitLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	if opts.StoreDataInterval.Truncate(time.Second) == 0 {
		opts.StoreDataInterval = time.Duration(DEFAULT_STORE_INTERVAL_SECONDS) * time.Second
	}

	if counter, ok := factory.clusterCounterVectors.Load(opts.Name); ok {
		return counter.(*ClusterCounterVec), nil
	}

	if opts.InitLocalTrafficRatio == 0 {
		opts.InitLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	clusterCounterVec := &ClusterCounterVec{
		factory:               factory,
		beginTime:             opts.BeginTime,
		endTime:               opts.EndTime,
		periodInterval:        opts.PeriodInterval,
		storeInterval:         opts.StoreDataInterval.Truncate(time.Second),
		name:                  opts.Name,
		labelNames:            append([]string{}, labelNames...),
		initLocalTrafficRatio: opts.InitLocalTrafficRatio,
		discardPreviousData:   opts.DiscardPreviousData,
		declineExpRatio:       opts.DeclineExpRatio,
	}

	factory.clusterCounterVectors.Store(opts.Name, clusterCounterVec)

	return factory.NewClusterCounterVec(opts, labelNames)
}

func (factory *ClusterCounterFactory) NewClusterCounter(opts *ClusterCounterOpts,
) (*ClusterCounter, error) {
	if opts == nil || len(opts.Name) == 0 {
		return nil, errors.New("name error")
	}

	if opts.InitLocalTrafficRatio == 0.0 {
		opts.InitLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	if opts.StoreDataInterval.Truncate(time.Second) == 0 {
		opts.StoreDataInterval = time.Duration(DEFAULT_STORE_INTERVAL_SECONDS) * time.Second
	}

	if counter, ok := factory.clusterCounters.Load(opts.Name); ok {
		return counter.(*ClusterCounter), nil
	}

	if opts.InitLocalTrafficRatio == 0 {
		opts.InitLocalTrafficRatio = factory.defaultLocalTrafficRatio
	}

	clusterCounter := &ClusterCounter{
		factory:               factory,
		beginTime:             opts.BeginTime,
		endTime:               opts.EndTime,
		periodInterval:        opts.PeriodInterval,
		storeInterval:         opts.StoreDataInterval.Truncate(time.Second),
		name:                  opts.Name,
		initLocalTrafficRatio: opts.InitLocalTrafficRatio,
		discardPreviousData:   opts.DiscardPreviousData,
		declineExpRatio:       opts.DeclineExpRatio,
	}
	clusterCounter.Init()

	factory.clusterCounters.Store(opts.Name, clusterCounter)

	return factory.NewClusterCounter(opts)
}

func (factory *ClusterCounterFactory) Start() {
	if factory.ticker == nil {
		factory.ticker = time.NewTicker(factory.heartbeatInterval)
	}
	go factory.WatchAndSync()
}

func (factory *ClusterCounterFactory) Stop() {
	if factory.ticker != nil {
		factory.ticker.Stop()
	}
}

func (factory *ClusterCounterFactory) WatchAndSync() {
	for range factory.ticker.C {
		factory.Heartbeat()
	}
}

func (factory *ClusterCounterFactory) Heartbeat() {
	factory.clusterCounterVectors.Range(func(k interface{}, v interface{}) bool {
		if counterVec, ok := v.(*ClusterCounterVec); ok {
			counterVec.Heartbeat()

			if counterVec.Expire() {
				factory.clusterCounters.Delete(k)
			}
		}
		return true
	})

	factory.clusterCounters.Range(func(k interface{}, v interface{}) bool {
		if counter, ok := v.(*ClusterCounter); ok {
			counter.Heartbeat()

			if counter.Expire() {
				factory.clusterCounters.Delete(k)
			}
		}
		return true
	})
}

func (factory *ClusterCounterFactory) Delete(name string) {
	factory.clusterCounters.Delete(name)
}

func (factory *ClusterCounterFactory) DeleteVec(name string) {
	factory.clusterCounterVectors.Delete(name)
}
