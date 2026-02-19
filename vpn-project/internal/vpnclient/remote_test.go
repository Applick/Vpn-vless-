package vpnclient

import "testing"

func TestRemoteStartCommandTargetsVLESSService(t *testing.T) {
	got := remoteStartCommand()
	want := "docker start vlessserver >/dev/null 2>&1 || docker compose -f /root/vpn-project/docker-compose.yml up -d vlessserver"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRemoteStopCommandTargetsVLESSService(t *testing.T) {
	got := remoteStopCommand()
	want := "docker stop vlessserver || true"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
