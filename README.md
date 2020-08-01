>本项目可用做集群环境下实现流量控制的服务模块，可以在服务限流、消耗控制、实验分流等需要做流量控制的场景下使用。
本项目使用非中心化流量控制算法，有效降低了对网络资源的消耗和依赖。

## 和单体流控器的区别
&nbsp;&nbsp;&nbsp;&nbsp;
单体流控器主要控制本地流量使用，可以使用计数器、漏桶和令牌桶等算法来实现。
单体流控器不依赖外部环境，流控效果好，资源消耗少，有广泛的应用场景。

&nbsp;&nbsp;&nbsp;&nbsp;
但实现单体流量控制的算法无法在分区模式下运行，常见的办法是通过调用外部流控(RPC)服务接口来控制流量。
集群中心流控通过网络进行，对网络稳定性和请求时间延迟要求都很高。
集群中心流控容易形成单一热点和消耗大量的资源，限制了其应用范围。

&nbsp;&nbsp;&nbsp;&nbsp;
本项目使用非中心化的流量控制算法，把控制策略的下方到各个节点，降低了对网络环境的依赖。
本项目控制算法要求流量在大部分的时间要满足如下要求：
 * 在很短的时间(<10s), 集群整体流量稳定。 
 * 在很短的时间(<10s), 集群内各节点的流量稳定。
 
&nbsp;&nbsp;&nbsp;&nbsp;
本项目的算法对流量在超过一定间隔(>10s)的流量变化敏感，可以动态计算和适应流量变化。

&nbsp;&nbsp;&nbsp;&nbsp;
在请求流量经常是非持续的或有大量瞬间爆发请求的场景下，本项目的流控算法可能无法工作。

## 支持的流量控制方式
&nbsp;&nbsp;&nbsp;&nbsp;
本项目流控器可以设置的最小流量控制间隔是节点进行全局数据同步的间隔(一般2s~10s)。
针对服务限流的场景，可以设置每分钟(或以上)的总通过(pass)量，在这个时间周期内，可以实现流量的平滑释放。
针对预算控制、实验分流等场景，可以设置任务的起始和结束时间，在这个任务周期内， 可以实现流量的平滑释放。

&nbsp;&nbsp;&nbsp;&nbsp;
本项目流控器可以设置以下游转化指标(reward)为目标来控制流量释放。
比如通过控制广告的投放量(pass)，达成点击量(reward)的目标。
目标指标(reward)要求和通过量(pass)是正相关的，否则本项目流控器可能无法实现控制。

&nbsp;&nbsp;&nbsp;&nbsp;
本项目流控器提供分级流控功能。
如果请求流量携带了打分(score)信息, 本项目分级流控器可以自动让打分较高的流量通过，达到流量分级挑选的目标。
流量分级挑选优先保障高价值的流量，是最大化系统价值的利器。

## 算法原理
&nbsp;&nbsp;&nbsp;&nbsp;
本项目流控算算法以固定周期(大约2s~10s)重新评估流量情况，通过参数调整来调价请求的通过量。

#### 1. 通过本地流量估计当前的集群流量
&nbsp;&nbsp;&nbsp;&nbsp;
本项目流控算法认为集群流量和本地流量的短时稳定，所以本地流量在集群流量里的占比也短时稳定。
 
&nbsp;&nbsp;&nbsp;&nbsp;
本地流量占比参会(LocalTrafficRatio)的动态更新算法如下：

     本周期流量占比 :=  上周期流量占比 * P + （本周期本地请求/本周期集群请求）* （1 - P）
     

> 注： P代表衰减系数，用来控制平滑度。

&nbsp;&nbsp;&nbsp;&nbsp;
估计当前集群流量的公式：

     当前集群流量 := 同步的集群流量 + 从同步时间到当前时间本地的新增流量/本地流量占比

#### 2. 计算当前从流量控制器的通过量到目标量的转化比率
&nbsp;&nbsp;&nbsp;&nbsp;
由于流量控制器不直接控制目标量，需要通过计算转化比率来计算达成预计目标（reward)的通过量(pass)。
更新转化比率(rewardRate)的公式：

    本周期转化比 := 上周期转化比 * P + (本周期转化量/本周期通过量) * ( 1 – P)

