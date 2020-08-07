package main

import (
	"flag"
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter/prometheus_reporter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"math/rand"
	"net/http"
	"time"
)

var (
	counterName            string
	testerName             string
	discardPreviousData    bool
	startTime              int64
	endTime                int64
	periodInterval         int64
	mockTrafficFactor      float64
	listenPort             int64
	localTrafficProportion float64
	redisAddr              string
	redisPass              string
)

func init() {
	flag.StringVar(&counterName, "a", "test_cluster_counter", "cluster counter's unique name")
	flag.StringVar(&testerName, "b", "test1", "test instance name")
	flag.Int64Var(&periodInterval, "c", 60, "reset data interval")
	flag.Float64Var(&localTrafficProportion, "e", 1.0, "proportion of local traffic in cluster")
	flag.StringVar(&redisAddr, "f", "127.0.0.1:6379", "store: redis address")
	flag.StringVar(&redisPass, "g", "", "store: redis pass")
	flag.Int64Var(&listenPort, "h", 20001, "prometheus listen port")
	flag.Float64Var(&mockTrafficFactor, "i", 1.0, "mock traffic factor")
	flag.BoolVar(&discardPreviousData, "j", true, "whether discard previous data")
	flag.Int64Var(&startTime, "k", 0, "start time since now [seconds]")
	flag.Int64Var(&endTime, "l", 0, "end time since now [seconds]")
}

func main() {
	flag.Parse()

	store, err := redis_store.NewStore(redisAddr, redisPass, "blcl:")
	if err != nil {
		log.Println("newStore Error:", err)
	}

	reporter := prometheus_reporter.NewCounterReporter("boostlearn")

	factory := cluster_counter.NewFactory(
		&cluster_counter.ClusterCounterFactoryOpts{
			DefaultLocalTrafficProportion: localTrafficProportion,
			HeartbeatInterval:             100 * time.Millisecond,
			Store:                         store,
			Reporter:                      reporter,
		})
	factory.Start()

	counterVec, err := factory.NewClusterCounterVec(
		&cluster_counter.ClusterCounterOpts{
			Name:                       counterName,
			BeginTime:                  time.Now().Add(time.Duration(startTime) * time.Second).Truncate(time.Second),
			EndTime:                    time.Now().Add(time.Duration(endTime) * time.Second).Truncate(time.Second),
			PeriodInterval:             time.Duration(periodInterval) * time.Second,
			DiscardPreviousData:        discardPreviousData,
			InitLocalTrafficProportion: localTrafficProportion,
		},
		[]string{})
	if err != nil {
		fmt.Println(err)
		return
	}

	var lbs []string
	counter := counterVec.WithLabelValues(lbs)

	go mockTraffic(counter)

	http.Handle("/metrics", promhttp.Handler())
	err = http.ListenAndServe(fmt.Sprintf("0.0.0.0:%v", listenPort), nil)
	log.Fatal(err)

}

func mockTraffic(counter *cluster_counter.ClusterCounter) {
	rand.Seed(time.Now().Unix())

	ticker := time.NewTicker(100000 * time.Microsecond)
	var gen float64
	for range ticker.C {
		k := (time.Now().Unix() / 600) % 6
		if k >= 3 {
			k = 6 - k
		}
		v := float64(k + 3)
		gen += v * mockTrafficFactor

		for gen > 1.0 {
			counter.Add(float64(1))
			gen -= 1
		}

		time.Sleep(time.Duration(10) * time.Microsecond)
	}
}
