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
	Name          string
	BeginTime     time.Time
	EndTime       time.Time
	ResetInterval time.Duration

	DiscardPreviousData bool
	StoreDataInterval   time.Duration

	InitLocalTrafficProportion float64
	DeclineExpRatio            float64
}

// Producer of counter
type ClusterCounterFactory struct {
	name  string
	Store DataStoreI

	ticker            *time.Ticker
	heartbeatInterval time.Duration

	clusterCounterVectors sync.Map
	clusterCounters       sync.Map
}

// options for creating counter's factory
type ClusterCounterFactoryOpts struct {
	Name              string
	HeartbeatInterval time.Duration
	Store             DataStoreI
}

// create new counter's factory
func NewFactory(opts *ClusterCounterFactoryOpts) *ClusterCounterFactory {
	if len(opts.Name) == 0 {
		opts.Name = DefaultFactoryName
	}

	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = time.Duration(DefaultHeartbeatIntervalMilliseconds) * time.Millisecond
	}

	factory := &ClusterCounterFactory{
		name:              opts.Name,
		Store:             opts.Store,
		heartbeatInterval: opts.HeartbeatInterval,
	}
	factory.Start()
	return factory
}

// create new counter vector
func (factory *ClusterCounterFactory) NewClusterCounterVec(opts *ClusterCounterOpts,
	labelNames []string,
) (*ClusterCounterVec, error) {
	if opts == nil || len(opts.Name) == 0 {
		return nil, errors.New("name cannot be nil")
	}

	if opts.StoreDataInterval.Truncate(time.Second) == 0 {
		opts.StoreDataInterval = time.Duration(DefaultStoreIntervalSeconds) * time.Second
	}

	if opts.InitLocalTrafficProportion == 0 {
		opts.InitLocalTrafficProportion = 1.0
	}

	clusterCounterVec := &ClusterCounterVec{
		Options:                    opts,
		factory:                    factory,
		beginTime:                  opts.BeginTime,
		endTime:                    opts.EndTime,
		resetInterval:              opts.ResetInterval,
		storeInterval:              opts.StoreDataInterval.Truncate(time.Second),
		name:                       opts.Name,
		labelNames:                 append([]string{}, labelNames...),
		initLocalTrafficProportion: opts.InitLocalTrafficProportion,
		discardPreviousData:        opts.DiscardPreviousData,
		declineExpRatio:            opts.DeclineExpRatio,
	}

	factory.clusterCounterVectors.Store(opts.Name, clusterCounterVec)

	return factory.GetClusterCounterVec(opts.Name), nil
}

// get counter vector
func (factory *ClusterCounterFactory) GetClusterCounterVec(name string) *ClusterCounterVec {
	if counter, ok := factory.clusterCounterVectors.Load(name); ok {
		return counter.(*ClusterCounterVec)
	}
	return nil
}

// create new counter
func (factory *ClusterCounterFactory) NewClusterCounter(opts *ClusterCounterOpts,
) (*ClusterCounter, error) {
	if opts == nil || len(opts.Name) == 0 {
		return nil, errors.New("name error")
	}

	if opts.StoreDataInterval.Truncate(time.Second) == 0 {
		opts.StoreDataInterval = time.Duration(DefaultStoreIntervalSeconds) * time.Second
	}

	if opts.InitLocalTrafficProportion == 0 {
		opts.InitLocalTrafficProportion = 1.0
	}

	if opts.DeclineExpRatio == 0 {
		opts.DeclineExpRatio = DefaultDeclineExpRatio
	}

	clusterCounter := &ClusterCounter{
		factory: factory,
		Options: opts,
		name:    opts.Name,

		beginTime:                  opts.BeginTime,
		endTime:                    opts.EndTime,
		resetInterval:              opts.ResetInterval,
		storeInterval:              opts.StoreDataInterval.Truncate(time.Second),
		initLocalTrafficProportion: opts.InitLocalTrafficProportion,
		discardPreviousData:        opts.DiscardPreviousData,
		declineExpRatio:            opts.DeclineExpRatio,
	}
	clusterCounter.Initialize()

	factory.clusterCounters.Store(opts.Name, clusterCounter)

	return factory.GetClusterCounter(opts.Name), nil
}

// create new counter
func (factory *ClusterCounterFactory) GetClusterCounter(name string) *ClusterCounter {
	if counter, ok := factory.clusterCounters.Load(name); ok {
		return counter.(*ClusterCounter)
	}
	return nil
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