#### 3. 计算当前理想的流量控制器的通过率
&nbsp;&nbsp;&nbsp;&nbsp;
本项目流控器的通过控制通过比例参数(passRate)来完成的。
理想通过率的计算公式如下：

    本周期理想通过率 := 上周期理想通过率 * P + (（本周期目标值/本周期请求量）/ 转化率） * （1 - P ） 

#### 4. 调节流量控制器的实际通过率
&nbsp;&nbsp;&nbsp;&nbsp;
理想通过率(idealPassRate)有一定的滞后性, 需要根据实际转化情况对通过率进行调节。

&nbsp;&nbsp;&nbsp;&nbsp;
如果当前实际转化量小于平滑后的理想转化量，实际转化率（realPassRate）的计算公式：

    实际通过率 := 理想通过率 * ( 1 + 滞后时间/加速周期)

&nbsp;&nbsp;&nbsp;&nbsp;
如果当前实际转化量大于平滑后的理想转化量。实际转化率（realPassRate）的计算公式如下：

     实际通过率 := 理想通过率 * ( 1 - 超量时间/加速周期)

## 性能测试
&nbsp;&nbsp;&nbsp;&nbsp;
访问耗时如下：

|模块|1CPU|2CPU|3CPU|4CPU|
|----|----|----|----|---|
|集群计数器|51.9 ns/op|71.8 ns/op|72.1 ns/op|73.5 ns/op|
|限流器|465 ns/op|411 ns/op|265 ns/op|271 ns/op|
|分级限流器|492 ns/op|493 ns/op|528 ns/op|545 ns/op|

&nbsp;&nbsp;&nbsp;&nbsp;
结论： 
* 单核心QPS服务在200万左右, 对于单机业务能力在10万QPS以内的应用影响很小，可以满足大部分使用场景。
* 多核加速效果不好，可以通过创建多个限制器副本来调高单机服务能力。

## 使用说明
#### 中心存储器
&nbsp;&nbsp;&nbsp;&nbsp;
对存储的要求是：
* 可以与短暂的不可用，但已经存储的数据不应丢失
* 正常情况下的数据查询的响应时间在100ms以内

&nbsp;&nbsp;&nbsp;&nbsp;
常用的redis，influxdb，mysql都是可以满足这些条件的。
目前仅支持redis。

构建存储器：

    import "github.com/boostlearn/go-cluster-limiter/cluster_limiter"
    
    counterStore := redis_store.NewStore("127.0.0.1:6379","","")

#### 集群计数器
构建计数器：

     import "github.com/boostlearn/go-cluster-limiter/cluster_counter"
     import "github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
     	
     counterFactory := cluster_counter.NewFactory(
    	&cluster_counter.ClusterCounterFactoryOpts{
    		Name:                     "test",
    	}, counterStore)
     counterFactory.Start()
    
     counter, _ = counterFactory.NewClusterCounter(
    	&cluster_counter.ClusterCounterOpts{
    		Name:                  "test",
    		PeriodInterval:        time.Duration(60) * time.Second,
    		DiscardPreviousData:   true,
    	})
    
计数器使用: 

    counter.Add(1) // 增加计数器
    localCurrentValue := counter.LocalValue(0) // 本地值
    cluterCurrentValue := counter.ClusterValue(0) // 集群值
    
#### 流控器
限流器构建：
    
    import "github.com/boostlearn/go-cluster-limiter/cluster_limiter"
    
    limiterFactory := cluster_limiter.NewFactory(
    	&cluster_limiter.ClusterLimiterFactoryOpts{
    		Name:                  "test",
    		HeartbeatInterval:     1000 * time.Millisecond,
    		InitLocalTrafficRatio: 1.0,
    	}, counterStore)
    limiterFactory.Start()
    
    limiter, err := limiterFactory.NewClusterLimiter(
    		&cluster_limiter.ClusterLimiterOpts{
    			Name:                "test",
    			PeriodInterval:      time.Duration(60) * time.Second,
    			DiscardPreviousData: true,
    		})

限流器使用：
    
    if limiter.Acquire(1) { // 获取
    	doSomething()
    }
    ...
    
    limiter.Reward(1) // 反馈


#### 分级流控器
分级限流器构建：

    import "github.com/boostlearn/go-cluster-limiter/cluster_limiter"
    
    limiterFactory := cluster_limiter.NewFactory(
    	&cluster_limiter.ClusterLimiterFactoryOpts{
    		Name:                  "test",
    	}, counterStore)
    limiterFactory.Start()
    
    scorelimiter, err = limiterFactory.NewClusterLimiter(
    	&cluster_limiter.ClusterLimiterOpts{
    		Name:                     "test",
    		PeriodInterval:           time.Duration(60) * time.Second,
    		ScoreSamplesMax:          10000,
    		ScoreSamplesSortInterval: 10 * time.Second,
    		DiscardPreviousData:      true,
    	})
    		
分级限流器使用：
    
    if limiter.ScoreAcquire(1, score) { // score代表评分
    	doSomething()
    }
    ...
    limiter.Reward(1) // 反馈