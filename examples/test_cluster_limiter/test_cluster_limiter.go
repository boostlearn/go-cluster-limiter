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

	discardPreviousData bool
	localTrafficRatio   float64

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
	flag.Int64Var(&targetNum, "a", 10000, "total target num")
	flag.Int64Var(&resetInterval, "b", 600, "reset data interval")
	flag.Int64Var(&mockTrafficFactor, "c", 1, "mock traffic factor")
	flag.StringVar(&limiterName, "d", "test_cluster_limiter", "limiter's unique name")
	flag.StringVar(&instanceName, "e", "test1", "test instance name")
	flag.StringVar(&redisAddr, "f", "127.0.0.1:6379", "store: redis address")
	flag.StringVar(&redisPass, "g", "", "store: redis pass")
	flag.Int64Var(&listenPort, "h", 20001, "prometheus: listen port")
	flag.BoolVar(&discardPreviousData, "i", true, "whether discard previous data")
	flag.Float64Var(&localTrafficRatio, "j", 0.1, "default local traffic ratio of all cluster")

	prometheus.MustRegister(metrics)
}

func main() {
	flag.Parse()

	counterStore := redis_store.NewStore(redisAddr,
		redisPass,
		"blcl:")
	limiterFactory := cluster_limiter.NewFactory(
		&cluster_limiter.ClusterLimiterFactoryOpts{
			Name:                     "test",
			DefaultHeartbeatInterval: 100 * time.Millisecond,
			DefaultLocalTrafficRate:  localTrafficRatio,
		},
		counterStore)
	limiterFactory.Start()

	limiterVec, err := limiterFactory.NewClusterLimiterVec(
		&cluster_limiter.ClusterLimiterOpts{
			Name:                limiterName,
			BeginTime:           time.Time{},
			EndTime:             time.Time{},
			CompletionTime:      time.Time{},
			PeriodInterval:      time.Duration(resetInterval) * time.Second,
			ReserveInterval:     0,
			BurstInterval:       0,
			MaxBoostFactor:      0,
			DiscardPreviousData: true,
		},
		[]string{"label1", "label2"})

	if err != nil {
		log.Fatal(err)
	}

	lbs := []string{"c1", "c2"}
	limiter := limiterVec.WithLabelValues(lbs)
	limiter.SetTarget(float64(targetNum))

	go httpServer()
	go fakeTraffic(limiter)

	ticker := time.NewTicker(100000 * time.Microsecond)
	i := 0
	for range ticker.C {
		data := make(map[string]float64)

		data["pass_rate"] = float64(limiter.PassRate())
		data["ideal_rate"] = float64(limiter.IdealPassRate())
		data["total_target"] = float64(limiter.GetTarget())
		data["pacing_target"] = float64(limiter.PacingReward())

		rewardCur, rewardTime := limiter.RewardCounter.ClusterValue(0)
		data["lag_time"] = float64(limiter.LagTime(rewardCur, rewardTime))

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

func fakeTraffic(counter *cluster_limiter.ClusterLimiter) {
	rand.Seed(time.Now().Unix())

	ticker := time.NewTicker(100000 * time.Microsecond)
	for range ticker.C {
		k := (time.Now().Unix() / 600) % 6
		if k >= 3 {
			k = 6 - k
		}
		v := k + 3
		v = v * mockTrafficFactor

		for j := 0; j < int(v); j++ {
			if counter.Take(float64(1)) == true {
				counter.Reward(float64(1))
			}
		}
	}
}
