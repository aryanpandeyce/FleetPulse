package consistenthash

import (
	"fmt"
	"math"
	"sync"
	"testing"
)

func TestRingEmpty(t *testing.T) {
	r := New(50, nil)
	_, err := r.Get("device-001")
	if err != ErrEmptyRing {
		t.Fatalf("expected ErrEmptyRing, got %v", err)
	}
}

func TestRingAddAndGet(t *testing.T) {
	r := New(100, nil)
	r.Add("worker-1", "worker-2", "worker-3")

	if r.Count() != 3 {
		t.Fatalf("expected 3 nodes, got %d", r.Count())
	}

	nodes := r.Nodes()
	if len(nodes) != 3 || nodes[0] != "worker-1" || nodes[1] != "worker-2" || nodes[2] != "worker-3" {
		t.Fatalf("unexpected nodes list: %v", nodes)
	}

	node, err := r.Get("device-100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node == "" {
		t.Fatalf("expected non-empty node")
	}

	// Repeated lookup of the exact same key must return the exact same node
	for i := 0; i < 10; i++ {
		nodeAgain, err := r.Get("device-100")
		if err != nil || nodeAgain != node {
			t.Fatalf("expected deterministic lookup %s, got %s (err: %v)", node, nodeAgain, err)
		}
	}
}

func TestRingRemove(t *testing.T) {
	r := New(100, nil)
	r.Add("worker-1", "worker-2")

	r.Remove("worker-1")
	if r.Count() != 1 {
		t.Fatalf("expected 1 node, got %d", r.Count())
	}

	for i := 0; i < 50; i++ {
		node, err := r.Get(fmt.Sprintf("key-%d", i))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node != "worker-2" {
			t.Fatalf("expected all keys to map to worker-2, got %s", node)
		}
	}
}

func TestRingDistribution(t *testing.T) {
	r := New(200, nil)
	nodeCount := 5
	for i := 0; i < nodeCount; i++ {
		r.Add(fmt.Sprintf("node-%d", i))
	}

	keyCount := 10000
	counts := make(map[string]int)
	for i := 0; i < keyCount; i++ {
		node, err := r.Get(fmt.Sprintf("device-serial-%06d", i))
		if err != nil {
			t.Fatalf("failed to get node: %v", err)
		}
		counts[node]++
	}

	expectedPerNode := float64(keyCount) / float64(nodeCount)
	t.Logf("Distribution across %d nodes (expected ~%.0f keys per node):", nodeCount, expectedPerNode)

	for node, count := range counts {
		diffPercent := math.Abs(float64(count)-expectedPerNode) / expectedPerNode * 100.0
		t.Logf("  %s: %d keys (%.2f%% deviation)", node, count, diffPercent)
		// Max allowed deviation for 200 vnodes on 10k keys is typically < 20%
		if diffPercent > 25.0 {
			t.Errorf("Node %s has excessive deviation of %.2f%% from expected", node, diffPercent)
		}
	}
}

func TestRingMinimalRemapping(t *testing.T) {
	r := New(200, nil)
	initialNodes := []string{"node-0", "node-1", "node-2", "node-3", "node-4"}
	r.Add(initialNodes...)

	keyCount := 3000
	initialMappings := make(map[string]string)
	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("device-ucs-%d", i)
		node, _ := r.Get(key)
		initialMappings[key] = node
	}

	// Add 6th node. Theoretically, ~1/6 (~16.7%) of keys should move to the new node,
	// and keys not mapped to node-5 should NOT change their existing mapping.
	r.Add("node-5")

	remappedToNew := 0
	remappedToWrong := 0
	for key, originalNode := range initialMappings {
		newNode, _ := r.Get(key)
		if newNode == "node-5" {
			remappedToNew++
		} else if newNode != originalNode {
			remappedToWrong++
		}
	}

	if remappedToWrong > 0 {
		t.Errorf("Keys were remapped between existing nodes! count: %d", remappedToWrong)
	}

	movedPercent := float64(remappedToNew) / float64(keyCount) * 100.0
	t.Logf("Remapped keys after adding 6th node: %d / %d (%.2f%%). Ideal: ~16.67%%",
		remappedToNew, keyCount, movedPercent)

	// Must be within reasonable range of 1/6 (e.g., between 10% and 25%)
	if movedPercent < 10.0 || movedPercent > 25.0 {
		t.Errorf("Remapped percentage %.2f%% outside expected bounds [10%%, 25%%]", movedPercent)
	}
}

func TestRingConcurrency(t *testing.T) {
	r := New(50, nil)
	r.Add("node-1", "node-2")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				_, _ = r.Get(key)
			}
		}(i)
	}

	// Concurrent node additions
	for i := 3; i <= 6; i++ {
		wg.Add(1)
		go func(nodeId int) {
			defer wg.Done()
			r.Add(fmt.Sprintf("node-%d", nodeId))
		}(i)
	}

	wg.Wait()
}

func BenchmarkRingGet(b *testing.B) {
	r := New(150, nil)
	r.Add("node-1", "node-2", "node-3", "node-4", "node-5")
	key := "UCS-C220-M7-SN991823"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Get(key)
	}
}
