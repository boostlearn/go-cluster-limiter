>This project provides lightweight and high-performance cluster flow control service.

[中文](./README-ZH.md)

## Difference from other flow control algorithms
Traditional flow control algorithms include counters, leaky buckets, token buckets, etc. 
These algorithms are easy to implement in the local mode, but the cost of running in the cluster's service partition mode is relatively high.
The common solution is to use global memory or call RPC services to achieve, which requires a lot of network resources and depends on the stability of the network, which limits the scope of its use.

The algorithm of this project is based on the assumption that the overall flow of the cluster is stable:
  * The overall flow of the cluster is stable.
  * The flow of each node in the cluster is stable.

The features of this project algorithm include:
  * Sensitive to changes in flow (more than 10s), the algorithm can adjust flow control parameters by capturing changes.
  * Run on each flow control node, with low resource consumption.
  * Low requirements for network delay and network stability, and may be suitable for deployment in multiple IDCs.
  * Can support delayed non-deterministic feedback.
  * Can support hierarchical flow control.
  
![avatar](https://github.com/boostlearn/go-cluster-limiter/raw/master/doc/pictures/limiter_frame.png)

## Features
The flow limiter of this project can set the downstream reward as the target to control the passing of a request flow. 
For example, by controlling the number of advertisements cast, and set the reward of ad clicks as target to be achieved. 
The reward and the flow's passing traffic should be positively correlated, otherwise the target of the limiter may cannot be able to be achieved.

![avatar](https://github.com/boostlearn/go-cluster-limiter/raw/master/doc/pictures/limiter-reward.png)

The flow limiter of this project also provides multi-level flow limiter. If the requested traffic carries a score value as its priority, 
the scored limiter of this project can automatically select traffic that with a higher score to be passed, and achieve the goal of traffic hierarchical selection. 
Traffic hierarchical selection to prioritize high-value traffic is a good weapon to maximize the system's value.

![avatar](https://github.com/boostlearn/go-cluster-limiter/raw/master/doc/pictures/limiter-multilevel.png)

For the scenario of service protection, the total number of reward in each cycle(minute or more) can be set to achieve,
For scenarios such as budget control and experimental diversion, the start time and end time of the task can be set to achieve too.
During this limiter's lifetime, the target reward can be reached smoothly.
>The minimum flow control interval that can be set by the flow limiter of this project is the interval at which the client node performs global data synchronization (generally 2s~10s). 

## Examples
#### Synchronization
>The cluster limiter of this project needs a centralized storage to synchronize global information.
the storage can be unavailable for a short time, but the stored data should not be lost.
The response time of data query from the storage under normal conditions is within 100ms
The commonly used database like redis, influxdb, and mysql can all meet these conditions.Currently only redis is supported.

**build the cluster's synchronization storage**:

    import	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
        	
    counterStore, err := redis_store.NewStore("127.0.0.1:6379","","")

#### Limiter
**build the limiters factory**：

    import	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
    
    limiterFactory := cluster_limiter.NewFactory(
    	&cluster_limiter.ClusterLimiterFactoryOpts{
    		Name:                  "test",
    		Store: counterStore,
    	})
    limiterFactory.Start()
 
**build limiter with start-end time**:
    
    beginTime,_ := time.Parse("2006-01-02 15:04:05", "2020-01-01 09:00:00")
    endTime,_ := time.Parse("2006-01-02 15:04:05", "2020-01-01 18:00:00")
    limiter, err := limiterFactory.NewClusterLimiter(
    		&cluster_limiter.ClusterLimiterOpts{
    			Name:                "limiter-1",
    			RewardTarget: 10000,
    			BeginTime: beginTime,
    			EndTime: endTime,
    		})
    		
**build limiter with  reset period**:
     
    limiter, err := limiterFactory.NewClusterLimiter(
    		&cluster_limiter.ClusterLimiterOpts{
    			Name:                "limiter-2",
    			RewardTarget: 10000,
    			PeriodInterval:      time.Duration(60) * time.Second,
    			DiscardPreviousData: true,
    		})   		

**limiter's take and reward**:
    
    limiter := limiterFactory.GetClusterLimiter("test")
    if limiter != nil {
        if limiter.Take(1) { 
    	    doSomething()
    	    if inSomeCondition {
    	        limiter.Reward(1) 
    	    }
        } else { 
            doFail()
        }
    } else {
       doSomething()
    }


#### Limiter With Score
**build limiter with score samples**：
    
    scorelimiter, err = limiterFactory.NewClusterLimiter(
    	&cluster_limiter.ClusterLimiterOpts{
    		Name:                     "limiter-3",
    		RewardTarget: 10000,
    		PeriodInterval:           time.Duration(60) * time.Second,
    		TakeWithScore: true,
    		DiscardPreviousData:      true,
    	})
    		
**score limiter's take and reward**：
    
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
    
  
## Algorithms
>The flow control calculation algorithm of this project re-evaluates the flow situation in a fixed period (about 2s~10s), 
>and adapt to changes in traffic through parameter adjustment.

  
![avatar](https://github.com/boostlearn/go-cluster-limiter/raw/master/doc/pictures/limiter_algorithm_en.png)

#### Estimate the cluster's traffic
>The flow control algorithm of this project believes that the cluster's traffic and local traffic are stable for a short while, 
so the proportion of local traffic in the cluster traffic is also stable for a short time.

The dynamic update algorithm of LocalTrafficProportion is as follows:
    
    LocalTrafficProportion : = LocalTrafficProportion * P + (LocalRequestRecently/ClusterRequestRecently) * (1-P)
    
   >Note: P stands for attenuation coefficient, used to control smoothness.

Formula for estimating current cluster's traffic:

    CurrentClusterTraffic: = LastSynchronizedClusterTraffic + LocalRequestSinceSynchronized/LocalTrafficProportion
    
    
#### Estimate the ratio of passed to reward
>Since the flow limiter does not directly control the reward, 
the conversion ratio needs to be calculated to calculate the pass quantum to achieve the target reward. 

Update ConversionRate formula:
    
    ConversionRatio: = ConversionRatio * P + (RewardRecently / PassRecently) * (1 – P)

#### Estimate the ideal ratio of requests to passing
>Whether a request can pass through the limiter can be controlled by the pass rate parameter

The formula for calculating the IdealPassingRate is as follows:

    IdealPassRate : = IdealPassRate * P + ((RewardRecently/RequestRecently)/RewardRate) * (1-P)
    
>The IdealPassRate may have certain lag, and the real working pass rate needs to be adjusted according to the actual achieved reward situation.

If the current actual reward is less than the smoothed ideal reward quantum, 
the calculation formula of the WorkingPassRate:

    WorkingPassRate: = WorkingPassRate * (1 + LagTime/AccelerationPeriod)
 
If the current actual reward is greater than the smoothed ideal reward quantum. 
The calculation formula of the WorkingPassRate is as follows:

    WorkingPassRate: = IdealPassRate * (1 - ExcessTime/AccelerationPeriod)
      
## Benchmark
benchmark test results:

|module|1-core cpu|2-core cpu|3-core cpu|4-core cpu|
|----|----|----|----|---|
|limiter|430 ns/op|422 ns/op|258 ns/op|416 ns/op|
|score limiter|484 ns/op|521 ns/op|554 ns/op|725 ns/op|

in conclusion: 
* The single-core service capacity  is about 2,000,000 qps, which has little impact on services with a service capacity of about 100,000 QPS. This is useful in most scenarios.
* The multi-core acceleration effect is not good. You can increase the stand-alone service's capability by using multiple copies of the same limiter.
  