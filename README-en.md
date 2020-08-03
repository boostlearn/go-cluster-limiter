>This project can be used as a service module that implements flow limiter in a cluster environment, 
>and can be used in scenarios that require flow limiter such as service protection, consumption control, or experiment shunting. 
>the project uses a decentralized flow control algorithm, 
>which effectively reduces the consumption and dependence on network resources.

## The difference with other limiters
     
the local host limiter mainly controls the use of local traffic, 
and can be implemented using algorithms such as counters, leaky buckets, and token buckets. 
the local limiter does not depend on the external environment, has a good flow control effect, 
low resource consumption, and has a wide range of application scenarios.

However, the algorithm that implements local host limiter cannot run in the service partition mode. 
The common method is to control the flow within the cluster by calling the external flow control (RPC) service interface. 
however, cluster limiter carried out through the network, requires high network stability, certain time delay, 
easily forms a single hot spot and consumes a lot of resources, which limits its usage.

This project uses a decentralized flow control algorithm to move the control strategy to  decentralized nodes, 
reducing the dependence on the network. 
the control algorithm of this project requires the request flow to meet the following requirements in most of the time:
* In a short time (<10s), the overall traffic flow of the cluster is stable.
* In a short time (<10s), the traffic of each node in the cluster is stable.
     
The algorithm of this project is sensitive to traffic changes in the flow over a certain interval (>10s), and can dynamically calculate and adapt to changes.

In scenarios where the flow is often non-continuous or there are many of instantaneous burst, the algorithm of this project may not work.

## Supported flow limiter methods
The minimum flow control interval that can be set by the flow limiter of this project is the interval at which the client node performs global data synchronization (generally 2s~10s). 

For the scenario of service flow limitation, the total number of passes per minute (or more) can be set. During this time period, the smooth release of traffic can be achieved. 

For scenarios such as budget control and experimental diversion, the start and end time of the task can be set. During this task period, the smooth release of traffic can be achieved too.

The flow limter of this project can set the downstream conversion reward as the target to control the request's passing. 
For example, by controlling the number of advertisements cast(passed), the goal of ad clicks (reward) can be achieved finally. 
The target reward  and the traffic pass should be positively correlated, otherwise the flow controller of this project may not be able to achieve control.

The flow limiter of this project provides hierarchical flow limiter. If the requested traffic carries score value information, 
the hierarchical flow limiter of this project can automatically pass traffic with a higher score to achieve the goal of traffic hierarchical selection. 
Traffic classification selection to prioritize high-value traffic is a weapon to maximize system value.

## Principles of algorithm
The flow control calculation algorithm of this project re-evaluates the flow situation in a fixed period (about 2s~10s), and adapt to changes in traffic through parameter adjustment.

#### Estimate current cluster traffic by local traffic volume
The flow control algorithm of this project believes that the cluster traffic and local traffic are stable for a short time, 
so the proportion of local traffic in the cluster traffic is also stable for a short time.

The dynamic update algorithm of LocalTrafficProportion is as follows:
    
    LocalTrafficProportion : = LocalTrafficProportion * P + (RecentLocalRequest/RecentClusterRequest) * (1-P)
    
   >Note: P stands for attenuation coefficient, used to control smoothness.

Formula for estimating current cluster traffic:

    CurrentClusterTraffic: = LastSynchronizedClusterTraffic + LocalRequestSinceSynchronized/LocalTrafficProportion
    
    
#### Calculate the current conversion ratio from passing to rewarding
Since the flow limiter does not directly control the reward volume, 
the conversion ratio needs to be calculated to calculate the pass to achieve the target reward. 

Update ConversionRate formula:
    ConversionRatio: = ConversionRatio * P + (RecentReward / RecentPassed) * (1 – P)

#### Calculate the ideal limiter pass rate
The flow controller of this project is controlled by the proportional parameter (passRate). 

The formula for calculating the IdealPassingRate is as follows:

    IdealPassRate : = IdealPassRate * P + ((RecentReward/RecentRequest)/RewardRate) * (1-P)
    
#### Adjust the working pass rate of the flow limiter
The IdealPassRate has a certain lag, and the working pass rate needs to be adjusted according to the actual conversion situation.

If the current actual reward volume is less than the smoothed ideal reward volume, 
the calculation formula of the WorkingPassRate:

    WorkingPassRate: = WorkingPassRate * (1 + LagTime/AccelerationPeriod)
 
If the current actual reward volume is greater than the smoothed ideal reward volume. 

The calculation formula of the WorkingPassRate is as follows:

    WorkingPassRate: = IdealPassRate * (1-ExcessTime / AccelerationPeriod)
    
