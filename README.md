>This project can be used as a service module that implements flow limiter in a cluster environment, 
>and can be used in scenarios that require flow limiter such as service protection, consumption control, or experiment shunting and so on. 
>this project uses a decentralized flow control algorithm, which could effectively reduce the resource consumption and dependence on network resources.

## The difference with other limiters
     
the stand-alone limiter mainly controls the use of local traffic, 
which can be implemented using algorithms such as counters, leaky buckets, or token buckets.
the stand-alone limiter does not depend on the external environment and needs low resource consumption.

However, the algorithm which used in stand-alone limiter cannot run in the cluster's service partition mode. 
The common method is to control the cluster's flow by calling the external flow control (RPC) service. 
however, cluster limiter carried out through the network, requires high network stability, consume certain request time delay, 
easily forms a single hot spot, and consumes a lot of resources, limits its scope of usage.

This project uses a decentralized flow control algorithm to move the control strategy to  decentralized clients, 
the goal is to reduce the dependence on the network. 
the control algorithm of this project requires the request flow to meet the following requirements in most of the time:
* In a short time (<10s), the overall traffic flow of the cluster is stable.
* In a short time (<10s), the traffic flow of each node in the cluster is stable.
     
The algorithm of this project is also sensitive to traffic changes in the flow over a certain interval (>10s), and can dynamically calculate and adapt to changes.

In scenarios where the traffic flow is often non-continuous or has many of instantaneous burst traffic, the algorithm of this project may not work well.

## Supported interface
The minimum flow control interval that can be set by the flow limiter of this project is the interval at which the client node performs global data synchronization (generally 2s~10s). 

For the scenario of service flow protection limitation, the total number of passes per minute (or more) can be set. During this time period, the smooth release of traffic can be achieved. 

For scenarios such as budget control and experimental diversion, the start and end time of the task can be set. During this task period, the smooth release of traffic can be achieved too.

The flow limiter of this project can set the downstream conversion reward as the target to control the passing of request flow. 
For example, by controlling the number of advertisements cast, the goal of ad clicks can be achieved finally. 
The target reward volume and the pass volume should be positively correlated, otherwise the goal of the limiter may not be able to achieve control.

The flow limiter of this project provides hierarchical flow limiter. If the requested traffic carries score value information, 
the hierarchical flow limiter of this project can automatically pass traffic with a higher score to achieve the goal of traffic hierarchical selection. 
Traffic classification selection to prioritize high-value traffic is a weapon to maximize system value.

## Examples
#### Storage Support
The storage requirements are:
* Can be unavailable for a short time, but the stored data should not be lost
* The response time of data query under normal conditions is within 100ms

The commonly used database like redis, influxdb, and mysql can all meet these conditions.
Currently only redis is supported.

Build:

    import "github.com/boostlearn/go-cluster-limiter/cluster_limiter"
    counterStore, err := redis_store.NewStore("127.0.0.1:6379","","")

#### Limiter
**build limiter's factory**：
    
    limiterFactory := cluster_limiter.NewFactory(
    	&cluster_limiter.ClusterLimiterFactoryOpts{
    		Name:                  "test",
    		HeartbeatInterval:     1000 * time.Millisecond,
    		InitLocalTrafficProportion: 1.0,
    	}, counterStore)
    limiterFactory.Start()
 
**build limiter with start-end time**:
    
    beginTime,_ := time.Parse("2006-01-02 15:04:05", "2020-01-01 09:00:00"),
    endTime,_ := time.Parse("2006-01-02 15:04:05", "2020-01-01 18:00:00"),
    limiter, err := limiterFactory.NewClusterLimiter(
    		&cluster_limiter.ClusterLimiterOpts{
    			Name:                "test",
    			RewardTarget: 10000,
    			BeginTime: beginTime,
    			EndTime: endTime,
    			DiscardPreviousData: true,
    		})
    		
