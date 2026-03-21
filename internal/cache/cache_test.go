package cache

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryCacheGetSet(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value []byte
	}{
		{name: "simple string", key: "supi:imsi-001010000000001", value: []byte(`{"authMethod":"5G_AKA"}`)},
		{name: "empty value", key: "empty", value: []byte{}},
		{name: "binary data", key: "bin", value: []byte{0x00, 0xFF, 0x42}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := NewMemoryCache(500_000_000, 10*time.Second)
			mc.Set(tt.key, tt.value)

			got, ok := mc.Get(tt.key)
			if !ok {
				t.Fatalf("Get(%q) returned ok=false, want true", tt.key)
			}
			if !bytes.Equal(got, tt.value) {
				t.Errorf("Get(%q) = %v, want %v", tt.key, got, tt.value)
			}
		})
	}
}

func TestMemoryCacheTTLExpiry(t *testing.T) {
	mc := NewMemoryCache(500_000_000, 50*time.Millisecond)
	mc.Set("expiring-key", []byte("data"))

	// Should be present immediately
	if _, ok := mc.Get("expiring-key"); !ok {
		t.Fatal("expected key to be present before TTL expiry")
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	if _, ok := mc.Get("expiring-key"); ok {
		t.Fatal("expected key to be absent after TTL expiry")
	}
}

func TestMemoryCacheDelete(t *testing.T) {
	mc := NewMemoryCache(500_000_000, 10*time.Second)
	mc.Set("del-key", []byte("value"))

	mc.Delete("del-key")

	if _, ok := mc.Get("del-key"); ok {
		t.Fatal("expected key to be absent after Delete")
	}
}

func TestMemoryCacheGetNonExistent(t *testing.T) {
	mc := NewMemoryCache(500_000_000, 10*time.Second)

	val, ok := mc.Get("no-such-key")
	if ok {
		t.Fatal("expected ok=false for non-existent key")
	}
	if val != nil {
		t.Errorf("expected nil value for non-existent key, got %v", val)
	}
}

func TestMemoryCacheConcurrentAccess(t *testing.T) {
	mc := NewMemoryCache(500_000_000, 10*time.Second)
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			key := "concurrent-key"
			value := []byte("value")

			mc.Set(key, value)
			mc.Get(key)

			// Even IDs delete, odd IDs set — exercises concurrent read/write/delete
			if id%2 == 0 {
				mc.Delete(key)
			} else {
				mc.Set(key, value)
			}
		}(i)
	}

	wg.Wait()
}

func TestCacheL1OnlyGetSet(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value []byte
	}{
		{name: "basic set and get", key: "k1", value: []byte("v1")},
		{name: "overwrite value", key: "k2", value: []byte("v2-updated")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(Config{
				L1MaxSize: 500_000_000,
				L1TTL:     10 * time.Second,
			})
			defer c.Close()

			ctx := context.Background()
			c.Set(ctx, tt.key, tt.value)

			got, ok := c.Get(ctx, tt.key)
			if !ok {
				t.Fatalf("Get(%q) returned ok=false, want true", tt.key)
			}
			if !bytes.Equal(got, tt.value) {
				t.Errorf("Get(%q) = %v, want %v", tt.key, got, tt.value)
			}
		})
	}
}

func TestCacheL1OnlyDelete(t *testing.T) {
	c := New(Config{
		L1MaxSize: 500_000_000,
		L1TTL:     10 * time.Second,
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "del-key", []byte("value"))
	c.Delete(ctx, "del-key")

	if _, ok := c.Get(ctx, "del-key"); ok {
		t.Fatal("expected key to be absent after Delete")
	}
}

func TestCacheL1Miss(t *testing.T) {
	c := New(Config{
		L1MaxSize: 500_000_000,
		L1TTL:     10 * time.Second,
	})
	defer c.Close()

	val, ok := c.Get(context.Background(), "missing-key")
	if ok {
		t.Fatal("expected ok=false for missing key")
	}
	if val != nil {
		t.Errorf("expected nil value, got %v", val)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.L1MaxSize != 500_000_000 {
		t.Errorf("L1MaxSize = %d, want 500000000", cfg.L1MaxSize)
	}
	if cfg.L1TTL != 10*time.Second {
		t.Errorf("L1TTL = %v, want 10s", cfg.L1TTL)
	}
	if cfg.L2TTL != 60*time.Second {
		t.Errorf("L2TTL = %v, want 60s", cfg.L2TTL)
	}
	if len(cfg.RedisAddrs) != 0 {
		t.Errorf("RedisAddrs = %v, want empty", cfg.RedisAddrs)
	}
}

func TestNewWithEmptyRedisAddrs(t *testing.T) {
	c := New(Config{
		L1MaxSize:  500_000_000,
		L1TTL:      10 * time.Second,
		RedisAddrs: []string{},
	})
	defer c.Close()

	if c.l1 == nil {
		t.Fatal("expected l1 to be initialized")
	}
	if c.l2 != nil {
		t.Fatal("expected l2 to be nil when RedisAddrs is empty")
	}
}
