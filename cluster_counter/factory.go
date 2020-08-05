package cluster_counter

import (
	"errors"
	"sync"
	"time"
)

const DefaultHeartbeatIntervalMilliseconds = 100
const DefaultFactoryName = "clct"

// Options for creating counter
type ClusterCounterOpts struct {
	Name           string
	BeginTime      time.Time
	EndTime        time.Time
	PeriodInterval time.Duration

	DiscardPreviousData bool
	StoreDataInterval   time.Duration

	InitLocalTrafficProportion float64
	DeclineExpRatio            float64
}

// Producer of counter
type ClusterCounterFactory struct {
	name   string
	Store  DataStoreI
	ticker *time.Ticker

	defaultLocalTrafficProportion float64
	heartbeatInterval             time.Duration

	clusterCounterVectors sync.Map
	clusterCounters       sync.Map
}

// options for creating counter's factory
type ClusterCounterFactoryOpts struct {
	Name                          string
	DefaultLocalTrafficProportion float64
	HeartbeatInterval             time.Duration
}

// create new counter's factory
func NewFactory(opts *ClusterCounterFactoryOpts, store DataStoreI) *ClusterCounterFactory {
	if len(opts.Name) == 0 {
		opts.Name = DefaultFactoryName
	}
	if opts.DefaultLocalTrafficProportion > 1.0 {
		opts.DefaultLocalTrafficProportion = DefaultTrafficProportion
	}

	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = time.Duration(DefaultHeartbeatIntervalMilliseconds) * time.Millisecond
	}

	factory := &ClusterCounterFactory{
		name:  opts.Name,
		Store: store,

		defaultLocalTrafficProportion: opts.DefaultLocalTrafficProportion,
		heartbeatInterval:             opts.HeartbeatInterval,
	}
	factory.Start()
	return factory
}

// create new counter vector
func (factory *ClusterCounterFactory) NewClusterCounterVec(opts *ClusterCounterOpts,
	labelNames []string,
) (*ClusterCounterVec, error) {
	if opts == nil || len(opts.Name) == 0 {
		return nil, errors.New("name error")
	}

	if len(labelNames) == 0 {
		return nil, errors.New("need label names")
	}

	if opts.InitLocalTrafficProportion == 0.0 {
		opts.InitLocalTrafficProportion = factory.defaultLocalTrafficProportion
	}

	if opts.StoreDataInterval.Truncate(time.Second) == 0 {
		opts.StoreDataInterval = time.Duration(DefaultStoreIntervalSeconds) * time.Second
	}

	if counter, ok := factory.clusterCounterVectors.Load(opts.Name); ok {
		return counter.(*ClusterCounterVec), nil
	}

	if opts.InitLocalTrafficProportion == 0 {
		opts.InitLocalTrafficProportion = factory.defaultLocalTrafficProportion
	}

	clusterCounterVec := &ClusterCounterVec{
		factory:                    factory,
		beginTime:                  opts.BeginTime,
		endTime:                    opts.EndTime,
		periodInterval:             opts.PeriodInterval,
		storeInterval:              opts.StoreDataInterval.Truncate(time.Second),
		name:                       opts.Name,
		labelNames:                 append([]string{}, labelNames...),
		initLocalTrafficProportion: opts.InitLocalTrafficProportion,
		discardPreviousData:        opts.DiscardPreviousData,
		declineExpRatio:            opts.DeclineExpRatio,
	}

	factory.clusterCounterVectors.Store(opts.Name, clusterCounterVec)

	return factory.NewClusterCounterVec(opts, labelNames)
}

// create new counter
func (factory *ClusterCounterFactory) NewClusterCounter(opts *ClusterCounterOpts,
) (*ClusterCounter, error) {
	if opts == nil || len(opts.Name) == 0 {
		return nil, errors.New("name error")
	}

	if opts.InitLocalTrafficProportion == 0.0 {
		opts.InitLocalTrafficProportion = factory.defaultLocalTrafficProportion
	}

	if opts.StoreDataInterval.Truncate(time.Second) == 0 {
		opts.StoreDataInterval = time.Duration(DefaultStoreIntervalSeconds) * time.Second
	}

	if counter, ok := factory.clusterCounters.Load(opts.Name); ok {
		return counter.(*ClusterCounter), nil
	}

	if opts.InitLocalTrafficProportion == 0 {
		opts.InitLocalTrafficProportion = factory.defaultLocalTrafficProportion
	}

	clusterCounter := &ClusterCounter{
		factory:                    factory,
		beginTime:                  opts.BeginTime,
		endTime:                    opts.EndTime,
		periodInterval:             opts.PeriodInterval,
		storeInterval:              opts.StoreDataInterval.Truncate(time.Second),
		name:                       opts.Name,
		initLocalTrafficProportion: opts.InitLocalTrafficProportion,
		discardPreviousData:        opts.DiscardPreviousData,
		declineExpRatio:            opts.DeclineExpRatio,
	}
	clusterCounter.Initialize()

	factory.clusterCounters.Store(opts.Name, clusterCounter)

	return factory.NewClusterCounter(opts)
}

// start update
func (factory *ClusterCounterFactory) Start() {
	if factory.ticker == nil {
		factory.ticker = time.NewTicker(factory.heartbeatInterval)
		go factory.WatchAndSync()
	}
}

// stop update
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
