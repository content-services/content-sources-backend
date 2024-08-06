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

// Converts any struct to a pointer to that struct
func Ptr[T any](item T) *T {
	return &item
}
