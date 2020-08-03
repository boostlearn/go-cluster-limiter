package main

import (
	"flag"
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
	"log"
	"math/rand"
	"time"
)

var (
	counterName            string
	testerName             string
	discardPreviousData    bool
	periodInterval         int64
	mockTrafficFactor      float64
	listenPort             int64
	localTrafficProportion float64
	redisAddr              string
	redisPass              string

	//metrics = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	//	Namespace: "boostlearn",
	//	Subsystem: "test",
	//	Name:      "cluster_counter",
	//}, []string{"counter_instance", "metric_name"})
)

func init() {
	flag.StringVar(&counterName, "a", "test_cluster_counter", "cluster counter's unique name")
	flag.StringVar(&testerName, "b", "test1", "test instance name")
	flag.Int64Var(&periodInterval, "c", 60, "reset data interval")
	flag.Float64Var(&localTrafficProportion, "e", 0.1, "proportion of local traffic in cluster")
	flag.StringVar(&redisAddr, "f", "127.0.0.1:6379", "store: redis address")
	flag.StringVar(&redisPass, "g", "", "store: redis pass")
	flag.Int64Var(&listenPort, "h", 20001, "prometheus listen port")
	flag.Float64Var(&mockTrafficFactor, "i", 1.0, "mock traffic factor")
	flag.BoolVar(&discardPreviousData, "j", true, "whether discard previous data")
	//prometheus.MustRegister(metrics)
}

func main() {
	flag.Parse()

	store, err := redis_store.NewStore(redisAddr, redisPass, "blcl:")
	if err != nil {
		log.Println("newStore Error:", err)
	}
	factory := cluster_counter.NewFactory(
		&cluster_counter.ClusterCounterFactoryOpts{
			DefaultLocalTrafficProportion: localTrafficProportion,
			HeartbeatInterval:             100 * time.Millisecond,
		}, store)
	factory.Start()

	counterVec, err := factory.NewClusterCounterVec(
		&cluster_counter.ClusterCounterOpts{
			Name:                       counterName,
			PeriodInterval:             time.Duration(periodInterval) * time.Second,
			DiscardPreviousData:        discardPreviousData,
			InitLocalTrafficProportion: localTrafficProportion,
		},
		[]string{"label1", "label2"})
	if err != nil {
		fmt.Println(err)
		return
	}

	lbs := []string{"c1", "c2"}
	counter := counterVec.WithLabelValues(lbs)

	go mockTraffic(counter)
	go httpServer()

	i := 0
	ticker := time.NewTicker(100000 * time.Microsecond)
	for range ticker.C {
		clusterLast, _ := counter.ClusterValue(-1)
		clusterCur, _ := counter.ClusterValue(0)
		localCur, _ := counter.LocalValue(0)
		var data = map[string]float64{
			"local_current":            localCur,
			"cluster_last":             clusterLast,
			"cluster_pred":             clusterCur,
			"local_traffic_proportion": counter.LocalTrafficProportion(),
			"local_recently":           counter.LocalRecently(),
			"cluster_recently":         counter.ClusterRecently(),
		}

		//for k, v := range data {
		//	metrics.WithLabelValues(testerName, k).Set(v)
		//}

		if i%10 == 0 {
			fmt.Println(data)
		}
		i++
	}

}

func httpServer() {
	//http.Handle("/metrics", promhttp.Handler())
	//err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%v", listenPort), nil)
	//log.Fatal(err)
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
