package cluster_limiter

import (
	"errors"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"sync"
	"time"
)

const LimiterPrefix = "lmt:"

type ClusterLimiterOpts struct {
	Name                string
	BeginTime           time.Time
	EndTime             time.Time
	PeriodInterval      time.Duration
	BoostInterval       time.Duration
	SilentInterval      time.Duration
	BurstInterval       time.Duration
	MaxBoostFactor      float64
	DiscardPreviousData bool
}

type ClusterLimiterFactory struct {
	status                          bool
	limiterVectors                  sync.Map
	limiters                        sync.Map
	counterFactory                  *cluster_counter.ClusterCounterFactory
	defaultBoostInterval            time.Duration
	updateInterval                  time.Duration
	defaultMaxBoostFactor           float64
	name                            string
	defaultClusterLocalTrafficRatio float64
}

// 构建参数
type ClusterLimiterFactoryOpts struct {
	Name                     string
	DefaultBoostInterval     time.Duration
	DefaultHeartBeatInterval time.Duration
	DefaultMaxBoostFactor    float64
	DefaultLocalTrafficRate  float64
}

func NewFactory(opts *ClusterLimiterFactoryOpts,
	counterFactory *cluster_counter.ClusterCounterFactory,
) *ClusterLimiterFactory {

	if opts.DefaultBoostInterval == 0 {
		opts.DefaultBoostInterval = time.Duration(60) * time.Second
	}

	if opts.DefaultHeartBeatInterval == 0 {
		opts.DefaultHeartBeatInterval = time.Duration(1) * time.Second
	}

	if len(opts.Name) == 0 {
		opts.Name = LimiterPrefix
	}

	factory := &ClusterLimiterFactory{
		limiterVectors:       sync.Map{},
		counterFactory:       counterFactory,
		defaultBoostInterval: opts.DefaultBoostInterval,
		updateInterval:       opts.DefaultHeartBeatInterval,
		name:                 opts.Name,
	}
	factory.Start()
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

	if opts.SilentInterval == 0 {
		opts.SilentInterval = factory.updateInterval
	}

	if opts.BurstInterval == 0 {
		opts.BurstInterval = factory.updateInterval
	}

	if opts.BoostInterval == 0 {
		opts.BoostInterval = factory.defaultBoostInterval
	}

	var limiterVec = &ClusterLimiterVec{
		name:                opts.Name,
		beginTime:           opts.BeginTime,
		endTime:             opts.EndTime,
		periodInterval:      opts.PeriodInterval,
		boostInterval:       opts.BoostInterval,
		maxBoostFactor:      opts.MaxBoostFactor,
		silentInterval:      opts.SilentInterval,
		burstInterval:       opts.BurstInterval,
		discardPreviousData: opts.DiscardPreviousData,
	}

	var err error
	limiterVec.RequestCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":request",
		PeriodInterval:      opts.PeriodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.PassCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":pass",
		PeriodInterval:      limiterVec.periodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.RewardCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":reward",
		PeriodInterval:      limiterVec.periodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
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

	if opts.SilentInterval == 0 {
		opts.SilentInterval = factory.updateInterval
	}

	if opts.BoostInterval == 0 {
		opts.BoostInterval = factory.defaultBoostInterval
	}

	var limiterVec = &ClusterLimiter{
		name:                opts.Name,
		RequestCounter:      nil,
		PassCounter:         nil,
		RewardCounter:       nil,
		beginTime:           opts.BeginTime,
		endTime:             opts.EndTime,
		periodInterval:      opts.PeriodInterval,
		boostInterval:       opts.BoostInterval,
		maxBoostFactor:      opts.MaxBoostFactor,
		silentInterval:      opts.SilentInterval,
		discardPreviousData: opts.DiscardPreviousData,
	}

	var err error
	limiterVec.RequestCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":request",
		PeriodInterval:      opts.PeriodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.PassCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":pass",
		PeriodInterval:      opts.PeriodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.RewardCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.name + opts.Name + ":reward",
		PeriodInterval:      opts.PeriodInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	factory.limiterVectors.Store(opts.Name, limiterVec)
	return factory.NewClusterLimiter(opts)
}

func (factory *ClusterLimiterFactory) Delete(name string) {
	factory.counterFactory.Delete(LimiterPrefix + name + ":request")
	factory.counterFactory.Delete(LimiterPrefix + name + ":pass")
	factory.counterFactory.Delete(LimiterPrefix + name + ":reward")
}

func (factory *ClusterLimiterFactory) Start() {
	factory.status = true
	go factory.WatchAndSync()
}

func (factory *ClusterLimiterFactory) Stop() {
	factory.status = false
}

func (factory *ClusterLimiterFactory) WatchAndSync() {
	for factory.status {
		factory.limiters.Range(func(k interface{}, v interface{}) bool {
			if limiter, ok := v.(*ClusterLimiter); ok {
				limiter.HeartBeat()
				if limiter.Expire() {
					factory.limiters.Delete(k)
				}
			}
			return true
		})

		factory.limiterVectors.Range(func(k interface{}, v interface{}) bool {
			if limiter, ok := v.(*ClusterLimiterVec); ok {
				limiter.HeartBeat()
				if limiter.Expire() {
					factory.limiterVectors.Delete(k)
				}
			}
			return true
		})
		time.Sleep(factory.updateInterval)
	}
}
