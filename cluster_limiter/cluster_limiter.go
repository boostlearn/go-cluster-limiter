package cluster_limiter

import (
	//"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"math/rand"
	"sort"
	"sync"
	"time"
)

const DEFAULT_MAX_BOOST_FACTOR = 2.0
const DEFAULT_BOOST_BURST_FACTOR = 10.0
const DEFAULT_BURST_INERVAL_SECONDS = 5
const DEFAULT_DECLINE_EXP_RATIO = 0.5
const DEFAULT_SCORE_SAMPLES_SORT_INTERVAL_SECONDS = 60

type ClusterLimiter struct {
	mu sync.RWMutex

	name     string
	lbs      []string
	initTime time.Time

	beginTime       time.Time
	endTime         time.Time
	completionTime  time.Time
	periodInterval  time.Duration
	reserveInterval time.Duration

	targetReward        float64
	discardPreviousData bool

	RequestCounter *cluster_counter.ClusterCounter
	PassCounter    *cluster_counter.ClusterCounter
	RewardCounter  *cluster_counter.ClusterCounter

	maxBoostFactor         float64
	burstInterval          time.Duration
	lastIdealPassRateTime  time.Time
	lastRewardPassRateTime time.Time
	lastRealPassRateTime   time.Time

	realPassRate    float64
	idealPassRate   float64
	idealRewardRate float64
	declineExpRatio float64

	localRequestIncrease      float64
	localPassIncrease         float64
	localRewardIncrease       float64
	localPacingRewardIncrease float64

	clusterRequestIncrease      float64
	clusterPassIncrease         float64
	clusterRewardIncrease       float64
	clusterPacingRewardIncrease float64

	lastLocalPass         float64
	lastLocalReward       float64
	lastLocalRequest      float64
	lastLocalPacingReward float64

	scoreSamplesSortInterval          time.Duration
	lastScoreSortTime time.Time

	scoreSamples []float64
	scoreSamplesSorted []float64
	scoreSamplesMax int64
	scoreSamplesPos int64
	scoreCutReady bool
	scoreCutValue float64
}

func (limiter *ClusterLimiter) Init() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now()
	limiter.initTime = timeNow
	if limiter.burstInterval.Truncate(time.Second) == 0 {
		limiter.burstInterval = DEFAULT_BURST_INERVAL_SECONDS * time.Second
	}

	if limiter.maxBoostFactor == 0.0 {
		limiter.maxBoostFactor = DEFAULT_MAX_BOOST_FACTOR
	}

	if limiter.declineExpRatio == 0.0 {
		limiter.declineExpRatio = DEFAULT_DECLINE_EXP_RATIO
	}

	if limiter.scoreSamplesSortInterval == 0 {
		limiter.scoreSamplesSortInterval = DEFAULT_SCORE_SAMPLES_SORT_INTERVAL_SECONDS * time.Second
	}

	if limiter.scoreSamplesMax > 0 {
		limiter.scoreSamples = make([]float64, int(limiter.scoreSamplesMax))
		limiter.scoreSamplesPos = 0
		limiter.scoreCutReady = false
	}

	limiter.idealRewardRate = 1.0
	limiter.idealPassRate = 0.0

	if limiter.reserveInterval > 0 {
		limiter.beginTime = timeNow.Truncate(limiter.periodInterval)
		limiter.endTime = limiter.beginTime.Add(limiter.periodInterval)
	}
	if limiter.reserveInterval > 0 && limiter.endTime.After(limiter.beginTime.Add(limiter.reserveInterval)) {
		limiter.completionTime = limiter.endTime.Add(-limiter.reserveInterval)
	} else {
		limiter.completionTime = limiter.endTime
	}
}

func (limiter *ClusterLimiter) Reward(v float64) {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	limiter.RewardCounter.Add(v)
}

func (limiter *ClusterLimiter) Take(v float64) bool {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	if limiter.targetReward == 0 {
		return false
	}

	limiter.RequestCounter.Add(v)
	if rand.Float64() > limiter.realPassRate {
		return false
	}

	clusterPred, _ := limiter.RewardCounter.ClusterValue(0)
	if clusterPred+v > limiter.getPacingReward(time.Now()) {
		return false
	}

	limiter.PassCounter.Add(v)
	return true
}

