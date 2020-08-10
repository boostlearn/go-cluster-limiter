package cluster_limiter

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"io/ioutil"
	"sync"
	"time"
)

const DefaultLimiterName = "lmt:"
const DefaultHeartbeatIntervalMilliseconds = 100
const DefaultMaxBoostFactor = 2.0
const DefaultBoostBurstFactor = 10.0
const DefaultBurstIntervalSeconds = 5
const DefaultDeclineExpRatio = 0.5
const DefaultRewardRatioDeclineExpRatio = 0.5
const DefaultScoreSamplesSortIntervalSeconds = 10
const DefaultInitPassRate = 0.0
const DefaultInitRewardRate = 1.0
const DefaultUpdatePassRateMinCount = 100
const DefaultUpdateRewardRateMinCount = 100

// options for creating limiter
type ClusterLimiterOpts struct {
	Name         string
	RewardTarget float64

	BeginTime      time.Time
	EndTime        time.Time
	CompletionTime time.Time

	PeriodInterval  time.Duration
	ReserveInterval time.Duration

	BurstInterval  time.Duration
	MaxBoostFactor float64

	DiscardPreviousData bool

	InitLocalTrafficProportion float64
	InitIdealPassRate          float64
	InitRewardRate             float64

	UpdatePassRateMinCount int64
	UpdateRewardRateMinCount int64

	DeclineExpRatio            float64
	RewardRatioDeclineExpRatio float64

	TakeWithScore            bool
	ScoreSamplesSortInterval time.Duration
	ScoreSamplesMax          int64
}

// Producer of limiter
type ClusterLimiterFactory struct {
	name              string
	ticker            *time.Ticker
	heartbeatInterval time.Duration

	limiters       sync.Map
	counterFactory *cluster_counter.ClusterCounterFactory
	Reporter       ReporterI
}

// options of creating limiter's factory
type ClusterLimiterFactoryOpts struct {
	Name                       string
	HeartbeatInterval          time.Duration
	InitLocalTrafficProportion float64
	Store                      cluster_counter.DataStoreI
	Reporter                   ReporterI
}

// build new factory
func NewFactory(opts *ClusterLimiterFactoryOpts) *ClusterLimiterFactory {
	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = time.Duration(DefaultHeartbeatIntervalMilliseconds) * time.Millisecond
	}

	if len(opts.Name) == 0 {
		opts.Name = DefaultLimiterName
	}

	counterFactory := cluster_counter.NewFactory(&cluster_counter.ClusterCounterFactoryOpts{
		Name:              opts.Name + ":cls_ct:",
		HeartbeatInterval: opts.HeartbeatInterval,
		Store:             opts.Store,
	})
	factory := &ClusterLimiterFactory{
		counterFactory:    counterFactory,
		heartbeatInterval: opts.HeartbeatInterval,
		name:              opts.Name,
		Reporter:          opts.Reporter,
	}
	return factory
}

