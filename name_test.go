// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.package service

package service

import (
	"testing"
)

func TestPlatformName(t *testing.T) {
	t.Logf("Platform is %v", Platform())
}
