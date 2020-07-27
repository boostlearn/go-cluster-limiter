package main

import (
	"flag"
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
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

	discardPreviousData bool

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
		Help:      "数量",
	}, []string{"counter_instance", "metric_name"})
)

func init() {
	flag.Int64Var(&targetNum, "a", 1000000, "total target num")
	flag.Int64Var(&resetInterval, "b", 600, "reset data interval")
	flag.Int64Var(&mockTrafficFactor, "c", 60, "mock traffic factor")
	flag.StringVar(&limiterName, "d", "test_cluster_limiter", "limiter's unique name")
	flag.StringVar(&instanceName, "e", "test1", "test instance name")
	flag.StringVar(&redisAddr, "f", "127.0.0.1:6379", "store: redis address")
	flag.StringVar(&redisPass, "g", "", "store: redis pass")
	flag.Int64Var(&listenPort, "h", 20001, "prometheus: listen port")
	flag.BoolVar(&discardPreviousData, "i", true, "whether discard previous data")

	prometheus.MustRegister(metrics)
}

func main() {
	flag.Parse()

	counterStore := redis_store.NewStore(redisAddr, redisPass, "blcl:")
	counterFactory := cluster_counter.NewFactory(&cluster_counter.ClusterCounterFactoryOpts{}, counterStore)
	limiterFactory := cluster_limiter.NewFactory(&cluster_limiter.ClusterLimiterFactoryOpts{}, counterFactory)

	limiterVec, err := limiterFactory.NewClusterLimiterVec(&cluster_limiter.ClusterLimiterOpts{
		Name:                limiterName,
		PeriodInterval:      time.Duration(resetInterval) * time.Second,
		DiscardPreviousData: true,
	}, []string{"label1", "label2"})

	if err != nil {
		log.Fatal(err)
	}

	lbs := []string{"c1", "c2"}
	limiter := limiterVec.WithLabelValues(lbs)
	limiter.SetTarget(float64(targetNum))

	if err != nil {
		fmt.Println(err)
		return
	}

	go httpServer()
	go fakeTraffic(limiter)

	ticker := time.NewTicker(100 * time.Microsecond)
	for range ticker.C {
		data := make(map[string]float64)

		data["pass_rate"] = float64(limiter.PassRate())
		data["ideal_rate"] = float64(limiter.IdealPassRate())
		data["total_target"] = float64(limiter.GetTarget())
		data["pacing_target"] = float64(limiter.PacingReward())

		rewardCur, rewardTime := limiter.RewardCounter.ClusterValue(0)
		data["lost_time"] = float64(limiter.LostTime(rewardCur, rewardTime))

		data["request_local"], _ = limiter.RequestCounter.LocalValue(0)
		data["request_pred"], _ = limiter.RequestCounter.ClusterValue(0)
		requestLast, _ := limiter.RequestCounter.ClusterValue(-1)
		data["request_last"] = float64(requestLast)

		data["pass_local"], _ = limiter.PassCounter.LocalValue(0)
		data["pass_pred"], _ = limiter.PassCounter.ClusterValue(0)
		passLast, _ := limiter.PassCounter.ClusterValue(-1)
		data["pass_last"] = float64(passLast)

		data["reward_local"], _ = limiter.RewardCounter.LocalValue(0)
		data["reward_pred"], _ = limiter.RewardCounter.ClusterValue(0)
		rewardLast, _ := limiter.RewardCounter.ClusterValue(-1)
		data["reward_last"] = float64(rewardLast)
		data["request_local_traffic_ratio"] = limiter.RequestCounter.LocalTrafficRatio()
		data["reward_local_traffic_ratio"] = limiter.RewardCounter.LocalTrafficRatio()

		for k, v := range data {
			metrics.WithLabelValues(instanceName, k).Set(v)
		}

		fmt.Println(data)
	}

}

func httpServer() {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%v", listenPort), nil)
	log.Fatal(err)
}

func fakeTraffic(counter *cluster_limiter.ClusterLimiter) {
	rand.Seed(time.Now().Unix())

	var i = 0
	for {
		i += 1

		k := (time.Now().Unix() / mockTrafficFactor) % mockTrafficFactor
		if k > mockTrafficFactor/2 {
			k = mockTrafficFactor - k
		}
		v := k + mockTrafficFactor/2

		if counter.Take(float64(v)) == true {
			counter.Reward(float64(v))
		}
		time.Sleep(time.Duration(10) * time.Microsecond)
	}
}
