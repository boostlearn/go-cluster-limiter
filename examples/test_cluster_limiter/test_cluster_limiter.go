package main

import (
	"flag"
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter/prometheus_reporter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"math/rand"
	"net/http"
	"time"
)

var (
	limiterName  string
	instanceName string

	discardPreviousData    bool
	localTrafficProportion float64
	rewardRate float64

	targetNum         int64
	startTime         int64
	endTime           int64
	resetInterval     int64
	mockTrafficFactor int64

	redisAddr string
	redisPass string

	listenPort int64
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
	flag.Int64Var(&startTime, "k", 0, "start time since now [seconds]")
	flag.Int64Var(&endTime, "l", 0, "end time since now [seconds]")
	flag.Float64Var(&rewardRate, "m", 0.5, "reward rate")
}

func main() {
	flag.Parse()

	store, err := redis_store.NewStore(redisAddr, redisPass, "blcl:")
	if err != nil {
		log.Println("new store error:", err)
	}

	reporter := prometheus_reporter.NewLimiterReporter("boostlearn")

	limiterFactory := cluster_limiter.NewFactory(
		&cluster_limiter.ClusterLimiterFactoryOpts{
			Name:                       "test",
			HeartbeatInterval:          100 * time.Millisecond,
			InitLocalTrafficProportion: localTrafficProportion,
			Reporter:                   reporter,
			Store:                      store,
		})
	limiterFactory.Start()

	limiterVec, err := limiterFactory.NewClusterLimiterVec(
		&cluster_limiter.ClusterLimiterOpts{
			Name:                limiterName,
			BeginTime:           time.Now().Add(time.Duration(startTime) * time.Second).Truncate(time.Second),
			EndTime:             time.Now().Add(time.Duration(endTime) * time.Second).Truncate(time.Second),
			PeriodInterval:      time.Duration(resetInterval) * time.Second,
			DiscardPreviousData: true,
		},
		[]string{})

	if err != nil {
		log.Fatal(err)
	}

	var lbs []string
	limiter := limiterVec.WithLabelValues(lbs, float64(targetNum))
	go fakeTraffic(limiter)

	http.Handle("/metrics", promhttp.Handler())
	err = http.ListenAndServe(fmt.Sprintf("0.0.0.0:%v", listenPort), nil)
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
			if limiter.Take(float64(1)) == true {
				if rand.Float64() < rewardRate {
					limiter.Reward(float64(1))
				}
			}
		}
	}
}
