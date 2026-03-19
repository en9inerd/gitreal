package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestGetSet(t *testing.T) {
	c := New[string](time.Minute, 10)

	c.Set("key1", "value1")
	v, ok := c.Get("key1")
	if !ok || v != "value1" {
		t.Fatalf("expected value1, got %q ok=%v", v, ok)
	}
}

func TestGetMiss(t *testing.T) {
	c := New[string](time.Minute, 10)

	v, ok := c.Get("nonexistent")
	if ok || v != "" {
		t.Fatalf("expected miss, got %q ok=%v", v, ok)
	}
}

func TestTTLExpiry(t *testing.T) {
	c := New[string](50*time.Millisecond, 10)

	c.Set("key1", "value1")
	v, ok := c.Get("key1")
	if !ok || v != "value1" {
		t.Fatal("expected hit before expiry")
	}

	time.Sleep(80 * time.Millisecond)

	_, ok = c.Get("key1")
	if ok {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestEvictionAtMaxSize(t *testing.T) {
	c := New[int](time.Minute, 3)

	c.Set("a", 1)
	time.Sleep(time.Millisecond)
	c.Set("b", 2)
	time.Sleep(time.Millisecond)
	c.Set("c", 3)

	// Cache is full (3/3). Adding a 4th should evict "a" (oldest expiry).
	c.Set("d", 4)

	if _, ok := c.Get("a"); ok {
		t.Fatal("expected 'a' to be evicted")
	}
	for _, key := range []string{"b", "c", "d"} {
		if _, ok := c.Get(key); !ok {
			t.Fatalf("expected %q to still be present", key)
		}
	}
}

func TestOverwrite(t *testing.T) {
	c := New[string](time.Minute, 10)

	c.Set("key", "old")
	c.Set("key", "new")

	v, ok := c.Get("key")
	if !ok || v != "new" {
		t.Fatalf("expected 'new', got %q", v)
	}

	c.mu.RLock()
	count := len(c.items)
	c.mu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 item, got %d", count)
	}
}

func TestPointerValues(t *testing.T) {
	type data struct{ Name string }
	c := New[*data](time.Minute, 10)

	d := &data{Name: "test"}
	c.Set("ptr", d)

	got, ok := c.Get("ptr")
	if !ok || got.Name != "test" {
		t.Fatalf("expected test, got %v", got)
	}
}

func TestCleanupRemovesExpired(t *testing.T) {
	ttl := 50 * time.Millisecond
	c := New[string](ttl, 100)

	c.Set("expire", "val")
	time.Sleep(ttl + 80*time.Millisecond)

	c.mu.RLock()
	_, exists := c.items["expire"]
	c.mu.RUnlock()
	if exists {
		t.Fatal("expected cleanup goroutine to remove expired entry")
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	c := New[int](time.Minute, 1000)
	var wg sync.WaitGroup
	const goroutines = 50
	const ops = 200

	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range ops {
				key := fmt.Sprintf("key-%d-%d", id, j)
				c.Set(key, j)
				c.Get(key)
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrentEviction(t *testing.T) {
	c := New[int](time.Minute, 10)
	var wg sync.WaitGroup
	const goroutines = 20
	const ops = 100

	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range ops {
				key := fmt.Sprintf("g%d-k%d", id, j)
				c.Set(key, j)
			}
		}(i)
	}

	wg.Wait()

	c.mu.RLock()
	count := len(c.items)
	c.mu.RUnlock()
	if count > 10 {
		t.Fatalf("cache exceeded max size: %d", count)
	}
}
