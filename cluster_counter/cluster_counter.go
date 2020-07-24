// 集群环境下计数器
package cluster_counter

import (
	"sync"
	"sync/atomic"
	"time"
)

const SEP = "####"

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
	localCurrentValue int64

	lastStoreDataTime time.Time
	localPushedValue  int64
	localLastValue    int64
	localPrevValue    int64

	// 集群全局数据
	lastLoadDataTime time.Time
	clusterLastValue int64
	clusterPrevValue int64
	clusterInitValue int64

	localTrafficRatio float64

	beginTime time.Time
	endTime   time.Time

	initTime time.Time

	discardPreviousData bool
}

// 计数器增加
func (counter *ClusterCounter) Add(v int64) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	atomic.AddInt64(&counter.localCurrentValue, v)
}

// 本地当前值
func (counter *ClusterCounter) LocalCurrent() int64 {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	return atomic.LoadInt64(&counter.localCurrentValue)
}

// 集群最后更新值
func (counter *ClusterCounter) ClusterLast() (int64, time.Time) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	clusterLast := atomic.LoadInt64(&counter.clusterLastValue)
	if counter.discardPreviousData {
		clusterLast -= counter.clusterInitValue
	}
	return clusterLast, counter.lastLoadDataTime
}

// 集群预测值
func (counter *ClusterCounter) ClusterPredict() int64 {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	clusterLast := atomic.LoadInt64(&counter.clusterLastValue)
	localValue := atomic.LoadInt64(&counter.localCurrentValue)
	localLast := atomic.LoadInt64(&counter.localLastValue)

	localTrafficRatio := counter.localTrafficRatio
	if localTrafficRatio == 0.0 {
		counter.localTrafficRatio = counter.defaultTrafficRatio
	}

	clusterPred := clusterLast + int64(float64(localValue-localLast)/localTrafficRatio)
	if counter.discardPreviousData {
		clusterPred -= counter.clusterInitValue
	}
	return clusterPred
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

func (counter *ClusterCounter) Init() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	counter.initTime = timeNow
	counter.resetInterval = counter.resetInterval.Truncate(time.Second)

	if counter.resetInterval > 0 {
		counter.beginTime = timeNow.Truncate(counter.resetInterval)
		counter.endTime = counter.beginTime.Add(counter.resetInterval)
	}

	if counter.defaultTrafficRatio == 0.0 {
		counter.defaultTrafficRatio = 1.0
	}

	if counter.localTrafficRatio == 0.0 {
		counter.localTrafficRatio = counter.defaultTrafficRatio
	}

	counter.mu.Unlock()
	value, err := counter.factory.Store.Load(counter.name, counter.beginTime, counter.endTime, counter.lbs)
	counter.mu.Lock()

	if err == nil {
		atomic.StoreInt64(&counter.clusterPrevValue, atomic.LoadInt64(&counter.clusterLastValue))
		atomic.StoreInt64(&counter.clusterLastValue, value)
		atomic.StoreInt64(&counter.clusterInitValue, value)
		counter.lastLoadDataTime = timeNow
		return
	}

}

// 周期更新
func (counter *ClusterCounter) Update() {
	counter.ResetData()

	counter.StoreData()

	counter.LoadData()

}

func (counter *ClusterCounter) ResetData() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	if timeNow.After(counter.endTime) && counter.resetInterval > 0 {
		var notPushValue = atomic.SwapInt64(&counter.localCurrentValue, 0) - atomic.SwapInt64(&counter.localPushedValue, 0)
		var intervalName = counter.intervalKey

		atomic.StoreInt64(&counter.localLastValue, 0)
		atomic.StoreInt64(&counter.localPrevValue, 0)
		atomic.StoreInt64(&counter.clusterLastValue, 0)
		atomic.StoreInt64(&counter.clusterPrevValue, 0)
		atomic.StoreInt64(&counter.clusterInitValue, 0)

		counter.endTime = timeNow.Truncate(counter.resetInterval)
		counter.endTime = counter.beginTime.Add(counter.resetInterval)

		if counter.defaultTrafficRatio < 1.0 {
			counter.defaultTrafficRatio = 1.0
		}

		if counter.localTrafficRatio == 0.0 {
			counter.localTrafficRatio = counter.defaultTrafficRatio
		}

		if notPushValue > 0 && len(intervalName) > 0 {
			counter.mu.Unlock()
			err := counter.factory.Store.Store(counter.name, counter.beginTime, counter.endTime, counter.lbs, notPushValue)
			counter.mu.Lock()

			if err != nil {
				counter.lastStoreDataTime = time.Now()
			}
		}
		return
	}

}

func (counter *ClusterCounter) LoadData() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	if counter.loadDataInterval > 0 &&
		timeNow.After(counter.lastLoadDataTime.Add(counter.loadDataInterval)) {
		counter.mu.Unlock()
		value, err := counter.factory.Store.Load(counter.name, counter.beginTime, counter.endTime,
			counter.lbs)
		counter.mu.Lock()

		if err != nil {
			counter.lastLoadDataTime = timeNow
			return
		}
		atomic.StoreInt64(&counter.localPrevValue, atomic.LoadInt64(&counter.localLastValue))
		atomic.StoreInt64(&counter.localLastValue, atomic.LoadInt64(&counter.localCurrentValue))

		lastValue := atomic.LoadInt64(&counter.clusterLastValue)
		atomic.StoreInt64(&counter.clusterPrevValue, atomic.LoadInt64(&counter.clusterLastValue))
		atomic.StoreInt64(&counter.clusterLastValue, value)
		if value > lastValue {
			counter.updateLocalTrafficRatio()
		}

	}

}

func (counter *ClusterCounter) StoreData() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	if counter.storeDataInterval > 0 &&
		timeNow.UnixNano()-counter.lastLoadDataTime.Add(1*time.Second).UnixNano() > counter.storeDataInterval.Nanoseconds() {
		pushValue := atomic.LoadInt64(&counter.localCurrentValue) - atomic.LoadInt64(&counter.localPushedValue)
		if pushValue > 0 {
			counter.mu.Unlock()
			err := counter.factory.Store.Store(counter.name, counter.beginTime, counter.endTime, counter.lbs,
				pushValue)
			counter.mu.Lock()

			if err == nil {
				counter.lastStoreDataTime = time.Now()
				atomic.AddInt64(&counter.localPushedValue, pushValue)
			}
			counter.lastStoreDataTime = time.Now()
		}
	}
}

// 重新计算放大系数
func (counter *ClusterCounter) updateLocalTrafficRatio() {
	clusterPrev := atomic.LoadInt64(&counter.clusterPrevValue)
	localPrev := atomic.LoadInt64(&counter.localPrevValue)
	localLast := atomic.LoadInt64(&counter.localLastValue)
	clusterLast := atomic.LoadInt64(&counter.clusterLastValue)

	timeNow := time.Now().Truncate(time.Second)
	if clusterPrev == 0 || localPrev == 0 ||
		clusterPrev >= clusterLast || localPrev >= localLast ||
		timeNow.Before(counter.initTime.Add(2*counter.storeDataInterval)) {
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
