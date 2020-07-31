package tests

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
	"log"
	"math/rand"
	"sort"
	"testing"
	"time"
)

var counter *cluster_counter.ClusterCounter
var limiter *cluster_limiter.ClusterLimiter

func init() {
	counterStore := redis_store.NewStore("127.0.0.1:6379", "", "blcl:")

	counterFactory := cluster_counter.NewFactory(
		&cluster_counter.ClusterCounterFactoryOpts{
			Name:                     "",
			DefaultLocalTrafficRatio: 1.0,
			HeartbeatInterval:        100 * time.Millisecond,
		},
		counterStore)
	counterFactory.Start()

	counter, _ = counterFactory.NewClusterCounter(
		&cluster_counter.ClusterCounterOpts{
			Name:                  "test",
			BeginTime:             time.Time{},
			EndTime:               time.Time{},
			PeriodInterval:        time.Duration(60) * time.Second,
			DiscardPreviousData:   true,
			StoreDataInterval:     0,
			InitLocalTrafficRatio: 1.0,
		})

	limiterFactory := cluster_limiter.NewFactory(
		&cluster_limiter.ClusterLimiterFactoryOpts{
			Name:                  "test",
			HeartbeatInterval:     100 * time.Millisecond,
			InitLocalTrafficRatio: 1.0,
		},
		counterStore)
	limiterFactory.Start()

	var err error
	limiter, err = limiterFactory.NewClusterLimiter(
		&cluster_limiter.ClusterLimiterOpts{
			Name:                "test",
			PeriodInterval:      time.Duration(60) * time.Second,
			ReserveInterval:     0,
			BurstInterval:       0,
			MaxBoostFactor:      0,
			DiscardPreviousData: true,
		})

	if err != nil {
		log.Fatal(err)
	}

	limiter.SetTarget(10000000)
}

func initNumber(cnt int) []int {
	numbers := make([]int, 0, cnt)
	for i := 0; i < cnt; i++ {
		numbers = append(numbers, rand.Int())
	}
	return numbers
}

func doSomething(numbers []int) {
	sort.Ints(numbers)
	//rand.Shuffle(len(numbers), func(i, j int) { numbers[i], numbers[j] = numbers[j], numbers[i] })
}

func doSomethingWithCounter(numbers []int) {
	counter.Add(1)
	doSomething(numbers)
}

func doSomethingWithLimiter(numbers []int) {
	if limiter.Acquire(1) {
		limiter.Reward(1)
	}
	doSomething(numbers)
}

func doOnlyCounter() {
	counter.Add(1)
}

func doOnlyLimiter() {
	if limiter.Acquire(1) {
		limiter.Reward(1)
	}
}