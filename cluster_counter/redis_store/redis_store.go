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
	cli *redis.Client
	prefix string
}

const SEP = "####"

func NewStore(address string, pass string, prefix string) *RedisStore {
	options := &redis.Options{
		Addr:         address,
		Password:     pass,
		DB:           0,
		DialTimeout:  1 * time.Second,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	}
	cli := redis.NewClient(options)

	return NewRedisStore(cli, prefix)
}

func NewRedisStore(cli *redis.Client, prefix string) *RedisStore {
	return &RedisStore{cli: cli, prefix: prefix}
}

func generateRedisKey(name string, startTime time.Time, endTime time.Time, lbs map[string]string) string {
	var labels []string
	for _, v := range lbs {
		labels = append(labels, v)
	}
	sort.Stable(sort.StringSlice(labels))
	key := fmt.Sprintf("%v%v%v_%v%v%v", name, SEP, startTime.Unix(), endTime.Unix(), SEP, strings.Join(labels, SEP))
	return key
}

func (store *RedisStore) Store(name string, startTime time.Time, endTime time.Time, lbs map[string]string, value int64) error {
	v := store.cli.IncrBy(store.prefix + generateRedisKey(name, startTime, endTime, lbs), value)
	return v.Err()
}

func (store *RedisStore) Load(name string, startTime time.Time, endTime time.Time, lbs map[string]string) (int64, error) {
	v := store.cli.Get(store.prefix + generateRedisKey(name, startTime, endTime, lbs))
	if v.Err() == nil {
		t, err := v.Result()
		if err == nil {
			return strconv.ParseInt(t, 10, 64)
		}
		return 0, err
	} else {
		return 0, v.Err()
	}
}
