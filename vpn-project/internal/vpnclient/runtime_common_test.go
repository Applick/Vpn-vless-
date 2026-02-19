package vpnclient

import (
	"path/filepath"
	"testing"
)

func TestConfigAbsPath(t *testing.T) {
	got, err := configAbsPath("client.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute path, got %q", got)
	}
}
