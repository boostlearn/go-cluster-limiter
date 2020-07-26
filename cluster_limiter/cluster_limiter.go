package cluster_limiter

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"math/rand"
	"sync"
	"time"
)

type ClusterLimiter struct {
	mu sync.RWMutex

	name string
	lbs  []string
	initTime  time.Time

	beginTime time.Time
	endTime   time.Time
	periodInterval time.Duration

	targetReward        float64
	discardPreviousData bool

	RequestCounter *cluster_counter.ClusterCounter
	PassCounter    *cluster_counter.ClusterCounter
	RewardCounter  *cluster_counter.ClusterCounter

	boostInterval  time.Duration
	maxBoostFactor float64
	silentInterval time.Duration
	burstInterval  time.Duration

	realPassRate  float64
	idealPassRate float64
}

func (limiter *ClusterLimiter) Init() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	limiter.initTime = timeNow

	if limiter.boostInterval.Truncate(time.Second) == 0 { // 未设置
		limiter.boostInterval = 60 * time.Second
	}

	if limiter.silentInterval.Truncate(time.Second) == 0 {
		limiter.silentInterval = 10 * time.Second
	}

	if limiter.burstInterval.Truncate(time.Second) == 0 {
		limiter.burstInterval = 5 * time.Second
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
	if clusterPred > limiter.targetReward {
		return false
	}

	limiter.PassCounter.Add(v)
	return true
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
	if limiter.discardPreviousData && limiter.initTime.Before(limiter.endTime) &&
		limiter.initTime.After(limiter.beginTime) {
		targetTotalReward := limiter.targetReward
		pacingReward := (targetTotalReward) *
			float64(t.UnixNano()-limiter.initTime.UnixNano()) /
			float64(limiter.endTime.UnixNano()-limiter.beginTime.UnixNano()-limiter.silentInterval.Nanoseconds())
		if pacingReward > limiter.targetReward {
			pacingReward = limiter.targetReward
		}
		return pacingReward
	} else {
		targetTotalReward := limiter.targetReward
		pacingReward := float64(targetTotalReward) *
			float64(t.UnixNano()-limiter.beginTime.UnixNano()) /
			float64(limiter.endTime.UnixNano()-limiter.beginTime.UnixNano()-limiter.silentInterval.Nanoseconds())
		if pacingReward > limiter.targetReward {
			pacingReward = limiter.targetReward
		}
		return pacingReward
	}
}

func (limiter *ClusterLimiter) LostTime(reward float64, t time.Time) float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getLostTime(reward, t)
}

func (limiter *ClusterLimiter) getLostTime(reward float64, t time.Time) float64 {
	if limiter.targetReward == 0 || limiter.endTime.After(limiter.beginTime) == false {
		return 0
	}

	if t.Before(limiter.initTime.Add(limiter.silentInterval)) ||
		t.After(limiter.endTime.Add(-limiter.silentInterval)) ||
		t.Before(limiter.beginTime.Add(limiter.silentInterval)) {
		return 0
	}

	pacingReward := limiter.getPacingReward(t)
	interval := float64(limiter.endTime.UnixNano()-limiter.beginTime.UnixNano()) / 1e9
	return (pacingReward - reward) * interval / limiter.targetReward
}

// 获取通过率
func (limiter *ClusterLimiter) PassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.realPassRate
}

// 获取通过率
func (limiter *ClusterLimiter) IdealPassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.idealPassRate
}

func (limiter *ClusterLimiter) HeartBeat() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	if timeNow.After(limiter.endTime) || timeNow.Before(limiter.beginTime) {
		return
	}

	if limiter.targetReward == 0 {
		limiter.realPassRate = 0.0
		limiter.idealPassRate = 0.0
		return
	}

	if limiter.updateIdealPassRate(-1) == false {
		limiter.updateIdealPassRate(-2)
	}

	limiter.updateRealPassRate()
}

func (limiter *ClusterLimiter) updateIdealPassRate(last int) bool {
	timeNow := time.Now()
	var curReward = limiter.getPacingReward(time.Now())
	var prevReward, prevRewardTime = limiter.RewardCounter.ClusterValue(last)
	if prevReward == 0.0 || prevRewardTime.Unix() == 0 {
		curRequest, _ := limiter.RequestCounter.ClusterValue(0)
		if curRequest > 0 {
			limiter.idealPassRate = curReward / curRequest
		}
		return true
	}

	var prevPacingReward = limiter.getPacingReward(prevRewardTime)
	if prevPacingReward == 0 {
		return true
	}

	var prevRequest, _ = limiter.RequestCounter.ClusterValue(-1)
	var prevPass, _ = limiter.PassCounter.ClusterValue(-1)

	var curRequest, _ = limiter.RequestCounter.ClusterValue(0)
	var curPass, _ = limiter.PassCounter.ClusterValue(0)
	curPacingTarget := limiter.getPacingReward(timeNow)

	if prevRequest == curRequest ||
		prevPass == curPass || prevReward == curReward ||
		prevPacingReward == curPacingTarget {
		return false
	}

	idealPassRate := (curPacingTarget - prevPacingReward) * (curPass - prevPass) /
		(curRequest - prevRequest) * (curReward - prevReward)

	if idealPassRate <= 0.0 || idealPassRate > 1.0 {
		return false
	}
	limiter.idealPassRate = limiter.idealPassRate*0.5 + idealPassRate*0.5
	return true
}

func (limiter *ClusterLimiter) updateRealPassRate() {
	timeNow := time.Now().Truncate(time.Second)
	if timeNow.Before(limiter.initTime.Add(limiter.silentInterval)) ||
		timeNow.After(limiter.endTime.Add(-limiter.silentInterval)) ||
		timeNow.Before(limiter.beginTime.Add(limiter.silentInterval)) {
		limiter.realPassRate = limiter.idealPassRate
		return
	}

	curReward, _ := limiter.RewardCounter.ClusterValue(0)
	lostTime := limiter.getLostTime(curReward, timeNow)
	if lostTime > 0 {
		smoothPassRate := limiter.idealPassRate * (1 + lostTime*1e9/
			float64(limiter.boostInterval.Nanoseconds()))
		if limiter.maxBoostFactor > 1.0 && smoothPassRate > limiter.maxBoostFactor*limiter.idealPassRate {
			smoothPassRate = limiter.maxBoostFactor * limiter.idealPassRate
		}
		if smoothPassRate > 1.0 {
			limiter.realPassRate = 1.0
		} else {
			limiter.realPassRate = smoothPassRate
		}
	} else {
		smoothPassRate := limiter.idealPassRate * (1 + lostTime*4*1e9/
			float64(limiter.boostInterval.Nanoseconds()))
		if smoothPassRate < 0 {
			limiter.realPassRate = limiter.idealPassRate / 10000
		} else {
			limiter.realPassRate = smoothPassRate
		}
	}
}