func (limiter *ClusterLimiter) ScoreTake(v float64, score float64) bool {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	if limiter.targetReward == 0 {
		return false
	}

	if limiter.scoreSamplesMax > 0 {
		limiter.scoreSamples[limiter.scoreSamplesPos%limiter.scoreSamplesMax] = score
		limiter.scoreSamplesPos++
	}

	limiter.RequestCounter.Add(v)

	if limiter.scoreCutReady == false || limiter.scoreSamplesMax == 0 {
		if rand.Float64() > limiter.realPassRate {
			return false
		}
	} else {
		if score < limiter.scoreCutValue {
			return false
		}
	}

	clusterPred, _ := limiter.RewardCounter.ClusterValue(0)
	if clusterPred+v > limiter.getPacingReward(time.Now()) {
		return false
	}

	limiter.PassCounter.Add(v)
	return true
}


func (limiter *ClusterLimiter) Acquire(v float64) bool {
	if limiter.Take(v) {
		limiter.Reward(v)
		return true
	} else {
		return false
	}
}

func (limiter *ClusterLimiter) ScoreAcquire(v float64, score float64) bool {
	if limiter.ScoreTake(v, score) {
		limiter.Reward(v)
		return true
	} else {
		return false
	}
}

func (limiter *ClusterLimiter) SetTarget(target float64) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.targetReward = target
}

func (limiter *ClusterLimiter) GetTarget() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.targetReward
}

func (limiter *ClusterLimiter) PacingReward() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getPacingReward(time.Now())
}

func (limiter *ClusterLimiter) getPacingReward(t time.Time) float64 {
	timeNow := time.Now()
	if timeNow.Before(limiter.beginTime) || timeNow.After(limiter.endTime) ||
		!limiter.beginTime.Before(limiter.endTime) {
		return 0
	}

	if limiter.discardPreviousData && limiter.initTime.Before(limiter.endTime) &&
		limiter.initTime.After(limiter.beginTime) {
		targetTotalReward := limiter.targetReward
		pacingReward := (targetTotalReward) *
			float64(t.UnixNano()-limiter.initTime.UnixNano()) /
			float64(limiter.completionTime.UnixNano()-limiter.beginTime.UnixNano())
		if pacingReward > limiter.targetReward {
			pacingReward = limiter.targetReward
		}
		if pacingReward < 0 {
			pacingReward = 0
		}
		return pacingReward
	} else {
		targetTotalReward := limiter.targetReward
		pacingReward := targetTotalReward *
			float64(t.UnixNano()-limiter.beginTime.UnixNano()) /
			float64(limiter.completionTime.UnixNano()-limiter.beginTime.UnixNano())
		if pacingReward > limiter.targetReward {
			pacingReward = limiter.targetReward
		}
		if pacingReward < 0 {
			pacingReward = 0
		}
		return pacingReward
	}
}

func (limiter *ClusterLimiter) LagTime(reward float64, t time.Time) float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getLagTime(reward, t)
}

func (limiter *ClusterLimiter) getLagTime(reward float64, t time.Time) float64 {
	if limiter.targetReward == 0 || limiter.endTime.After(limiter.beginTime) == false {
		return 0
	}

	pacingReward := limiter.getPacingReward(t)
	interval := float64(limiter.completionTime.UnixNano()-limiter.beginTime.UnixNano()) / 1e9
	return (pacingReward - reward) * interval / limiter.targetReward
}

//
func (limiter *ClusterLimiter) PassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.realPassRate
}

func (limiter *ClusterLimiter) ScoreCut() (bool, float64) {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.scoreCutReady, limiter.scoreCutValue
}

//
func (limiter *ClusterLimiter) IdealPassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.idealPassRate
}

//
func (limiter *ClusterLimiter) IdealRewardRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.idealRewardRate
}

func (limiter *ClusterLimiter) Expire() bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now()
	if limiter.periodInterval > 0 {
		if timeNow.After(limiter.endTime) {
			limiter.beginTime = timeNow.Truncate(limiter.periodInterval)
			limiter.endTime = limiter.beginTime.Add(limiter.periodInterval)

			if limiter.reserveInterval > 0 && limiter.endTime.After(limiter.beginTime.Add(limiter.reserveInterval)) {
				limiter.completionTime = limiter.endTime.Add(-limiter.reserveInterval)
			} else {
				limiter.completionTime = limiter.endTime
			}
		}
		return false
	} else {
		return timeNow.After(limiter.endTime)
	}
}

