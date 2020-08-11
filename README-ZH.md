>本项目提供轻量、高效的集群流控服务。有以下特点：
>* 流控算法运行在集群的各个流控节点上，不需大量的同步操作。
>* 流控算法通过对时间序列分析来动态地调整参数来进行流量控制，有很好的准确度和适应性。 
>* 提供延迟反馈限流、分级流量挑选多种流控模式。

[English](./README.md)

## 和其他流控算法的区别
传统的流控算法包括计数器、漏桶和令牌桶等，这些算法在本地模式下容易实现，但在集群的服务分区模式下运行的代价较大。
常见的处理办法是使用全局存储器或RPC服务来实现，需要消耗大量的网络资源和依赖于网络的稳定，限制了其使用的范围。

本项目算法基于对集群整体流量平稳的假设：
 * 集群整体流量平稳。 
 * 集群内各节点的流量平稳。

本项目算法的特点包括：
 * 对流量的变化敏感(超过10s)，可以通过捕捉变化来调整流控参数。
 * 运行在各个流控节点，资源消耗少。
 * 对网络延迟和网络稳定性要求低，可能适应多机房部署的场景。
 * 可以支持有延迟的非确定性反馈。
 * 可以支持分级流控。

![avatar](https://github.com/boostlearn/go-cluster-limiter/raw/master/doc/pictures/limiter_frame.png)

## 主要功能
本项目流控器可以用下游的转化指标(reward)为目标来控制流量释放。比如通过控制广告投放(pass)来达成广告点击量(reward)控制的目标。
下游转化目标要求和请求的通过量是正相关的，否则流控器可能无法实现流控目标。

![avatar](https://github.com/boostlearn/go-cluster-limiter/raw/master/doc/pictures/limiter-reward.png)

本项目流控器可提供流量分级控制的功能。如果请求流量携带了对请求的打分级别信息, 
本项目分级流控器可以自动让打分较高的流量通过，达到流量对分级挑选的目标。
流量分级挑选可以优先保障高价值的流量通过，是最大化系统价值的利器。

![avatar](https://github.com/boostlearn/go-cluster-limiter/raw/master/doc/pictures/limiter-multilevel.png)

针对服务限流的场景，可以按周期设置目标的达成。针对预算控制、实验分流等场景，可以设置任务的起始和结束时间来的设置目标的达成。
在整个任务周期内， 限流器实现了目标值的平滑释放。
>本项目流控器可以设置的最小流量控制间隔是节点进行全局数据同步的间隔(一般2s~10s)。

## 使用示例
#### 同步信息
>本项目流控器需要一个全局的存储来同步集群信息。这个存储器可以短时不可用，但要求存储的数据不丢失。
>常见的数据库比如REDIS，INFLUXDB，MYSQL都可以满足要求。本项目目前仅支持REDIS。

**构建全局存储**:

    import	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
        	
    counterStore, err := redis_store.NewStore("127.0.0.1:6379","","")

#### 限流器
**构建限流器生产工厂**：

    import	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
    
    limiterFactory := cluster_limiter.NewFactory(
    	&cluster_limiter.ClusterLimiterFactoryOpts{
    		Name:                  "test",
    		Store: counterStore,
    	})
    limiterFactory.Start()
 
**构建带起始结束时间的限流器**:
    
    beginTime,_ := time.Parse("2006-01-02 15:04:05", "2020-01-01 09:00:00")
    endTime,_ := time.Parse("2006-01-02 15:04:05", "2020-01-01 18:00:00")
    limiter, err := limiterFactory.NewClusterLimiter(
    		&cluster_limiter.ClusterLimiterOpts{
    			Name:                "test",
    			RewardTarget: 10000,
    			BeginTime: beginTime,
    			EndTime: endTime,
    			DiscardPreviousData: true,
    		})
    		
**构建周期性限流器**:
     
    limiter, err := limiterFactory.NewClusterLimiter(
    		&cluster_limiter.ClusterLimiterOpts{
    			Name:                "test",
    			RewardTarget: 10000,
    			PeriodInterval:      time.Duration(60) * time.Second,
    			DiscardPreviousData: true,
    		})   		


**限流器器的通过获取和反馈**:
    
    limiter := limiterFactory.GetClusterLimiter("test")
    if limiter != nil {
        if limiter.Take(1) { 
    	    doSomething()
    	    if inCentainCondition {
    	        limiter.Reward(1) 
    	    }
        } else { 
            doFail()
        }
    } else {
       doSomething()
    }

#### 分级限流器
**构建分级限流器**：
    
    scorelimiter, err = limiterFactory.NewClusterLimiter(
    	&cluster_limiter.ClusterLimiterOpts{
    		Name:                     "limiter-3",
    		RewardTarget: 10000,
    		PeriodInterval:           time.Duration(60) * time.Second,
    		TakeWithScore: true,
    		DiscardPreviousData:      true,
    	})
    		
**分级限流器的通过获取和反馈**：
    
    scoreLimiter := limiterFactory.GetClusterLimiter("limiter-3")
    if limiter != nil {
        score := getScoreValue(...)
        if scoreLimiter.TakeWithScore(1, score) { 
    	    doSomething()
    	    if inCentainCondition {
    	        scoreLimiter.Reward(1) // reward
    	    }
        } else {
           doFail()
        }
    } else {
        doSomething()
    }
    
## 集群流控算法原理
>本项目流控算算法以固定周期(大约2s~10s)重新评估流量情况，通过参数调整来调价请求的通过量。

#### 通过本地流量估计当前的集群流量
>本项目流控算法认为集群流量和本地流量的短时稳定，所以本地流量在集群流量里的占比也短时稳定。
 
本地流量占比参会(LocalTrafficProportion)的动态更新算法如下：

     本周期流量占比 :=  上周期流量占比 * P + （本周期本地请求/本周期集群请求）* （1 - P）
     

> 注： P代表衰减系数，用来控制平滑度。

估计当前集群流量的公式：

     当前集群流量 := 同步的集群流量 + 从同步时间到当前时间本地的新增流量/本地流量占比

#### 计算当前从流控器的通过量到目标量的转化比率
>由于流量控制器不直接控制目标量，需要通过计算转化比率来计算达成预计目标（reward)的通过量(pass)。

更新转化比率(rewardRate)的公式：

    本周期转化比 := 上周期转化比 * P + (本周期转化量/本周期通过量) * ( 1 – P)

#### 计算流控器的通过率
>本项目流控器的通过控制通过比例参数(passRate)来完成的。

理想通过率的计算公式如下：

    本周期理想通过率 := 上周期理想通过率 * P + (（本周期目标值/本周期请求量）/ 转化率） * （1 - P ） 

理想通过率(idealPassRate)有一定的滞后性, 需要根据实际转化情况对通过率进行调节。

如果当前实际转化量小于平滑后的理想转化量，实际转化率（workingPassRate）的计算公式：

    实际通过率 := 理想通过率 * ( 1 + 滞后时间/加速周期)

如果当前实际转化量大于平滑后的理想转化量。实际转化率（workingPassRate）的计算公式如下：

     实际通过率 := 理想通过率 * ( 1 - 超量时间/加速周期)

## 性能测试结果
访问耗时如下：

|模块|1CPU|2CPU|3CPU|4CPU|
|----|----|----|----|---|
|限流器|430 ns/op|422 ns/op|258 ns/op|416 ns/op|
|分级限流器|484 ns/op|521 ns/op|554 ns/op|725 ns/op|

结论： 
* 单核心QPS服务在200万左右, 对于单机业务能力在10万QPS以内的应用影响很小，可以满足大部分使用场景。
* 多核加速效果不好，可以通过创建多个限制器副本来调高单机服务能力。