// create new limiter
func (factory *ClusterLimiterFactory) NewClusterLimiter(opts *ClusterLimiterOpts,
) (*ClusterLimiter, error) {
	if len(opts.Name) == 0 {
		return nil, errors.New("name cannot be nil")
	}

	if opts.PeriodInterval.Truncate(time.Second) == 0 &&
		opts.BeginTime.Truncate(time.Second).Before(opts.EndTime.Truncate(time.Second)) == false {
		return nil, errors.New("period interval not set or begin time bigger than end time")
	}

	if opts.CompletionTime.Unix() == 0 {
		opts.CompletionTime = opts.EndTime
	}

	if opts.InitLocalTrafficProportion == 0 {
		opts.InitLocalTrafficProportion = 1.0
	}

	if opts.UpdatePassRateMinCount == 0 {
		opts.UpdatePassRateMinCount = DefaultUpdatePassRateMinCount
	}

	if opts.UpdateRewardRateMinCount == 0 {
		opts.UpdateRewardRateMinCount = DefaultUpdateRewardRateMinCount
	}

	if opts.BurstInterval == 0 {
		opts.BurstInterval = DefaultBurstIntervalSeconds * time.Second
	}

	if opts.MaxBoostFactor == 0 {
		opts.MaxBoostFactor = DefaultMaxBoostFactor
	}

	if opts.TakeWithScore && opts.ScoreSamplesMax == 0 {
		opts.ScoreSamplesMax = 10000
	}
	if opts.TakeWithScore && opts.ScoreSamplesMax > 0 && opts.ScoreSamplesSortInterval == 0 {
		opts.ScoreSamplesSortInterval = DefaultScoreSamplesSortIntervalSeconds * time.Second
	}

	if opts.DeclineExpRatio == 0.0 {
		opts.DeclineExpRatio = DefaultDeclineExpRatio
	}

	if opts.RewardRatioDeclineExpRatio == 0.0 {
		opts.RewardRatioDeclineExpRatio = DefaultRewardRatioDeclineExpRatio
	}

	var limiter = &ClusterLimiter{
		name:                      opts.Name,
		Options:                   opts,
		factory:                   factory,
		rewardTarget:              opts.RewardTarget,
		beginTime:                 opts.BeginTime,
		endTime:                   opts.EndTime,
		completionTime:            opts.CompletionTime,
		periodInterval:            opts.PeriodInterval,
		reserveInterval:           opts.ReserveInterval,
		discardPreviousData:       opts.DiscardPreviousData,
		idealPassRate:             opts.InitIdealPassRate,
		idealRewardRate:           opts.InitRewardRate,
		scoreSamplesSortInterval:  opts.ScoreSamplesSortInterval,
		scoreSamplesMax:           opts.ScoreSamplesMax,
	}

	if opts.PeriodInterval > 0 {
		opts.BeginTime = time.Date(1900, 1, 1, 0, 0, 0, 0, time.Local)
		opts.EndTime = time.Date(3000, 1, 1, 0, 0, 0, 0, time.Local)
	}

	var err error
	limiter.RequestCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                       factory.name + opts.Name + ":request",
		BeginTime:                  opts.BeginTime,
		EndTime:                    opts.EndTime,
		DiscardPreviousData:        opts.DiscardPreviousData,
		StoreDataInterval:          opts.BurstInterval,
		InitLocalTrafficProportion: opts.InitLocalTrafficProportion,
	})
	if err != nil {
		return nil, err
	}

	limiter.PassCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                       factory.name + opts.Name + ":pass",
		BeginTime:                  opts.BeginTime,
		EndTime:                    opts.EndTime,
		DiscardPreviousData:        opts.DiscardPreviousData,
		StoreDataInterval:          opts.BurstInterval,
		InitLocalTrafficProportion: opts.InitLocalTrafficProportion,
	})
	if err != nil {
		return nil, err
	}

	limiter.RewardCounter, err = factory.counterFactory.NewClusterCounter(&cluster_counter.ClusterCounterOpts{
		Name:                       factory.name + opts.Name + ":reward",
		BeginTime:                  opts.BeginTime,
		EndTime:                    opts.EndTime,
		DiscardPreviousData:        opts.DiscardPreviousData,
		StoreDataInterval:          opts.BurstInterval,
		InitLocalTrafficProportion: opts.InitLocalTrafficProportion,
	})
	if err != nil {
		return nil, err
	}
	limiter.Initialize()

	factory.limiters.Store(opts.Name, limiter)
	return factory.GetClusterLimiter(opts.Name), nil
}

// get limiter
func (factory *ClusterLimiterFactory) GetClusterLimiter(name string) *ClusterLimiter {
	if l, ok := factory.limiters.Load(name); ok {
		return l.(*ClusterLimiter)
	}
	return nil
}

func (factory *ClusterLimiterFactory) LoadOptions(options []*ClusterLimiterOpts) error {
	var err error
	for _, opts := range options {
		_, err = factory.NewClusterLimiter(opts)
	}
	return err
}

func (factory *ClusterLimiterFactory) LoadFile(filePath string) error {
	fs, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Println("open file error: ", filePath)
		return err
	}
	var options []*ClusterLimiterOpts
	_ = json.Unmarshal(fs, &options)

	return factory.LoadOptions(options)
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
			limiter.CollectMetrics()

			if limiter.Expire() {
				factory.limiters.Delete(k)
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

func (factory *ClusterLimiterFactory) AllOptions() []*ClusterLimiterOpts {
	var opts []*ClusterLimiterOpts
	factory.limiters.Range(func(k interface{}, v interface{}) bool {
		if limiter, ok := v.(*ClusterLimiter); ok {
			if limiter.Expire() == false {
				opts = append(opts, limiter.Options)
			}
		}
		return true
	})
	return opts
}

func (factory *ClusterLimiterFactory) AllLimiters() []*ClusterLimiter {
	var limiters []*ClusterLimiter
	factory.limiters.Range(func(k interface{}, v interface{}) bool {
		if limiter, ok := v.(*ClusterLimiter); ok {
			if limiter.Expire() == false {
				limiters = append(limiters, limiter)
			}
		}
		return true
	})
	return limiters
}
