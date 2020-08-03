package cluster_counter

import (
	"reflect"

	//"fmt"
	"sync"
	"time"
)

const HistoryMax = 30
const DefaultDeclineExpRatio = 0.8
const DefaultStoreIntervalSeconds = 2
const DefaultTrafficProportion = 1.0

// counter within cluster
type ClusterCounter struct {
	mu sync.RWMutex

	name     string
	lbs      map[string]string
	factory  *ClusterCounterFactory
	initTime time.Time

	beginTime      time.Time
	endTime        time.Time
	periodInterval time.Duration
	storeInterval  time.Duration

	localValue        float64
	storeHistoryPos   int64
	lastStoreTime     time.Time
	storeLocalHistory [HistoryMax]float64
	storeTimeHistory  [HistoryMax]time.Time
	lastStoreValue    float64

	discardPreviousData bool
	loadInitValue       float64
	loadHistoryPos      int64
	lastLoadTime        time.Time
	loadTimeHistory     [HistoryMax]time.Time
	loadLocalHistory    [HistoryMax]float64
	loadClusterHistory  [HistoryMax]float64

	initLocalTrafficProportion float64
	localTrafficProportion     float64

	localIncrease   float64
	clusterIncrease float64
	declineExpRatio float64
}

// init counter
func (counter *ClusterCounter) Init() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now()
	counter.initTime = timeNow

	counter.periodInterval = counter.periodInterval.Truncate(time.Second)
	if counter.periodInterval > 0 {
		counter.beginTime = timeNow.Truncate(counter.periodInterval)
		counter.endTime = counter.beginTime.Add(counter.periodInterval)
	}

	if counter.initLocalTrafficProportion == 0.0 {
		counter.initLocalTrafficProportion = DefaultTrafficProportion
	}

	if counter.storeInterval == 0 {
		counter.storeInterval = DefaultStoreIntervalSeconds * time.Second
	}

	if counter.localTrafficProportion == 0.0 {
		counter.localTrafficProportion = counter.initLocalTrafficProportion
	}

	if counter.factory != nil && reflect.ValueOf(counter.factory.Store).IsNil() == false {
		counter.mu.Unlock()
		value, err := counter.factory.Store.Load(counter.name, counter.beginTime, counter.endTime, counter.lbs)
		counter.mu.Lock()

		if err == nil {
			counter.loadClusterHistory[(counter.loadHistoryPos)%HistoryMax] = value
			counter.loadLocalHistory[(counter.loadHistoryPos)%HistoryMax] = 0
			counter.loadTimeHistory[(counter.loadHistoryPos)%HistoryMax] = time.Now()
			counter.loadHistoryPos += 1
			counter.loadInitValue = value
			counter.lastLoadTime = timeNow.Truncate(counter.storeInterval.Truncate(counter.storeInterval)).Add(counter.storeInterval / 2)
			return
		}
	}

	if counter.declineExpRatio <= 0.0 || counter.declineExpRatio > 1.0 {
		counter.declineExpRatio = DefaultDeclineExpRatio
	}

	counter.storeLocalHistory[counter.storeHistoryPos%HistoryMax] = counter.localValue
	counter.storeTimeHistory[counter.storeHistoryPos%HistoryMax] = timeNow
	counter.storeHistoryPos += 1
	counter.lastStoreTime = timeNow.Truncate(counter.storeInterval)

}

