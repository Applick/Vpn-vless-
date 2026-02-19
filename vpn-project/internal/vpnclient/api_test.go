package vpnclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildClientConfigURL_DefaultPort(t *testing.T) {
	got, err := BuildClientConfigURL("127.0.0.1", "client-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://127.0.0.1:8080/clients/client-1/config"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildClientConfigURL_WithSchemeAndPort(t *testing.T) {
	got, err := BuildClientConfigURL("https://vpn.example.com:9090", "client-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://vpn.example.com:9090/clients/client-1/config"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFetchClientConfigWithToken_SendsBearerToken(t *testing.T) {
	headerCh := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerCh <- r.Header.Get("Authorization")
		if _, err := w.Write([]byte(`{"id":"c1","name":"c1","config":"[Interface]\nPrivateKey = x\n","qr_base64":""}`)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	resp, err := FetchClientConfigWithToken(server.URL, "c1", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "c1" {
		t.Fatalf("got id %q, want %q", resp.ID, "c1")
	}
	var authHeader string
	select {
	case authHeader = <-headerCh:
	default:
		t.Fatalf("authorization header was not captured")
	}
	if authHeader != "Bearer test-token" {
		t.Fatalf("authorization header mismatch: %q", authHeader)
	}
}
