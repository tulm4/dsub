package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides an L2 cache backed by Redis Cluster.
// Based on: docs/service-decomposition.md §3.4
type RedisCache struct {
	client redis.UniversalClient
	ttl    time.Duration
}

// NewRedisCache creates a new Redis cache client.
// Supports both single-node and cluster modes based on address count.
func NewRedisCache(addrs []string, password string, ttl time.Duration) *RedisCache {
	client := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:    addrs,
		Password: password,
	})
	return &RedisCache{
		client: client,
		ttl:    ttl,
	}
}

// Get retrieves a value from Redis.
func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	return val, nil
}

// Set stores a value in Redis with TTL.
func (r *RedisCache) Set(ctx context.Context, key string, value []byte) error {
	if err := r.client.Set(ctx, key, value, r.ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// Delete removes a key from Redis.
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

// Ping checks Redis connectivity.
func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Close shuts down the Redis client.
func (r *RedisCache) Close() error {
	return r.client.Close()
}
