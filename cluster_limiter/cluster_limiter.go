package cluster_limiter

import (
	//"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"math/rand"
	"reflect"
	"sort"
	"sync"
	"time"
)

// limiter: limit traffic within cluster
type ClusterLimiter struct {
	mu      sync.RWMutex
	expired bool

	Options *ClusterLimiterOpts
	factory *ClusterLimiterFactory

	name     string
	initTime time.Time

	beginTime       time.Time
	endTime         time.Time
	completionTime  time.Time
	periodInterval  time.Duration
	reserveInterval time.Duration

	rewardTarget        float64
	discardPreviousData bool

	periodRewardBase cluster_counter.CounterValue

	RequestCounter *cluster_counter.ClusterCounter
	PassCounter    *cluster_counter.ClusterCounter
	RewardCounter  *cluster_counter.ClusterCounter

	maxBoostFactor          float64
	burstInterval           time.Duration
	lastIdealPassRateTime   time.Time
	lastRewardPassRateTime  time.Time
	lastWorkingPassRateTime time.Time

	workingPassRate           float64
	idealPassRate             float64
	idealRewardRate           float64
	declineExpRatio           float64
	rewardRateDeclineExpRatio float64

	localRequestRecently     cluster_counter.CounterValue
	localPassRecently        cluster_counter.CounterValue
	localRewardRecently      cluster_counter.CounterValue
	localIdealRewardRecently cluster_counter.CounterValue

	clusterRequestRecently     cluster_counter.CounterValue
	clusterPassRecently        cluster_counter.CounterValue
	clusterRewardRecently      cluster_counter.CounterValue
	clusterIdealRewardRecently cluster_counter.CounterValue

	prevLocalPass        cluster_counter.CounterValue
	prevLocalReward      cluster_counter.CounterValue
	prevLocalRequest     cluster_counter.CounterValue
	prevLocalIdealReward cluster_counter.CounterValue

	prevClusterRequest     cluster_counter.CounterValue
	prevClusterRequestTime time.Time

	scoreSamplesSortInterval time.Duration
	lastScoreSortTime        time.Time

	scoreSamples       []float64
	scoreSamplesSorted []float64
	scoreSamplesMax    int64
	scoreSamplesPos    int64
	scoreCutReady      bool
	scoreCutValue      float64
}

// init limiter
func (limiter *ClusterLimiter) Initialize() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now()
	limiter.initTime = timeNow
	limiter.expired = false

	if limiter.burstInterval.Truncate(time.Second) == 0 {
		limiter.burstInterval = DefaultBurstIntervalSeconds * time.Second
	}

	if limiter.maxBoostFactor == 0.0 {
		limiter.maxBoostFactor = DefaultMaxBoostFactor
	}

	if limiter.declineExpRatio == 0.0 {
		limiter.declineExpRatio = DefaultDeclineExpRatio
	}

	if limiter.rewardRateDeclineExpRatio == 0.0 {
		limiter.rewardRateDeclineExpRatio = DefaultRewardRatioDeclineExpRatio
	}

	if limiter.scoreSamplesSortInterval == 0 {
		limiter.scoreSamplesSortInterval = DefaultScoreSamplesSortIntervalSeconds * time.Second
	}

	if limiter.scoreSamplesMax > 0 {
		limiter.scoreSamples = make([]float64, int(limiter.scoreSamplesMax))
		limiter.scoreSamplesPos = 0
		limiter.scoreCutReady = false
	}

	if limiter.idealRewardRate == 0 {
		limiter.idealRewardRate = DefaultInitRewardRate
	}

	if limiter.idealPassRate == 0 {
		limiter.idealPassRate = DefaultInitPassRate
	}

	if limiter.periodInterval > 0 {
		limiter.beginTime = timeNow.Truncate(limiter.periodInterval)
		limiter.endTime = limiter.beginTime.Add(limiter.periodInterval)
	}
	if limiter.reserveInterval > 0 && limiter.endTime.After(limiter.beginTime.Add(limiter.reserveInterval)) {
		limiter.completionTime = limiter.endTime.Add(-limiter.reserveInterval)
	} else {
		limiter.completionTime = limiter.endTime
	}

	limiter.periodRewardBase, _ = limiter.RewardCounter.ClusterValue(0)
}

