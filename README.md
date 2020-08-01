# Go-cluster-limiter是什么项目？
本项目用作集群环境下实现流量控制的服务模块，可以在服务限流、消耗控制、实验分流等需要做流量控制的场景下使用。
本项目使用非中心化流量控制算法，有效降低了对网络资源的消耗和依赖。

# 和单体的流量控制器有什么区别？
单体流量控制器主要控制本地流量使用，可以使用包括计数器、漏桶和令牌桶等算法来实现流量控制。
单体流量控制所有控制在本地进行，无需依赖外部环境，资源消耗也较少，有广泛应用。

但是单体流量的控制算法无法支持分区，在大部分集群服务里都无法直接使用。常见的办法是通过调用中心化的流量控制RPC服务来控制集群服务的整体流量。
这样做的坏处是很容易形成集群单一热点，需要消耗大量的资源，限制了集群的服务能力和适用的范围。
调用RPC服务对网络稳定性和时间延迟要求都比较高，尤其无法适应集群跨机房部署的情形。

本模块使用非中心化的流量控制算法，把控制策略的执行放在各个服务节点，降低了对网络资源的依赖。
不实时同步全局状态做有效的流量控制，要求应用场景满足如下两个要求：
 * 在很短的时间(<10s), 集群整体流量稳定。 
 * 在很短的时间(<10s), 集群内单节点的流量稳定。
 
本模块对流量在超过一定间隔(>10s)的流量变化敏感，可以动态计算和适应流量变化。
对于一些请求流量是非持续或瞬间爆发的场景(比如秒杀商品)，本模块的算法可能无法工作。

# 支持哪些流量控制方式？
本模块可以设置的最小流量控制间隔不应小于全局数据同步的间隔(2s~10s)。
针对服务限流的场景，可以设置每分钟(或以上)的总通过(pass)量，本模块会通过对总通过量进行匀速的释放，达到平滑流量控制的效果。
针对预算控制、实验分流等场景，可以设置任务的起始和结束时间，本模块可以在任务周期内平滑释放通过量。

本模块可以用下游指标(reward)为目标来控制流量，比如通过控制广告的投放量(pass)，达成特定点击量(reward)的目标。目标指标(reward)要求和通过量(pass)是正相关的，否则本模块可能无法实现控制。

本模块提供分级别流量控制的功能。如果上游提供了对请求流量的打分(score), 本模块可以自动让打分较高的流量通过，达到流量分级挑选的目标。流量分级挑选优先保障高价值的流量，是最大化系统价值的利器。

# 算法原理是什么？
本模块算法基于对流量短时稳定的假定，需要做的对流量变化的捕捉来及时调整参数达到流量控制的目标。

## 1. 通过本地流量估计当前的集群流量
由集群流量和本地流量的短时稳定，本模块算法认为本地流量在集群流量里的占比(LocalTrafficRatio)短时稳定。
 
本周期本地流量占比参会(LocalTrafficRatio)的动态更新算法如下：

> 本周期流量占比 =  上周期流量占比 * P + （本周期本地请求/本周期集群请求）* （1 - P）

其中P是衰减系数，用来控制平滑度。

估计当前集群流量的公式：
> 当前集群流量 = 同步的集群流量 + 从同步时间到当前时间本地的新增流量/本地流量占比

## 2. 计算当前从流量控制器的通过量到目标量的转化比率
由于流量控制器不直接控制目标量，需要通过计算转化比率(rewardRate)来计算完成目标（reward)的通过量(pass)。
更新转化比率的公式：
> 本周期转化比 = 上周期转化比 * P + (本周期转化量/本周期通过量) * ( 1 – P)

其中P是衰减系数，用来控制平滑度。

## 3. 计算当前理想的流量控制器的通过率
本模块流量控制器的通过控制通过比例参数(passRate)来完成的。
理想通过率的计算公式如下：
>本周期理想通过率 = 上周期理想通过率 * P + (（本周期目标值/本周期请求量）/ 转化率） * （1 - P ） 

其中P是衰减系数，用来控制平滑度。

## 4. 调节流量控制器的实际通过率
理想通过率(idealPassRate)有一定的滞后性, 需要根据实际转化情况对通过率进行调节。

如果当前实际转化量小于平滑后的理想转化量，实际转化率（realPassRate）的计算公式：
> 实际通过率 = 理想通过率 * ( 1 + 滞后时间/加速周期)

如果当前实际转化量大于平滑后的理想转化量。实际转化率（realPassRate）的计算公式如下：
> 实际通过率 = 理想通过率 * ( 1 - 超量时间/加速周期)

# 性能如何？
访问耗时如下：

|模块|1CPU|2CPU|3CPU|4CPU|
|----|----|----|----|---|
|集群计数器(cluster_counter)|51.9 ns/op|71.8 ns/op|72.1 ns/op|73.5 ns/op|
|集群限流器(cluster_limiter)|465 ns/op|411 ns/op|265 ns/op|271 ns/op|
|集群分级限流器(cluster_score_limiter)|492 ns/op|493 ns/op|528 ns/op|545 ns/op|

结论： 
* 单核心QPS服务在200万左右, 对于单机业务能力在10万QPS以内的应用影响很小，可以满足大部分使用场景。
* 多核加速效果不好，可以通过自示例内创建多个相同的限制器来提供服务能力。

# 如何使用？
本模块依赖于一个统一的中心存储器来汇集集群的数据。

对存储的要求是：
* 可以与短暂的不可用，但已经存储的数据不应丢失
* 正常情况下的数据查询的响应时间在100ms以内
常用的redis，influxdb，mysql都是可以使用的(目前仅支持redis)。