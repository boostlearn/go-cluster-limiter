package cluster_limiter

import (
	"errors"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"sync"
	"time"
)

const DEFAULT_LIMITER_NAME = "lmt:"
const DEFAULT_HEARTBEAT_INTERVAL_MILLISECONDS = 100

type ClusterLimiterOpts struct {
	Name                string
	BeginTime           time.Time
	EndTime             time.Time
	CompletionTime      time.Time
	PeriodInterval      time.Duration
	ReserveInterval     time.Duration
	BurstInterval       time.Duration
	MaxBoostFactor      float64
	DiscardPreviousData bool
}

type ClusterLimiterFactory struct {
	name                            string
	ticker                          *time.Ticker
	heartbeatInterval               time.Duration
	defaultClusterLocalTrafficRatio float64

	limiterVectors sync.Map
	limiters       sync.Map
	counterFactory *cluster_counter.ClusterCounterFactory
}

// 构建参数
type ClusterLimiterFactoryOpts struct {
	Name                     string
	DefaultHeartbeatInterval time.Duration
	DefaultLocalTrafficRate  float64
}

func NewFactory(opts *ClusterLimiterFactoryOpts,
	dataStore cluster_counter.DataStoreI,
) *ClusterLimiterFactory {
	if opts.DefaultHeartbeatInterval == 0 {
		opts.DefaultHeartbeatInterval = time.Duration(DEFAULT_HEARTBEAT_INTERVAL_MILLISECONDS) * time.Millisecond
	}

	if len(opts.Name) == 0 {
		opts.Name = DEFAULT_LIMITER_NAME
	}

	counterFactory := cluster_counter.NewFactory(&cluster_counter.ClusterCounterFactoryOpts{
		Name:                     opts.Name + ":cls_ct:",
		DefaultLocalTrafficRatio: opts.DefaultLocalTrafficRate,
		HeartbeatInterval:        opts.DefaultHeartbeatInterval,
	}, dataStore)
	factory := &ClusterLimiterFactory{
		limiterVectors:    sync.Map{},
		counterFactory:    counterFactory,
		heartbeatInterval: opts.DefaultHeartbeatInterval,
		name:              opts.Name,
	}
	return factory
}

func (factory *ClusterLimiterFactory) NewClusterLimiterVec(opts *ClusterLimiterOpts,
	labelNames []string,
) (*ClusterLimiterVec, error) {
	if len(opts.Name) == 0 {
		return nil, errors.New("need name")
	}
	if opts.PeriodInterval.Truncate(time.Second) == 0 &&
		opts.BeginTime.Truncate(time.Second).Before(opts.EndTime.Truncate(time.Second)) == false {
		return nil, errors.New("period interval not set or begin time bigger than end time")
	}

	if len(labelNames) == 0 {
		return nil, errors.New("need label names")
	}

	if l, ok := factory.limiterVectors.Load(opts.Name); ok {
		return l.(*ClusterLimiterVec), nil
	}

	if opts.BurstInterval == 0 {
		opts.BurstInterval = DEFAULT_BURST_INERVAL_SECONDS * time.Second
	}

	if opts.CompletionTime.Unix() == 0 {
		opts.CompletionTime = opts.EndTime
	}

	var limiterVec = &ClusterLimiterVec{
		name:                opts.Name,
		beginTime:           opts.BeginTime,
		endTime:             opts.EndTime,
		completionTime:      opts.CompletionTime,
		periodInterval:      opts.PeriodInterval,
		reserveInterval:     opts.ReserveInterval,
		maxBoostFactor:      opts.MaxBoostFactor,
		burstInterval:       opts.BurstInterval,
		discardPreviousData: opts.DiscardPreviousData,
	}

	var err error
	limiterVec.RequestCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":request",
		PeriodInterval:      opts.PeriodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
		StoreDataInterval:   opts.BurstInterval,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.PassCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":pass",
		PeriodInterval:      limiterVec.periodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
		StoreDataInterval:   opts.BurstInterval,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.RewardCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":reward",
		PeriodInterval:      limiterVec.periodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
		StoreDataInterval:   opts.BurstInterval,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	factory.limiterVectors.Store(opts.Name, limiterVec)
	return factory.NewClusterLimiterVec(opts, labelNames)
}

