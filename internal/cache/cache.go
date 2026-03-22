// Package cache provides a two-tier caching layer (L1 in-memory + L2 Redis).
// Based on: docs/service-decomposition.md §3.4 (udm-cache)
//
// Cache lookup sequence:
//
//  1. Try L1 (in-memory) → hit → return
//  2. Try L2 (Redis) → hit → populate L1 & return
//  3. Return miss (caller queries database, then calls Set)
//  4. On write: invalidate L2 (DEL), then L1
package cache

import (
	"context"
	"time"
)

// Cache provides a unified two-tier cache interface.
type Cache struct {
	l1 *MemoryCache
	l2 *RedisCache
}

// Config holds cache configuration.
type Config struct {
	L1MaxSize     int64
	L1TTL         time.Duration
	L2TTL         time.Duration
	RedisAddrs    []string
	RedisPassword string
}

// DefaultConfig returns default cache configuration values.
func DefaultConfig() Config {
	return Config{
		L1MaxSize: 500_000_000, // 500MB
		L1TTL:     10 * time.Second,
		L2TTL:     60 * time.Second,
	}
}

// New creates a new two-tier cache.
// If redisAddrs is empty, operates as L1-only cache.
func New(cfg Config) *Cache {
	c := &Cache{
		l1: NewMemoryCache(cfg.L1TTL),
	}
	if len(cfg.RedisAddrs) > 0 {
		c.l2 = NewRedisCache(cfg.RedisAddrs, cfg.RedisPassword, cfg.L2TTL)
	}
	return c
}

// Get retrieves a value from the cache using the two-tier lookup sequence.
// Returns the value and true if found, nil and false if not found.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	// L1 lookup
	if val, ok := c.l1.Get(key); ok {
		return val, true
	}

	// L2 lookup
	if c.l2 != nil {
		if val, err := c.l2.Get(ctx, key); err == nil {
			// Populate L1 on L2 hit
			c.l1.Set(key, val)
			return val, true
		}
	}

	return nil, false
}

// Set stores a value in both L1 and L2 caches.
func (c *Cache) Set(ctx context.Context, key string, value []byte) {
	c.l1.Set(key, value)
	if c.l2 != nil {
		// Fail-open: silently drop Redis write errors to avoid blocking callers.
		// Structured logging for Redis failures will be added in Phase 8 (Observability).
		_ = c.l2.Set(ctx, key, value)
	}
}

// Delete invalidates a key from both L2 and L1 caches.
// Write-through invalidation: L2 first, then L1.
func (c *Cache) Delete(ctx context.Context, key string) {
	if c.l2 != nil {
		_ = c.l2.Delete(ctx, key)
	}
	c.l1.Delete(key)
}

// Close shuts down the cache and releases resources.
func (c *Cache) Close() error {
	if c.l2 != nil {
		return c.l2.Close()
	}
	return nil
}
