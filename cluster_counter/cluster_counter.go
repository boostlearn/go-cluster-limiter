// 集群环境下计数器
package cluster_counter

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const SEP = "####"

//
type ClusterCounterI interface {
	Add(value int64)

	LocalCurrent() int64

	ClusterLast() (int64, time.Time)

	ClusterPredict() int64

	LocalTrafficRatio() float64
}

// 集群计数器
type ClusterCounter struct {
	mu sync.RWMutex

	// 名称
	name        string
	intervalKey string
	// 标签
	lbs map[string]string

	// 生产者
	factory *ClusterCounterFactory

	// 重置周期
	resetInterval time.Duration

	// 更新周期
	loadDataInterval time.Duration

	// 更新周期
	storeDataInterval time.Duration

	defaultTrafficRatio float64

	// 本地数据
	localValue int64

	lastStoreDataTime time.Time
	localPushed       int64
	localLast         int64
	localPrev         int64

	// 集群全局数据
	lastLoadDataTime time.Time
	clusterLast      int64
	clusterPrev      int64

	localTrafficRatio float64

	expireTime time.Time

	initTime time.Time

	discardPreviousData bool
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
	return atomic.LoadInt64(&counter.clusterLast), counter.lastLoadDataTime
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
		counter.intervalKey = fmt.Sprintf("%v%v_%v", SEP, startTs, interval)
		if counter.discardPreviousData {
			counter.intervalKey += fmt.Sprintf("%v%v", SEP, timeNow.UnixNano())
		}
		counter.expireTime = time.Now().Add(time.Duration(endTs-nowTs+1) * time.Second)
	}

	if counter.defaultTrafficRatio == 0.0 {
		counter.defaultTrafficRatio = 1.0
	}

	if counter.localTrafficRatio == 0.0 {
		counter.localTrafficRatio = counter.defaultTrafficRatio
	}

	if counter.loadDataInterval > 0 {
		counter.mu.Unlock()
		value, err := counter.factory.Store.Load(counter.name, counter.intervalKey,
			counter.lbs)
		counter.mu.Lock()
		if err == nil {
			atomic.StoreInt64(&counter.clusterPrev, atomic.LoadInt64(&counter.clusterLast))
			atomic.StoreInt64(&counter.clusterLast, value)
			counter.lastLoadDataTime = timeNow
			return
		}
	}
}

// 周期更新
func (counter *ClusterCounter) Update() {
	counter.CheckReset()

	counter.CheckStoreData()

	counter.CheckStoreData()

}

func (counter *ClusterCounter) CheckReset() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now()
	if counter.expireTime.Unix() < timeNow.Unix() && counter.resetInterval.Seconds() > 0 {
		var notPushValue = atomic.SwapInt64(&counter.localValue, 0) - atomic.SwapInt64(&counter.localPushed, 0)
		var intervalName = counter.intervalKey

		atomic.StoreInt64(&counter.localLast, 0)
		atomic.StoreInt64(&counter.localPrev, 0)
		atomic.StoreInt64(&counter.clusterLast, 0)
		atomic.StoreInt64(&counter.clusterPrev, 0)

		nowTs := timeNow.Unix()
		interval := int64(counter.resetInterval.Seconds())
		startTs := interval * (nowTs / interval)
		endTs := startTs + interval
		counter.intervalKey = fmt.Sprintf("%v%v_%v", SEP, startTs, interval)
		counter.expireTime = time.Now().Add(time.Duration(endTs-nowTs+1) * time.Second)

		if counter.defaultTrafficRatio < 1.0 {
			counter.defaultTrafficRatio = 1.0
		}

		if counter.localTrafficRatio == 0.0 {
			counter.localTrafficRatio = counter.defaultTrafficRatio
		}

		if notPushValue > 0 && len(intervalName) > 0 {
			counter.mu.Unlock()
			err := counter.factory.Store.Store(counter.name, intervalName, counter.lbs, notPushValue)
			counter.mu.Lock()

			if err != nil {
				counter.lastStoreDataTime = time.Now()
			}
		}
		return
	}

}

func (counter *ClusterCounter) CheckLoadData() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now()
	if counter.loadDataInterval > 0 &&
		timeNow.UnixNano()-counter.lastLoadDataTime.UnixNano() > counter.loadDataInterval.Nanoseconds() {
		counter.mu.Unlock()
		value, err := counter.factory.Store.Load(counter.name, counter.intervalKey,
			counter.lbs)
		counter.mu.Lock()

		if err != nil {
			counter.lastLoadDataTime = timeNow
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

}

func (counter *ClusterCounter) CheckStoreData() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now()
	if counter.storeDataInterval > 0 &&
		timeNow.UnixNano()-counter.lastLoadDataTime.Add(1*time.Second).UnixNano() > counter.storeDataInterval.Nanoseconds() {
		pushValue := atomic.LoadInt64(&counter.localValue) - atomic.LoadInt64(&counter.localPushed)
		if pushValue > 0 {
			counter.mu.Unlock()
			err := counter.factory.Store.Store(counter.name, counter.intervalKey, counter.lbs,
				pushValue)
			counter.mu.Lock()

			if err == nil {
				counter.lastStoreDataTime = time.Now()
				atomic.AddInt64(&counter.localPushed, pushValue)
			}
			counter.lastStoreDataTime = time.Now()
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
