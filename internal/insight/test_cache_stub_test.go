package insight_test

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type stubKV struct {
	values map[string]string
}

func (s *stubKV) Get(_ context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(context.Background(), "get", key)
	if value, ok := s.values[key]; ok {
		cmd.SetVal(value)
	} else {
		cmd.SetErr(redis.Nil)
	}
	return cmd
}

func (s *stubKV) Set(_ context.Context, key string, value interface{}, _ time.Duration) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(context.Background(), "set", key)
	if s.values == nil {
		s.values = map[string]string{}
	}
	text, _ := value.(string)
	s.values[key] = text
	cmd.SetVal("OK")
	return cmd
}

func (s *stubKV) Del(_ context.Context, keys ...string) *redis.IntCmd {
	cmd := redis.NewIntCmd(context.Background(), "del")
	var deleted int64
	for _, key := range keys {
		if _, ok := s.values[key]; ok {
			delete(s.values, key)
			deleted++
		}
	}
	cmd.SetVal(deleted)
	return cmd
}

func (s *stubKV) Close() error {
	return nil
}
