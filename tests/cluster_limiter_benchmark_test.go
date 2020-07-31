package tests

import (
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"github.com/boostlearn/go-cluster-limiter/cluster_limiter"
	"log"
	"math/rand"
	"sort"
	"testing"
	"time"
)

var numbers = make([]int, 0)
var counter *cluster_counter.ClusterCounter
var limiter *cluster_limiter.ClusterLimiter

func init() {
	factory := cluster_counter.NewFactory(
		&cluster_counter.ClusterCounterFactoryOpts{
			Name:                     "",
			DefaultLocalTrafficRatio: 0.1,
			HeartbeatInterval:        100 * time.Millisecond,
		},
		nil )
	factory.Start()

	counter, _ = factory.NewClusterCounter(
		&cluster_counter.ClusterCounterOpts{
			Name:                     "test",
			BeginTime:                time.Time{},
			EndTime:                  time.Time{},
			PeriodInterval:           time.Duration(3600) * time.Second,
			DiscardPreviousData:      true,
			StoreDataInterval:        0,
			InitLocalTrafficRatio: 1.0,
		})

	limiterFactory := cluster_limiter.NewFactory(
		&cluster_limiter.ClusterLimiterFactoryOpts{
			Name:                     "test",
			HeartbeatInterval: 100 * time.Millisecond,
			InitLocalTrafficRatio:  1.0,
		},
		nil)
	limiterFactory.Start()

	var err error
	limiter, err = limiterFactory.NewClusterLimiter(
		&cluster_limiter.ClusterLimiterOpts{
			Name:                "test",
			PeriodInterval:      time.Duration(3600) * time.Second,
			ReserveInterval:     0,
			BurstInterval:       0,
			MaxBoostFactor:      0,
			DiscardPreviousData: true,
		})

	if err != nil {
		log.Fatal(err)
	}

	limiter.SetTarget(100000)
}

func initNumber() {
	numbers = make([]int, 0, 50)
	for i := 0; i < 50; i++ {
		numbers = append(numbers, rand.Int())
	}
}

func doSomething() {
	sort.Ints(numbers)
	rand.Shuffle(len(numbers), func(i, j int) { numbers[i], numbers[j] = numbers[j], numbers[i] })
}

func doSomethingWithCounter() {
	counter.Add(1)
	doSomething()
}

func doSomethingWithLimiter() {
	if limiter.Acquire(1) {
		limiter.Reward(1)
	}
	doSomething()
}

func Benchmark_Single_Directly(b *testing.B) {
	initNumber()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomething()
	}
}

func Benchmark_Single_Counter(b *testing.B) {
	initNumber()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomethingWithCounter()
	}
}


func Benchmark_Single_Limiter(b *testing.B) {
	initNumber()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomethingWithLimiter()
	}
}
