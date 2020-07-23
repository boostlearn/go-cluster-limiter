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

	discardPreviousData bool

	RequestCounter cluster_counter.ClusterCounterI
	PassCounter    cluster_counter.ClusterCounterI
	RewardCounter  cluster_counter.ClusterCounterI

	startTime      time.Time
	endTime        time.Time

	initTime       time.Time

	resetDataInterval  time.Duration

	boostInterval  time.Duration

	silentInterval time.Duration

	maxBoostFactor float64

	totalTarget   int64
	curPassRate   float64
	idealPassRate float64

	prevRequest      int64
	prevPass         int64
	prevReward       int64
	prevPacingTarget int64
	prevUpdateTime   time.Time
}

func (limiter *ClusterLimiter) Reward(v int64) {
	limiter.RewardCounter.Add(v)
}

func (limiter *ClusterLimiter) Take(v int64) bool {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	if limiter.totalTarget == 0 {
		return false
	}

	limiter.RequestCounter.Add(v)
	if rand.Float64() > limiter.curPassRate {
		return false
	}
	limiter.PassCounter.Add(v)
	return true
}

func (limiter *ClusterLimiter) SetTarget(target int64) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.totalTarget = target
}

func (limiter *ClusterLimiter) GetTarget() int64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.totalTarget
}

// 获取通过率
func (limiter *ClusterLimiter) PassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.curPassRate
}

// 获取通过率
func (limiter *ClusterLimiter) IdealPassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.idealPassRate
}

func (limiter *ClusterLimiter) Init() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now()
	nowTs := timeNow.Unix()
	if limiter.resetDataInterval > 0 {
		interval := int64(limiter.resetDataInterval.Seconds())
		startTs := interval * (nowTs / interval)
		endTs := startTs + interval
		limiter.endTime = timeNow.Add(time.Duration(endTs-nowTs) * time.Second)
		limiter.startTime = timeNow.Add(time.Duration(startTs-nowTs) * time.Second)
	}

	limiter.initTime = time.Now()
}

// 更新通过率
func (limiter *ClusterLimiter) Update() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	timeNow := time.Now()
	nowTs := timeNow.Unix()
	if limiter.resetDataInterval.Seconds() > 0 && limiter.endTime.Unix() <= nowTs {
		interval := int64(limiter.resetDataInterval.Seconds())
		startTs := interval * (nowTs / interval)
		endTs := startTs + interval
		limiter.endTime = timeNow.Add(time.Duration(endTs-nowTs) * time.Second)
		limiter.startTime = timeNow.Add(time.Duration(startTs-nowTs) * time.Second)
	}

	if limiter.resetDataInterval > 0 && (
		time.Now().UnixNano() < limiter.initTime.UnixNano()+int64(limiter.silentInterval.Nanoseconds())/2 ||
			time.Now().UnixNano() >= limiter.endTime.UnixNano()-int64(limiter.silentInterval.Nanoseconds())/2 ||
			time.Now().UnixNano() < limiter.startTime.UnixNano()+int64(limiter.silentInterval.Nanoseconds())/2) {
		limiter.curPassRate = limiter.idealPassRate
		return
	}

	if limiter.totalTarget == 0 {
		limiter.curPassRate = 0.0
		limiter.idealPassRate = 0.0
		return
	}

	limiter.updateIdealPassRate()
	limiter.updatePassRate()

	if limiter.resetDataInterval > 0 &&
		time.Now().UnixNano() < limiter.initTime.UnixNano()+int64(limiter.silentInterval.Nanoseconds())/2 {
		limiter.curPassRate = limiter.idealPassRate
		return
	}
}

func (limiter *ClusterLimiter) PacingReward() int64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getPacingReward()
}

func (limiter *ClusterLimiter) getPacingReward() int64 {
	targetTotalReward := limiter.totalTarget
	pacingReward := int64(float64(targetTotalReward) *
		float64(time.Now().UnixNano()-limiter.startTime.UnixNano()) / float64(limiter.endTime.UnixNano()-limiter.startTime.UnixNano()))
	return pacingReward
}

