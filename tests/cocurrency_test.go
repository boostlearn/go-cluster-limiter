package tests

import "testing"

func Benchmark_Directly_Concurrency2(b *testing.B) {
	initNumber(200)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(2)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doSomething()
		}
	})
}

func Benchmark_Directly_Concurrency4(b *testing.B) {
	initNumber(200)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doSomething()
		}
	})
}

func Benchmark_Directly_Concurrency8(b *testing.B) {
	initNumber(200)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(8)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doSomething()
		}
	})
}

func Benchmark_Counter_Concurrency2(b *testing.B) {
	initNumber(200)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(2)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doSomethingWithCounter()
		}
	})
}

func Benchmark_Counter_Concurrency4(b *testing.B) {
	initNumber(200)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doSomethingWithCounter()
		}
	})
}

func Benchmark_Counter_Concurrency8(b *testing.B) {
	initNumber(200)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(8)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doSomethingWithCounter()
		}
	})
}

func Benchmark_Limiter_Concurrency2(b *testing.B) {
	initNumber(200)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(2)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doSomethingWithLimiter()
		}
	})
}

func Benchmark_Limiter_Concurrency4(b *testing.B) {
	initNumber(200)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doSomethingWithLimiter()
		}
	})
}

func Benchmark_Limiter_Concurrency8(b *testing.B) {
	initNumber(200)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doSomethingWithLimiter()
		}
	})
}