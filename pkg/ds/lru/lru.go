// Package lru implements a thread-safe, generic Least Recently Used (LRU) cache
// with an embedded doubly linked list and optional per-item or default TTL expiration.
//
// Time Complexity:
//   - Get:    O(1) - hash table lookup + pointer manipulation to move node to MRU head.
//   - Put:    O(1) - hash table insert/update + pointer manipulation.
//   - Remove: O(1) - hash table delete + pointer detachment.
//   - Len:    O(1) - counter access.
//
// Space Complexity:
//   - O(C) where C is the maximum cache capacity.
package lru

import (
	"sync"
	"sync/atomic"
	"time"
)

// entry represents an individual node in the doubly linked list.
type entry[K comparable, V any] struct {
	key       K
	val       V
	expiresAt time.Time // zero time means no TTL
	prev      *entry[K, V]
	next      *entry[K, V]
}

// Cache is a thread-safe generic LRU cache.
type Cache[K comparable, V any] struct {
	mu         sync.RWMutex
	capacity   int
	defaultTTL time.Duration
	items      map[K]*entry[K, V]
	head       *entry[K, V] // Most Recently Used (MRU)
	tail       *entry[K, V] // Least Recently Used (LRU)

	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

// New creates an LRU cache with the specified capacity and optional default TTL.
// If defaultTTL is <= 0, items do not expire by default.
func New[K comparable, V any](capacity int, defaultTTL time.Duration) *Cache[K, V] {
	if capacity <= 0 {
		capacity = 128
	}
	return &Cache[K, V]{
		capacity:   capacity,
		defaultTTL: defaultTTL,
		items:      make(map[K]*entry[K, V]),
	}
}

// Get looks up a key in the cache. If found and not expired, the item is moved
// to the head (MRU) and returned with true. Otherwise, returns zero value and false.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zero V
	node, exists := c.items[key]
	if !exists {
		c.misses.Add(1)
		return zero, false
	}

	// Check TTL expiration
	if !node.expiresAt.IsZero() && time.Now().After(node.expiresAt) {
		c.removeNode(node)
		delete(c.items, key)
		c.misses.Add(1)
		return zero, false
	}

	c.moveToHead(node)
	c.hits.Add(1)
	return node.val, true
}

// Put inserts or updates a key-value pair using the default TTL.
// If the cache is at capacity, the least recently used item is evicted.
func (c *Cache[K, V]) Put(key K, val V) {
	c.PutWithTTL(key, val, c.defaultTTL)
}

// PutWithTTL inserts or updates a key-value pair with a specific custom TTL.
func (c *Cache[K, V]) PutWithTTL(key K, val V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	// If key already exists, update value, refresh TTL, and move to MRU head
	if node, exists := c.items[key]; exists {
		node.val = val
		node.expiresAt = expiresAt
		c.moveToHead(node)
		return
	}

	// Evict tail (LRU) if capacity exceeded
	if len(c.items) >= c.capacity {
		c.evictTail()
	}

	newNode := &entry[K, V]{
		key:       key,
		val:       val,
		expiresAt: expiresAt,
	}
	c.addToHead(newNode)
	c.items[key] = newNode
}

// Remove deletes a key from the cache. Returns true if key was present.
func (c *Cache[K, V]) Remove(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.items[key]
	if !exists {
		return false
	}

	c.removeNode(node)
	delete(c.items, key)
	return true
}

// Len returns the number of active items in the cache.
func (c *Cache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Purge removes all items from the cache.
func (c *Cache[K, V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[K]*entry[K, V])
	c.head = nil
	c.tail = nil
}

// Stats returns the cumulative hits, misses, and evictions count.
func (c *Cache[K, V]) Stats() (hits, misses, evictions int64) {
	return c.hits.Load(), c.misses.Load(), c.evictions.Load()
}

// moveToHead moves an existing node to the head of the list.
func (c *Cache[K, V]) moveToHead(node *entry[K, V]) {
	if c.head == node {
		return
	}
	c.removeNode(node)
	c.addToHead(node)
}

// addToHead inserts a node at the head (MRU).
func (c *Cache[K, V]) addToHead(node *entry[K, V]) {
	node.prev = nil
	node.next = c.head

	if c.head != nil {
		c.head.prev = node
	}
	c.head = node

	if c.tail == nil {
		c.tail = node
	}
}

// removeNode unlinks an existing node from the list.
func (c *Cache[K, V]) removeNode(node *entry[K, V]) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		c.head = node.next
	}

	if node.next != nil {
		node.next.prev = node.prev
	} else {
		c.tail = node.prev
	}

	node.prev = nil
	node.next = nil
}

// evictTail removes the least recently used item at the tail.
func (c *Cache[K, V]) evictTail() {
	if c.tail == nil {
		return
	}
	lruNode := c.tail
	c.removeNode(lruNode)
	delete(c.items, lruNode.key)
	c.evictions.Add(1)
}
