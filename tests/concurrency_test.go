package tests

import "testing"

func Benchmark_Directly_Concurrency(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
	    numbers := initNumber(200)
		for pb.Next() {
			doSomething(numbers)
		}
	})
}


func Benchmark_Counter_Concurrency(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
	    numbers := initNumber(200)
		for pb.Next() {
			doSomethingWithCounter(numbers)
		}
	})
}


func Benchmark_Limiter_Concurrency(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
	    numbers := initNumber(200)
		for pb.Next() {
			doSomethingWithLimiter(numbers)
		}
	})
}
