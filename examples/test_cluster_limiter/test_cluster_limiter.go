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
	"math/rand"
	"net/http"
	"time"
)

var (
	fakeTrafficFactor int64
	fakeRewardRate    float64

	redisAddr string
	redisPass string

	listenPort int64
)

func init() {
	flag.Int64Var(&fakeTrafficFactor, "c", 2, "mock traffic factor")
	flag.StringVar(&redisAddr, "f", "127.0.0.1:6379", "store: redis address")
	flag.StringVar(&redisPass, "g", "", "store: redis pass")
	flag.Int64Var(&listenPort, "h", 20002, "prometheus: listen port")
	flag.Float64Var(&fakeRewardRate, "m", 0.5, "fake reward rate")
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
			Reporter:                   reporter,
			Store:                      store,
		})
	limiterFactory.Start()

	err = limiterFactory.LoadFile(flag.Arg(0))
	if err != nil  {
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
	for range ticker.C {
		k := (time.Now().Unix() / 60) % 60
		if k >= 30 {
			k = 60 - k
		}
		v := k + 30
		v = v * fakeTrafficFactor

		for j := 0; j < int(v); j++ {
			for _, limiter := range limiters {
				if limiter.Take(float64(1)) == true {
					if rand.Float64() < fakeRewardRate {
						limiter.Reward(float64(1))
					}
				}
			}
		}
	}
}


func fakeTraffic2(limiter *cluster_limiter.ClusterLimiter) {
	rand.Seed(time.Now().Unix())

	ticker := time.NewTicker(100000 * time.Microsecond)
	for range ticker.C {
		k := (time.Now().Unix() / 60) % 60
		if k >= 30 {
			k = 60 - k
		}
		v := k + 30
		v = v * fakeTrafficFactor

		for j := 0; j < int(v); j++ {
			if limiter.TakeWithScore(float64(1), rand.Float64()) == true {
				if rand.Float64() < fakeRewardRate {
					limiter.Reward(float64(1))
				}
			}
		}
	}
}
