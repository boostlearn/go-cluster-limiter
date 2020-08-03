package cluster_counter

import (
	"testing"
	"time"
)

func TestClusterCounter_Expire(t *testing.T) {
	counter := &ClusterCounter{
		initTime:                   time.Time{},
		beginTime:                  time.Time{},
		endTime:                    time.Time{},
		periodInterval:             0,
	}

	if counter.Expire() == false {
		t.Fatal("expired error")
	}

	counter.periodInterval = 100 * time.Second
	if counter.Expire() == true {
		t.Fatal("expired error")
	}

	counter.beginTime = time.Now().Add(-100*time.Second)
	counter.endTime = time.Now().Add(100*time.Second)
	if counter.Expire() == true {
		t.Fatal("expired error")
	}
}