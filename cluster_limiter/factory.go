package cluster_limiter

import (
	"errors"
	"github.com/boostlearn/go-cluster-counter/cluster_counter"
	"sync"
	"time"
)

const LimiterPrefix = "lmt:"

type ClusterLimiterFactory struct {
	status bool

	limiterVectors sync.Map
	limiters       sync.Map

	counterFactory *cluster_counter.ClusterCounterFactory

	defaultBoostInterval  time.Duration
	updateInterval time.Duration

	defaultMaxBoostFactor float64

	limiterKeyPrefix string

	defaultClusterLocalTrafficRatio float64
}

// 构建参数
type ClusterLimiterFactoryOpts struct {
	// 加速观察周期
	DefaultBoostInterval  time.Duration
	DefaultUpdateInterval time.Duration

	DefaultMaxBoostFactor float64

	// KEY前缀
	KeyPrefix string

	// 集群内成员数目
	DefaultLocalTrafficRate float64
}

type ClusterLimiterOpts struct {
	Name          string
	StartTime     time.Time
	EndTime       time.Time
	ResetInterval time.Duration

	BoostInterval time.Duration

	SilentInterval time.Duration

	BurstInterval time.Duration

	MaxBoostFactor float64

	DiscardPreviousData bool
}

func NewFactory(opts *ClusterLimiterFactoryOpts,
	counterFactory *cluster_counter.ClusterCounterFactory,
) *ClusterLimiterFactory {

	if opts.DefaultBoostInterval == 0 {
		opts.DefaultBoostInterval = time.Duration(60) * time.Second
	}

	if opts.DefaultUpdateInterval == 0 {
		opts.DefaultUpdateInterval = time.Duration(5) * time.Second
	}

	if len(opts.KeyPrefix) == 0 {
		opts.KeyPrefix = LimiterPrefix
	}

	factory := &ClusterLimiterFactory{
		limiterVectors:        sync.Map{},
		counterFactory:        counterFactory,
		defaultBoostInterval:  opts.DefaultBoostInterval,
		updateInterval: opts.DefaultUpdateInterval,
		limiterKeyPrefix:      opts.KeyPrefix,
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
	if opts.StartTime.UnixNano() >= opts.EndTime.UnixNano() || opts.ResetInterval.Seconds() < 5 {
		return nil, errors.New("time error")
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
		RequestCounter:      nil,
		PassCounter:         nil,
		RewardCounter:       nil,
		startTime:           opts.StartTime,
		endTime:             opts.EndTime,
		resetDataInterval:   opts.ResetInterval,
		boostInterval:       opts.BoostInterval,
		maxBoostFactor:      opts.MaxBoostFactor,
		silentInterval:      opts.SilentInterval,
		burstInterval: opts.BurstInterval,
		discardPreviousData: opts.DiscardPreviousData,
	}

	var err error
	limiterVec.RequestCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":request",
		ResetInterval:       opts.ResetInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.PassCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":pass",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.RewardCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":reward",
		ResetInterval:       limiterVec.resetDataInterval,
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
	if opts.StartTime.UnixNano() >= opts.EndTime.UnixNano() || opts.ResetInterval.Seconds() < 5 {
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
		startTime:           opts.StartTime,
		endTime:             opts.EndTime,
		resetDataInterval:   opts.ResetInterval,
		boostInterval:       opts.BoostInterval,
		maxBoostFactor:      opts.MaxBoostFactor,
		silentInterval:      opts.SilentInterval,
		discardPreviousData: opts.DiscardPreviousData,
	}

	var err error
	limiterVec.RequestCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":request",
		ResetInterval:       opts.ResetInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.PassCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":pass",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.RewardCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":reward",
		ResetInterval:       limiterVec.resetDataInterval,
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
				limiter.Update()
			}
			return true
		})

		factory.limiterVectors.Range(func(k interface{}, v interface{}) bool {
			if limiter, ok := v.(*ClusterLimiterVec); ok {
				limiter.Update()
			}
			return true
		})
		time.Sleep(factory.updateInterval)
	}
}
