package rediscache

import (
	"context"
	"testing"
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

func TestNewClientRejectsEmptyAddr(t *testing.T) {
	client, err := NewClientWithKV("", "", 0, &stubKV{})
	if err == nil {
		t.Fatalf("expected error, got client=%v", client)
	}
}

func TestNewClientRejectsNegativeDB(t *testing.T) {
	client, err := NewClientWithKV("127.0.0.1:6379", "", -1, &stubKV{})
	if err == nil {
		t.Fatalf("expected error, got client=%v", client)
	}
}

func TestNewClientAcceptsConfig(t *testing.T) {
	client, err := NewClientWithKV("127.0.0.1:6379", "secret", 2, &stubKV{})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if client.Addr() != "127.0.0.1:6379" || client.DB() != 2 {
		t.Fatalf("unexpected client config: addr=%q db=%d", client.Addr(), client.DB())
	}
}

func TestClientSetGetDelete(t *testing.T) {
	client, err := NewClientWithKV("127.0.0.1:6379", "", 0, &stubKV{})
	if err != nil {
		t.Fatalf("expected client build success, got %v", err)
	}

	if err := client.Set(context.Background(), "dashboard:alice", `{"mode":"materialized"}`, time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	got, err := client.Get(context.Background(), "dashboard:alice")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got != `{"mode":"materialized"}` {
		t.Fatalf("unexpected value: %q", got)
	}
	if err := client.Delete(context.Background(), "dashboard:alice"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := client.Get(context.Background(), "dashboard:alice"); err != redis.Nil {
		t.Fatalf("expected redis nil after delete, got %v", err)
	}
}
