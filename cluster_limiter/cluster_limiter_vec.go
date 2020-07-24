package cluster_limiter

import (
	"github.com/boostlearn/go-cluster-counter/cluster_counter"
	"strings"
	"sync"
	"time"
)

const SEP = "####"

type ClusterLimiterVec struct {
	name string

	discardPreviousData bool

	RequestCounter *cluster_counter.ClusterCounterVec
	PassCounter    *cluster_counter.ClusterCounterVec
	RewardCounter  *cluster_counter.ClusterCounterVec

	startTime time.Time
	endTime   time.Time

	resetDataInterval time.Duration

	boostInterval  time.Duration
	maxBoostFactor float64

	silentInterval time.Duration

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
		name:                limiterVec.name,
		lbs:                 append([]string{}, lbs...),
		totalTarget:         0,
		curPassRate:         0,
		idealPassRate:       0,
		RequestCounter:      limiterVec.RequestCounter.WithLabelValues(lbs),
		PassCounter:         limiterVec.PassCounter.WithLabelValues(lbs),
		RewardCounter:       limiterVec.RewardCounter.WithLabelValues(lbs),
		startTime:           limiterVec.startTime,
		endTime:             limiterVec.endTime,
		resetDataInterval:   limiterVec.resetDataInterval,
		boostInterval:       limiterVec.boostInterval,
		maxBoostFactor:      limiterVec.maxBoostFactor,
		silentInterval:      limiterVec.silentInterval,
		mu:                  sync.RWMutex{},
		prevRequest:         0,
		prevPass:            0,
		prevReward:          0,
		prevPacingTarget:    0,
		prevUpdateTime:      time.Time{},
		discardPreviousData: limiterVec.discardPreviousData,
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