func (factory *ClusterLimiterFactory) NewClusterLimiter(opts *ClusterLimiterOpts,
) (*ClusterLimiter, error) {
	if len(opts.Name) == 0 {
		return nil, errors.New("need name")
	}
	if opts.BeginTime.UnixNano() >= opts.EndTime.UnixNano() {
		return nil, errors.New("time error")
	}

	if l, ok := factory.limiters.Load(opts.Name); ok {
		return l.(*ClusterLimiter), nil
	}

	if opts.CompletionTime.Unix() == 0 {
		opts.CompletionTime = opts.EndTime
	}

	var limiterVec = &ClusterLimiter{
		name:                opts.Name,
		RequestCounter:      nil,
		PassCounter:         nil,
		RewardCounter:       nil,
		beginTime:           opts.BeginTime,
		endTime:             opts.EndTime,
		completionTime:      opts.EndTime,
		periodInterval:      opts.PeriodInterval,
		reserveInterval:     opts.ReserveInterval,
		maxBoostFactor:      opts.MaxBoostFactor,
		discardPreviousData: opts.DiscardPreviousData,
	}

	var err error
	limiterVec.RequestCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":request",
		PeriodInterval:      opts.PeriodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
		StoreDataInterval:   opts.BurstInterval,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.PassCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":pass",
		PeriodInterval:      opts.PeriodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
		StoreDataInterval:   opts.BurstInterval,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.RewardCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":reward",
		PeriodInterval:      opts.PeriodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
		StoreDataInterval:   opts.BurstInterval,
	})
	if err != nil {
		return nil, err
	}

	factory.limiterVectors.Store(opts.Name, limiterVec)
	return factory.NewClusterLimiter(opts)
}

func (factory *ClusterLimiterFactory) Delete(name string) {
	factory.limiters.Delete(name)
	factory.counterFactory.Delete(factory.name + name + ":request")
	factory.counterFactory.Delete(factory.name + name + ":pass")
	factory.counterFactory.Delete(factory.name + name + ":reward")
}

func (factory *ClusterLimiterFactory) DeleteVec(name string) {
	factory.limiterVectors.Delete(name)
	factory.counterFactory.DeleteVec(factory.name + name + ":request")
	factory.counterFactory.DeleteVec(factory.name + name + ":pass")
	factory.counterFactory.DeleteVec(factory.name + name + ":reward")
}

func (factory *ClusterLimiterFactory) Start() {
	if factory.ticker == nil {
		factory.ticker = time.NewTicker(factory.heartbeatInterval)
	}
	go factory.WatchAndSync()
}

func (factory *ClusterLimiterFactory) Stop() {
	if factory.ticker != nil {
		factory.ticker.Stop()
	}
}

func (factory *ClusterLimiterFactory) WatchAndSync() {
	for range factory.ticker.C {
		factory.Heartbeat()
	}
}

func (factory *ClusterLimiterFactory) Heartbeat() {
	factory.counterFactory.Heartbeat()

	factory.limiters.Range(func(k interface{}, v interface{}) bool {
		if limiter, ok := v.(*ClusterLimiter); ok {
			limiter.Heartbeat()
			if limiter.Expire() {
				factory.limiters.Delete(k)
			}
		}
		return true
	})

	factory.limiterVectors.Range(func(k interface{}, v interface{}) bool {
		if limiter, ok := v.(*ClusterLimiterVec); ok {
			limiter.Heartbeat()
			if limiter.Expire() {
				factory.limiterVectors.Delete(k)
			}
		}
		return true
	})
}
