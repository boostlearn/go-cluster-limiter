package cluster_counter

type CounterValue struct {
	Sum   float64
	Count int64
}

func (counterValue *CounterValue) Add(c CounterValue) CounterValue {
	return CounterValue{
		Sum:   counterValue.Sum + c.Sum,
		Count: counterValue.Count + c.Count,
	}
}

func (counterValue *CounterValue) Sub(c CounterValue) CounterValue {
	return CounterValue{
		Sum:   counterValue.Sum - c.Sum,
		Count: counterValue.Count - c.Count,
	}
}

func (counterValue *CounterValue) Decline(c CounterValue, ratio float64) {
	counterValue.Sum = counterValue.Sum*ratio + c.Sum*(1-ratio)
	counterValue.Count = int64(float64(counterValue.Count)*ratio + float64(c.Count)*(1-ratio))
}
