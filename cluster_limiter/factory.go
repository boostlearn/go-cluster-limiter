package cluster_limiter

import (
	"errors"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"sync"
	"time"
)

const DefaultLimiterName = "lmt:"
const DefaultHeartbeatIntervalMilliseconds = 100

// options for creating limiter
type ClusterLimiterOpts struct {
	Name                       string
	BeginTime                  time.Time
	EndTime                    time.Time
	CompletionTime             time.Time
	PeriodInterval             time.Duration
	ReserveInterval            time.Duration
	BurstInterval              time.Duration
	MaxBoostFactor             float64
	DiscardPreviousData        bool
	InitLocalTrafficProportion float64
	InitIdealPassRate          float64
	InitRewardRate             float64
	ScoreSamplesSortInterval   time.Duration
	ScoreSamplesMax            int64
}

// Producer of limiter
type ClusterLimiterFactory struct {
	name              string
	ticker            *time.Ticker
	heartbeatInterval time.Duration
	limiterVectors    sync.Map
	limiters          sync.Map
	counterFactory    *cluster_counter.ClusterCounterFactory
}

// options of creating limiter's factory
type ClusterLimiterFactoryOpts struct {
	Name                       string
	HeartbeatInterval          time.Duration
	InitLocalTrafficProportion float64
}

// build new factory
func NewFactory(opts *ClusterLimiterFactoryOpts,
	dataStore cluster_counter.DataStoreI,
) *ClusterLimiterFactory {
	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = time.Duration(DefaultHeartbeatIntervalMilliseconds) * time.Millisecond
	}

	if len(opts.Name) == 0 {
		opts.Name = DefaultLimiterName
	}

	counterFactory := cluster_counter.NewFactory(&cluster_counter.ClusterCounterFactoryOpts{
		Name:                          opts.Name + ":cls_ct:",
		DefaultLocalTrafficProportion: opts.InitLocalTrafficProportion,
		HeartbeatInterval:             opts.HeartbeatInterval,
	}, dataStore)
	factory := &ClusterLimiterFactory{
		limiterVectors:    sync.Map{},
		counterFactory:    counterFactory,
		heartbeatInterval: opts.HeartbeatInterval,
		name:              opts.Name,
	}
	return factory
}

// create new limiter vector
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
		opts.BurstInterval = DefaultBurstIntervalSeconds * time.Second
	}

	if opts.CompletionTime.Unix() == 0 {
		opts.CompletionTime = opts.EndTime
	}

	var limiterVec = &ClusterLimiterVec{
		name:                     opts.Name,
		beginTime:                opts.BeginTime,
		endTime:                  opts.EndTime,
		completionTime:           opts.CompletionTime,
		periodInterval:           opts.PeriodInterval,
		reserveInterval:          opts.ReserveInterval,
		maxBoostFactor:           opts.MaxBoostFactor,
		burstInterval:            opts.BurstInterval,
		discardPreviousData:      opts.DiscardPreviousData,
		initIdealPassRate:        opts.InitIdealPassRate,
		initRewardRate:           opts.InitRewardRate,
		scoreSamplesSortInterval: opts.ScoreSamplesSortInterval,
		scoreSamplesMax:          opts.ScoreSamplesMax,
	}

	var err error
	limiterVec.RequestCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                       factory.name + opts.Name + ":request",
		PeriodInterval:             opts.PeriodInterval,
		DiscardPreviousData:        opts.DiscardPreviousData,
		StoreDataInterval:          opts.BurstInterval,
		InitLocalTrafficProportion: opts.InitLocalTrafficProportion,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.PassCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                       factory.name + opts.Name + ":pass",
		PeriodInterval:             limiterVec.periodInterval,
		DiscardPreviousData:        opts.DiscardPreviousData,
		StoreDataInterval:          opts.BurstInterval,
		InitLocalTrafficProportion: opts.InitLocalTrafficProportion,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.RewardCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                       factory.name + opts.Name + ":reward",
		PeriodInterval:             limiterVec.periodInterval,
		DiscardPreviousData:        opts.DiscardPreviousData,
		StoreDataInterval:          opts.BurstInterval,
		InitLocalTrafficProportion: opts.InitLocalTrafficProportion,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	factory.limiterVectors.Store(opts.Name, limiterVec)
	return factory.NewClusterLimiterVec(opts, labelNames)
}

// create new limiter
func (factory *ClusterLimiterFactory) NewClusterLimiter(opts *ClusterLimiterOpts,
) (*ClusterLimiter, error) {
	if len(opts.Name) == 0 {
		return nil, errors.New("need name")
	}
	if opts.PeriodInterval.Truncate(time.Second) == 0 &&
		opts.BeginTime.Truncate(time.Second).Before(opts.EndTime.Truncate(time.Second)) == false {
		return nil, errors.New("period interval not set or begin time bigger than end time")
	}

	if l, ok := factory.limiters.Load(opts.Name); ok {
		return l.(*ClusterLimiter), nil
	}

	if opts.CompletionTime.Unix() == 0 {
		opts.CompletionTime = opts.EndTime
	}

	var limiter = &ClusterLimiter{
		name:                     opts.Name,
		RequestCounter:           nil,
		PassCounter:              nil,
		RewardCounter:            nil,
		beginTime:                opts.BeginTime,
		endTime:                  opts.EndTime,
		completionTime:           opts.EndTime,
		periodInterval:           opts.PeriodInterval,
		reserveInterval:          opts.ReserveInterval,
		maxBoostFactor:           opts.MaxBoostFactor,
		discardPreviousData:      opts.DiscardPreviousData,
		idealPassRate:            opts.InitIdealPassRate,
		idealRewardRate:          opts.InitRewardRate,
		scoreSamplesSortInterval: opts.ScoreSamplesSortInterval,
		scoreSamplesMax:          opts.ScoreSamplesMax,
	}

	var err error
	limiter.RequestCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                       factory.name + opts.Name + ":request",
		PeriodInterval:             opts.PeriodInterval,
		DiscardPreviousData:        opts.DiscardPreviousData,
		StoreDataInterval:          opts.BurstInterval,
		InitLocalTrafficProportion: opts.InitLocalTrafficProportion,
	})
	if err != nil {
		return nil, err
	}

	limiter.PassCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                       factory.name + opts.Name + ":pass",
		PeriodInterval:             opts.PeriodInterval,
		DiscardPreviousData:        opts.DiscardPreviousData,
		StoreDataInterval:          opts.BurstInterval,
		InitLocalTrafficProportion: opts.InitLocalTrafficProportion,
	})
	if err != nil {
		return nil, err
	}

	limiter.RewardCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                       factory.name + opts.Name + ":reward",
		PeriodInterval:             opts.PeriodInterval,
		DiscardPreviousData:        opts.DiscardPreviousData,
		StoreDataInterval:          opts.BurstInterval,
		InitLocalTrafficProportion: opts.InitLocalTrafficProportion,
	})
	if err != nil {
		return nil, err
	}

	factory.limiters.Store(opts.Name, limiter)
	return factory.NewClusterLimiter(opts)
}

func (factory *ClusterLimiterFactory) Start() {
	if factory.ticker == nil {
		factory.ticker = time.NewTicker(factory.heartbeatInterval)
		go factory.WatchAndSync()
	}
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

// update
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