func (limiter *ClusterLimiter) Heartbeat() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now()
	if timeNow.After(limiter.endTime) || timeNow.Before(limiter.beginTime) {
		return
	}

	if limiter.targetReward == 0 {
		limiter.realPassRate = 0.0
		limiter.idealPassRate = 0.0
		limiter.idealRewardRate = 1.0
		return
	}

	limiter.updateIdealRewardRate()
	limiter.updateIdealPassRate()
	limiter.updateRealPassRate()
	limiter.sortScoreSamples()
}

func (limiter *ClusterLimiter) updateIdealPassRate() {
	timeNow := time.Now()
	if timeNow.Before(limiter.lastIdealPassRateTime.Add(limiter.burstInterval)) {
		return
	}
	limiter.lastIdealPassRateTime = time.Now()

	if timeNow.Before(limiter.initTime.Add(limiter.burstInterval)) ||
		timeNow.After(limiter.endTime.Add(-limiter.burstInterval)) ||
		timeNow.Before(limiter.beginTime.Add(limiter.burstInterval)) {
		limiter.realPassRate = limiter.idealPassRate
		return
	}

	var _, lastLoadTime = limiter.RequestCounter.ClusterValue(-1)
	if timeNow.After(lastLoadTime.Add(limiter.burstInterval * 10)) {
		var curRequest, _ = limiter.RequestCounter.LocalStoreValue(0)
		var curPacingReward = limiter.getPacingReward(timeNow) * limiter.RequestCounter.LocalTrafficRatio()

		limiter.localRequestIncrease = limiter.localRequestIncrease*limiter.declineExpRatio + (curRequest-limiter.lastLocalRequest)*(1-limiter.declineExpRatio)
		limiter.localPacingRewardIncrease = limiter.localPacingRewardIncrease*limiter.declineExpRatio + (curPacingReward-limiter.lastLocalPacingReward)*(1-limiter.declineExpRatio)
		limiter.lastLocalRequest = curRequest
		limiter.lastLocalPacingReward = curPacingReward

		if limiter.localPacingRewardIncrease == 0 && limiter.localRequestIncrease == 0 || limiter.idealRewardRate == 0 {
			return
		}

		idealPassRate := (limiter.localPacingRewardIncrease / limiter.localRequestIncrease) / limiter.idealRewardRate
		if idealPassRate <= 0.0 {
			idealPassRate = 0.0
		}
		if idealPassRate > 1.0 {
			idealPassRate = 1.0
		}
		limiter.idealPassRate = limiter.idealPassRate*limiter.declineExpRatio + idealPassRate*(1-limiter.declineExpRatio)
		return
	} else {
		prev := -limiter.RequestCounter.LoadHistorySize() + 1
		last := -1
		if last <= prev {
			return
		}

		var prevRequest, prevTime = limiter.RequestCounter.ClusterValue(prev)
		var lastRequest, lastTime = limiter.RequestCounter.ClusterValue(last)
		var prevPacingReward = limiter.getPacingReward(prevTime)
		var lastPacingReward = limiter.getPacingReward(lastTime)

		limiter.clusterRequestIncrease = limiter.clusterRequestIncrease*limiter.declineExpRatio + (lastRequest-prevRequest)*(1-limiter.declineExpRatio)
		limiter.clusterPacingRewardIncrease = limiter.clusterPacingRewardIncrease*limiter.declineExpRatio + (lastPacingReward-prevPacingReward)*(1-limiter.declineExpRatio)

		if limiter.clusterRequestIncrease == 0.0 ||
			limiter.clusterPacingRewardIncrease == 0.0 {
			return
		}

		idealPassRate := (limiter.clusterPacingRewardIncrease / limiter.clusterRequestIncrease) / limiter.idealRewardRate

		//fmt.Println("-----pacing: ", limiter.clusterPacingRewardIncrease, " ", lastPacingReward-prevPacingReward)
		//fmt.Println("-----request:", limiter.clusterRequestIncrease, " ", lastRequest-prevRequest)
		//fmt.Println("-----reward rate: ", limiter.idealRewardRate)
		//fmt.Println("-----idea_rate:", idealPassRate)
		if idealPassRate > 0 {
			if idealPassRate > 1.0 {
				idealPassRate = 1.0
			}
			limiter.idealPassRate = limiter.idealPassRate*limiter.declineExpRatio + idealPassRate*(1-limiter.declineExpRatio)
		}
	}
}

