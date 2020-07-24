package cluster_level_limiter

import (
	"github.com/boostlearn/go-cluster-counter/cluster_counter"
	"strings"
	"sync"
	"time"
)

const SEP = "####"

type ClusterLevelLimiterVec struct {
	name string

	discardPreviousData bool

	HighRequestCounter *cluster_counter.ClusterCounterVec
	HighPassCounter    *cluster_counter.ClusterCounterVec
	HighRewardCounter  *cluster_counter.ClusterCounterVec

	MiddleRequestCounter *cluster_counter.ClusterCounterVec
	MiddlePassCounter    *cluster_counter.ClusterCounterVec
	MiddleRewardCounter  *cluster_counter.ClusterCounterVec

	LowRequestCounter *cluster_counter.ClusterCounterVec
	LowPassCounter    *cluster_counter.ClusterCounterVec
	LowRewardCounter  *cluster_counter.ClusterCounterVec

	startTime         time.Time
	endTime           time.Time
	resetDataInterval time.Duration

	boostInterval  time.Duration
	maxBoostFactor float64

	silentInterval time.Duration

	levelSampleMax int64

	mu       sync.RWMutex
	limiters sync.Map
}

func (limiterVec *ClusterLevelLimiterVec) WithLabelValues(lbs []string) *ClusterLevelLimiter {
	key := strings.Join(lbs, SEP)
	if v, ok := limiterVec.limiters.Load(key); ok {
		if limiter, ok2 := v.(*ClusterLevelLimiter); ok2 {
			return limiter
		}
	}

	newLimiter := &ClusterLevelLimiter{
		name:                 limiterVec.name,
		lbs:                  append([]string{}, lbs...),
		HighRequestCounter:   limiterVec.HighRequestCounter.WithLabelValues(lbs),
		HighPassCounter:      limiterVec.HighPassCounter.WithLabelValues(lbs),
		HighRewardCounter:    limiterVec.HighRewardCounter.WithLabelValues(lbs),
		MiddleRequestCounter: limiterVec.MiddleRequestCounter.WithLabelValues(lbs),
		MiddlePassCounter:    limiterVec.MiddlePassCounter.WithLabelValues(lbs),
		MiddleRewardCounter:  limiterVec.MiddleRewardCounter.WithLabelValues(lbs),
		LowRequestCounter:    limiterVec.LowRequestCounter.WithLabelValues(lbs),
		LowPassCounter:       limiterVec.LowPassCounter.WithLabelValues(lbs),
		LowRewardCounter:     limiterVec.LowRewardCounter.WithLabelValues(lbs),
		startTime:            limiterVec.startTime,
		endTime:              limiterVec.endTime,
		resetDataInterval:    limiterVec.resetDataInterval,
		boostInterval:        limiterVec.boostInterval,
		maxBoostFactor:       limiterVec.maxBoostFactor,
		silentInterval:       limiterVec.silentInterval,
		mu:                   sync.RWMutex{},
		prevPacingTarget:     0,
		prevUpdateTime:       time.Time{},
		levelSampleMax:       limiterVec.levelSampleMax,
		levelSamples:         make([]float64, limiterVec.levelSampleMax),
		discardPreviousData:  limiterVec.discardPreviousData,
	}
	newLimiter.Init()
	newLimiter.Update()

	limiterVec.limiters.Store(key, newLimiter)
	return limiterVec.WithLabelValues(lbs)
}

func (limiterVec *ClusterLevelLimiterVec) Update() {
	limiterVec.limiters.Range(func(k interface{}, v interface{}) bool {
		if limiter, ok := v.(*ClusterLevelLimiter); ok {
			limiter.Update()
		}
		return true
	})
}