// request passed
func (limiter *ClusterLimiter) Take(v float64) bool {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	timeNow := time.Now()
	if timeNow.Before(limiter.beginTime) || timeNow.After(limiter.endTime) {
		return false
	}

	limiter.RequestCounter.Add(v)
	if rand.Float64() > limiter.workingPassRate {
		return false
	}

	clusterPred, _ := limiter.RewardCounter.ClusterValue(0)
	clusterCur := clusterPred.Sum - limiter.periodRewardBase.Sum
	if clusterCur+v > limiter.getIdealReward(timeNow) {
		return false
	}

	limiter.PassCounter.Add(v)
	return true
}

// reward feedback
func (limiter *ClusterLimiter) Reward(v float64) {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	timeNow := time.Now()
	if timeNow.Before(limiter.beginTime) || timeNow.After(limiter.endTime) {
		return
	}

	limiter.RewardCounter.Add(v)
}

// request passed with score
func (limiter *ClusterLimiter) TakeWithScore(v float64, score float64) bool {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	timeNow := time.Now()
	if timeNow.Before(limiter.beginTime) || timeNow.After(limiter.endTime) {
		return false
	}

	if limiter.scoreSamplesMax > 0 {
		limiter.scoreSamples[limiter.scoreSamplesPos%limiter.scoreSamplesMax] = score
		limiter.scoreSamplesPos++
	}

	limiter.RequestCounter.Add(v)

	if limiter.scoreCutReady == false || limiter.scoreSamplesMax == 0 {
		if rand.Float64() > limiter.workingPassRate {
			return false
		}
	} else {
		if score < limiter.scoreCutValue {
			return false
		}
	}

	clusterPred, _ := limiter.RewardCounter.ClusterValue(0)
	clusterCur := clusterPred.Sum - limiter.periodRewardBase.Sum
	if clusterCur+v > limiter.getIdealReward(timeNow) {
		return false
	}

	limiter.PassCounter.Add(v)
	return true
}

// request passed and reward for short
func (limiter *ClusterLimiter) Acquire(v float64) bool {
	if limiter.Take(v) {
		limiter.Reward(v)
		return true
	} else {
		return false
	}
}

// request passed with score and reward for short
func (limiter *ClusterLimiter) AcquireWithScore(v float64, score float64) bool {
	if limiter.TakeWithScore(v, score) {
		limiter.Reward(v)
		return true
	} else {
		return false
	}
}

func (limiter *ClusterLimiter) SetRewardTarget(target float64) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.rewardTarget = target
}

func (limiter *ClusterLimiter) GetRewardTarget() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.rewardTarget
}

//
func (limiter *ClusterLimiter) IdealReward() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getIdealReward(time.Now())
}

func (limiter *ClusterLimiter) getIdealReward(t time.Time) float64 {
	timeNow := time.Now()
	if timeNow.Before(limiter.beginTime) || !limiter.beginTime.Before(limiter.endTime) {
		return 0
	}

	if timeNow.After(limiter.endTime) {
		return limiter.rewardTarget
	}

	if limiter.discardPreviousData && limiter.initTime.Before(limiter.endTime) &&
		limiter.initTime.After(limiter.beginTime) {
		targetTotalReward := limiter.rewardTarget
		idealReward := (targetTotalReward) *
			float64(t.UnixNano()-limiter.initTime.UnixNano()) /
			float64(limiter.completionTime.UnixNano()-limiter.beginTime.UnixNano())
		if idealReward > limiter.rewardTarget {
			idealReward = limiter.rewardTarget
		}
		if idealReward < 0 {
			idealReward = 0
		}
		return idealReward
	} else {
		targetTotalReward := limiter.rewardTarget
		idealReward := targetTotalReward *
			float64(t.UnixNano()-limiter.beginTime.UnixNano()) /
			float64(limiter.completionTime.UnixNano()-limiter.beginTime.UnixNano())
		if idealReward > limiter.rewardTarget {
			idealReward = limiter.rewardTarget
		}
		if idealReward < 0 {
			idealReward = 0
		}
		return idealReward
	}
}

