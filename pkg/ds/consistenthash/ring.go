// Package consistenthash implements a consistent hash ring with virtual nodes.
//
// Consistent hashing allows mapping arbitrary keys (such as device serials)
// to a dynamic set of worker nodes or consumer partitions with minimal remapping
// when nodes join or leave.
//
// Time Complexity:
//   - Add Node: O(V * log(N * V)), where N is the number of physical nodes and V is the number of virtual nodes.
//   - Remove Node: O(V * log(N * V)).
//   - Get Node: O(log(N * V)) via binary search over sorted uint32 hashes.
//
// Space Complexity:
//   - O(N * V) to store sorted virtual node ring and lookup map.
package consistenthash

import (
	"errors"
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
	"sync"
)

// ErrEmptyRing is returned when attempting to locate a key on a ring with no members.
var ErrEmptyRing = errors.New("consistent hash ring is empty")

// HashFunc maps a byte slice to a 32-bit unsigned integer hash.
type HashFunc func(data []byte) uint32

// DefaultHashFunc uses CRC32-IEEE hashing, standard in consistent hashing systems (e.g. Ketama).
func DefaultHashFunc(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// Ring represents a consistent hash ring with virtual nodes.
// All exported methods are safe for concurrent use.
type Ring struct {
	mu       sync.RWMutex
	vnodes   int              // number of virtual replicas per physical node
	hashFn   HashFunc         // hash algorithm
	ring     []uint32         // sorted slice of virtual node hashes
	nodeMap  map[uint32]string // mapping from vnode hash to physical node name
	nodes    map[string]bool   // set of unique physical nodes
}

// New creates a new consistent hash ring.
// If vnodes is <= 0, a default of 150 virtual nodes is used.
// If fn is nil, DefaultHashFunc (FNV-1a) is used.
func New(vnodes int, fn HashFunc) *Ring {
	if vnodes <= 0 {
		vnodes = 150
	}
	if fn == nil {
		fn = DefaultHashFunc
	}
	return &Ring{
		vnodes:  vnodes,
		hashFn:  fn,
		ring:    make([]uint32, 0),
		nodeMap: make(map[uint32]string),
		nodes:   make(map[string]bool),
	}
}

// Add adds one or more physical nodes to the hash ring.
// For each physical node, vnodes virtual nodes are inserted.
func (r *Ring) Add(nodes ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, node := range nodes {
		if node == "" || r.nodes[node] {
			continue
		}
		r.nodes[node] = true
		for i := 0; i < r.vnodes; i++ {
			// Virtual node token format: "<node>#<vnode_index>"
			vnodeKey := node + "#" + strconv.Itoa(i)
			hash := r.hashFn([]byte(vnodeKey))
			r.ring = append(r.ring, hash)
			r.nodeMap[hash] = node
		}
	}
	sort.Slice(r.ring, func(i, j int) bool {
		return r.ring[i] < r.ring[j]
	})
}

// Remove removes a physical node and all its virtual replicas from the ring.
func (r *Ring) Remove(node string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.nodes[node] {
		return
	}
	delete(r.nodes, node)

	// Filter out vnodes belonging to this physical node
	newRing := make([]uint32, 0, len(r.ring)-r.vnodes)
	for _, hash := range r.ring {
		if r.nodeMap[hash] == node {
			delete(r.nodeMap, hash)
		} else {
			newRing = append(newRing, hash)
		}
	}
	r.ring = newRing
}

// Get locates the closest physical node on the ring clockwise for the given key.
// Returns ErrEmptyRing if no physical nodes have been registered.
func (r *Ring) Get(key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.ring) == 0 {
		return "", ErrEmptyRing
	}

	hash := r.hashFn([]byte(key))
	// Binary search for the first virtual node hash >= key hash
	idx := sort.Search(len(r.ring), func(i int) bool {
		return r.ring[i] >= hash
	})

	// Wrap around to 0 if key hash is greater than all hashes on the ring
	if idx == len(r.ring) {
		idx = 0
	}

	targetHash := r.ring[idx]
	node, ok := r.nodeMap[targetHash]
	if !ok {
		return "", fmt.Errorf("node lookup failed for hash %d", targetHash)
	}
	return node, nil
}

// Nodes returns a list of all active physical nodes in the ring.
func (r *Ring) Nodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]string, 0, len(r.nodes))
	for n := range r.nodes {
		res = append(res, n)
	}
	sort.Strings(res)
	return res
}

// Count returns the number of physical nodes currently registered.
func (r *Ring) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodes)
}
