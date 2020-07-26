package main

import (
	"flag"
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
	"math/rand"
	"time"
)

var (
	counterName  string
	instanceName string

	discardPreviousData bool

	updateInterval int64
	periodInterval  int64

	mockTrafficFactor int64

	listenPort int64

	localTrafficRate float64

	redisAddr string
	redisPass string
)

func init() {
	flag.StringVar(&counterName, "a", "test_cluster_counter", "cluster counter's unique name")
	flag.StringVar(&instanceName, "b", "test1", "test instance name")
	flag.Int64Var(&periodInterval, "c", 60, "reset data interval")
	flag.Int64Var(&updateInterval, "d", 1000, "calculate traffic ratio interval [ms]")
	flag.Float64Var(&localTrafficRate, "e", 0.5, "default local traffic ratio of all cluster")
	flag.StringVar(&redisAddr, "f", "127.0.0.1:6379", "store: redis address")
	flag.StringVar(&redisPass, "g", "", "store: redis pass")
	flag.Int64Var(&listenPort, "h", 20002, "prometheus listen port")
	flag.Int64Var(&mockTrafficFactor, "i", 60, "mock traffic factor")
	flag.BoolVar(&discardPreviousData, "j", true, "whether discard previous data")
}

func main() {
	flag.Parse()
	factory := cluster_counter.NewFactory(&cluster_counter.ClusterCounterFactoryOpts{
		DefaultLocalTrafficRatio: localTrafficRate,
		HeartBeatInterval:           time.Duration(updateInterval) * time.Millisecond,
	}, redis_store.NewStore(redisAddr, redisPass, "blcl:"))

	counterVec, err := factory.NewClusterCounterVec(&cluster_counter.ClusterCounterOpts{
		Name:                     counterName,
		PeriodInterval:            time.Duration(periodInterval) * time.Second,
		DefaultLocalTrafficRatio: localTrafficRate,
		DiscardPreviousData:      discardPreviousData,
	}, []string{"label1", "label2"})
	if err != nil {
		fmt.Println(err)
		return
	}

	lbs := []string{"c1", "c2"}
	counter := counterVec.WithLabelValues(lbs)

	go mockTraffic(counter)

	for {
		clusterLast, _ := counter.ClusterValue(-1)
		clusterCur, _ := counter.ClusterValue(0)
		localCur, _ := counter.LocalValue(0)
		var data = map[string]float64{
			"local_current":       localCur,
			"cluster_last":        clusterLast,
			"cluster_pred":        clusterCur,
			"local_traffic_ratio": counter.LocalTrafficRatio(),
		}

		fmt.Println(data)
		time.Sleep(10 * time.Millisecond)
	}

}

func mockTraffic(counter *cluster_counter.ClusterCounter) {
	rand.Seed(time.Now().Unix())

	var i = 0
	for {
		i += 1
		k := (time.Now().Unix() / mockTrafficFactor) % mockTrafficFactor
		if k > mockTrafficFactor/2 {
			k = mockTrafficFactor - k
		}
		v := k + mockTrafficFactor/2

		counter.Add(float64(v))
		time.Sleep(time.Duration(10) * time.Microsecond)
	}
}
