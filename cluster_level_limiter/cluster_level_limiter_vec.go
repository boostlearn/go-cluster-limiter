package cluster_level_limiter

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"strings"
	"sync"
	"time"
)

const KEYSEP = "####"

type ClusterLevelLimiterVec struct {
	Name string

	HighRequestCounter cluster_counter.ClusterCounterVecI
	HighPassCounter    cluster_counter.ClusterCounterVecI
	HighRewardCounter  cluster_counter.ClusterCounterVecI

	MiddleRequestCounter cluster_counter.ClusterCounterVecI
	MiddlePassCounter    cluster_counter.ClusterCounterVecI
	MiddleRewardCounter  cluster_counter.ClusterCounterVecI

	LowRequestCounter cluster_counter.ClusterCounterVecI
	LowPassCounter    cluster_counter.ClusterCounterVecI
	LowRewardCounter  cluster_counter.ClusterCounterVecI

	StartTime     time.Time
	EndTime       time.Time
	ResetInterval time.Duration

	BoostInterval  time.Duration
	MaxBoostFactor float64

	UpdateInterval time.Duration

	LevelSampleMax int64

	mu       sync.RWMutex
	limiters sync.Map
}

func (limiterVec *ClusterLevelLimiterVec) WithLabelValues(lbs []string) *ClusterLevelLimiter {
	key := strings.Join(lbs, KEYSEP)
	if v, ok := limiterVec.limiters.Load(key); ok {
		if limiter, ok2 := v.(*ClusterLevelLimiter); ok2 {
			return limiter
		}
	}

	newLimiter := &ClusterLevelLimiter{
		Name:                 limiterVec.Name,
		lbs:                  append([]string{}, lbs...),
		totalTarget:          0,
		curPassRate:          0,
		idealPassRate:        0,
		HighRequestCounter:   limiterVec.HighRequestCounter.WithLabelValues(lbs),
		HighPassCounter:      limiterVec.HighPassCounter.WithLabelValues(lbs),
		HighRewardCounter:    limiterVec.HighRewardCounter.WithLabelValues(lbs),
		MiddleRequestCounter: limiterVec.MiddleRequestCounter.WithLabelValues(lbs),
		MiddlePassCounter:    limiterVec.MiddlePassCounter.WithLabelValues(lbs),
		MiddleRewardCounter:  limiterVec.MiddleRewardCounter.WithLabelValues(lbs),
		LowRequestCounter:    limiterVec.LowRequestCounter.WithLabelValues(lbs),
		LowPassCounter:       limiterVec.LowPassCounter.WithLabelValues(lbs),
		LowRewardCounter:     limiterVec.LowRewardCounter.WithLabelValues(lbs),
		StartTime:            limiterVec.StartTime,
		EndTime:              limiterVec.EndTime,
		ResetInterval:        limiterVec.ResetInterval,
		BoostInterval:        limiterVec.BoostInterval,
		MaxBoostFactor:       limiterVec.MaxBoostFactor,
		UpdateInterval:       limiterVec.UpdateInterval,
		mu:                   sync.RWMutex{},
		prevPacingTarget:     0,
		prevUpdateTime:       time.Time{},
		LevelSampleMax:       limiterVec.LevelSampleMax,
		levelSamples:         make([]float64, limiterVec.LevelSampleMax),
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
