package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter/prometheus_reporter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"
)

var (
	redisAddr  string
	redisPass  string
	listenPort int64

	limiterType       string
	fakeTrafficFactor float64
	fakeRewardFactor  float64
)

func init() {
	flag.StringVar(&redisAddr, "s", "127.0.0.1:6379", "store: redis address")
	flag.StringVar(&redisPass, "p", "", "store: redis pass")
	flag.Int64Var(&listenPort, "l", 20001, "prometheus: listen port")
	flag.StringVar(&limiterType, "m", "normal", "limiter type: normal or score")
	flag.Float64Var(&fakeTrafficFactor, "f", 2, "mock traffic factor")
	flag.Float64Var(&fakeRewardFactor, "r", 1, "fake reward factor")

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
			Name:              "test",
			HeartbeatInterval: 100 * time.Millisecond,
			Reporter:          reporter,
			Store:             store,
		})
	limiterFactory.Start()

	err = limiterFactory.LoadFile(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	options := limiterFactory.AllOptions()
	optionsEncode, _ := json.Marshal(options)
	fmt.Println(string(optionsEncode))

	go fakeTraffic(limiterFactory.AllLimiters())

	http.Handle("/metrics", promhttp.Handler())
	err = http.ListenAndServe(fmt.Sprintf("0.0.0.0:%v", listenPort), nil)
	log.Fatal(err)
}

func fakeTraffic(limiters []*cluster_limiter.ClusterLimiter) {
	rand.Seed(time.Now().Unix())

	ticker := time.NewTicker(100000 * time.Microsecond)
	v := 0.0
	for range ticker.C {
		now := time.Now()
		hours := now.Sub(now.Truncate(time.Duration(3600) * time.Second)).Hours()
		v += (math.Sin(hours* 2 * math.Pi) + 2.0) * 30 * fakeTrafficFactor

        fmt.Println(">>>>", hours, " ", v, " ", math.Sin(hours* 2 * math.Pi))
		for ; v > 1.0; v -= 1 {
			for _, limiter := range limiters {
				if limiter.Options.TakeWithScore == false {
					if limiter.Take(float64(1)) == true {
						rewardRate := (math.Cos(hours*2*math.Pi)/4 + 0.5) * fakeRewardFactor
						if rand.Float64() < rewardRate {
							limiter.Reward(float64(1))
						}
					}
				} else {
					if limiter.TakeWithScore(float64(1), rand.Float64()) == true {
						rewardRate := (math.Cos(hours*2*math.Pi)/4 + 0.5) * fakeRewardFactor
						if rand.Float64() < rewardRate {
							limiter.Reward(float64(1))
						}
					}
				}
			}
		}
	}
}
