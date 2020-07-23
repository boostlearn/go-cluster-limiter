// 集群环境下计数器
package cluster_counter

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const KEYSEP = "####"

//
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
	LocalTrafficRatio() float64
}

type ClusterCounterOpts struct {
	Name             string
	ResetInterval    time.Duration
	LoadDataInterval time.Duration

	StoreDataInterval        time.Duration
	DefaultLocalTrafficRatio float64
}

// 集群计数器
type ClusterCounter struct {
	mu sync.RWMutex

	// 名称
	name      string
	cycleName string
	// 标签
	lbs []string

	// 生产者
	Factory ClusterCounterFactoryI

	// 重置周期
	resetInterval time.Duration

	// 更新周期
	loadDataInterval time.Duration

	// 更新周期
	storeDataInterval time.Duration

	defaultTrafficRatio float64

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

	localTrafficRatio float64

	expireTime time.Time

	initTime time.Time
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

	localTrafficRatio := counter.localTrafficRatio
	if localTrafficRatio == 0.0 {
		counter.localTrafficRatio = counter.defaultTrafficRatio
	}

	return clusterLast + int64(float64(localValue-localLast)/localTrafficRatio)

}

// 集群放大系数
func (counter *ClusterCounter) LocalTrafficRatio() float64 {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if counter.localTrafficRatio == 0.0 {
		return counter.defaultTrafficRatio
	}

	return counter.localTrafficRatio
}

func (counter *ClusterCounter) init() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now()
	counter.initTime = timeNow

	if counter.resetInterval.Seconds() > 0 {
		nowTs := timeNow.Unix()
		interval := int64(counter.resetInterval.Seconds())
		startTs := interval * (nowTs / interval)
		endTs := startTs + interval
		intervalKey := fmt.Sprintf("%v%v_%v", KEYSEP, startTs, interval)
		counter.cycleName = intervalKey
		counter.expireTime = time.Now().Add(time.Duration(endTs-nowTs+1) * time.Second)
	}

	if counter.defaultTrafficRatio < 1.0 {
		counter.defaultTrafficRatio = 1.0
	}

	if counter.localTrafficRatio == 0.0 {
		counter.localTrafficRatio = counter.defaultTrafficRatio
	}

	counter.mu.Unlock()
	value, err := counter.Factory.LoadData(counter.name, counter.cycleName,
		strings.Join(counter.lbs, KEYSEP))
	counter.mu.Lock()
	if err == nil {
		atomic.StoreInt64(&counter.clusterPrev, atomic.LoadInt64(&counter.clusterLast))
		atomic.StoreInt64(&counter.clusterLast, value)
		counter.lastPullTime = timeNow
		return
	}
}

// 周期更新
func (counter *ClusterCounter) Update() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now()
	if counter.expireTime.Unix() < timeNow.Unix() && counter.resetInterval.Seconds() > 0 {
		var notPushValue = atomic.SwapInt64(&counter.localValue, 0) - atomic.SwapInt64(&counter.localPushed, 0)
		var cycleName = counter.cycleName

		atomic.StoreInt64(&counter.localLast, 0)
		atomic.StoreInt64(&counter.localPrev, 0)
		atomic.StoreInt64(&counter.clusterLast, 0)
		atomic.StoreInt64(&counter.clusterPrev, 0)

		nowTs := timeNow.Unix()
		interval := int64(counter.resetInterval.Seconds())
		startTs := interval * (nowTs / interval)
		endTs := startTs + interval
		intervalKey := fmt.Sprintf("%v%v_%v", KEYSEP, startTs, interval)
		counter.cycleName = intervalKey
		counter.expireTime = time.Now().Add(time.Duration(endTs-nowTs+1) * time.Second)

		if counter.defaultTrafficRatio < 1.0 {
			counter.defaultTrafficRatio = 1.0
		}

		if counter.localTrafficRatio == 0.0 {
			counter.localTrafficRatio = counter.defaultTrafficRatio
		}

		if notPushValue > 0 && len(cycleName) > 0 {
			counter.mu.Unlock()
			err := counter.Factory.StoreData(counter.name, cycleName, strings.Join(counter.lbs, KEYSEP), notPushValue)
			counter.mu.Lock()

			if err != nil {
				counter.lastPushTime = time.Now()
			}
		}
		return
	}

	if timeNow.UnixNano()-counter.lastPullTime.UnixNano() > counter.loadDataInterval.Nanoseconds() {
		counter.mu.Unlock()
		value, err := counter.Factory.LoadData(counter.name, counter.cycleName,
			strings.Join(counter.lbs, KEYSEP))
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
			counter.updateLocalTrafficRatio()
		}

	}

	if timeNow.UnixNano()-counter.lastPullTime.Add(1 * time.Second).UnixNano() > counter.storeDataInterval.Nanoseconds() {
		pushValue := atomic.LoadInt64(&counter.localValue) - atomic.LoadInt64(&counter.localPushed)
		if pushValue > 0 {
			counter.mu.Unlock()
			err := counter.Factory.StoreData(counter.name, counter.cycleName, strings.Join(counter.lbs, KEYSEP),
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
func (counter *ClusterCounter) updateLocalTrafficRatio() {
	clusterPrev := atomic.LoadInt64(&counter.clusterPrev)
	localPrev := atomic.LoadInt64(&counter.localPrev)
	localLast := atomic.LoadInt64(&counter.localLast)
	clusterLast := atomic.LoadInt64(&counter.clusterLast)

	if clusterPrev == 0 || localPrev == 0 ||
		clusterPrev >= clusterLast || localPrev >= localLast ||
		time.Now().UnixNano()-counter.initTime.UnixNano() < 2*counter.storeDataInterval.Nanoseconds() {
		if counter.localTrafficRatio == 0.0 {
			counter.localTrafficRatio = counter.defaultTrafficRatio
		}
	} else {
		var ratio = float64(localLast-localPrev) / float64(clusterLast-clusterPrev)
		if ratio > 1.0 {
			ratio = 1.0
		}
		if ratio > 0.0001 {
			counter.localTrafficRatio = counter.localTrafficRatio*0.5 + ratio*0.5
		}
	}
}
