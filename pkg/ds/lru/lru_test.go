package lru

import (
	"sync"
	"testing"
	"time"
)

func TestLRUBasic(t *testing.T) {
	c := New[string, int](3, 0)

	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	if c.Len() != 3 {
		t.Fatalf("expected len 3, got %d", c.Len())
	}

	val, ok := c.Get("a")
	if !ok || val != 1 {
		t.Fatalf("expected key 'a' -> 1, got (%d, %v)", val, ok)
	}

	val, ok = c.Get("nonexistent")
	if ok || val != 0 {
		t.Fatalf("expected nonexistent key to return false")
	}
}

func TestLRUEvictionOrder(t *testing.T) {
	// Capacity of 3: items inserted: "k1", "k2", "k3"
	c := New[string, string](3, 0)

	c.Put("k1", "v1") // LRU
	c.Put("k2", "v2")
	c.Put("k3", "v3") // MRU

	// Inserting "k4" should evict "k1"
	c.Put("k4", "v4")

	if _, ok := c.Get("k1"); ok {
		t.Fatalf("expected k1 to be evicted")
	}

	for _, k := range []string{"k2", "k3", "k4"} {
		if _, ok := c.Get(k); !ok {
			t.Fatalf("expected %s to still exist", k)
		}
	}

	// Access "k2" -> moves "k2" to MRU. "k3" is now the LRU.
	c.Get("k2")

	// Inserting "k5" should evict "k3"
	c.Put("k5", "v5")

	if _, ok := c.Get("k3"); ok {
		t.Fatalf("expected k3 to be evicted after k2 was refreshed")
	}

	if _, ok := c.Get("k2"); !ok {
		t.Fatalf("expected k2 to survive because it was accessed recently")
	}
}

func TestLRUTTL(t *testing.T) {
	c := New[string, string](5, 50*time.Millisecond)

	c.Put("fast", "val1")
	c.PutWithTTL("slow", "val2", 300*time.Millisecond)

	// Immediate lookup succeeds
	if val, ok := c.Get("fast"); !ok || val != "val1" {
		t.Fatalf("immediate lookup failed")
	}

	// Wait 80ms -> fast should be expired, slow should still be valid
	time.Sleep(80 * time.Millisecond)

	if _, ok := c.Get("fast"); ok {
		t.Fatalf("expected 'fast' to have expired")
	}
	if val, ok := c.Get("slow"); !ok || val != "val2" {
		t.Fatalf("expected 'slow' to still be valid")
	}
}

func TestLRURemoveAndPurge(t *testing.T) {
	c := New[int, int](5, 0)
	c.Put(1, 10)
	c.Put(2, 20)

	if !c.Remove(1) {
		t.Fatalf("expected Remove(1) to return true")
	}
	if c.Remove(99) {
		t.Fatalf("expected Remove(99) to return false")
	}

	if c.Len() != 1 {
		t.Fatalf("expected length 1, got %d", c.Len())
	}

	c.Purge()
	if c.Len() != 0 {
		t.Fatalf("expected length 0 after purge, got %d", c.Len())
	}
}

func TestLRUStats(t *testing.T) {
	c := New[string, int](2, 0)

	c.Put("a", 1)
	c.Put("b", 2)

	c.Get("a") // Hit 1
	c.Get("a") // Hit 2
	c.Get("c") // Miss 1

	c.Put("d", 4) // Eviction 1 (b is evicted)

	hits, misses, evictions := c.Stats()
	if hits != 2 {
		t.Fatalf("expected 2 hits, got %d", hits)
	}
	if misses != 1 {
		t.Fatalf("expected 1 miss, got %d", misses)
	}
	if evictions != 1 {
		t.Fatalf("expected 1 eviction, got %d", evictions)
	}
}

func TestLRUConcurrency(t *testing.T) {
	c := New[int, int](100, 0)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := (workerID * 10) + (j % 20)
				c.Put(key, j)
				_, _ = c.Get(key)
			}
		}(i)
	}

	wg.Wait()
}

func BenchmarkLRUPutGet(b *testing.B) {
	c := New[int, int](1000, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(i%500, i)
		_, _ = c.Get(i % 500)
	}
}
