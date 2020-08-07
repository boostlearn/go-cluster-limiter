package tests

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter/redis_store"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
	"log"
	"math/rand"
	"sort"
	"time"
)

var counter *cluster_counter.ClusterCounter
var limiter *cluster_limiter.ClusterLimiter
var scorelimiter *cluster_limiter.ClusterLimiter

func init() {
	counterStore, err := redis_store.NewStore("127.0.0.1:6379", "", "blcl:")
	if err != nil {
		log.Println("new store error:", err)
	}

	counterFactory := cluster_counter.NewFactory(
		&cluster_counter.ClusterCounterFactoryOpts{
			Name:              "test",
			HeartbeatInterval: 1000 * time.Millisecond,
			Store:             counterStore,
		})
	counterFactory.Start()

	counter, _ = counterFactory.NewClusterCounter(
		&cluster_counter.ClusterCounterOpts{
			Name:                       "test",
			ResetInterval:              time.Duration(60) * time.Second,
			DiscardPreviousData:        true,
			StoreDataInterval:          0,
			InitLocalTrafficProportion: 1.0,
		})

	limiterFactory := cluster_limiter.NewFactory(
		&cluster_limiter.ClusterLimiterFactoryOpts{
			Name:                       "test",
			HeartbeatInterval:          1000 * time.Millisecond,
			InitLocalTrafficProportion: 1.0,
			Store:                      counterStore,
		})
	limiterFactory.Start()

	limiter, err = limiterFactory.NewClusterLimiter(
		&cluster_limiter.ClusterLimiterOpts{
			Name:                "test",
			RewardTarget:        10000000,
			PeriodInterval:      time.Duration(60) * time.Second,
			DiscardPreviousData: true,
		})

	if err != nil {
		log.Fatal(err)
	}

	scorelimiter, err = limiterFactory.NewClusterLimiter(
		&cluster_limiter.ClusterLimiterOpts{
			Name:                     "test",
			RewardTarget:             10000000,
			PeriodInterval:           time.Duration(60) * time.Second,
			ScoreSamplesMax:          10000,
			ScoreSamplesSortInterval: 10 * time.Second,
			DiscardPreviousData:      true,
		})

	if err != nil {
		log.Fatal(err)
	}
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

func doOnlyScoreLimiter() {
	if scorelimiter.TakeWithScore(1, rand.Float64()) {
		scorelimiter.Reward(1)
	}
}
