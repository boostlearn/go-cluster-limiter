>This project can be used as a service module that implements flow limiter in a cluster environment, and can be used in scenarios that require flow control such as service request limiting, consumption control, and experimental shunting. 
This project uses a decentralized flow control algorithm, which effectively reduces the consumption and dependence on network resources.

## The difference with the monomer flow limiter
     
The monomer flow limiter mainly controls the use of local traffic, and can be implemented using algorithms such as counters, leaky buckets, and token buckets. 
The single flow limiter does not depend on the external environment, has a good flow control effect, low resource consumption, and has a wide range of application scenarios.

However, the algorithm that implements single flow control cannot run in the service partition mode. 
The common method is to control the flow by calling the external flow control (RPC) service interface. 
In this way, cluster's flow limiter is carried out through the network, which requires high network stability and request time delay. 
The cluster's flow limiter easily forms a single hot spot and consumes a lot of resources, which limits its application range.

This project uses a decentralized flow control algorithm to move the control strategy to  decentralized nodes, reducing the dependence on the network environment. 
Our control algorithm of this project requires the flow to meet the following requirements in most of the time:
* In a short time (<10s), the overall flow of the cluster is stable.
* In a short time (<10s), the traffic of each node in the cluster is stable.
     
The algorithm of this project is sensitive to traffic changes in the flow over a certain interval (>10s), and can dynamically calculate and adapt to changes in the flow.

In scenarios where the request traffic is often non-continuous or there are many of instantaneous burst requests, the flow control algorithm of this project may not work.

## 2. Supported flow control methods
The minimum flow control interval that can be set by the flow controller of this project is the interval at which the node performs global data synchronization (generally 2s~10s). 

For the scenario of service flow limitation, the total number of passes per minute (or more) can be set. During this time period, the smooth release of traffic can be achieved. 

For scenarios such as budget control and experimental diversion, the start and end time of the task can be set. During this task period, the smooth release of traffic can be achieved too.

The flow controller of this project can set the downstream conversion reward as the target to control the request's pass. 
For example, by controlling the number of advertisements (passed), the goal of clicks (reward) can be achieved. 
The target reward requirement and the pass are positively correlated, otherwise the flow controller of this project may not be able to achieve control.

The flow limiter of this project provides hierarchical flow control function. If the requested traffic carries score value information, 
the hierarchical flow limiter of this project can automatically pass traffic with a higher score to achieve the goal of traffic hierarchical selection. 
Traffic classification selection to prioritize high-value traffic is a weapon to maximize system value.

## 3. Principles of cluster flow limiter algorithm
The flow control calculation algorithm of this project re-evaluates the flow situation in a fixed period (about 2s~10s), and adapt to changes in traffic through parameter adjustment.

#### 3.1. Estimate current cluster traffic by local traffic
The flow control algorithm of this project believes that the cluster traffic and local traffic are stable for a short time, so the proportion of local traffic in the cluster traffic is also stable for a short time.

The dynamic update algorithm of LocalTrafficProportion is as follows:
    
    CurrentLocalTrafficProportion : = LastLocalTrafficProportion * P + (RecentLocalRequest/RecentClusterRequest) * (1-P)
    
   >Note: P stands for attenuation coefficient, used to control smoothness.

Formula for estimating current cluster traffic:

    CurrentClusterTraffic: = LastSynchronizedClusterTraffic + LocalRequestSinceSyn/CurrentLocalTrafficProportion
    
    
#### 3.2. Calculate the current conversion ratio from the passed to the reward
Since the flow limiter does not directly control the reward volume, the conversion ratio needs to be calculated to calculate the pass to achieve the expected reward. 

Update conversion rate (rewardRate) formula:
    CurrentConversionRatio: = LastConversionRatio * P + (RecentReward / RecentPassed) * (1 – P)

#### 3.3. Calculate the ideal pass rate
The flow controller of this project is controlled by the proportional parameter (passRate). 

The formula for calculating the ideal passing rate is as follows:

    NextIdealPassRate : = LastIdealPassRate * P + ((RecentReward/RecentRequest)/CurrentRewardRate) * (1-P)
    
#### 3.4. Adjust the actual pass rate of the flow controller
The ideal pass rate (idealPassRate) has a certain lag, and the pass rate needs to be adjusted according to the actual conversion situation.

If the current actual conversion volume is less than the smoothed ideal conversion volume, the calculation formula of the actual conversion rate (realPassRate):

    ActualPassRate: = IdealPassRate * (1 + LagTime/AccelerationPeriod)
 
If the current actual conversion volume is greater than the smoothed ideal conversion volume. 

The calculation formula of the actual conversion rate (realPassRate) is as follows:

    ActualPassRate: = IdealPassRate * (1-ExcessTime / AccelerationPeriod