func (limiter *ClusterLimiter) updateIdealPassRate() {
	timeNow := time.Now()
	if time.Now().UnixNano()-limiter.prevUpdateTime.UnixNano() < limiter.silentInterval.Nanoseconds() {
		return
	}

	pacingTarget := limiter.getPacingReward()
	targetUnit := int64(float64(limiter.totalTarget) / float64(limiter.endTime.Unix()-limiter.startTime.Unix()))

	requestCur := limiter.RequestCounter.ClusterPredict()
	passCur := limiter.PassCounter.ClusterPredict()
	rewardCur := limiter.RewardCounter.ClusterPredict()
	if requestCur <= limiter.prevRequest ||
		passCur < limiter.prevPass ||
		rewardCur < limiter.prevReward ||
		pacingTarget < limiter.prevPacingTarget {
		limiter.prevRequest = requestCur
		limiter.prevPass = passCur
		limiter.prevReward = rewardCur
		limiter.prevPacingTarget = pacingTarget
		limiter.prevUpdateTime = timeNow
		return
	}

	request := requestCur - limiter.prevRequest
	pass := passCur - limiter.prevPass
	reward := rewardCur - limiter.prevReward
	if limiter.prevPacingTarget > 0 {
		target := pacingTarget - limiter.prevPacingTarget
		var idealPassRate float64
		if (limiter.idealPassRate == 0.0 || passCur < targetUnit*5) && target > 0 && request > 0 {
			idealPassRate = float64(target) / float64(request)
			if idealPassRate > 1.0 {
				idealPassRate = 1.0
			}
			limiter.idealPassRate = idealPassRate
		} else if reward > 0 && request > 0 && pass > 0 && target > 0 {
			idealPassRate = float64(target*pass+1) / float64(reward*request+1)
			if idealPassRate > 1.0 {
				idealPassRate = 1.0
			}
			limiter.idealPassRate = limiter.idealPassRate*0.5 + idealPassRate*0.5
		}
	}

	limiter.prevUpdateTime = timeNow
	limiter.prevRequest = requestCur
	limiter.prevPass = passCur
	limiter.prevReward = rewardCur
	limiter.prevPacingTarget = pacingTarget
}

func (limiter *ClusterLimiter) LostTime() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getLostTime()
}

func (limiter *ClusterLimiter) getLostTime() float64 {
	if limiter.resetDataInterval > 0 && (
		time.Now().UnixNano() < limiter.initTime.UnixNano()+int64(limiter.silentInterval.Nanoseconds())/2 ||
			time.Now().UnixNano() >= limiter.endTime.UnixNano()-int64(limiter.silentInterval.Nanoseconds())/2 ||
			time.Now().UnixNano() < limiter.startTime.UnixNano()+int64(limiter.silentInterval.Nanoseconds())/2) {
		limiter.curPassRate = limiter.idealPassRate
		return 0
	}

	curRewardValue := limiter.RewardCounter.ClusterPredict()
	pacingReward := limiter.getPacingReward()
	interval := float64(limiter.endTime.UnixNano()-limiter.startTime.UnixNano()) / 1e9
	if interval < 1.0 {
		return 0
	}

	targetUnit := float64(limiter.totalTarget) / interval
	if targetUnit == 0 {
		return 0
	}

	lostTime := float64(pacingReward-curRewardValue) / targetUnit
	return lostTime
}

func (limiter *ClusterLimiter) updatePassRate() {
	needBoostTime := limiter.getLostTime()

	if limiter.boostInterval == 0 { // 未设置
		limiter.boostInterval = time.Duration(60) * time.Second
	}

	if needBoostTime > 0 {
		smoothPassRate := limiter.idealPassRate * (1 + needBoostTime*1e9/
			float64(limiter.boostInterval.Nanoseconds()))
		if limiter.maxBoostFactor > 1.0 && smoothPassRate > limiter.maxBoostFactor*limiter.idealPassRate {
			smoothPassRate = limiter.maxBoostFactor * limiter.idealPassRate
		}
		if smoothPassRate > 1.0 {
			limiter.curPassRate = 1.0
		} else {
			limiter.curPassRate = smoothPassRate
		}
	} else {
		smoothPassRate := limiter.idealPassRate * (1 + needBoostTime*4*1e9/
			float64(limiter.boostInterval.Nanoseconds()))
		if smoothPassRate < 0 {
			limiter.curPassRate = limiter.idealPassRate / 10000
		} else {
			limiter.curPassRate = smoothPassRate
		}
	}
}
