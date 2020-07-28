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

	beginTime      time.Time
	endTime        time.Time
	completionTime  time.Time
	periodInterval time.Duration
	reserveInterval time.Duration

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
		beginTime:           limiterVec.beginTime,
		endTime:             limiterVec.endTime,
		reserveInterval:limiterVec.reserveInterval,
		periodInterval:      limiterVec.periodInterval,
		maxBoostFactor:      limiterVec.maxBoostFactor,
		burstInterval:       limiterVec.burstInterval,
		mu:                  sync.RWMutex{},
		discardPreviousData: limiterVec.discardPreviousData,
	}
	newLimiter.Init()
	newLimiter.Heartbeat()

	limiterVec.limiters.Store(key, newLimiter)
	return limiterVec.WithLabelValues(lbs)
}

func (limiterVec *ClusterLimiterVec) Heartbeat() {
	limiterVec.limiters.Range(func(k interface{}, v interface{}) bool {
		if limiter, ok := v.(*ClusterLimiter); ok {
			limiter.Heartbeat()
		}
		return true
	})
}

func (limiterVec *ClusterLimiterVec) Expire() bool {
	limiterVec.mu.RLock()
	defer limiterVec.mu.RUnlock()

	allExpired := true
	limiterVec.limiters.Range(func(k interface{}, v interface{}) bool {
		if counter, ok := v.(*ClusterLimiter); ok {
			if counter.Expire() {
				limiterVec.limiters.Delete(k)
			} else {
				allExpired = false
			}
		}
		return true
	})

	timeNow := time.Now().Truncate(time.Second)
	if limiterVec.periodInterval > 0 {
		if timeNow.After(limiterVec.endTime) {
			limiterVec.beginTime = timeNow.Truncate(limiterVec.periodInterval)
			limiterVec.endTime = limiterVec.beginTime.Add(limiterVec.periodInterval)
			if limiterVec.reserveInterval > 0 && limiterVec.endTime.After(
				limiterVec.beginTime.Add(limiterVec.reserveInterval)) {
				limiterVec.completionTime = limiterVec.endTime.Add(-limiterVec.reserveInterval)
			} else {
				limiterVec.completionTime = limiterVec.endTime
			}
		}
		return false
	}

	return allExpired
}
