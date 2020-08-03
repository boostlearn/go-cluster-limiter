package redis_store

import (
	"fmt"
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
func (store *RedisStore) Store(name string, beginTime time.Time, endTime time.Time, lbs map[string]string, value float64, force bool) error {
	key := store.keyPrefix + generateRedisKey(name, beginTime, endTime, lbs)
	v := store.client.IncrBy(key, int64(value*10000))
	if endTime.After(beginTime) {
		store.client.Expire(key, endTime.Sub(beginTime))
	}
	return v.Err()
}

// load cluster's data for clients
func (store *RedisStore) Load(name string, beginTime time.Time, endTime time.Time, lbs map[string]string) (float64, error) {
	v := store.client.Get(store.keyPrefix + generateRedisKey(name, beginTime, endTime, lbs))
	if v.Err() == nil {
		t, err := v.Result()
		if err == nil {
			value, err := strconv.ParseFloat(t, 64)
			return value / 10000, err
		}
		return 0, err
	} else {
		return 0, v.Err()
	}
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
