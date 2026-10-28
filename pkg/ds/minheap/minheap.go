// Package minheap implements a generic binary min-heap priority queue.
//
// Unlike the standard library's container/heap, this implementation uses Go 1.18+
// generics directly without interface assertions (interface{}), providing zero-allocation
// type safety and maximum cache locality.
//
// Time Complexity:
//   - Push: O(log N) - element inserted at end and sifted up.
//   - Pop:  O(log N) - root swapped with tail and sifted down.
//   - Peek: O(1)     - inspects slice root.
//   - Len:  O(1)     - slice length.
//   - NewFromSlice: O(N) - linear-time bottom-up heapification.
//
// Space Complexity:
//   - O(N) - contiguous memory storage backed by a Go dynamic slice.
package minheap

import (
	"sync"
)

// LessFunc defines a comparator function where less(a, b) returns true if a should have
// higher priority (closer to the top of the min-heap) than b.
type LessFunc[T any] func(a, b T) bool

// Heap is a thread-safe generic binary min-heap.
type Heap[T any] struct {
	mu    sync.RWMutex
	items []T
	less  LessFunc[T]
}

// New creates an empty binary min-heap with the provided comparator function.
func New[T any](less LessFunc[T]) *Heap[T] {
	return &Heap[T]{
		items: make([]T, 0),
		less:  less,
	}
}

// NewFromSlice creates a min-heap initialized with the given slice in O(N) time.
func NewFromSlice[T any](items []T, less LessFunc[T]) *Heap[T] {
	copied := make([]T, len(items))
	copy(copied, items)
	h := &Heap[T]{
		items: copied,
		less:  less,
	}
	// Bottom-up heapify: start from last non-leaf parent down to root
	for i := (len(h.items) / 2) - 1; i >= 0; i-- {
		h.siftDown(i)
	}
	return h
}

// Push adds an item to the heap and restores the heap property in O(log N) time.
func (h *Heap[T]) Push(item T) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.items = append(h.items, item)
	h.siftUp(len(h.items) - 1)
}

// Pop removes and returns the minimum item from the heap in O(log N) time.
// Returns the zero value of T and false if the heap is empty.
func (h *Heap[T]) Pop() (T, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var zero T
	n := len(h.items)
	if n == 0 {
		return zero, false
	}

	root := h.items[0]
	lastIdx := n - 1
	h.items[0] = h.items[lastIdx]
	h.items = h.items[:lastIdx]

	if len(h.items) > 0 {
		h.siftDown(0)
	}
	return root, true
}

// Peek returns the minimum item from the heap without removing it in O(1) time.
// Returns zero value of T and false if the heap is empty.
func (h *Heap[T]) Peek() (T, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var zero T
	if len(h.items) == 0 {
		return zero, false
	}
	return h.items[0], true
}

// Len returns the number of items currently in the heap.
func (h *Heap[T]) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.items)
}

// Clear removes all elements from the heap.
func (h *Heap[T]) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = h.items[:0]
}

// Values returns a shallow copy of all items currently in the heap (un-ordered).
func (h *Heap[T]) Values() []T {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]T, len(h.items))
	copy(res, h.items)
	return res
}

// siftUp restores heap property by moving the element at idx upwards toward root.
func (h *Heap[T]) siftUp(idx int) {
	for idx > 0 {
		parent := (idx - 1) / 2
		if !h.less(h.items[idx], h.items[parent]) {
			break
		}
		h.items[idx], h.items[parent] = h.items[parent], h.items[idx]
		idx = parent
	}
}

// siftDown restores heap property by moving the element at idx downwards toward leaves.
func (h *Heap[T]) siftDown(idx int) {
	n := len(h.items)
	for {
		smallest := idx
		leftChild := 2*idx + 1
		rightChild := 2*idx + 2

		if leftChild < n && h.less(h.items[leftChild], h.items[smallest]) {
			smallest = leftChild
		}
		if rightChild < n && h.less(h.items[rightChild], h.items[smallest]) {
			smallest = rightChild
		}
		if smallest == idx {
			break
		}
		h.items[idx], h.items[smallest] = h.items[smallest], h.items[idx]
		idx = smallest
	}
}