func (limiter *ClusterLimiter) updateIdealRewardRate() {
	timeNow := time.Now()
	if timeNow.Before(limiter.lastRewardPassRateTime.Add(limiter.burstInterval)) {
		return
	}
	limiter.lastRewardPassRateTime = time.Now()

	var curReward, _ = limiter.RewardCounter.LocalStoreValue(0)
	var curPass, _ = limiter.PassCounter.LocalStoreValue(0)
	limiter.localPassIncrease = limiter.localPassIncrease*limiter.declineExpRatio + (curPass-limiter.lastLocalPass)*(1-limiter.declineExpRatio)
	limiter.localRewardIncrease = limiter.localRewardIncrease*limiter.declineExpRatio + (curReward-limiter.lastLocalReward)*(1-limiter.declineExpRatio)
	if limiter.localRewardIncrease != 0 && limiter.localPassIncrease != 0 {
		idealRewardRate := limiter.localRewardIncrease / limiter.localPassIncrease
		if idealRewardRate > 0 {
			limiter.idealRewardRate = limiter.idealRewardRate*limiter.declineExpRatio + idealRewardRate*(1-limiter.declineExpRatio)
		}
	}
	limiter.lastLocalReward = curReward
	limiter.lastLocalPass = curPass

	return

}

func (limiter *ClusterLimiter) updateRealPassRate() {
	timeNow := time.Now()
	if timeNow.Before(limiter.initTime.Add(limiter.burstInterval*2)) ||
		timeNow.After(limiter.endTime.Add(-limiter.burstInterval)) ||
		timeNow.Before(limiter.beginTime.Add(limiter.burstInterval*2)) {
		limiter.realPassRate = limiter.idealPassRate
		return
	}

	if timeNow.Before(limiter.lastRealPassRateTime.Add(limiter.burstInterval / 4)) {
		return
	}
	limiter.lastRealPassRateTime = time.Now()

	curReward, _ := limiter.RewardCounter.ClusterValue(0)
	lagTime := limiter.getLagTime(curReward, timeNow)
	if lagTime > 0 {
		smoothPassRate := limiter.idealPassRate * (1 + lagTime*1e9/
			(DEFAULT_BOOST_BURST_FACTOR*float64(limiter.burstInterval.Nanoseconds())))
		if limiter.maxBoostFactor > 1.0 && smoothPassRate > limiter.maxBoostFactor*limiter.idealPassRate {
			smoothPassRate = limiter.maxBoostFactor * limiter.idealPassRate
		}
		if smoothPassRate > 1.0 {
			limiter.realPassRate = 1.0
		} else {
			limiter.realPassRate = smoothPassRate
		}
	} else {
		smoothPassRate := limiter.idealPassRate * (1 + lagTime*4*1e9/
			(DEFAULT_BOOST_BURST_FACTOR*float64(limiter.burstInterval.Nanoseconds())))
		if smoothPassRate < 0 {
			limiter.realPassRate = limiter.idealPassRate / 10000
		} else {
			limiter.realPassRate = smoothPassRate
		}
	}

	if len(limiter.scoreSamplesSorted) > 0 && limiter.realPassRate > 0 && limiter.realPassRate < 1.0{
		limiter.scoreCutValue = limiter.scoreSamplesSorted[
        int(float64((len(limiter.scoreSamplesSorted) - 1))*( 1 - limiter.realPassRate))]
		limiter.scoreCutReady = true
	} else {
		limiter.scoreCutReady = false
    }
}

func (limiter *ClusterLimiter) sortScoreSamples() {
	if limiter.scoreSamplesMax <= 100 || limiter.scoreSamplesPos <= 100 {
		return
	}

	timeNow := time.Now()
	if timeNow.Before(limiter.lastScoreSortTime.Add(limiter.scoreSamplesSortInterval)) {
		return
	}
	sampleNum := limiter.scoreSamplesPos
	if sampleNum > limiter.scoreSamplesMax {
		sampleNum = limiter.scoreSamplesMax
	}
	samples := append([]float64{}, limiter.scoreSamples[:sampleNum]...)
	limiter.mu.Unlock()
	sort.Stable(sort.Float64Slice(samples))
	limiter.mu.Lock()

	limiter.lastScoreSortTime = time.Now()
	limiter.scoreSamplesSorted = samples
}
