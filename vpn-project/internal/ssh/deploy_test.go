package ssh

import "testing"

func TestKnownHostsCandidatesIncludesExplicitPathFirst(t *testing.T) {
	explicit := `C:\tmp\known_hosts_custom`
	paths := knownHostsCandidates(explicit)
	if len(paths) == 0 {
		t.Fatalf("expected at least one known_hosts candidate")
	}
	if paths[0] != explicit {
		t.Fatalf("got %q, want %q", paths[0], explicit)
	}
}

func TestBuildHostKeyCallbackAllowsExplicitInsecureMode(t *testing.T) {
	callback, err := buildHostKeyCallback(ClientConfig{InsecureHostKey: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callback == nil {
		t.Fatalf("expected non-nil callback")
	}
}
