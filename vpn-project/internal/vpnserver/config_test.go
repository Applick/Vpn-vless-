package vpnserver

import "testing"

func TestNormalizeWebsocketPath(t *testing.T) {
	got := normalizeWebsocketPath("vpn")
	want := "/vpn"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
