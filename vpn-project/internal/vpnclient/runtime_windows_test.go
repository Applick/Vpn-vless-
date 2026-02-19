//go:build windows

package vpnclient

import "testing"

func TestRuntimePIDPath(t *testing.T) {
	got := runtimePIDPath(`C:\tmp\client.json`)
	want := `C:\tmp\client.json.pid`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDedupePaths(t *testing.T) {
	in := []string{"a", "a", " ", "", "b"}
	out := dedupePaths(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 unique entries, got %d (%v)", len(out), out)
	}
	if out[0] != "a" || out[1] != "b" {
		t.Fatalf("unexpected order/content: %v", out)
	}
}
