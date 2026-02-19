package vpnserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequiresAPIAuth(t *testing.T) {
	statusReq := httptest.NewRequest(http.MethodGet, "http://localhost/status", nil)
	if requiresAPIAuth(statusReq) {
		t.Fatalf("/status must stay publicly readable")
	}

	protectedReq := httptest.NewRequest(http.MethodGet, "http://localhost/clients/default-client/config", nil)
	if !requiresAPIAuth(protectedReq) {
		t.Fatalf("client config endpoint must require auth")
	}
}

func TestRequestAPIToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/clients/x/config", nil)
	req.Header.Set("Authorization", "Bearer token-1")
	if got := requestAPIToken(req); got != "token-1" {
		t.Fatalf("got %q, want %q", got, "token-1")
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/clients/x/config", nil)
	req2.Header.Set("X-API-Token", "token-2")
	if got := requestAPIToken(req2); got != "token-2" {
		t.Fatalf("got %q, want %q", got, "token-2")
	}
}

func TestSecureTokenEqual(t *testing.T) {
	if !secureTokenEqual("abc", "abc") {
		t.Fatalf("equal tokens must match")
	}
	if secureTokenEqual("abc", "def") {
		t.Fatalf("different tokens must not match")
	}
	if secureTokenEqual("", "def") {
		t.Fatalf("empty token must not match")
	}
}

func TestIsTrustedLocalRemote(t *testing.T) {
	if !isTrustedLocalRemote("127.0.0.1:12345") {
		t.Fatalf("loopback must be trusted")
	}
	if !isTrustedLocalRemote("192.168.1.10:8080") {
		t.Fatalf("private IPv4 must be trusted")
	}
	if isTrustedLocalRemote("8.8.8.8:53") {
		t.Fatalf("public IPv4 must not be trusted")
	}
}

func TestAPIAuthMiddleware_NoTokenConfigured_BlocksPublicProtectedEndpoint(t *testing.T) {
	handler := apiAuthMiddleware("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost/clients/default-client/config", nil)
	req.RemoteAddr = "8.8.8.8:44321"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAPIAuthMiddleware_NoTokenConfigured_AllowsPrivateProtectedEndpoint(t *testing.T) {
	handler := apiAuthMiddleware("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost/clients/default-client/config", nil)
	req.RemoteAddr = "192.168.1.10:44321"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAPIAuthMiddleware_WithToken_RequiresBearerToken(t *testing.T) {
	handler := apiAuthMiddleware("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost/clients/default-client/config", nil)
	req.RemoteAddr = "8.8.8.8:44321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/clients/default-client/config", nil)
	req2.RemoteAddr = "8.8.8.8:44321"
	req2.Header.Set("Authorization", "Bearer secret-token")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec2.Code, http.StatusOK)
	}
}

func TestAPIAuthMiddleware_StatusEndpointStaysOpen(t *testing.T) {
	handler := apiAuthMiddleware("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost/status", nil)
	req.RemoteAddr = "8.8.8.8:44321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}
