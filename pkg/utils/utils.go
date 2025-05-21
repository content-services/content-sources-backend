package utils

// SlicesEqual returns true if s1 and s2 have the same elements in the same order
func SlicesEqual[T comparable](s1 []T, s2 []T) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

// SubtractSlices returns a copy of s1, where the elements of s2 are removed from s1,
// if those elements exist in s1. Duplicate matches are removed.
func SubtractSlices[T comparable](s1 []T, s2 []T) []T {
	set := make(map[T]bool)
	for _, elem := range s2 {
		set[elem] = true
	}

	res := []T{}
	for _, elem := range s1 {
		if !set[elem] {
			res = append(res, elem)
		}
	}
	return res
}

// AtIndexes returns a slice of the indexes of elems that contain v
func AtIndexes[T comparable](elems []T, v T) []int {
	var indexes []int
	for i, s := range elems {
		if s == v {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

// Contains returns true if elems contains v
func Contains[T comparable](elems []T, v T) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

// Ptr converts any value to a pointer to that value
func Ptr[T any](item T) *T {
	return &item
}
