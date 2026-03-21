package cache

import (
	"sync"
	"time"
)

// MemoryCache provides an in-memory L1 cache with TTL-based expiry.
// Uses sync.Map for concurrent access with LRU-like behavior via TTL expiry.
// Based on: docs/service-decomposition.md §3.4
type MemoryCache struct {
	data    sync.Map
	maxSize int64
	ttl     time.Duration
}

// entry represents a cached item with expiry.
type entry struct {
	value     []byte
	expiresAt time.Time
}

// NewMemoryCache creates a new in-memory cache.
func NewMemoryCache(maxSize int64, ttl time.Duration) *MemoryCache {
	return &MemoryCache{
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get retrieves a value from the in-memory cache.
// Returns nil, false if the key is not found or has expired.
func (m *MemoryCache) Get(key string) ([]byte, bool) {
	v, ok := m.data.Load(key)
	if !ok {
		return nil, false
	}
	e := v.(*entry)
	if time.Now().After(e.expiresAt) {
		m.data.Delete(key)
		return nil, false
	}
	return e.value, true
}

// Set stores a value in the in-memory cache with TTL.
func (m *MemoryCache) Set(key string, value []byte) {
	m.data.Store(key, &entry{
		value:     value,
		expiresAt: time.Now().Add(m.ttl),
	})
}

// Delete removes a key from the in-memory cache.
func (m *MemoryCache) Delete(key string) {
	m.data.Delete(key)
}
