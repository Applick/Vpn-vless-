package vpnserver

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
)

func NewHTTPHandler(mgr *Manager, logger *log.Logger) http.Handler {
	api := &apiServer{
		mgr:      mgr,
		logger:   logger,
		apiToken: strings.TrimSpace(mgr.cfg.APIToken),
	}
	return api.routes()
}

type apiServer struct {
	mgr      *Manager
	logger   *log.Logger
	apiToken string
}

func (a *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", a.handleStatus)
	mux.HandleFunc("/clients", a.handleClients)
	mux.HandleFunc("/clients/", a.handleClientConfig)
	mux.HandleFunc("/start", a.handleStart)
	mux.HandleFunc("/stop", a.handleStop)
	return accessLogMiddleware(a.logger, apiAuthMiddleware(a.apiToken, mux))
}

func (a *apiServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	status, err := a.mgr.GetStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *apiServer) handleClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var req struct {
		Name string `json:"name"`
	}

	if r.Body != nil {
		defer r.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
			if err := json.Unmarshal(body, &req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
				return
			}
		}
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = fmt.Sprintf("client-%d", time.Now().Unix())
	}

	c, config, err := a.mgr.CreateClient(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	vlessURI := a.mgr.ClientShareURI(c)
	qrPayload := strings.TrimSpace(vlessURI)
	if qrPayload == "" {
		qrPayload = config
	}

	qrB64, err := configToQRBase64(qrPayload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := map[string]any{
		"id":          c.ID,
		"name":        c.Name,
		"address":     c.Address,
		"created_at":  c.CreatedAt,
		"config":      config,
		"vless_uri":   vlessURI,
		"config_path": c.ConfigPath,
		"qr_base64":   qrB64,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (a *apiServer) handleClientConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/clients/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "config" {
		http.NotFound(w, r)
		return
	}

	clientID, err := url.PathUnescape(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("client id is empty"))
		return
	}

	c, config, err := a.mgr.GetClientConfig(clientID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, fmt.Errorf("client %s not found", clientID))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	vlessURI := a.mgr.ClientShareURI(c)
	qrPayload := strings.TrimSpace(vlessURI)
	if qrPayload == "" {
		qrPayload = config
	}

	qrB64, err := configToQRBase64(qrPayload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := map[string]any{
		"id":         c.ID,
		"name":       c.Name,
		"address":    c.Address,
		"created_at": c.CreatedAt,
		"config":     config,
		"vless_uri":  vlessURI,
		"qr_base64":  qrB64,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *apiServer) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if err := a.mgr.StartInterface(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (a *apiServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if err := a.mgr.StopInterface(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	writeJSON(w, statusCode, map[string]string{
		"error": err.Error(),
	})
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}

func configToQRBase64(config string) (string, error) {
	pngBytes, err := qrcode.Encode(config, qrcode.Medium, 320)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pngBytes), nil
}

type loggingWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lw *loggingWriter) WriteHeader(code int) {
	lw.statusCode = code
	lw.ResponseWriter.WriteHeader(code)
}

func accessLogMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lw, r)
		logger.Printf("%s %s status=%d duration=%s remote=%s",
			r.Method,
			r.URL.Path,
			lw.statusCode,
			time.Since(start).Truncate(time.Millisecond),
			r.RemoteAddr,
		)
	})
}

func apiAuthMiddleware(apiToken string, next http.Handler) http.Handler {
	token := strings.TrimSpace(apiToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresAPIAuth(r) {
			next.ServeHTTP(w, r)
			return
		}

		if token == "" {
			if isTrustedLocalRemote(r.RemoteAddr) {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusUnauthorized, fmt.Errorf("API token is required for non-local access"))
			return
		}

		clientToken := requestAPIToken(r)
		if !secureTokenEqual(clientToken, token) {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func requiresAPIAuth(r *http.Request) bool {
	return r.URL.Path != "/status"
}

func requestAPIToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth != "" {
		lower := strings.ToLower(auth)
		if strings.HasPrefix(lower, "bearer ") {
			return strings.TrimSpace(auth[len("Bearer "):])
		}
	}
	return strings.TrimSpace(r.Header.Get("X-API-Token"))
}

func secureTokenEqual(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if got == "" || want == "" {
		return false
	}
	gotSum := sha256.Sum256([]byte(got))
	wantSum := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotSum[:], wantSum[:]) == 1
}

func isTrustedLocalRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	return false
}
