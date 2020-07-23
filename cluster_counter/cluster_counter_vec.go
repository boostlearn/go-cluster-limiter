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
	Name string

	// 标签
	LabelNames []string

	// factory
	Factory ClusterCounterFactoryI

	// 重置周期
	ResetInterval time.Duration

	// 更新周期
	PullInterval time.Duration

	// 更新周期
	PushInterval time.Duration

	// 集群内机器数目
	DefaultClusterAmpFactor float64

	measureName string

	counters sync.Map
}

func (counterVec *ClusterCounterVec) WithLabelValues(lbs []string) ClusterCounterI {
	key := strings.Join(lbs, KEY_SEP)
	if v, ok := counterVec.counters.Load(key); ok {
		if limiter, ok2 := v.(*ClusterCounter); ok2 {
			return limiter
		}
	}

	newCounter := &ClusterCounter{
		Name:                    counterVec.Name,
		lbs:                     append([]string{}, lbs...),
		mu:                      sync.RWMutex{},
		Factory:                 counterVec.Factory,
		ResetInterval:           counterVec.ResetInterval,
		PullInterval:            counterVec.PullInterval,
		PushInterval:            counterVec.PushInterval,
		DefaultClusterAmpFactor: counterVec.DefaultClusterAmpFactor,
		measureName:             counterVec.measureName,
	}
	newCounter.Update()
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
