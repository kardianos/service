//go:build darwin || linux

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// trustedGroups may have group write access to the executable and its
// parent directories. On darwin, admin (80) owns /Applications.
var trustedGroups = func() []uint32 {
	if runtime.GOOS == "darwin" {
		return []uint32{0, 80} // wheel, admin
	}
	return []uint32{0}
}()

// checkExecutableOwnership returns the resolved path, and  ensures that files
// aren't writable by non-root users
func checkExecutableOwnership(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	for p := resolved; ; {
		uid, gid, mode, err := ownerStat(p)
		if err != nil {
			return "", err
		}
		if !trusted(uid, gid, mode) {
			return "", fmt.Errorf("%s: writable by non-admin users", p)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return resolved, nil
		}
		p = parent
	}
}

// trusted reports whether an entry with this owner, group and mode can only
// be modified by root or an admin group.
func trusted(uid, gid uint32, mode os.FileMode) bool {
	if uid != 0 || mode&0002 != 0 {
		return false
	}
	if mode&0020 == 0 {
		return true
	}

	for _, g := range trustedGroups {
		if g == gid {
			return true
		}
	}
	return false
}

// ownerStat returns the owner, group and mode of path. Replaced in tests.
var ownerStat = func(path string) (uid, gid uint32, mode os.FileMode, err error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, 0, 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 0, fmt.Errorf("%s: cannot read owner", path)
	}
	return st.Uid, st.Gid, fi.Mode(), nil
}