// check whether expired
func (counter *ClusterCounter) Expire() bool {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	timeNow := time.Now()
	if counter.periodInterval > 0 {
		if timeNow.After(counter.endTime) {
			//fmt.Println(timeNow.Format("2006-01-02 15:04:05 .9999"), counter.endTime.Format("2006-01-02 15:04:05 .9999"))
			lastBeginTime := counter.beginTime
			lastEndTime := counter.endTime
			counter.beginTime = timeNow.Truncate(counter.periodInterval)
			counter.endTime = counter.beginTime.Add(counter.periodInterval)
			pushValue := counter.localValue - counter.lastStoreValue
			counter.localValue = 0
			counter.lastStoreValue = 0
			counter.loadHistoryPos = 0
			counter.loadInitValue = 0
			counter.storeHistoryPos = 0
			if pushValue > 0 && counter.factory != nil && reflect.ValueOf(counter.factory.Store).IsNil() == false {
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

// add value into counter
func (counter *ClusterCounter) Add(v float64) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	counter.localValue += v
}

// get local value
// last: A negative number represents the query history data
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

// get local stored history data
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

// get cluster value
// last: A negative number represents the query history data
func (counter *ClusterCounter) ClusterValue(last int) (float64, time.Time) {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if last == 0 {
		if counter.localTrafficProportion == 0.0 {
			counter.localTrafficProportion = counter.initLocalTrafficProportion
		}

		var clusterLast float64 = 0
		var localLast float64 = 0
		localValue := counter.localValue
		if counter.loadHistoryPos > 0 {
			clusterLast = counter.loadClusterHistory[(counter.loadHistoryPos-1+HistoryMax)%HistoryMax]
			localLast = counter.loadLocalHistory[(counter.loadHistoryPos-1+HistoryMax)%HistoryMax]
		}

		clusterPred := clusterLast + (localValue-localLast)/counter.localTrafficProportion
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

func (counter *ClusterCounter) LocalIncrease() float64 {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	return counter.localIncrease
}

func (counter *ClusterCounter) ClusterIncrease() float64 {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	return counter.clusterIncrease
}

// Proportion of local traffic in cluster
func (counter *ClusterCounter) LocalTrafficProportion() float64 {
	counter.mu.RLock()
	defer counter.mu.RUnlock()

	if counter.localTrafficProportion == 0.0 {
		return counter.initLocalTrafficProportion
	}

	return counter.localTrafficProportion
}

func (counter *ClusterCounter) LoadHistorySize() int {
	if counter.loadHistoryPos < HistoryMax {
		return int(counter.loadHistoryPos)
	} else {
		return HistoryMax
	}
}

func (counter *ClusterCounter) StoreHistorySize() int {
	if counter.storeHistoryPos < HistoryMax {
		return int(counter.storeHistoryPos)
	} else {
		return HistoryMax
	}
}

// update data heartbeat
func (counter *ClusterCounter) Heartbeat() {
	counter.StoreData()
	counter.LoadData()
}

// load data from cluster's storage
func (counter *ClusterCounter) LoadData() bool {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	if counter.factory == nil || reflect.ValueOf(counter.factory.Store).IsNil() == true {
		return false
	}

	timeNow := time.Now()
	if counter.storeInterval > 0 && (counter.loadHistoryPos == 0 ||
		timeNow.After(counter.lastLoadTime.Add(counter.storeInterval))) {
		counter.mu.Unlock()
		value, err := counter.factory.Store.Load(counter.name, counter.beginTime, counter.endTime,
			counter.lbs)
		counter.mu.Lock()

		if err != nil {
			return false
		}

		if counter.loadHistoryPos > 0 {
			counter.loadLocalHistory[counter.loadHistoryPos%HistoryMax] = counter.storeLocalHistory[(counter.storeHistoryPos+-1+HistoryMax)%HistoryMax]
		} else {
			counter.loadLocalHistory[counter.loadHistoryPos%HistoryMax] = 0
		}
		counter.loadClusterHistory[counter.loadHistoryPos%HistoryMax] = value
		counter.loadTimeHistory[counter.loadHistoryPos%HistoryMax] = time.Now()
		counter.lastLoadTime = timeNow.Truncate(counter.storeInterval.Truncate(counter.storeInterval)).Add(counter.storeInterval / 2)
		counter.loadHistoryPos += 1
		counter.updateLocalTrafficProportion()
		return true
	}
	return false
}

func (counter *ClusterCounter) updateLocalTrafficProportion() {
	if counter.localTrafficProportion == 0.0 || (counter.loadHistoryPos < 4 && counter.initTime.After(counter.beginTime)) {
		counter.localTrafficProportion = counter.initLocalTrafficProportion
		return
	}

	if counter.loadHistoryPos > 2 {
		clusterPrev := counter.loadClusterHistory[(counter.loadHistoryPos-2+HistoryMax)%HistoryMax]
		clusterCur := counter.loadClusterHistory[(counter.loadHistoryPos-1+HistoryMax)%HistoryMax]
		counter.clusterIncrease = counter.clusterIncrease*counter.declineExpRatio +
			(clusterCur-clusterPrev)*(1-counter.declineExpRatio)

		localPrev := counter.loadLocalHistory[(counter.loadHistoryPos-2+HistoryMax)%HistoryMax]
		localCur := counter.loadLocalHistory[(counter.loadHistoryPos-1+HistoryMax)%HistoryMax]
		counter.localIncrease = counter.localIncrease*counter.declineExpRatio +
			(localCur-localPrev)*(1-counter.declineExpRatio)
	}

	if counter.localIncrease != 0.0 && counter.clusterIncrease != 0.0 {
		ratio := counter.localIncrease / counter.clusterIncrease
		if ratio > 1.0 {
			ratio = 1.0
		}
		counter.localTrafficProportion = counter.localTrafficProportion*0.5 + ratio*0.5
	}
}

// store local data into cluster's storage
func (counter *ClusterCounter) StoreData() bool {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	if counter.factory == nil || reflect.ValueOf(counter.factory.Store).IsNil() == true {
		return false
	}

	timeNow := time.Now()
	if counter.storeInterval > 0 && (counter.storeHistoryPos == 0 ||
		timeNow.After(counter.lastStoreTime.Add(counter.storeInterval))) {

		counter.storeLocalHistory[counter.storeHistoryPos%HistoryMax] = counter.localValue
		counter.storeTimeHistory[counter.storeHistoryPos%HistoryMax] = timeNow
		counter.storeHistoryPos += 1
		counter.lastStoreTime = timeNow.Truncate(counter.storeInterval)

		pushValue := counter.localValue - counter.lastStoreValue
		if pushValue > 0 {
			counter.mu.Unlock()
			err := counter.factory.Store.Store(counter.name, counter.beginTime, counter.endTime, counter.lbs,
				pushValue, false)
			counter.mu.Lock()
			if err == nil {
				counter.lastStoreValue += pushValue
				return true
			}

		}
	}
	return false
}
