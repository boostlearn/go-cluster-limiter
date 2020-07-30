package cluster_counter

import (
	"strings"
	"sync"
	"time"
)

//
type ClusterCounterVec struct {
	mu                       sync.RWMutex
	name                     string
	labelNames               []string
	factory                  *ClusterCounterFactory
	beginTime                time.Time
	endTime                  time.Time
	periodInterval           time.Duration
	storeInterval            time.Duration
	initLocalTrafficRatio float64
	discardPreviousData      bool
	declineExpRatio          float64
	counters                 sync.Map
}

func (counterVec *ClusterCounterVec) WithLabelValues(lbs []string) *ClusterCounter {
	if len(counterVec.labelNames) != len(lbs) {
		return nil
	}

	key := strings.Join(lbs, SEP)
	if v, ok := counterVec.counters.Load(key); ok {
		if limiter, ok2 := v.(*ClusterCounter); ok2 {
			return limiter
		}
	}

	counterLabels := make(map[string]string)
	for i, labelName := range counterVec.labelNames {
		counterLabels[labelName] = lbs[i]
	}

	newCounter := &ClusterCounter{
		name:                counterVec.name,
		lbs:                 counterLabels,
		beginTime:           counterVec.beginTime,
		endTime:             counterVec.endTime,
		periodInterval:      counterVec.periodInterval,
		mu:                  sync.RWMutex{},
		factory:             counterVec.factory,
		storeInterval:       counterVec.storeInterval,
		localTrafficRatio:   counterVec.initLocalTrafficRatio,
		initLocalTrafficRatio: counterVec.initLocalTrafficRatio,
		discardPreviousData: counterVec.discardPreviousData,
		declineExpRatio:     counterVec.declineExpRatio,
	}
	newCounter.Init()

	counterVec.counters.Store(key, newCounter)
	return counterVec.WithLabelValues(lbs)
}

func (counterVec *ClusterCounterVec) Heartbeat() {
	counterVec.counters.Range(func(k interface{}, v interface{}) bool {
		if counter, ok := v.(*ClusterCounter); ok {
			counter.Heartbeat()
		}
		return true
	})
}

func (counterVec *ClusterCounterVec) Expire() bool {
	counterVec.mu.RLock()
	defer counterVec.mu.RUnlock()

	allExpired := true
	counterVec.counters.Range(func(k interface{}, v interface{}) bool {
		if counter, ok := v.(*ClusterCounter); ok {
			if counter.Expire() {
				counterVec.counters.Delete(k)
			} else {
				allExpired = false
			}
		}
		return true
	})

	timeNow := time.Now().Truncate(time.Second)
	if counterVec.periodInterval > 0 {
		if timeNow.After(counterVec.endTime) {
			counterVec.beginTime = timeNow.Truncate(counterVec.periodInterval)
			counterVec.endTime = counterVec.beginTime.Add(counterVec.periodInterval)
		}
		return false
	}

	return allExpired
}
