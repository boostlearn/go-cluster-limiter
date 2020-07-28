package cluster_limiter

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"math/rand"
	"sync"
	"time"
    "fmt"
)

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
	lastUpdatePassRateTime time.Time

	realPassRate  float64
	idealPassRate float64
}

func (limiter *ClusterLimiter) Init() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now().Truncate(time.Second)
	limiter.initTime = timeNow
	if limiter.burstInterval.Truncate(time.Second) == 0 {
		limiter.burstInterval = 5 * time.Second
	}

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
	if clusterPred > limiter.targetReward {
		return false
	}

	if clusterPred > limiter.getPacingReward(time.Now().Add(limiter.burstInterval)) {
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

func (limiter *ClusterLimiter) LostTime(reward float64, t time.Time) float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getLostTime(reward, t)
}

func (limiter *ClusterLimiter) getLostTime(reward float64, t time.Time) float64 {
	if limiter.targetReward == 0 || limiter.endTime.After(limiter.beginTime) == false {
		return 0
	}

	//if t.Before(limiter.initTime.Add(limiter.burstInterval)) ||
	//	t.After(limiter.endTime.Add(-limiter.burstInterval)) ||
	//	t.Before(limiter.beginTime.Add(limiter.burstInterval)) {
	//	return 0
	//}

	pacingReward := limiter.getPacingReward(t)
	interval := float64(limiter.completionTime.UnixNano()-limiter.beginTime.UnixNano()) / 1e9
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

	if timeNow.After(limiter.lastUpdatePassRateTime.Add(limiter.burstInterval)) {
		if limiter.updateIdealPassRate(-2) == false {
			limiter.updateIdealPassRate(-4)
		}

        limiter.lastUpdatePassRateTime = time.Now()
	}
	limiter.updateRealPassRate()
}

func (limiter *ClusterLimiter) updateIdealPassRate(last int) bool {
	timeNow := time.Now()
	var _, lastLoadTime = limiter.RewardCounter.ClusterValue(last)
	if lastLoadTime.Unix() == 0 {
		var curPacingReward = limiter.getPacingReward(timeNow)
		var prevReward, prevRewardTime = limiter.RewardCounter.LocalStoreValue(last)
		if prevRewardTime.Unix() == 0 {
			return true
		}

		var curPass, _ = limiter.PassCounter.LocalStoreValue(0)
		var curRequest, _ = limiter.RequestCounter.LocalStoreValue(0)
		var curReward, _ = limiter.RewardCounter.LocalStoreValue(0)
		if curPass == 0 || curPacingReward == 0 || limiter.idealPassRate == 0.0 {
			if curRequest > 0 {
				initPacingReward := limiter.getPacingReward(limiter.initTime)
				limiter.idealPassRate = (curPacingReward - initPacingReward) / curRequest
				if limiter.idealPassRate > 1.0 {
					limiter.idealPassRate = 1.0
				}
			}
			return true
		}

		var prevPacingReward = limiter.getPacingReward(prevRewardTime)
		if prevPacingReward == 0 {
			return true
		}
		var prevRequest, _ = limiter.RequestCounter.LocalStoreValue(last)
		var prevPass, _ = limiter.PassCounter.LocalStoreValue(last)
		if prevRequest == curRequest ||
			prevPass == curPass || prevReward == curPacingReward ||
			prevPacingReward == curPacingReward {
			return false
		}

		idealPassRate := (curPacingReward - prevPacingReward) * (curPass - prevPass) /
			((curRequest - prevRequest) * (curReward - prevReward))

		if idealPassRate <= 0.0 {
			idealPassRate = 0.0
		}
		if idealPassRate > 1.0 {
			idealPassRate = 1.0
		}
		limiter.idealPassRate = limiter.idealPassRate*0.5 + idealPassRate*0.5
		return true
	} else {
		var curPacingReward = limiter.getPacingReward(timeNow)
		var prevReward, prevRewardTime = limiter.RewardCounter.ClusterValue(last)
		var curPass, _ = limiter.PassCounter.ClusterValue(0)
		var curRequest, _ = limiter.RequestCounter.ClusterValue(0)
		var curReward, _ = limiter.RewardCounter.ClusterValue(0)
		if curPass == 0 || curReward == 0 || limiter.idealPassRate == 0 {
			if curRequest > 0 {
				initPacingReward := limiter.getPacingReward(limiter.initTime)
				limiter.idealPassRate = (curPacingReward - initPacingReward) / curRequest
				if limiter.idealPassRate > 1.0 {
					limiter.idealPassRate = 1.0
				}
			}
			return true
		}

		var prevPacingReward = limiter.getPacingReward(prevRewardTime)
		if prevPacingReward == 0 {
			return true
		}
		var prevRequest, _ = limiter.RequestCounter.ClusterValue(last)
		var prevPass, _ = limiter.PassCounter.ClusterValue(last)
		if prevRequest == curRequest ||
			prevPass == curPass || prevReward == curReward ||
			prevPacingReward == curPacingReward {
			return false
		}

		idealPassRate := (curPacingReward - prevPacingReward) * (curPass - prevPass) /
			((curRequest - prevRequest) * (curReward - prevReward))

        fmt.Println(">>>>>>>", (curPacingReward - prevPacingReward), " ",  (curPass - prevPass),  " ",
        (curRequest - prevRequest), " ",  (curReward - prevReward),  " ", idealPassRate)
		if idealPassRate <= 0.0 {
			idealPassRate = 0.0
		}
		if idealPassRate > 1.0 {
			idealPassRate = 1.0
		}
		limiter.idealPassRate = limiter.idealPassRate*0.5 + idealPassRate*0.5
		return true
	}
}

func (limiter *ClusterLimiter) updateRealPassRate() {
	timeNow := time.Now().Truncate(time.Second)
	if timeNow.Before(limiter.initTime.Add(limiter.burstInterval)) ||
		timeNow.After(limiter.endTime.Add(-limiter.burstInterval)) ||
		timeNow.Before(limiter.beginTime.Add(limiter.burstInterval)) {
		limiter.realPassRate = limiter.idealPassRate
		return
	}

	curReward, _ := limiter.RewardCounter.ClusterValue(0)
	lostTime := limiter.getLostTime(curReward, timeNow)
	if lostTime > 0 {
		smoothPassRate := limiter.idealPassRate * (1 + lostTime*1e9/
			float64(limiter.burstInterval.Nanoseconds()))
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
			float64(limiter.burstInterval.Nanoseconds()))
		if smoothPassRate < 0 {
			limiter.realPassRate = limiter.idealPassRate / 10000
		} else {
			limiter.realPassRate = smoothPassRate
		}
	}
}
