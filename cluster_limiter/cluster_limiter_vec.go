package cluster_limiter

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
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

	burstInterval time.Duration

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
		RequestCounter:      limiterVec.RequestCounter.WithLabelValues(lbs),
		PassCounter:         limiterVec.PassCounter.WithLabelValues(lbs),
		RewardCounter:       limiterVec.RewardCounter.WithLabelValues(lbs),
		startTime:           limiterVec.startTime,
		endTime:             limiterVec.endTime,
		boostInterval:       limiterVec.boostInterval,
		maxBoostFactor:      limiterVec.maxBoostFactor,
		silentInterval:      limiterVec.silentInterval,
		burstInterval:       limiterVec.burstInterval,
		mu:                  sync.RWMutex{},
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
