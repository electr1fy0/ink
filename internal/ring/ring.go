package ring

import (
	"slices"
	"sort"
	"strconv"

	"github.com/cespare/xxhash/v2"
)

type Ring struct {
	// count of virtual replicas of each node
	replicas int

	// sorted keys
	keys []uint64

	// key -> nodeID
	hashMap map[uint64]string
}

func NewRing(replicas int) *Ring {
	return &Ring{
		replicas: replicas,
		hashMap:  make(map[uint64]string),
	}
}

func hashKey(key string) uint64 {
	return xxhash.Sum64String(key)
}

func (r *Ring) AddNode(node string) {
	for i := range r.replicas {
		virtualNode := node + "#" + strconv.Itoa(i)

		hash := hashKey(virtualNode)

		r.keys = append(r.keys, hash)
		r.hashMap[hash] = node
	}

	slices.SortFunc(r.keys, func(a, b uint64) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
}

func (r *Ring) RemoveNode(node string) {
	newKeys := r.keys[:0]

	for _, hash := range r.keys {
		if r.hashMap[hash] == node {
			delete(r.hashMap, hash)
			continue
		}
		newKeys = append(newKeys, hash)
	}
	r.keys = newKeys
}

func (r *Ring) GetNode(key string) (string, bool) {
	if len(r.keys) == 0 {
		return "", false
	}
	hash := hashKey(key)

	idx := sort.Search(len(r.keys), func(i int) bool {
		return r.keys[i] >= hash
	})

	if idx == len(r.keys) {
		idx = 0
	}

	node := r.hashMap[r.keys[idx]]

	return node, true
}

func (r *Ring) GetNodes(key string, n int) []string {
	if len(r.keys) == 0 {
		return nil
	}

	hash := hashKey(key)
	idx := sort.Search(len(r.keys), func(i int) bool {
		return r.keys[i] >= hash
	})

	if idx == len(r.keys) {
		idx = 0
	}

	var nodes []string
	// collect unique physical nodes starting from valid index
	seen := make(map[string]struct{})
	for len(nodes) < n && len(seen) < len(r.hashMap) {
		node := r.hashMap[r.keys[idx%len(r.keys)]]

		_, ok := seen[node]
		if !ok {
			seen[node] = struct{}{}
			nodes = append(nodes, node)
		}
		idx++
	}

	return nodes
}

