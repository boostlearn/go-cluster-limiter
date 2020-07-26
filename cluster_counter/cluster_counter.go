// 集群环境下计数器
package cluster_counter

import (
	"sync"
	"time"
)

const SEP = "####"
const HistoryMax = 10

type ClusterCounter struct {
	mu sync.RWMutex

	name     string
	lbs      map[string]string
	factory  *ClusterCounterFactory
	initTime time.Time

	beginTime      time.Time
	endTime        time.Time
	periodInterval time.Duration

	localValue        float64
	storeDataInterval time.Duration
	storeHistoryPos   int64
	storeLocalHistory [HistoryMax]float64
	storeTimeHistory  [HistoryMax]time.Time
	lastStoreValue    float64
	storeLastTime     time.Time

	loadDataInterval    time.Duration
	discardPreviousData bool
	clusterInitValue    float64

	loadHistoryPos          int64
	loadDataTimeHistory     [HistoryMax]time.Time
	loadLocalValueHistory   [HistoryMax]float64
	loadClusterValueHistory [HistoryMax]float64

	defaultTrafficRatio float64
	localTrafficRatio   float64
}

func (counter *ClusterCounter) Init() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	counter.initTime = timeNow

	counter.periodInterval = counter.periodInterval.Truncate(time.Second)
	if counter.periodInterval > 0 {
		counter.beginTime = timeNow.Truncate(counter.periodInterval)
		counter.endTime = counter.beginTime.Add(counter.periodInterval)
	}

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
			counter.loadClusterValueHistory[(counter.loadHistoryPos)%HistoryMax] = value
			counter.loadLocalValueHistory[(counter.loadHistoryPos)%HistoryMax] = counter.localValue
			counter.loadDataTimeHistory[(counter.loadHistoryPos)%HistoryMax] = timeNow
			counter.loadHistoryPos += 1
			counter.clusterInitValue = value
			return
		}
	}

}

func (counter *ClusterCounter) Expire() bool {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	if counter.periodInterval > 0 {
		if timeNow.After(counter.endTime) {
			lastBeginTime := counter.beginTime
			lastEndTime := counter.endTime
			counter.beginTime = timeNow.Truncate(counter.periodInterval)
			counter.endTime = counter.beginTime.Add(counter.periodInterval)
			pushValue := counter.localValue - counter.lastStoreValue
			counter.localValue = 0
			counter.lastStoreValue = 0
			counter.loadHistoryPos = 0

			if pushValue > 0 {
				counter.mu.Unlock()
				_ = counter.factory.Store.Store(counter.name, lastBeginTime, lastEndTime, counter.lbs,
					pushValue)
				counter.mu.Lock()
			}
		}
		return false
	} else {
		return timeNow.After(counter.endTime)
	}
}

//
func (counter *ClusterCounter) Add(v float64) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	counter.localValue += v
}

//
func (counter *ClusterCounter) LocalValue(last int) (float64, time.Time) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if last == 0 {
		return counter.localValue, time.Now()
	}
	if last < 0 && last > -HistoryMax && int64(last) > -counter.loadHistoryPos {
		return counter.loadLocalValueHistory[(counter.loadHistoryPos+int64(last)+HistoryMax)%HistoryMax],
			counter.loadDataTimeHistory[(counter.loadHistoryPos+int64(last)+HistoryMax)%HistoryMax]
	}

	return 0, time.Unix(0, 0)
}

func (counter *ClusterCounter) LocalStoreValue(last int) (float64, time.Time) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if last == 0 {
		return counter.localValue, time.Now()
	}
	if last < 0 && last > -HistoryMax && int64(last) > -counter.storeHistoryPos {
		return counter.storeLocalHistory[(counter.storeHistoryPos+int64(last)+HistoryMax)%HistoryMax],
			counter.storeTimeHistory[(counter.storeHistoryPos+int64(last)+HistoryMax)%HistoryMax]
	}

	return 0, time.Unix(0, 0)
}

