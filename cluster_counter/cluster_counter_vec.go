package cluster_counter

import (
	"strings"
	"sync"
	"time"
)

type ClusterCounterVecI interface {
	WithLabelValues(lbs []string) ClusterCounterI
}

// 集群环境计数器对象
type ClusterCounterVec struct {
	mu sync.RWMutex

	// 名称
	name string

	// 标签
	labelNames []string

	// factory
	Factory ClusterCounterFactoryI

	// 重置周期
	resetInterval time.Duration

	// 更新周期
	loadDataInterval time.Duration

	// 更新周期
	storeDataInterval time.Duration

	// 集群内机器数目
	defaultLocalTrafficRatio float64

	discardPreviousData bool

	counters sync.Map
}

func (counterVec *ClusterCounterVec) WithLabelValues(lbs []string) ClusterCounterI {
	key := strings.Join(lbs, SEP)
	if v, ok := counterVec.counters.Load(key); ok {
		if limiter, ok2 := v.(*ClusterCounter); ok2 {
			return limiter
		}
	}

	newCounter := &ClusterCounter{
		name:              counterVec.name,
		lbs:               append([]string{}, lbs...),
		mu:                sync.RWMutex{},
		Factory:           counterVec.Factory,
		resetInterval:     counterVec.resetInterval,
		loadDataInterval:  counterVec.loadDataInterval,
		storeDataInterval: counterVec.storeDataInterval,
		localTrafficRatio: counterVec.defaultLocalTrafficRatio,
		discardPreviousData:counterVec.discardPreviousData,
	}
	newCounter.init()

	counterVec.counters.Store(key, newCounter)
	return counterVec.WithLabelValues(lbs)
}

func (counterVec *ClusterCounterVec) Update() {
	counterVec.counters.Range(func(k interface{}, v interface{}) bool {
		if counter, ok := v.(*ClusterCounter); ok {
			counter.Update()
		}
		return true
	})
}
