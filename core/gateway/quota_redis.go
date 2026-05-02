// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisQuotaBackend stores token-usage counters in Redis. Required for
// horizontally-scaled deployments where multiple gateway replicas must
// share quota state for hard caps to actually be enforced.
//
// Key layout:
//
//	sovstack:quota:daily:{userID}:{YYYY-MM-DD}    (INTEGER, TTL 48h)
//	sovstack:quota:monthly:{userID}:{YYYY-MM}     (INTEGER, TTL 70d)
//
// TTLs are deliberately longer than the natural reset windows so a brief
// clock skew between gateway and Redis can't produce a phantom reset; the
// next "real" rollover (different day/month key) starts at 0 anyway.
//
// All operations are atomic via INCRBY. Latency overhead is one round-trip
// per quota check or record (~0.5–1 ms on a local Redis).
type RedisQuotaBackend struct {
	client    *redis.Client
	keyPrefix string
}

// RedisQuotaConfig is the configuration for connecting to Redis.
type RedisQuotaConfig struct {
	Addr      string // e.g. "localhost:6379"
	Password  string // empty for no auth
	DB        int    // 0 by default
	KeyPrefix string // e.g. "sovstack:quota:"
}

// NewRedisQuotaBackend opens a Redis connection and verifies it with PING.
// Returns an error if the server is unreachable.
func NewRedisQuotaBackend(cfg RedisQuotaConfig) (*RedisQuotaBackend, error) {
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "sovstack:quota:"
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisQuotaBackend{
		client:    client,
		keyPrefix: cfg.KeyPrefix,
	}, nil
}

func (b *RedisQuotaBackend) dailyKey(userID string) string {
	return b.keyPrefix + "daily:" + userID + ":" + dayKey(time.Now())
}

func (b *RedisQuotaBackend) monthlyKey(userID string) string {
	return b.keyPrefix + "monthly:" + userID + ":" + monthKey(time.Now())
}

func (b *RedisQuotaBackend) AddDaily(userID string, tokens int64) error {
	ctx := context.Background()
	key := b.dailyKey(userID)
	pipe := b.client.Pipeline()
	pipe.IncrBy(ctx, key, tokens)
	pipe.Expire(ctx, key, 48*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis add daily: %w", err)
	}
	return nil
}

func (b *RedisQuotaBackend) AddMonthly(userID string, tokens int64) error {
	ctx := context.Background()
	key := b.monthlyKey(userID)
	pipe := b.client.Pipeline()
	pipe.IncrBy(ctx, key, tokens)
	pipe.Expire(ctx, key, 70*24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis add monthly: %w", err)
	}
	return nil
}

func (b *RedisQuotaBackend) GetDaily(userID string) (int64, error) {
	ctx := context.Background()
	v, err := b.client.Get(ctx, b.dailyKey(userID)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("redis get daily: %w", err)
	}
	return v, nil
}

func (b *RedisQuotaBackend) GetMonthly(userID string) (int64, error) {
	ctx := context.Background()
	v, err := b.client.Get(ctx, b.monthlyKey(userID)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("redis get monthly: %w", err)
	}
	return v, nil
}

func (b *RedisQuotaBackend) Close() error {
	return b.client.Close()
}
