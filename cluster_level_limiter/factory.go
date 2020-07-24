package cluster_level_limiter

import (
	"errors"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"sync"
	"time"
)

const LimiterPrefix = "lmt:"

type ClusterLevelLimiterFactory struct {
	status bool

	limiterVectors sync.Map
	limiters       sync.Map

	counterFactory *cluster_counter.ClusterCounterFactory

	defaultBoostInterval  time.Duration
	defaultUpdateInterval time.Duration

	defaultMaxBoostFactor float64

	updateRatioInterval time.Duration

	limiterKeyPrefix string

	defaultClusterLocalTrafficRatio float64
}

// 构建参数
type ClusterLevelLimiterFactoryOpts struct {
	// 加速观察周期
	DefaultBoostInterval  time.Duration
	DefaultUpdateInterval time.Duration

	DefaultMaxBoostFactor float64

	// KEY前缀
	KeyPrefix string

	// 集群内成员数目
	DefaultLocalTrafficRate float64
}

type ClusterLevelLimiterOpts struct {
	Name          string
	StartTime     time.Time
	EndTime       time.Time
	ResetInterval time.Duration

	BoostInterval time.Duration

	UpdateInterval time.Duration

	MaxBoostFactor float64

	DiscardPreviousData bool
}

func NewFactory(opts *ClusterLevelLimiterFactoryOpts,
	counterFactory *cluster_counter.ClusterCounterFactory,
) *ClusterLevelLimiterFactory {

	if opts.DefaultBoostInterval == 0 {
		opts.DefaultBoostInterval = time.Duration(60) * time.Second
	}

	if opts.DefaultUpdateInterval == 0 {
		opts.DefaultUpdateInterval = time.Duration(5) * time.Second
	}

	if len(opts.KeyPrefix) == 0 {
		opts.KeyPrefix = LimiterPrefix
	}

	factory := &ClusterLevelLimiterFactory{
		limiterVectors:        sync.Map{},
		counterFactory:        counterFactory,
		defaultBoostInterval:  opts.DefaultBoostInterval,
		defaultUpdateInterval: opts.DefaultUpdateInterval,
		limiterKeyPrefix:      opts.KeyPrefix,
		updateRatioInterval:   opts.DefaultUpdateInterval,
	}
	factory.Start()
	return factory
}

func (factory *ClusterLevelLimiterFactory) NewClusterLevelLimiterVec(opts *ClusterLevelLimiterOpts,
	labelNames []string,
) (*ClusterLevelLimiterVec, error) {
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
		return l.(*ClusterLevelLimiterVec), nil
	}

	if opts.UpdateInterval == 0 {
		opts.UpdateInterval = factory.defaultUpdateInterval
	}

	if opts.BoostInterval == 0 {
		opts.BoostInterval = factory.defaultBoostInterval
	}

	var limiterVec = &ClusterLevelLimiterVec{
		name:                opts.Name,
		startTime:           opts.StartTime,
		endTime:             opts.EndTime,
		resetDataInterval:   opts.ResetInterval,
		boostInterval:       opts.BoostInterval,
		maxBoostFactor:      opts.MaxBoostFactor,
		silentInterval:      opts.UpdateInterval,
		levelSampleMax:      10000,
		discardPreviousData: opts.DiscardPreviousData,
	}

	var err error
	limiterVec.HighRequestCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":high_request",
		ResetInterval:       opts.ResetInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.HighPassCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":high_pass",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.HighRewardCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":high_reward",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.MiddleRequestCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":middle_request",
		ResetInterval:       opts.ResetInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.MiddlePassCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":middle_pass",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.MiddleRewardCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":middle_reward",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.LowRequestCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":low_request",
		ResetInterval:       opts.ResetInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.LowPassCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":low_pass",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	limiterVec.LowRewardCounter, err = factory.counterFactory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":low_reward",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	}, labelNames)
	if err != nil {
		return nil, err
	}

	factory.limiterVectors.Store(opts.Name, limiterVec)
	return factory.NewClusterLevelLimiterVec(opts, labelNames)
}

func (factory *ClusterLevelLimiterFactory) NewClusterLevelLimiter(opts *ClusterLevelLimiterOpts,
) (*ClusterLevelLimiter, error) {
	if len(opts.Name) == 0 {
		return nil, errors.New("need name")
	}
	if opts.StartTime.UnixNano() >= opts.EndTime.UnixNano() || opts.ResetInterval.Seconds() < 5 {
		return nil, errors.New("time error")
	}

	if l, ok := factory.limiters.Load(opts.Name); ok {
		return l.(*ClusterLevelLimiter), nil
	}

	if opts.UpdateInterval == 0 {
		opts.UpdateInterval = factory.defaultUpdateInterval
	}

	if opts.BoostInterval == 0 {
		opts.BoostInterval = factory.defaultBoostInterval
	}

	var limiterVec = &ClusterLevelLimiter{
		name:                opts.Name,
		startTime:           opts.StartTime,
		endTime:             opts.EndTime,
		resetDataInterval:   opts.ResetInterval,
		boostInterval:       opts.BoostInterval,
		maxBoostFactor:      opts.MaxBoostFactor,
		silentInterval:      opts.UpdateInterval,
		levelSampleMax:      10000,
		levelSamples:        make([]float64, 10000),
		discardPreviousData: opts.DiscardPreviousData,
	}

	var err error
	limiterVec.HighRequestCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":high_request",
		ResetInterval:       opts.ResetInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.HighPassCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":high_pass",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.HighRewardCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":high_reward",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.MiddleRequestCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":middle_request",
		ResetInterval:       opts.ResetInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.MiddlePassCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":middle_pass",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.MiddleRewardCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":middle_reward",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}
	limiterVec.LowRequestCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":low_request",
		ResetInterval:       opts.ResetInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.LowPassCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":low_pass",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	limiterVec.LowRewardCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                factory.limiterKeyPrefix + opts.Name + ":low_reward",
		ResetInterval:       limiterVec.resetDataInterval,
		DiscardPreviousData: opts.DiscardPreviousData,
	})
	if err != nil {
		return nil, err
	}

	factory.limiterVectors.Store(opts.Name, limiterVec)
	return factory.NewClusterLevelLimiter(opts)
}

func (factory *ClusterLevelLimiterFactory) Delete(name string) {
	factory.counterFactory.Delete(LimiterPrefix + name + ":request")
	factory.counterFactory.Delete(LimiterPrefix + name + ":pass")
	factory.counterFactory.Delete(LimiterPrefix + name + ":reward")
}

func (factory *ClusterLevelLimiterFactory) Start() {
	factory.status = true
	go factory.WatchAndSync()
}

func (factory *ClusterLevelLimiterFactory) Stop() {
	factory.status = false
}

func (factory *ClusterLevelLimiterFactory) WatchAndSync() {
	for factory.status {
		factory.limiters.Range(func(k interface{}, v interface{}) bool {
			if limiter, ok := v.(*ClusterLevelLimiter); ok {
				limiter.Update()
			}
			return true
		})

		factory.limiterVectors.Range(func(k interface{}, v interface{}) bool {
			if limiter, ok := v.(*ClusterLevelLimiterVec); ok {
				limiter.Update()
			}
			return true
		})
		time.Sleep(factory.updateRatioInterval)
	}
}
