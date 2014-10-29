package service

import (
	"testing"
)

func TestPlatformName(t *testing.T) {
	s, err := NewServiceConfig(&Config{
		Name: "Test",
	})
	if err != nil {
		t.Errorf("Failed to create service: %v", err)
	}
	t.Logf("Platform is %s", s.String())
}