// Lag time from ideal reward
func (limiter *ClusterLimiter) LagTime(reward float64, t time.Time) float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getLagTime(reward, t)
}

func (limiter *ClusterLimiter) getLagTime(reward float64, t time.Time) float64 {
	if limiter.rewardTarget == 0 || limiter.endTime.After(limiter.beginTime) == false {
		return 0
	}

	idealReward := limiter.getIdealReward(t)
	interval := float64(limiter.completionTime.UnixNano()-limiter.beginTime.UnixNano()) / 1e9
	return (idealReward - reward) * interval / limiter.rewardTarget
}

// limiters's current pass rate
func (limiter *ClusterLimiter) PassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.workingPassRate
}

// score discrimination threshold for TakeWithScore
func (limiter *ClusterLimiter) ScoreCut() (bool, float64) {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.scoreCutReady, limiter.scoreCutValue
}

// ideal pass rate
func (limiter *ClusterLimiter) IdealPassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.idealPassRate
}

// ideal reward rate
func (limiter *ClusterLimiter) IdealRewardRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.idealRewardRate
}

// check whether expired
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

			limiter.periodRewardBase, _ = limiter.RewardCounter.ClusterValue(0)
		}
		limiter.expired = false
		return limiter.expired
	} else {
		limiter.expired = timeNow.After(limiter.endTime)
		return limiter.expired
	}
}

// update data heartbeat
func (limiter *ClusterLimiter) Heartbeat() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now()
	if timeNow.After(limiter.endTime) || timeNow.Before(limiter.beginTime) {
		return
	}

	if limiter.rewardTarget == 0 {
		limiter.workingPassRate = 0.0
		limiter.idealPassRate = 0.0
		limiter.idealRewardRate = 1.0
		return
	}

	limiter.updateIdealRewardRate()
	limiter.updateIdealPassRate()
	limiter.updateWorkingPassRate()
	limiter.sortScoreSamples()
}

