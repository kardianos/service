//go:build linux || darwin || solaris || aix || freebsd

package service

import (
	"sort"
	"strings"
	"sync"
	"testing"
)

// recordCommands replaces runCommand for the rest of the test. Each call's
// argv is appended to the returned slice, and its output is looked up in
// outputs by the space-joined argv.
func recordCommands(t *testing.T, outputs map[string]string) *[][]string { //nolint:unused // Unused on freebsd, solaris and aix.
	t.Helper()
	return stubCommands(t, func(argv []string) (int, string, error) {
		return 0, outputs[strings.Join(argv, " ")], nil
	})
}

// stubCommands replaces runCommand for the rest of the test with result,
// which gets each call's argv. The calls are appended to the returned slice.
func stubCommands(t *testing.T, result func(argv []string) (int, string, error)) *[][]string { //nolint:unused // Unused on freebsd, solaris and aix.
	t.Helper()
	var (
		mu    sync.Mutex
		calls [][]string
	)
	prev := runCommand
	runCommand = func(command string, readStdout bool, arguments ...string) (int, string, error) {
		argv := append([]string{command}, arguments...)
		mu.Lock()
		calls = append(calls, argv)
		mu.Unlock()
		return result(argv)
	}
	t.Cleanup(func() { runCommand = prev })
	return &calls
}

// stubPrivileged makes isPrivileged return v for the rest of the test.
func stubPrivileged(t *testing.T, v bool) { //nolint:unused // Unused on freebsd, solaris and aix.
	t.Helper()
	prev := isPrivileged
	isPrivileged = func() bool { return v }
	t.Cleanup(func() { isPrivileged = prev })
}

func joinCalls(calls [][]string) []string { //nolint:unused // Unused on freebsd, solaris and aix.
	var out []string
	for _, c := range calls {
		out = append(out, strings.Join(c, " "))
	}
	// sorted as `calls` has no guaranteed ordering
	sort.Strings(out)
	return out
}
