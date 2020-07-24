package cluster_level_limiter

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"math/rand"
	"sort"
	"sync"
	"time"
)

type ClusterLevelLimiter struct {
	mu sync.RWMutex

	name string
	lbs  []string

	discardPreviousData bool

	HighRequestCounter *cluster_counter.ClusterCounter
	HighPassCounter    *cluster_counter.ClusterCounter
	HighRewardCounter  *cluster_counter.ClusterCounter

	MiddleRequestCounter *cluster_counter.ClusterCounter
	MiddlePassCounter    *cluster_counter.ClusterCounter
	MiddleRewardCounter  *cluster_counter.ClusterCounter

	LowRequestCounter *cluster_counter.ClusterCounter
	LowPassCounter    *cluster_counter.ClusterCounter
	LowRewardCounter  *cluster_counter.ClusterCounter

	startTime         time.Time
	endTime           time.Time
	initTime          time.Time
	resetDataInterval time.Duration
	boostInterval     time.Duration
	silentInterval    time.Duration

	maxBoostFactor float64

	levelSampleMax int64

	totalTarget   int64
	curPassRate   float64
	idealPassRate float64

	lowValueCut    float64
	hasLowLevelCut bool

	highValueCut    float64
	hasHighLevelCut bool

	reserveRate float64

	lowLevel  float64
	highLevel float64

	levelSamples     []float64
	levelSampleIdx   int64
	levelSampleAdded int64

	prevMiddleRequest int64
	prevMiddlePass    int64
	prevMiddleReward  int64

	prevHighRequest int64
	prevHighPass    int64
	prevHighReward  int64

	prevLowRequest int64
	prevLowPass    int64
	prevLowReward  int64

	prevPacingTarget int64
	prevUpdateTime   time.Time
}

func (limiter *ClusterLevelLimiter) Reward(v int64, level float64) {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	if limiter.hasHighLevelCut && level > limiter.highValueCut {
		limiter.HighRewardCounter.Add(v)
	} else if limiter.hasLowLevelCut && level < limiter.lowValueCut {
		limiter.LowRewardCounter.Add(v)
	} else {
		limiter.MiddleRewardCounter.Add(v)
	}

}

func (limiter *ClusterLevelLimiter) Take(v int64, level float64) bool {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	if limiter.totalTarget == 0 {
		return false
	}

	if limiter.levelSampleMax > 0 {
		limiter.levelSamples[limiter.levelSampleIdx] = level
		limiter.levelSampleIdx = (limiter.levelSampleAdded + 1) % limiter.levelSampleMax
		limiter.levelSampleAdded += 1
	}

	if limiter.hasHighLevelCut && level > limiter.highValueCut {
		limiter.HighRequestCounter.Add(v)
		limiter.HighPassCounter.Add(v)
		return true
	}

	if limiter.hasLowLevelCut && level < limiter.lowValueCut {
		limiter.LowRequestCounter.Add(v)
		return false
	}

	limiter.MiddleRequestCounter.Add(v)
	if rand.Float64() > limiter.curPassRate {
		return false
	}
	limiter.MiddlePassCounter.Add(v)

	return true
}

func (limiter *ClusterLevelLimiter) SetTarget(target int64) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.totalTarget = target
}

func (limiter *ClusterLevelLimiter) GetTarget() int64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.totalTarget
}

// 获取通过率
func (limiter *ClusterLevelLimiter) PassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.curPassRate
}

// 获取通过率
func (limiter *ClusterLevelLimiter) LowLevel() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.lowLevel
}

// 获取通过率
func (limiter *ClusterLevelLimiter) LowLevelCut() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.lowValueCut
}

// 获取通过率
func (limiter *ClusterLevelLimiter) HighLevel() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.highLevel
}

// 获取通过率
func (limiter *ClusterLevelLimiter) HighLevelCut() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.highValueCut
}

// 获取通过率
func (limiter *ClusterLevelLimiter) IdealPassRate() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.idealPassRate
}

func (limiter *ClusterLevelLimiter) Init() {
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
	limiter.highLevel = 1.0
	limiter.lowLevel = 0.0

	if limiter.reserveRate == 0.0 {
		limiter.reserveRate = 0.9
	}

	if limiter.maxBoostFactor == 0.0 {
		limiter.maxBoostFactor = 2.0
	}
}