func (counter *ClusterCounter) ClusterValue(last int) (float64, time.Time) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if last == 0 {
		clusterLast := counter.loadClusterValueHistory[(counter.loadHistoryPos-1+HistoryMax)%HistoryMax]
		localValue := counter.localValue
		localLast := counter.loadLocalValueHistory[(counter.loadHistoryPos-1+HistoryMax)%HistoryMax]

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

	if last < 0 && last > -HistoryMax && int64(last) > -counter.loadHistoryPos {
		clusterLast := counter.loadClusterValueHistory[(counter.loadHistoryPos+int64(last)+HistoryMax)%HistoryMax]
		if counter.discardPreviousData && counter.initTime.After(counter.beginTime) &&
			counter.initTime.Before(counter.endTime) {
			clusterLast -= counter.clusterInitValue
		}
		return clusterLast, counter.loadDataTimeHistory[(counter.loadHistoryPos+int64(last)+HistoryMax)%HistoryMax]
	}

	return 0, time.Unix(0, 0)
}

//
func (counter *ClusterCounter) LocalTrafficRatio() float64 {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if counter.localTrafficRatio == 0.0 {
		return counter.defaultTrafficRatio
	}

	return counter.localTrafficRatio
}

//
func (counter *ClusterCounter) HeartBeat() {
	counter.StoreData()
	counter.LoadData()
}

func (counter *ClusterCounter) LoadData() bool {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	if counter.factory == nil || counter.factory.Store == nil {
		return false
	}

	timeNow := time.Now()
	if counter.loadDataInterval > 0 &&
		timeNow.After(counter.loadDataTimeHistory[
			(counter.loadHistoryPos-1+HistoryMax)%HistoryMax].Add(counter.loadDataInterval)) {
		counter.mu.Unlock()
		value, err := counter.factory.Store.Load(counter.name, counter.beginTime, counter.endTime,
			counter.lbs)
		counter.mu.Lock()

		if err != nil {
			return false
		}

		counter.loadLocalValueHistory[counter.loadHistoryPos%HistoryMax] = counter.localValue
		counter.loadClusterValueHistory[counter.loadHistoryPos%HistoryMax] = value
		counter.loadDataTimeHistory[counter.loadHistoryPos%HistoryMax] = timeNow
		counter.loadHistoryPos += 1
		counter.updateLocalTrafficRatio()
		return true
	}
	return false
}

func (counter *ClusterCounter) updateLocalTrafficRatio() {
	if counter.localTrafficRatio == 0.0 {
		counter.localTrafficRatio = counter.defaultTrafficRatio
	}

	var localIncrease float64
	var clusterIncrease float64
	for i := -1; i > -int(counter.loadHistoryPos%HistoryMax); i-- {
		clusterPrev := counter.loadClusterValueHistory[(counter.loadHistoryPos+int64(i)-1+HistoryMax)%HistoryMax]
		clusterCur := counter.loadClusterValueHistory[(counter.loadHistoryPos+int64(i)+HistoryMax)%HistoryMax]
		clusterIncrease = clusterIncrease*0.5 + (clusterPrev-clusterCur)*0.5

		localPrev := counter.loadLocalValueHistory[(counter.loadHistoryPos+int64(i)-1+HistoryMax)%HistoryMax]
		localCur := counter.loadLocalValueHistory[(counter.loadHistoryPos+int64(i)+HistoryMax)%HistoryMax]
		localIncrease = localIncrease*0.5 + (localPrev-localCur)*0.5
	}

	if localIncrease > 0.0 && clusterIncrease > 0.0 {
		counter.localTrafficRatio = counter.localTrafficRatio*0.5 + (localIncrease/clusterIncrease)*0.5
	}
}

func (counter *ClusterCounter) StoreData() bool {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	if counter.factory == nil || counter.factory.Store == nil {
		return false
	}

	timeNow := time.Now()
	if counter.storeDataInterval > 0 &&
		timeNow.After(counter.storeLastTime.Add(counter.storeDataInterval)) {
		counter.storeLocalHistory[counter.storeHistoryPos%HistoryMax] = counter.localValue
		counter.storeTimeHistory[counter.storeHistoryPos%HistoryMax] = timeNow
		counter.storeHistoryPos += 1

		pushValue := counter.localValue - counter.lastStoreValue
		if pushValue > 0 {
			counter.mu.Unlock()
			err := counter.factory.Store.Store(counter.name, counter.beginTime, counter.endTime, counter.lbs,
				pushValue)
			counter.mu.Lock()
			if err == nil {
				counter.lastStoreValue += pushValue
				counter.storeLastTime = timeNow
				return true
			}

		}
	}
	return false
}
