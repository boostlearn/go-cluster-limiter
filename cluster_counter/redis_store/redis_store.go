package redis_store

import (
	"fmt"
	"github.com/go-redis/redis"
	"sort"
	"strconv"
	"strings"
	"time"
)

type RedisStore struct {
	cli    *redis.Client
	prefix string
}

const SEP = "####"

func NewStore(address string, pass string, prefix string) *RedisStore {
	options := &redis.Options{
		Addr:         address,
		Password:     pass,
		DB:           0,
		DialTimeout:  1 * time.Second,
		ReadTimeout:  500 * time.Millisecond,
		WriteTimeout: 500 * time.Millisecond,
	}
	cli := redis.NewClient(options)

	return NewRedisStore(cli, prefix)
}

func NewRedisStore(cli *redis.Client, prefix string) *RedisStore {
	return &RedisStore{cli: cli, prefix: prefix}
}

func generateRedisKey(name string, beginTime time.Time, endTime time.Time, lbs map[string]string) string {
	var labels []string
	for _, v := range lbs {
		labels = append(labels, v)
	}
	sort.Stable(sort.StringSlice(labels))
	key := fmt.Sprintf("%v%v%v_%v%v%v", name, SEP, beginTime.Unix(), endTime.Unix(), SEP, strings.Join(labels, SEP))
	return key
}

func (store *RedisStore) Store(name string, beginTime time.Time, endTime time.Time, lbs map[string]string, value float64, force bool) error {
	key := store.prefix + generateRedisKey(name, beginTime, endTime, lbs)
	v := store.cli.IncrBy(key, int64(value*10000))
	if endTime.After(beginTime) {
		store.cli.Expire(key, endTime.Sub(beginTime))
	}
	return v.Err()
}

func (store *RedisStore) Load(name string, beginTime time.Time, endTime time.Time, lbs map[string]string) (float64, error) {
	v := store.cli.Get(store.prefix + generateRedisKey(name, beginTime, endTime, lbs))
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
