package utils

import "sync"

type ConcurrentMap[K comparable, V any] struct {
	internalMap map[K]V
	mu          sync.RWMutex
}

func NewConcurrentMap[K comparable, V any]() *ConcurrentMap[K, V] {
	return &ConcurrentMap[K, V]{
		internalMap: make(map[K]V),
	}
}

func (c *ConcurrentMap[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.internalMap[key] = value
}

func (c *ConcurrentMap[K, V]) Get(key K) (value V, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, exists := c.internalMap[key]
	return val, exists
}

func (c *ConcurrentMap[K, V]) Remove(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.internalMap, key)
}
