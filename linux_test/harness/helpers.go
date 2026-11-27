package main

func contains[T comparable](s []T, v T) bool {
	return containsFunc(s, func(e T) bool { return e == v })
}

func containsFunc[T any](s []T, f func(T) bool) bool {
	for _, e := range s {
		if f(e) {
			return true
		}
	}
	return false
}
