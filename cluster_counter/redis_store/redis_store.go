package redis_store

import (
	"fmt"
	"github.com/boostlearn/go-cluster-limiter/cluster_counter"
	"github.com/go-redis/redis"
	"sort"
	"strconv"
	"strings"
	"time"
)

const RedisKeySep = "####"

type RedisStore struct {
	client    *redis.Client
	keyPrefix string
}

// build new store from redis's address
func NewStore(address string, pass string, keyPrefix string) (*RedisStore, error) {
	options := &redis.Options{
		Addr:         address,
		Password:     pass,
		DB:           0,
		DialTimeout:  1 * time.Second,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	}
	cli := redis.NewClient(options)

	return NewRedisStore(cli, keyPrefix)
}

// // build new store from redis cli
func NewRedisStore(cli *redis.Client, keyPrefix string) (*RedisStore, error) {
	_, err := cli.Ping().Result()
	if err != nil {
		return nil, err
	}
	return &RedisStore{client: cli, keyPrefix: keyPrefix}, nil
}

// store client's data within cluster
func (store *RedisStore) Store(name string, beginTime time.Time, endTime time.Time, lbs map[string]string,
	value cluster_counter.CounterValue, force bool) error {
	redisKey := store.keyPrefix + generateRedisKey(name, beginTime, endTime, lbs)

	_ = store.client.IncrByFloat(redisKey+":sum", value.Sum)
	result := store.client.IncrBy(redisKey+":cnt", int64(value.Count))
	if endTime.After(beginTime) {
		store.client.Expire(redisKey+":sum", endTime.Sub(beginTime))
		store.client.Expire(redisKey+":cnt", endTime.Sub(beginTime))
	}
	return result.Err()
}

// load cluster's data for clients
func (store *RedisStore) Load(name string, beginTime time.Time, endTime time.Time, lbs map[string]string,
) (cluster_counter.CounterValue, error) {
	key := store.keyPrefix + generateRedisKey(name, beginTime, endTime, lbs)

	var counterValue cluster_counter.CounterValue
	var err error
	result := store.client.Get(key + ":sum")
	err = result.Err()
	if err == nil {
		t, err := result.Result()
		if err == nil {
			value, _ := strconv.ParseFloat(t, 64)
			counterValue.Sum = value
		}
	}
	result = store.client.Get(key + ":cnt")
	err = result.Err()
	if err == nil {
		t, err := result.Result()
		if err == nil {
			counterValue.Count, err = strconv.ParseInt(t, 10, 64)

		}
	}
	if err == redis.Nil {
		return counterValue, nil
	}

	return counterValue, err
}

func generateRedisKey(name string, beginTime time.Time, endTime time.Time, lbs map[string]string) string {
	var labels []string
	for _, v := range lbs {
		labels = append(labels, v)
	}
	sort.Stable(sort.StringSlice(labels))
	key := fmt.Sprintf("%v%v%v_%v%v%v", name, RedisKeySep, beginTime.Unix(), endTime.Unix(), RedisKeySep, strings.Join(labels, RedisKeySep))
	return key
}
