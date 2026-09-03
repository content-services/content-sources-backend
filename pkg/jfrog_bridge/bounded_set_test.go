package jfrog_bridge

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBoundedSet_Eviction(t *testing.T) {
	s := newBoundedSet(3)
	s.Add("a")
	s.Add("b")
	s.Add("c")
	assert.True(t, s.Contains("a"))

	s.Add("d")
	assert.False(t, s.Contains("a"), "oldest entry should be evicted")
	assert.True(t, s.Contains("b"))
	assert.True(t, s.Contains("d"))
}

func TestBoundedSet_DuplicateInsert(t *testing.T) {
	s := newBoundedSet(3)
	s.Add("a")
	s.Add("b")
	s.Add("a")
	s.Add("c")

	assert.True(t, s.Contains("a"), "duplicate should not count toward capacity")
	assert.True(t, s.Contains("b"))
	assert.True(t, s.Contains("c"))
}

func TestBoundedSet_EvictionOrder(t *testing.T) {
	s := newBoundedSet(5)
	for i := 0; i < 10; i++ {
		s.Add(fmt.Sprintf("key-%d", i))
	}
	for i := 0; i < 5; i++ {
		assert.False(t, s.Contains(fmt.Sprintf("key-%d", i)))
	}
	for i := 5; i < 10; i++ {
		assert.True(t, s.Contains(fmt.Sprintf("key-%d", i)))
	}
}
