package main

func equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func indexFunc[T any](s []T, f func(T) bool) int {
	for i, e := range s {
		if f(e) {
			return i
		}
	}
	return -1
}
