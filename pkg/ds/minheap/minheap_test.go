package minheap

import (
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestMinHeapEmpty(t *testing.T) {
	h := New[int](func(a, b int) bool { return a < b })

	if h.Len() != 0 {
		t.Fatalf("expected length 0, got %d", h.Len())
	}

	val, ok := h.Peek()
	if ok || val != 0 {
		t.Fatalf("expected (0, false) on empty peek, got (%d, %v)", val, ok)
	}

	val, ok = h.Pop()
	if ok || val != 0 {
		t.Fatalf("expected (0, false) on empty pop, got (%d, %v)", val, ok)
	}
}

func TestMinHeapIntOrdering(t *testing.T) {
	h := New[int](func(a, b int) bool { return a < b })
	nums := []int{42, 10, 88, 1, 3, 99, 15, 2, 7}

	for _, n := range nums {
		h.Push(n)
	}

	if h.Len() != len(nums) {
		t.Fatalf("expected len %d, got %d", len(nums), h.Len())
	}

	sort.Ints(nums)
	for _, expected := range nums {
		peeked, ok := h.Peek()
		if !ok || peeked != expected {
			t.Fatalf("expected peek %d, got (%d, %v)", expected, peeked, ok)
		}

		popped, ok := h.Pop()
		if !ok || popped != expected {
			t.Fatalf("expected pop %d, got (%d, %v)", expected, popped, ok)
		}
	}

	if h.Len() != 0 {
		t.Fatalf("expected empty heap after draining, got len %d", h.Len())
	}
}

type TestJob struct {
	ID        string
	Priority  int // 1 = High, 2 = Normal, 3 = Low
	CreatedAt time.Time
}

func TestMinHeapJobScheduling(t *testing.T) {
	// Job scheduler comparator: lower priority number first, then earlier CreatedAt (FIFO)
	jobLess := func(a, b TestJob) bool {
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.CreatedAt.Before(b.CreatedAt)
	}

	h := New[TestJob](jobLess)
	now := time.Now()

	j1 := TestJob{ID: "normal-1", Priority: 2, CreatedAt: now.Add(1 * time.Second)}
	j2 := TestJob{ID: "high-1", Priority: 1, CreatedAt: now.Add(2 * time.Second)}
	j3 := TestJob{ID: "normal-2", Priority: 2, CreatedAt: now.Add(3 * time.Second)}
	j4 := TestJob{ID: "low-1", Priority: 3, CreatedAt: now.Add(4 * time.Second)}
	j5 := TestJob{ID: "high-2", Priority: 1, CreatedAt: now.Add(100 * time.Millisecond)}

	// Push in mixed order
	h.Push(j1)
	h.Push(j2)
	h.Push(j3)
	h.Push(j4)
	h.Push(j5)

	expectedOrder := []string{"high-2", "high-1", "normal-1", "normal-2", "low-1"}
	for _, expectedID := range expectedOrder {
		job, ok := h.Pop()
		if !ok {
			t.Fatalf("expected job %s, got empty heap", expectedID)
		}
		if job.ID != expectedID {
			t.Fatalf("expected job ID %s, got %s (priority: %d, created: %v)",
				expectedID, job.ID, job.Priority, job.CreatedAt)
		}
	}
}

func TestMinHeapFromSlice(t *testing.T) {
	raw := []int{50, 20, 80, 10, 40, 90, 30}
	h := NewFromSlice[int](raw, func(a, b int) bool { return a < b })

	if h.Len() != len(raw) {
		t.Fatalf("expected len %d, got %d", len(raw), h.Len())
	}

	sort.Ints(raw)
	for _, expected := range raw {
		val, ok := h.Pop()
		if !ok || val != expected {
			t.Fatalf("expected %d, got %d", expected, val)
		}
	}
}

func TestMinHeapConcurrency(t *testing.T) {
	h := New[int](func(a, b int) bool { return a < b })
	var wg sync.WaitGroup

	// 10 concurrent producers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(workerID)))
			for j := 0; j < 50; j++ {
				h.Push(r.Intn(1000))
			}
		}(i)
	}

	// 5 concurrent consumers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				_, _ = h.Pop()
				time.Sleep(10 * time.Microsecond)
			}
		}()
	}

	wg.Wait()
}

func BenchmarkMinHeapPushPop(b *testing.B) {
	h := New[int](func(a, b int) bool { return a < b })
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Push(i)
	}
	for i := 0; i < b.N; i++ {
		_, _ = h.Pop()
	}
}
