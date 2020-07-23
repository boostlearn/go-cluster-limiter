// 集群环境下计数器
package cluster_counter

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const KEY_SEP = "####"
const SPACE_SEP = "::"

const MAX_EXPAND_RATIO = 10000

// 机器计数器接口
type ClusterCounterI interface {
	// 增加
	Add(value int64)

	// 本地当前计数器
	LocalCurrent() int64

	// 集群最近一次计数
	ClusterLast() (int64, time.Time)

	// 集群预计当前值
	ClusterPredict() int64

	// 集群放大系数
	ClusterAmpFactor() float64
}

type ClusterCounterOpts struct {
	Name                    string
	ResetInterval           time.Duration
	PullInterval            time.Duration
	PushInterval            time.Duration
	DefaultClusterAmpFactor float64
}

// 集群计数器
type ClusterCounter struct {
	mu sync.RWMutex

	// 名称
	Name        string
	measureName string

	// 生产者
	Factory ClusterCounterFactoryI

	// 重置周期
	ResetInterval time.Duration

	// 更新周期
	PullInterval time.Duration

	// 更新周期
	PushInterval time.Duration

	// 标签
	lbs []string

	// 集群内机器数目
	DefaultClusterAmpFactor float64

	// 本地数据
	localValue int64

	lastPushTime time.Time
	localPushed  int64
	localLast    int64
	localPrev    int64

	// 集群全局数据
	lastPullTime time.Time
	clusterLast  int64
	clusterPrev  int64

	clusterAmpFactor float64

	expireTime time.Time
}

// 计数器增加
func (counter *ClusterCounter) Add(v int64) {
	atomic.AddInt64(&counter.localValue, v)
}

// 本地当前值
func (counter *ClusterCounter) LocalCurrent() int64 {
	return atomic.LoadInt64(&counter.localValue)
}

// 集群最后更新值
func (counter *ClusterCounter) ClusterLast() (int64, time.Time) {
	return atomic.LoadInt64(&counter.clusterLast), counter.lastPullTime
}

// 集群预测值
func (counter *ClusterCounter) ClusterPredict() int64 {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	clusterLast := atomic.LoadInt64(&counter.clusterLast)
	localValue := atomic.LoadInt64(&counter.localValue)
	localLast := atomic.LoadInt64(&counter.localLast)

	clusterAmpFactor := counter.clusterAmpFactor
	if clusterAmpFactor == 0.0 {
		clusterAmpFactor = counter.DefaultClusterAmpFactor
	}

	return clusterLast + int64(float64(localValue-localLast)*clusterAmpFactor)

}

// 集群放大系数
func (counter *ClusterCounter) ClusterAmpFactor() float64 {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if counter.clusterAmpFactor == 0.0 {
		return counter.DefaultClusterAmpFactor
	}

	return counter.clusterAmpFactor
}

// 周期更新
func (counter *ClusterCounter) Update() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now()
	if counter.expireTime.Unix() < timeNow.Unix() && counter.ResetInterval.Seconds() > 0 {
		var notPushValue = atomic.SwapInt64(&counter.localValue, 0) - atomic.SwapInt64(&counter.localPushed, 0)
		var measureName = counter.measureName

		atomic.StoreInt64(&counter.localLast, 0)
		atomic.StoreInt64(&counter.localPrev, 0)
		atomic.StoreInt64(&counter.clusterLast, 0)
		atomic.StoreInt64(&counter.clusterPrev, 0)

		nowTs := timeNow.Unix()
		interval := int64(counter.ResetInterval.Seconds())
		startTs := interval * (nowTs / interval)
		endTs := startTs + interval
		intervalKey := fmt.Sprintf("%v%v_%v", SPACE_SEP, startTs, interval)
		counter.measureName = counter.Name + intervalKey
		counter.expireTime = time.Now().Add(time.Duration(endTs-nowTs+1) * time.Second)

		if counter.clusterAmpFactor == 0.0 {
			counter.clusterAmpFactor = counter.DefaultClusterAmpFactor
		}
		if counter.clusterAmpFactor < 1.0 {
			counter.clusterAmpFactor = 1.0
		}

		if notPushValue > 0 && len(measureName) > 0 {
			counter.mu.Unlock()
			err := counter.Factory.PushData(counter.Name, measureName, strings.Join(counter.lbs, KEY_SEP), notPushValue)
			counter.mu.Lock()

			if err != nil {
				counter.lastPushTime = time.Now()
			}
		}
		return
	}

	if timeNow.UnixNano()-counter.lastPullTime.UnixNano() > counter.PullInterval.Nanoseconds() {
		counter.mu.Unlock()
		value, err := counter.Factory.PullData(counter.Name, counter.measureName,
			strings.Join(counter.lbs, KEY_SEP))
		counter.mu.Lock()

		if err != nil {
			counter.lastPullTime = timeNow
			return
		}
		atomic.StoreInt64(&counter.localPrev, atomic.LoadInt64(&counter.localLast))
		atomic.StoreInt64(&counter.localLast, atomic.LoadInt64(&counter.localValue))

		lastValue := atomic.LoadInt64(&counter.clusterLast)
		atomic.StoreInt64(&counter.clusterPrev, atomic.LoadInt64(&counter.clusterLast))
		atomic.StoreInt64(&counter.clusterLast, value)
		if value > lastValue {
			counter.updateClusterAmpFactor()
		}

	}

	if timeNow.UnixNano()-counter.lastPullTime.Add(1*time.Second).UnixNano() > counter.PushInterval.Nanoseconds() {
		pushValue := atomic.LoadInt64(&counter.localValue) - atomic.LoadInt64(&counter.localPushed)
		if pushValue > 0 {
			counter.mu.Unlock()
			err := counter.Factory.PushData(counter.Name, counter.measureName, strings.Join(counter.lbs, KEY_SEP),
				pushValue)
			counter.mu.Lock()

			if err == nil {
				counter.lastPushTime = time.Now()
				atomic.AddInt64(&counter.localPushed, pushValue)
			}
			counter.lastPushTime = time.Now()
		}
	}
}

// 重新计算放大系数
func (counter *ClusterCounter) updateClusterAmpFactor() {
	clusterPrev := atomic.LoadInt64(&counter.clusterPrev)
	localPrev := atomic.LoadInt64(&counter.localPrev)
	localLast := atomic.LoadInt64(&counter.localLast)
	clusterLast := atomic.LoadInt64(&counter.clusterLast)

	if clusterPrev == 0 || localPrev == 0 || clusterPrev >= clusterLast ||
		localPrev >= localLast {
		if counter.clusterAmpFactor == 0.0 {
			counter.clusterAmpFactor = counter.DefaultClusterAmpFactor
		}
		if counter.clusterAmpFactor < 1.0 {
			counter.clusterAmpFactor = 1.0
		}
	} else {
		var ratio = float64(clusterLast-clusterPrev) / float64(localLast-localPrev)
		if ratio < 1.0 {
			ratio = 1.0
		}
		if ratio < MAX_EXPAND_RATIO {
			counter.clusterAmpFactor = counter.clusterAmpFactor*0.5 + ratio*0.5
		}
	}
}