**build limiter with  reset period**:
     
    limiter, err := limiterFactory.NewClusterLimiter(
    		&cluster_limiter.ClusterLimiterOpts{
    			Name:                "test",
    			RewardTarget: 10000,
    			PeriodInterval:      time.Duration(60) * time.Second,
    			DiscardPreviousData: true,
    		})   		

**limiter's take and reward**:
    
    if limiter.Acquire(1) { 
    	doSomething()
    }
    ...
    limiter.Reward(1) 


#### Limiter With Score
**build limiter with score samples**：
    
    scorelimiter, err = limiterFactory.NewClusterLimiter(
    	&cluster_limiter.ClusterLimiterOpts{
    		Name:                     "test",
    		RewardTarget: 10000,
    		PeriodInterval:           time.Duration(60) * time.Second,
    		ScoreSamplesMax:          10000,
    		ScoreSamplesSortInterval: 10 * time.Second,
    		DiscardPreviousData:      true,
    	})
    		
**score limiter's take and reward**：
    
    if limiter.TakeWithScore(1, score) { 
    	doSomething()
    }
    ...
    limiter.Reward(1) // 反馈
    

## Benchmark
benchmark test results:

|module|1CPU|2CPU|3CPU|4CPU|
|----|----|----|----|---|
|counter|51.9 ns/op|71.8 ns/op|72.1 ns/op|73.5 ns/op|
|limiter|465 ns/op|411 ns/op|265 ns/op|271 ns/op|
|score limiter|492 ns/op|493 ns/op|528 ns/op|545 ns/op|

in conclusion: 
* The single-core serving is about 2 million qps, which has little impact on applications with a single-machine business capacity within 100,000 QPS, and can meet most usage scenarios.
* The multi-core acceleration effect is not good. You can increase the stand-alone service capability by creating multiple copies of the limiter.
    
## Principles of the limiter's algorithm
The flow control calculation algorithm of this project re-evaluates the flow situation in a fixed period (about 2s~10s), and adapt to changes in traffic through parameter adjustment.

#### Estimate current cluster's traffic volume by local traffic volume
The flow control algorithm of this project believes that the cluster traffic and local traffic are stable for a short time, 
so the proportion of local traffic in the cluster traffic is also stable for a short time.

The dynamic update algorithm of LocalTrafficProportion is as follows:
    
    LocalTrafficProportion : = LocalTrafficProportion * P + (RecentLocalRequest/RecentClusterRequest) * (1-P)
    
   >Note: P stands for attenuation coefficient, used to control smoothness.

Formula for estimating current cluster traffic:

    CurrentClusterTraffic: = LastSynchronizedClusterTraffic + LocalRequestSinceSynchronized/LocalTrafficProportion
    
    
#### Calculate pass to reward ratio
Since the flow limiter does not directly control the reward, 
the conversion ratio needs to be calculated to calculate the pass quantum to achieve the target reward. 

Update ConversionRate formula:
    
    ConversionRatio: = ConversionRatio * P + (RewardRecently / PassRecently) * (1 – P)

#### Calculate pass rate
The limiter's flow pass is controlled by the pass rate parameter. 

The formula for calculating the IdealPassingRate is as follows:

    IdealPassRate : = IdealPassRate * P + ((RewardRecently/RequestRecently)/RewardRate) * (1-P)
    
The IdealPassRate has a certain lag, 
and the real working pass rate needs to be adjusted according to the actual conversion situation.

If the current actual reward volume is less than the smoothed ideal reward quantum, 
the calculation formula of the WorkingPassRate:

    WorkingPassRate: = WorkingPassRate * (1 + LagTime/AccelerationPeriod)
 
If the current actual reward volume is greater than the smoothed ideal reward quantum. 

The calculation formula of the WorkingPassRate is as follows:

    WorkingPassRate: = IdealPassRate * (1 - ExcessTime/AccelerationPeriod)
    