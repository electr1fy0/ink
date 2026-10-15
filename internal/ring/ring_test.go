package ring

import (
	"fmt"
	"testing"
)

func TestGetNodesWithFewerPhysicalNodesThanReplicas(t *testing.T) {
	for _, count := range []int{0, 1, 2, 3, 5} {
		for _, requested := range []int{0, 1, 2, 3, 10} {
			t.Run(fmt.Sprintf("nodes=%d/requested=%d", count, requested), func(t *testing.T) {
				r := NewRing(10)
				for i := 0; i < count; i++ {
					r.AddNode(fmt.Sprintf("node-%d", i))
				}
				got := r.GetNodes("key", requested)
				if len(got) != min(count, requested) {
					t.Fatalf("nodes = %v", got)
				}
				seen := make(map[string]bool)
				for _, node := range got {
					if seen[node] {
						t.Fatalf("duplicate node: %v", got)
					}
					seen[node] = true
				}
				if requested > 0 && count > 0 {
					if first, _ := r.GetNode("key"); first != got[0] {
						t.Fatalf("first = %q, nodes = %v", first, got)
					}
				}
			})
		}
	}
}
