package cluster_limiter

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"strings"
	"sync"
	"time"
)

const SEP = "####"

type ClusterLimiterVec struct {
	Name string

	DiscardPreviousData bool

	RequestCounter cluster_counter.ClusterCounterVecI
	PassCounter    cluster_counter.ClusterCounterVecI
	RewardCounter  cluster_counter.ClusterCounterVecI

	StartTime     time.Time
	EndTime       time.Time
	ResetInterval time.Duration

	BoostInterval  time.Duration
	MaxBoostFactor float64

	UpdateInterval time.Duration

	mu       sync.RWMutex
	limiters sync.Map
}

func (limiterVec *ClusterLimiterVec) WithLabelValues(lbs []string) *ClusterLimiter {
	key := strings.Join(lbs, SEP)
	if v, ok := limiterVec.limiters.Load(key); ok {
		if limiter, ok2 := v.(*ClusterLimiter); ok2 {
			return limiter
		}
	}

	newLimiter := &ClusterLimiter{
		Name:                limiterVec.Name,
		lbs:                 append([]string{}, lbs...),
		totalTarget:         0,
		curPassRate:         0,
		idealPassRate:       0,
		RequestCounter:      limiterVec.RequestCounter.WithLabelValues(lbs),
		PassCounter:         limiterVec.PassCounter.WithLabelValues(lbs),
		RewardCounter:       limiterVec.RewardCounter.WithLabelValues(lbs),
		StartTime:           limiterVec.StartTime,
		EndTime:             limiterVec.EndTime,
		ResetInterval:       limiterVec.ResetInterval,
		BoostInterval:       limiterVec.BoostInterval,
		MaxBoostFactor:      limiterVec.MaxBoostFactor,
		UpdateInterval:      limiterVec.UpdateInterval,
		mu:                  sync.RWMutex{},
		prevRequest:         0,
		prevPass:            0,
		prevReward:          0,
		prevPacingTarget:    0,
		prevUpdateTime:      time.Time{},
		DiscardPreviousData: limiterVec.DiscardPreviousData,
	}
	newLimiter.Init()
	newLimiter.Update()

	limiterVec.limiters.Store(key, newLimiter)
	return limiterVec.WithLabelValues(lbs)
}

func (limiterVec *ClusterLimiterVec) Update() {
	limiterVec.limiters.Range(func(k interface{}, v interface{}) bool {
		if limiter, ok := v.(*ClusterLimiter); ok {
			limiter.Update()
		}
		return true
	})
}
