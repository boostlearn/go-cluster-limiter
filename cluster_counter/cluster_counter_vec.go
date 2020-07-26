package cluster_counter

import (
	"strings"
	"sync"
	"time"
)

//
type ClusterCounterVec struct {
	mu sync.RWMutex

	// 名称
	name string

	// 标签
	labelNames []string

	// factory
	factory *ClusterCounterFactory

	beginTime time.Time
	endTime   time.Time
	periodInterval time.Duration

	// 更新周期
	loadDataInterval time.Duration

	// 更新周期
	storeDataInterval time.Duration

	// 集群内机器数目
	defaultLocalTrafficRatio float64

	discardPreviousData bool

	counters sync.Map
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
		beginTime: counterVec.beginTime,
		endTime: counterVec.endTime,
		periodInterval: counterVec.periodInterval,
		mu:                  sync.RWMutex{},
		factory:             counterVec.factory,
		loadDataInterval:    counterVec.loadDataInterval,
		storeDataInterval:   counterVec.storeDataInterval,
		localTrafficRatio:   counterVec.defaultLocalTrafficRatio,
		discardPreviousData: counterVec.discardPreviousData,
	}
	newCounter.Init()

	counterVec.counters.Store(key, newCounter)
	return counterVec.WithLabelValues(lbs)
}

func (counterVec *ClusterCounterVec) HeartBeat() {
	counterVec.counters.Range(func(k interface{}, v interface{}) bool {
		if counter, ok := v.(*ClusterCounter); ok {
			counter.HeartBeat()
		}
		return true
	})
}

func (counterVec *ClusterCounterVec)Expire() bool {
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