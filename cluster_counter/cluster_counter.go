// 集群环境下计数器
package cluster_counter

import (
	"sync"
	"time"
)

const SEP = "####"
const HISTORYMAX = 10

// 集群计数器
type ClusterCounter struct {
	mu sync.RWMutex

	name    string
	lbs     map[string]string
	factory *ClusterCounterFactory

	beginTime time.Time
	endTime   time.Time
	initTime  time.Time

	historyPos int64

	storeDataInterval time.Duration
	localCurrentValue float64
	localPushedValue  float64
	lastStoreDataTime time.Time

	loadDataInterval    time.Duration
	discardPreviousData bool
	clusterInitValue    float64

	loadDataHistoryTime [HISTORYMAX]time.Time
	localHistoryValue   [HISTORYMAX]float64
	clusterHistoryValue [HISTORYMAX]float64

	defaultTrafficRatio float64
	localTrafficRatio   float64
}

func (counter *ClusterCounter) Init() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	counter.initTime = timeNow

	if counter.defaultTrafficRatio == 0.0 {
		counter.defaultTrafficRatio = 1.0
	}

	if counter.localTrafficRatio == 0.0 {
		counter.localTrafficRatio = counter.defaultTrafficRatio
	}

	if counter.factory != nil && counter.factory.Store != nil {
		counter.mu.Unlock()
		value, err := counter.factory.Store.Load(counter.name, counter.beginTime, counter.endTime, counter.lbs)
		counter.mu.Lock()

		if err == nil {
			counter.clusterHistoryValue[(counter.historyPos)%HISTORYMAX] = value
			counter.localHistoryValue[(counter.historyPos)%HISTORYMAX] = counter.localCurrentValue
			counter.loadDataHistoryTime[(counter.historyPos)%HISTORYMAX] = timeNow
			counter.historyPos += 1
			counter.clusterInitValue = value
			return
		}
	}

}

// 计数器增加
func (counter *ClusterCounter) Add(v float64) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	counter.localCurrentValue += v
}

// 本地当前值
func (counter *ClusterCounter) LocalValue(last int) (float64, time.Time) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if last == 0 {
		return counter.localCurrentValue, time.Now()
	}
	if last > HISTORYMAX || last < -HISTORYMAX {
		return 0, time.Unix(0, 0)
	}

	if last < 0 && last > -HISTORYMAX && int64(last) > -counter.historyPos {
		return counter.localHistoryValue[(counter.historyPos+int64(last)+HISTORYMAX)%HISTORYMAX],
			counter.loadDataHistoryTime[(counter.historyPos+int64(last)+HISTORYMAX)%HISTORYMAX]
	}

	return 0, time.Unix(0, 0)
}

func (counter *ClusterCounter) ClusterValue(last int) (float64, time.Time) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if last == 0 {
		clusterLast := counter.clusterHistoryValue[(counter.historyPos-1+HISTORYMAX)%HISTORYMAX]
		localValue := counter.localCurrentValue
		localLast := counter.localHistoryValue[(counter.historyPos-1+HISTORYMAX)%HISTORYMAX]

		localTrafficRatio := counter.localTrafficRatio
		if localTrafficRatio == 0.0 {
			counter.localTrafficRatio = counter.defaultTrafficRatio
		}

		clusterPred := clusterLast + (localValue-localLast)/localTrafficRatio
		if counter.discardPreviousData {
			clusterPred -= counter.clusterInitValue
		}
		return clusterPred, time.Now()
	}

	if last < 0 && last > -HISTORYMAX && int64(last) > -counter.historyPos {
		clusterLast := counter.clusterHistoryValue[(counter.historyPos+int64(last)+HISTORYMAX)%HISTORYMAX]
		if counter.discardPreviousData && counter.initTime.After(counter.beginTime) &&
			counter.initTime.Before(counter.endTime) {
			clusterLast -= counter.clusterInitValue
		}
		return clusterLast, counter.loadDataHistoryTime[(counter.historyPos+int64(last)+HISTORYMAX)%HISTORYMAX]
	}

	return 0, time.Unix(0, 0)
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

// 周期更新
func (counter *ClusterCounter) Update() {
	counter.StoreData()
	counter.LoadData()
}

func (counter *ClusterCounter) LoadData() bool {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	if counter.factory == nil ||counter.factory.Store == nil {
		return false
	}

	timeNow := time.Now()
	if counter.loadDataInterval > 0 &&
		timeNow.After(counter.loadDataHistoryTime[
			(counter.historyPos-1+HISTORYMAX)%HISTORYMAX].Add(counter.loadDataInterval)) {
		counter.mu.Unlock()
		value, err := counter.factory.Store.Load(counter.name, counter.beginTime, counter.endTime,
			counter.lbs)
		counter.mu.Lock()

		if err != nil {
			return false
		}

		counter.localHistoryValue[counter.historyPos%HISTORYMAX] = counter.localCurrentValue
		counter.clusterHistoryValue[counter.historyPos%HISTORYMAX] = value
		counter.loadDataHistoryTime[counter.historyPos%HISTORYMAX] = timeNow
		counter.historyPos += 1
		counter.updateLocalTrafficRatio()
		return true
	}
	return false
}

func (counter *ClusterCounter) StoreData() bool {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	if counter.factory == nil || counter.factory.Store == nil {
		return false
	}

	timeNow := time.Now()
	if counter.storeDataInterval > 0 &&
		timeNow.UnixNano()-counter.lastStoreDataTime.Add(1*time.Second).UnixNano() >
			counter.storeDataInterval.Nanoseconds() {
		pushValue := counter.localCurrentValue - counter.localPushedValue
		if pushValue > 0 {
			counter.mu.Unlock()
			err := counter.factory.Store.Store(counter.name, counter.beginTime, counter.endTime, counter.lbs,
				pushValue)
			counter.mu.Lock()
			if err == nil {
				counter.lastStoreDataTime = time.Now()
				counter.localPushedValue -= pushValue
				counter.lastStoreDataTime = time.Now()
				return true
			}

		}
	}
	return false
}

// 重新计算放大系数
func (counter *ClusterCounter) updateLocalTrafficRatio() {
	if counter.localTrafficRatio == 0.0 {
		counter.localTrafficRatio = counter.defaultTrafficRatio
	}

	var localIncrease float64
	var clusterIncrease float64
	for i := 1; i > -int(counter.historyPos%HISTORYMAX); i-- {
		clusterPrev := counter.clusterHistoryValue[(counter.historyPos-int64(i)-1+HISTORYMAX)%HISTORYMAX]
		clusterCur := counter.clusterHistoryValue[(counter.historyPos-int64(i)+HISTORYMAX)%HISTORYMAX]
		clusterIncrease = clusterIncrease*0.5 + (clusterPrev-clusterCur)*0.5

		localPrev := counter.localHistoryValue[(counter.historyPos-int64(i)-1+HISTORYMAX)%HISTORYMAX]
		localCur := counter.localHistoryValue[(counter.historyPos-int64(i)+HISTORYMAX)%HISTORYMAX]
		localIncrease = localIncrease*0.5 + (localPrev-localCur)*0.5
	}

	if localIncrease > 0.0 && clusterIncrease > 0.0 {
		counter.localTrafficRatio = counter.localTrafficRatio*0.5 + (localIncrease/clusterIncrease)*0.5
	}
}