// 更新通过率
func (limiter *ClusterLevelLimiter) Update() {
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

func (limiter *ClusterLevelLimiter) PacingReward() int64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getPacingReward()
}

func (limiter *ClusterLevelLimiter) getPacingReward() int64 {
	targetTotalReward := limiter.totalTarget
	pacingReward := int64(float64(targetTotalReward) *
		float64(time.Now().UnixNano()-limiter.startTime.UnixNano()) / float64(limiter.endTime.UnixNano()-limiter.startTime.UnixNano()))
	return pacingReward
}

func (limiter *ClusterLevelLimiter) updateIdealPassRate() {
	timeNow := time.Now()
	if time.Now().UnixNano()-limiter.prevUpdateTime.UnixNano() < limiter.silentInterval.Nanoseconds() {
		return
	}

	pacingTarget := limiter.getPacingReward()

	if pacingTarget < 0 {
		limiter.idealPassRate = 0.0
		return
	}

	targetUnit := int64(float64(limiter.totalTarget) / float64(limiter.endTime.Unix()-limiter.startTime.Unix()))

	highRequestCur := limiter.HighRequestCounter.ClusterPredict()
	highPassCur := limiter.HighPassCounter.ClusterPredict()
	highRewardCur := limiter.HighRewardCounter.ClusterPredict()
	middleRequestCur := limiter.MiddleRequestCounter.ClusterPredict()
	middlePassCur := limiter.MiddlePassCounter.ClusterPredict()
	middleRewardCur := limiter.MiddleRewardCounter.ClusterPredict()
	lowRequestCur := limiter.LowRequestCounter.ClusterPredict()
	lowPassCur := limiter.LowPassCounter.ClusterPredict()
	lowRewardCur := limiter.LowRewardCounter.ClusterPredict()
	if middleRequestCur <= limiter.prevMiddleRequest ||
		middlePassCur < limiter.prevMiddlePass ||
		middleRewardCur < limiter.prevMiddleReward ||
		highRequestCur < limiter.prevHighRequest ||
		highPassCur < limiter.prevHighPass ||
		highRewardCur < limiter.prevHighReward ||
		lowRequestCur < limiter.prevLowRequest ||
		lowPassCur < limiter.prevLowPass ||
		lowRewardCur < limiter.prevLowReward ||
		pacingTarget < limiter.prevPacingTarget {
		limiter.prevHighRequest = highRequestCur
		limiter.prevHighPass = highPassCur
		limiter.prevHighReward = highRewardCur

		limiter.prevMiddleRequest = middleRequestCur
		limiter.prevMiddlePass = middlePassCur
		limiter.prevMiddleReward = middleRewardCur

		limiter.prevLowRequest = lowRequestCur
		limiter.prevLowPass = lowPassCur
		limiter.prevLowReward = lowRewardCur

		limiter.prevPacingTarget = pacingTarget
		limiter.prevUpdateTime = timeNow
		return
	}

	request := highRequestCur + middleRequestCur + lowRequestCur - limiter.prevMiddleRequest - limiter.prevHighRequest - limiter.prevLowRequest
	pass := highPassCur + middlePassCur + lowPassCur - limiter.prevHighPass - limiter.prevMiddlePass - limiter.prevLowPass
	reward := highRewardCur + middleRewardCur + lowRewardCur - limiter.prevHighReward - limiter.prevMiddleReward - limiter.prevLowReward

	if limiter.prevPacingTarget > 0 {
		target := pacingTarget - limiter.prevPacingTarget
		var idealPassRate float64
		if (limiter.idealPassRate == 0.0 || middlePassCur < targetUnit*5) && target > 0 && request > 0 {
			idealPassRate = float64(target) / float64(request)
			if idealPassRate > 1.0 {
				idealPassRate = 1.0
			}

			//idealPassRate = idealPassRate/2
			limiter.idealPassRate = idealPassRate
		} else if reward > 0 && request > 0 && pass > 0 && target > 0 {
			idealPassRate = float64(target*pass+1) / float64(reward*request+1)
			if idealPassRate > 1.0 {
				idealPassRate = 1.0
			}

			//idealPassRate = idealPassRate/2
			limiter.idealPassRate = limiter.idealPassRate*0.5 + idealPassRate*0.5
		}
	}

	if limiter.lowLevel == 0.0 {
		limiter.lowLevel = limiter.idealPassRate / 2
	}

	if float64(highRewardCur-limiter.prevHighReward) > float64(pacingTarget-limiter.prevPacingTarget)*limiter.reserveRate*1.05 && limiter.highLevel < 1.0 {
		limiter.highLevel += limiter.idealPassRate * 0.02
		if limiter.highLevel > 1.0 {
			limiter.highLevel = 1.0
			limiter.hasHighLevelCut = false
		}
	} else if float64(highRewardCur-limiter.prevHighReward) < float64(pacingTarget-limiter.prevPacingTarget)*limiter.reserveRate*0.95 {
		limiter.highLevel -= limiter.idealPassRate * 0.02
		if limiter.highLevel < 0 {
			limiter.highLevel = 0
		}
	}

	if limiter.curPassRate < limiter.idealPassRate {
		if limiter.lowLevel == 0.0 {
			limiter.lowLevel = (1 - limiter.idealPassRate) / 4
		}
		limiter.lowLevel += limiter.idealPassRate * 0.02
		if limiter.lowLevel > 1.0 {
			limiter.lowLevel = 1.0
		}
	} else if limiter.curPassRate > limiter.idealPassRate*1.5 {
		limiter.lowLevel -= limiter.idealPassRate * 0.02
		if limiter.lowLevel < 0 {
			limiter.lowLevel = 0
		}
	}
	limiter.updateCutLevel()

	limiter.prevUpdateTime = timeNow

	limiter.prevHighRequest = highRequestCur
	limiter.prevHighPass = highPassCur
	limiter.prevHighReward = highRewardCur

	limiter.prevMiddleRequest = middleRequestCur
	limiter.prevMiddlePass = middlePassCur
	limiter.prevMiddleReward = middleRewardCur

	limiter.prevLowRequest = lowRequestCur
	limiter.prevLowPass = lowPassCur
	limiter.prevLowReward = lowRewardCur

	limiter.prevPacingTarget = pacingTarget
}

