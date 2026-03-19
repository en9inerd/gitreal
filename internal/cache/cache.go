package cache

import (
	"sync"
	"time"
)

type entry[T any] struct {
	value     T
	expiresAt time.Time
}

type Cache[T any] struct {
	mu      sync.RWMutex
	items   map[string]entry[T]
	ttl     time.Duration
	maxSize int
}

func New[T any](ttl time.Duration, maxSize int) *Cache[T] {
	c := &Cache[T]{
		items:   make(map[string]entry[T]),
		ttl:     ttl,
		maxSize: maxSize,
	}
	go c.cleanup()
	return c
}

func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.items[key]
	if !ok || time.Now().After(e.expiresAt) {
		var zero T
		return zero, false
	}
	return e.value, true
}

func (c *Cache[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.items) >= c.maxSize {
		c.evictOldest()
	}
	c.items[key] = entry[T]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache[T]) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	for k, e := range c.items {
		if first || e.expiresAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.expiresAt
			first = false
		}
	}
	if !first {
		delete(c.items, oldestKey)
	}
}

func (c *Cache[T]) cleanup() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, e := range c.items {
			if now.After(e.expiresAt) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}
