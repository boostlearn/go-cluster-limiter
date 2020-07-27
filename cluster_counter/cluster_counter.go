// 集群环境下计数器
package cluster_counter

import (
	"fmt"
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
	storeInterval     time.Duration
	storeHistoryPos   int64
	storeLocalHistory [HistoryMax]float64
	storeTimeHistory  [HistoryMax]time.Time
	lastStoreValue    float64
	lastStoreTime     time.Time

	loadInterval        time.Duration
	discardPreviousData bool
	loadInitValue       float64

	loadHistoryPos     int64
	loadTimeHistory    [HistoryMax]time.Time
	loadLocalHistory   [HistoryMax]float64
	loadClusterHistory [HistoryMax]float64

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
			counter.loadClusterHistory[(counter.loadHistoryPos)%HistoryMax] = value
			counter.loadLocalHistory[(counter.loadHistoryPos)%HistoryMax] = counter.localValue
			counter.loadTimeHistory[(counter.loadHistoryPos)%HistoryMax] = timeNow
			counter.loadHistoryPos += 1
			counter.loadInitValue = value
			return
		}
	}

}

func (counter *ClusterCounter) Expire() bool {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now()
	if counter.periodInterval > 0 {
		if timeNow.After(counter.endTime) {
			fmt.Println(timeNow.Format("2006-01-02 15:04:05 .9999"), counter.endTime.Format("2006-01-02 15:04:05 .9999"))
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
					pushValue, true)
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
		return counter.loadLocalHistory[(counter.loadHistoryPos+int64(last)+HistoryMax)%HistoryMax],
			counter.loadTimeHistory[(counter.loadHistoryPos+int64(last)+HistoryMax)%HistoryMax]
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
		clusterLast := counter.loadClusterHistory[(counter.loadHistoryPos-1+HistoryMax)%HistoryMax]
		localValue := counter.localValue
		localLast := counter.loadLocalHistory[(counter.loadHistoryPos-1+HistoryMax)%HistoryMax]

		localTrafficRatio := counter.localTrafficRatio
		if localTrafficRatio == 0.0 {
			counter.localTrafficRatio = counter.defaultTrafficRatio
		}

		clusterPred := clusterLast + (localValue-localLast)/localTrafficRatio
		if counter.discardPreviousData {
			clusterPred -= counter.loadInitValue
		}
		return clusterPred, time.Now()
	}

	if last < 0 && last > -HistoryMax && int64(last) > -counter.loadHistoryPos {
		clusterLast := counter.loadClusterHistory[(counter.loadHistoryPos+int64(last)+HistoryMax)%HistoryMax]
		if counter.discardPreviousData && counter.initTime.After(counter.beginTime) &&
			counter.initTime.Before(counter.endTime) {
			clusterLast -= counter.loadInitValue
		}
		return clusterLast, counter.loadTimeHistory[(counter.loadHistoryPos+int64(last)+HistoryMax)%HistoryMax]
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
	if counter.loadInterval > 0 &&
		timeNow.After(counter.loadTimeHistory[
			(counter.loadHistoryPos-1+HistoryMax)%HistoryMax].Add(counter.loadInterval)) {
		counter.mu.Unlock()
		value, err := counter.factory.Store.Load(counter.name, counter.beginTime, counter.endTime,
			counter.lbs)
		counter.mu.Lock()

		if err != nil {
			return false
		}

		counter.loadLocalHistory[counter.loadHistoryPos%HistoryMax] = counter.localValue
		counter.loadClusterHistory[counter.loadHistoryPos%HistoryMax] = value
		counter.loadTimeHistory[counter.loadHistoryPos%HistoryMax] = timeNow
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
		clusterPrev := counter.loadClusterHistory[(counter.loadHistoryPos+int64(i)-1+HistoryMax)%HistoryMax]
		clusterCur := counter.loadClusterHistory[(counter.loadHistoryPos+int64(i)+HistoryMax)%HistoryMax]
		clusterIncrease = clusterIncrease*0.5 + (clusterPrev-clusterCur)*0.5

		localPrev := counter.loadLocalHistory[(counter.loadHistoryPos+int64(i)-1+HistoryMax)%HistoryMax]
		localCur := counter.loadLocalHistory[(counter.loadHistoryPos+int64(i)+HistoryMax)%HistoryMax]
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
	if counter.storeInterval > 0 &&
		timeNow.After(counter.lastStoreTime.Add(counter.storeInterval)) {
		counter.storeLocalHistory[counter.storeHistoryPos%HistoryMax] = counter.localValue
		counter.storeTimeHistory[counter.storeHistoryPos%HistoryMax] = timeNow
		counter.storeHistoryPos += 1

		pushValue := counter.localValue - counter.lastStoreValue
		if pushValue > 0 {
			counter.mu.Unlock()
			err := counter.factory.Store.Store(counter.name, counter.beginTime, counter.endTime, counter.lbs,
				pushValue, false)
			counter.mu.Lock()
			if err == nil {
				counter.lastStoreValue += pushValue
				counter.lastStoreTime = timeNow
				return true
			}

		}
	}
	return false
}
