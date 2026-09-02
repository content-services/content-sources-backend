package jfrog_bridge

import "sync"

type boundedSet struct {
	mu      sync.Mutex
	items   map[string]struct{}
	order   []string
	maxSize int
}

func newBoundedSet(maxSize int) *boundedSet {
	return &boundedSet{
		items:   make(map[string]struct{}, maxSize),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

func (s *boundedSet) Contains(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[key]
	return ok
}

func (s *boundedSet) Add(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; ok {
		return
	}
	if len(s.items) >= s.maxSize {
		evict := s.order[0]
		s.order = s.order[1:]
		delete(s.items, evict)
	}
	s.items[key] = struct{}{}
	s.order = append(s.order, key)
}
