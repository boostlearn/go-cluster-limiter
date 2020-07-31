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

var numbers = make([]int, 0)
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
		counterStore)
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

func initNumber(cnt int) {
	numbers = make([]int, 0, cnt)
	for i := 0; i < cnt; i++ {
		numbers = append(numbers, rand.Int())
	}
}

func doSomething() {
	sort.Ints(numbers)
	//rand.Shuffle(len(numbers), func(i, j int) { numbers[i], numbers[j] = numbers[j], numbers[i] })
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

func Benchmark_Single_Directly_10(b *testing.B) {
	initNumber(10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomething()
	}
}

func Benchmark_Single_Counter_10(b *testing.B) {
	initNumber(10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomethingWithCounter()
	}
}


func Benchmark_Single_Limiter_10(b *testing.B) {
	initNumber(10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomethingWithLimiter()
	}
}

func Benchmark_Single_Directly_100(b *testing.B) {
	initNumber(100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomething()
	}
}

func Benchmark_Single_Counter_100(b *testing.B) {
	initNumber(100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomethingWithCounter()
	}
}


func Benchmark_Single_Limiter_100(b *testing.B) {
	initNumber(100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomethingWithLimiter()
	}
}

func Benchmark_Single_Directly_1000(b *testing.B) {
	initNumber(1000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomething()
	}
}

func Benchmark_Single_Counter_1000(b *testing.B) {
	initNumber(1000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomethingWithCounter()
	}
}


func Benchmark_Single_Limiter_1000(b *testing.B) {
	initNumber(1000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doSomethingWithLimiter()
	}
}