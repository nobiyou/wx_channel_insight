package rediscache

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type KV interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Close() error
}

type Client struct {
	addr     string
	password string
	db       int
	kv       KV
}

func NewClient(addr, password string, db int) (*Client, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, errors.New("redis addr is required")
	}
	if db < 0 {
		return nil, errors.New("redis db must be >= 0")
	}

	options := &redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	}
	kv := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := kv.Ping(ctx).Err(); err != nil {
		_ = kv.Close()
		return nil, err
	}

	return NewClientWithKV(addr, password, db, kv)
}

func NewClientWithKV(addr, password string, db int, kv KV) (*Client, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, errors.New("redis addr is required")
	}
	if db < 0 {
		return nil, errors.New("redis db must be >= 0")
	}
	if kv == nil {
		return nil, errors.New("redis kv is required")
	}
	return &Client{
		addr:     addr,
		password: password,
		db:       db,
		kv:       kv,
	}, nil
}

func (c *Client) Addr() string {
	if c == nil {
		return ""
	}
	return c.addr
}

func (c *Client) DB() int {
	if c == nil {
		return 0
	}
	return c.db
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	if c == nil || c.kv == nil {
		return "", errors.New("redis client is not configured")
	}
	return c.kv.Get(ctx, key).Result()
}

func (c *Client) Set(ctx context.Context, key, value string, expiration time.Duration) error {
	if c == nil || c.kv == nil {
		return errors.New("redis client is not configured")
	}
	return c.kv.Set(ctx, key, value, expiration).Err()
}

func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if c == nil || c.kv == nil {
		return errors.New("redis client is not configured")
	}
	if len(keys) == 0 {
		return nil
	}
	return c.kv.Del(ctx, keys...).Err()
}

func (c *Client) Close() error {
	if c == nil || c.kv == nil {
		return nil
	}
	return c.kv.Close()
}
