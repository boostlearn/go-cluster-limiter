package redis_store

import (
	"testing"
	"time"
)

func TestNewRedisStore(t *testing.T) {
	_, err := NewStore("127.0.0.22:6379", "", "")
	if err == nil {
		t.Error("should check redis")
	} else {
		t.Log(err)
	}
}

func TestRedisStore_LoadAndLoad(t *testing.T) {
	store, err := NewStore("127.0.0.1:6379", "", "")
	if err != nil {
		t.Fatal("check redis", err)
	}

	startTime := time.Now().Truncate(time.Second)
	endTime := startTime.Add(10 * time.Second)
	lbs := make(map[string]string)
	lbs["a1"] = "c2"
	lbs["a2"] = "c1"

	err2 := store.Store("test", startTime, endTime, lbs, 100, false)
	if err2 != nil {
		t.Fatal("store data error", err2)
	}

	v, err3 := store.Load("test", startTime, endTime, lbs)
	if err3 != nil {
		t.Fatal("load Data error", err3)
	}

	if v != 100 {
		t.Fatal("query value error")
	}

	err4 := store.Store("test", startTime, endTime, lbs, 200, false)
	if err4 != nil {
		t.Fatal("store data error", err2)
	}

	v2, err5 := store.Load("test", startTime, endTime, lbs)
	if err5 != nil {
		t.Fatal("load Data error", err3)
	}

	if v2 != 300 {
		t.Fatal("merge data error")
	}
}
