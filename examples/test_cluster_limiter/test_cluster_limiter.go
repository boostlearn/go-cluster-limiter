package main

import (
	"flag"
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"math/rand"
	"time"
)

var (
	limiterName  string
	instanceName string

	discardPreviousData    bool
	localTrafficProportion float64

	targetNum         int64
	resetInterval     int64
	mockTrafficFactor int64

	redisAddr string
	redisPass string

	listenPort int64

	metrics = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "boostlearn",
		Subsystem: "test",
		Name:      "cluster_limiter",
	}, []string{"limiter_instance", "metric_name"})
)

func init() {
	flag.Int64Var(&targetNum, "a", 1000000, "total target num")
	flag.Int64Var(&resetInterval, "b", 3600, "reset data interval")
	flag.Int64Var(&mockTrafficFactor, "c", 2, "mock traffic factor")
	flag.StringVar(&limiterName, "d", "test_cluster_limiter", "limiter's unique name")
	flag.StringVar(&instanceName, "e", "test1", "test instance name")
	flag.StringVar(&redisAddr, "f", "127.0.0.1:6379", "store: redis address")
	flag.StringVar(&redisPass, "g", "", "store: redis pass")
	flag.Int64Var(&listenPort, "h", 20002, "prometheus: listen port")
	flag.BoolVar(&discardPreviousData, "i", true, "whether discard previous data")
	flag.Float64Var(&localTrafficProportion, "j", 1, "proportion of local traffic in cluster")

	prometheus.MustRegister(metrics)
}

func main() {
	flag.Parse()

	counterStore, err := redis_store.NewStore(redisAddr, redisPass, "blcl:")
	if err != nil {
		log.Println("new store error:", err)
	}

	limiterFactory := cluster_limiter.NewFactory(
		&cluster_limiter.ClusterLimiterFactoryOpts{
			Name:                       "test",
			HeartbeatInterval:          100 * time.Millisecond,
			InitLocalTrafficProportion: localTrafficProportion,
		},
		counterStore)
	limiterFactory.Start()

	limiterVec, err := limiterFactory.NewClusterLimiterVec(
		&cluster_limiter.ClusterLimiterOpts{
			Name:                limiterName,
			RewardTarget:        float64(targetNum),
			PeriodInterval:      time.Duration(resetInterval) * time.Second,
			DiscardPreviousData: true,
		},
		[]string{"label1", "label2"})

	if err != nil {
		log.Fatal(err)
	}

	lbs := []string{"c1", "c2"}
	limiter := limiterVec.WithLabelValues(lbs)

	go httpServer()
	go fakeTraffic(limiter)

	ticker := time.NewTicker(100000 * time.Microsecond)
	i := 0
	for range ticker.C {
		data := make(map[string]float64)

		data["pass_rate"] = limiter.PassRate()
		data["ideal_rate"] = limiter.IdealPassRate()
		data["reward_rate"] = limiter.IdealRewardRate()
		data["total_target"] = limiter.GetRewardTarget()
		data["ideal_reward"] = limiter.IdealReward()

		rewardCur, rewardTime := limiter.RewardCounter.ClusterValue(0)
		data["lag_time"] = limiter.LagTime(rewardCur, rewardTime)

		data["request_local"], _ = limiter.RequestCounter.LocalValue(0)
		data["request_pred"], _ = limiter.RequestCounter.ClusterValue(0)
		requestLast, _ := limiter.RequestCounter.ClusterValue(-1)
		data["request_last"] = requestLast

		data["pass_local"], _ = limiter.PassCounter.LocalValue(0)
		data["pass_pred"], _ = limiter.PassCounter.ClusterValue(0)
		passLast, _ := limiter.PassCounter.ClusterValue(-1)
		data["pass_last"] = passLast

		data["reward_local"], _ = limiter.RewardCounter.LocalValue(0)
		data["reward_pred"], _ = limiter.RewardCounter.ClusterValue(0)
		rewardLast, _ := limiter.RewardCounter.ClusterValue(-1)
		data["reward_last"] = rewardLast
		data["request_local_traffic_proportion"] = limiter.RequestCounter.LocalTrafficProportion()
		data["reward_local_traffic_proportion"] = limiter.RewardCounter.LocalTrafficProportion()

		for k, v := range data {
			metrics.WithLabelValues(instanceName, k).Set(v)
		}

		if i%10 == 0 {
			fmt.Println(data)
		}
		i++
	}

}

func httpServer() {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%v", listenPort), nil)
	log.Fatal(err)
}

func fakeTraffic(limiter *cluster_limiter.ClusterLimiter) {
	rand.Seed(time.Now().Unix())

	ticker := time.NewTicker(100000 * time.Microsecond)
	for range ticker.C {
		k := (time.Now().Unix() / 60) % 60
		if k >= 30 {
			k = 60 - k
		}
		v := k + 30
		v = v * mockTrafficFactor

		for j := 0; j < int(v); j++ {
			metrics.WithLabelValues(instanceName, "request").Add(1)
			if limiter.Take(float64(1)) == true {
				metrics.WithLabelValues(instanceName, "pass").Add(1)
				if rand.Float64() > 0.5 {
					metrics.WithLabelValues(instanceName, "reward").Add(1)
					limiter.Reward(float64(1))
				}
			}
		}
	}
}