func (limiter *ClusterLevelLimiter) LostTime() float64 {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()

	return limiter.getLostTime()
}

func (limiter *ClusterLevelLimiter) getLostTime() float64 {
	if limiter.resetDataInterval > 0 && (
		time.Now().UnixNano() < limiter.initTime.UnixNano()+int64(limiter.silentInterval.Nanoseconds())/2 ||
			time.Now().UnixNano() >= limiter.endTime.UnixNano()-int64(limiter.silentInterval.Nanoseconds())/2 ||
			time.Now().UnixNano() < limiter.startTime.UnixNano()+int64(limiter.silentInterval.Nanoseconds())/2) {
		return 0.0
	}

	curRewardValue := limiter.MiddleRewardCounter.ClusterPredict() + limiter.HighRewardCounter.ClusterPredict() + limiter.LowRewardCounter.ClusterPredict()
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

func (limiter *ClusterLevelLimiter) updatePassRate() {
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

func (limiter *ClusterLevelLimiter) updateCutLevel() {
	sampleNum := limiter.levelSampleAdded
	if sampleNum > limiter.levelSampleMax {
		sampleNum = limiter.levelSampleMax
	}

	if sampleNum < 100 {
		return
	}

	data := append([]float64{}, limiter.levelSamples[:sampleNum]...)

	limiter.mu.Unlock()
	sort.Stable(sort.Float64Slice(data))
	limiter.mu.Lock()

	if limiter.highLevel < 1.0 && limiter.highLevel >= 0.0 {
		limiter.highValueCut = data[int(float64(sampleNum)*limiter.highLevel)]
		limiter.hasHighLevelCut = true
	} else {
		limiter.hasHighLevelCut = false
		limiter.highLevel = 1.0
	}

	if limiter.lowLevel > 0.0 && limiter.lowLevel < 1.0 {
		limiter.lowValueCut = data[int(float64(sampleNum)*limiter.lowLevel)]
		limiter.hasLowLevelCut = true
	} else {
		limiter.hasLowLevelCut = false
		limiter.lowLevel = 0
	}

}