func (limiter *ClusterLimiter) updateIdealPassRate() {
	timeNow := time.Now()
	if timeNow.Before(limiter.lastIdealPassRateTime.Add(limiter.burstInterval)) {
		return
	}
	limiter.lastIdealPassRateTime = time.Now()

	if timeNow.Before(limiter.initTime.Add(limiter.burstInterval)) {
		limiter.workingPassRate = limiter.idealPassRate
		return
	}

	var _, lastLoadTime = limiter.RequestCounter.ClusterValue(-1)
	if timeNow.After(lastLoadTime.Add(limiter.burstInterval * 10)) {
		var curRequest, _ = limiter.RequestCounter.LocalStoreValue(0)
		var curIdealReward = limiter.getIdealReward(timeNow) * limiter.RequestCounter.LocalTrafficProportion()
		if curRequest.Count < limiter.prevLocalRequest.Count+DefaultUpdatePassRateMinCount {
			return
		}

		limiter.localRequestRecently = limiter.localRequestRecently.Decline(curRequest.Sub(limiter.prevLocalRequest), limiter.declineExpRatio)
		limiter.prevLocalRequest = curRequest
		limiter.prevLocalIdealReward.Sum = curIdealReward

		idealPassRate := (limiter.localIdealRewardRecently.Sum / limiter.localRequestRecently.Sum) / limiter.idealRewardRate
		if idealPassRate <= 0.0 {
			idealPassRate = 0.0
		}
		if idealPassRate > 1.0 {
			idealPassRate = 1.0
		}
		limiter.idealPassRate = limiter.idealPassRate*limiter.declineExpRatio + idealPassRate*(1-limiter.declineExpRatio)
		return
	} else {
		var lastRequest, lastRequestTime = limiter.RequestCounter.ClusterValue(-1)
		if limiter.prevClusterRequestTime.Before(limiter.initTime) {
			limiter.prevClusterRequest = lastRequest
			limiter.prevClusterRequestTime = lastRequestTime
			return
		}

		if lastRequest.Count < limiter.prevClusterRequest.Count+DefaultUpdatePassRateMinCount {
			return
		}

		var idealReward = limiter.rewardTarget * lastRequestTime.Sub(limiter.prevClusterRequestTime).Seconds() / limiter.endTime.Sub(limiter.beginTime).Seconds()

		limiter.clusterRequestRecently = limiter.clusterRequestRecently.Decline(lastRequest.Sub(limiter.prevClusterRequest), limiter.declineExpRatio)
		limiter.clusterIdealRewardRecently.Sum = limiter.clusterIdealRewardRecently.Sum*limiter.declineExpRatio +
			idealReward*(1-limiter.declineExpRatio)

		limiter.prevClusterRequest = lastRequest
		limiter.prevClusterRequestTime = lastRequestTime

		idealPassRate := (limiter.clusterIdealRewardRecently.Sum / limiter.clusterRequestRecently.Sum) / limiter.idealRewardRate
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
	if curPass.Count < limiter.prevLocalPass.Count+DefaultUpdateRewardRateMinCount {
		return
	}

	limiter.localPassRecently = limiter.localPassRecently.Decline(
		curPass.Sub(limiter.prevLocalPass), limiter.rewardRateDeclineExpRatio)
	limiter.localRewardRecently = limiter.localRewardRecently.Decline(
		curReward.Sub(limiter.prevLocalReward), limiter.rewardRateDeclineExpRatio)

	idealRewardRate := limiter.localRewardRecently.Sum / limiter.localPassRecently.Sum
	if idealRewardRate > 0 {
		limiter.idealRewardRate = limiter.idealRewardRate*limiter.rewardRateDeclineExpRatio +
			idealRewardRate*(1-limiter.rewardRateDeclineExpRatio)
	}

	limiter.prevLocalReward = curReward
	limiter.prevLocalPass = curPass

	return

}

func (limiter *ClusterLimiter) updateWorkingPassRate() {
	timeNow := time.Now()
	if timeNow.Before(limiter.initTime.Add(limiter.burstInterval * 2)) {
		limiter.workingPassRate = limiter.idealPassRate
		return
	}

	if timeNow.Before(limiter.lastWorkingPassRateTime.Add(limiter.burstInterval / 4)) {
		return
	}
	limiter.lastWorkingPassRateTime = time.Now()

	curReward, _ := limiter.RewardCounter.ClusterValue(0)
	curReward = curReward.Sub(limiter.periodRewardBase)

	lagTime := limiter.getLagTime(curReward.Sum, timeNow)
	if lagTime > 0 {
		smoothPassRate := limiter.idealPassRate * (1 + lagTime*1e9/
			(DefaultBoostBurstFactor*float64(limiter.burstInterval.Nanoseconds())))
		if limiter.maxBoostFactor > 1.0 && smoothPassRate > limiter.maxBoostFactor*limiter.idealPassRate {
			smoothPassRate = limiter.maxBoostFactor * limiter.idealPassRate
		}
		if smoothPassRate > 1.0 {
			limiter.workingPassRate = 1.0
		} else {
			limiter.workingPassRate = smoothPassRate
		}
	} else {
		smoothPassRate := limiter.idealPassRate * (1 + lagTime*4*1e9/
			(DefaultBoostBurstFactor*float64(limiter.burstInterval.Nanoseconds())))
		if smoothPassRate < 0 {
			limiter.workingPassRate = limiter.idealPassRate / 10000
		} else {
			limiter.workingPassRate = smoothPassRate
		}
	}

	if len(limiter.scoreSamplesSorted) > 0 && limiter.workingPassRate > 0 && limiter.workingPassRate < 1.0 {
		limiter.scoreCutValue = limiter.scoreSamplesSorted[
			int(float64(len(limiter.scoreSamplesSorted)-1)*(1-limiter.workingPassRate))]
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

// update metrics
func (limiter *ClusterLimiter) CollectMetrics() bool {
	if limiter.factory == nil || limiter.factory.Reporter == nil {
		return false
	}

	if reflect.ValueOf(limiter.factory.Reporter).IsNil() == true {
		return false
	}

	metrics := make(map[string]float64)

	metrics["working_pass_rate"] = limiter.PassRate()
	metrics["ideal_pass_rate"] = limiter.IdealPassRate()
	metrics["reward_rate"] = limiter.IdealRewardRate()

	metrics["reward_target"] = limiter.GetRewardTarget()
	metrics["ideal_reward"] = limiter.IdealReward()

	rewardCur, rewardTime := limiter.RewardCounter.ClusterValue(0)
	rewardCur = rewardCur.Sub(limiter.periodRewardBase)

	metrics["lag_time"] = limiter.LagTime(rewardCur.Sum, rewardTime)

	requestLocal, _ := limiter.RequestCounter.LocalValue(0)
	metrics["request_local_sum"] = requestLocal.Sum
	metrics["request_local_cnt"] = float64(requestLocal.Count)

	requestCluster, _ := limiter.RequestCounter.ClusterValue(0)
	metrics["request_estimated_sum"] = requestCluster.Sum
	metrics["request_estimated_cnt"] = float64(requestCluster.Count)

	requestLast, _ := limiter.RequestCounter.ClusterValue(-1)
	metrics["request_last_sum"] = requestLast.Sum
	metrics["request_last_cnt"] = float64(requestLast.Count)

	passLocal, _ := limiter.PassCounter.LocalValue(0)
	metrics["pass_local_sum"] = passLocal.Sum
	metrics["pass_local_cnt"] = float64(passLocal.Sum)

	passCluster, _ := limiter.PassCounter.ClusterValue(0)
	metrics["pass_estimated_sum"] = passCluster.Sum
	metrics["pass_estimated_cnt"] = float64(passCluster.Count)
	passLast, _ := limiter.PassCounter.ClusterValue(-1)
	metrics["pass_last_sum"] = passLast.Sum
	metrics["pass_last_cnt"] = float64(passLast.Count)

	rewardLocal, _ := limiter.RewardCounter.LocalValue(0)
	metrics["reward_local_sum"] = rewardLocal.Sum
	metrics["reward_local_cnt"] = float64(rewardLocal.Count)
	rewardCluster, _ := limiter.RewardCounter.ClusterValue(0)
	metrics["reward_estimated_sum"] = rewardCluster.Sum
	metrics["reward_estimated_cnt"] = float64(rewardCluster.Count)
	rewardLast, _ := limiter.RewardCounter.ClusterValue(-1)
	metrics["reward_last_sum"] = rewardLast.Sum
	metrics["reward_last_cnt"] = float64(rewardLast.Count)

	metrics["request_local_traffic_proportion"] = limiter.RequestCounter.LocalTrafficProportion()
	metrics["reward_local_traffic_proportion"] = limiter.RewardCounter.LocalTrafficProportion()

	scoreFlag, scoreCutValue := limiter.ScoreCut()
	if scoreFlag == false {
		scoreCutValue = 1.0
	}
	metrics["score_cut"] = scoreCutValue

	limiter.factory.Reporter.Update(limiter.name, metrics)
	return true
}
