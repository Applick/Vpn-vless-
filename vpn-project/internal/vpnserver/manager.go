package vpnserver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	UUID       string    `json:"uuid"`
	Address    string    `json:"address,omitempty"` // legacy field kept for API compatibility
	ConfigPath string    `json:"config_path"`
	CreatedAt  time.Time `json:"created_at"`
}

type StatusClient struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UUID      string    `json:"uuid"`
	Address   string    `json:"address,omitempty"` // legacy field kept for API compatibility
	CreatedAt time.Time `json:"created_at"`
}

type StatusResponse struct {
	Running         bool           `json:"running"`
	Interface       string         `json:"interface"`
	ListenPort      int            `json:"listen_port"`
	ServerPublicKey string         `json:"server_public_key,omitempty"` // legacy field
	ClientSubnet    string         `json:"client_subnet,omitempty"`     // legacy field
	Protocol        string         `json:"protocol"`
	Transport       string         `json:"transport"`
	Endpoint        string         `json:"endpoint"`
	Clients         []StatusClient `json:"clients"`
}

type Manager struct {
	mu sync.Mutex

	logger *log.Logger
	cfg    Config

	clientsDir       string
	statePath        string
	serverConfigPath string
	serverLogPath    string
	serverCmd        *exec.Cmd
}

var clientIDRe = regexp.MustCompile(`[^a-z0-9._-]+`)

func NewManager(cfg Config, logger *log.Logger) *Manager {
	return &Manager{
		logger:           logger,
		cfg:              cfg,
		clientsDir:       filepath.Join(cfg.StateDir, "clients"),
		statePath:        filepath.Join(cfg.StateDir, "clients", "clients.json"),
		serverConfigPath: filepath.Join(cfg.StateDir, "server.json"),
		serverLogPath:    filepath.Join(cfg.StateDir, "sing-box.log"),
	}
}

func (m *Manager) InitState() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.clientsDir, 0o700); err != nil {
		return fmt.Errorf("create clients dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.cfg.TLSCertPath), 0o700); err != nil {
		return fmt.Errorf("create tls cert dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.cfg.TLSKeyPath), 0o700); err != nil {
		return fmt.Errorf("create tls key dir: %w", err)
	}
	if err := os.Chmod(m.cfg.StateDir, 0o700); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("chmod %s: %w", m.cfg.StateDir, err)
	}
	if err := os.Chmod(m.clientsDir, 0o700); err != nil {
		return fmt.Errorf("chmod %s: %w", m.clientsDir, err)
	}

	if err := m.ensureTLSMaterialLocked(); err != nil {
		return err
	}

	clients, err := m.loadClientsLocked()
	if err != nil {
		return err
	}
	if len(clients) == 0 {
		if _, _, err := m.createClientLocked("default-client", clients); err != nil {
			return err
		}
		clients, err = m.loadClientsLocked()
		if err != nil {
			return err
		}
	}

	if err := m.rewriteServerConfigLocked(clients); err != nil {
		return err
	}
	return nil
}

func (m *Manager) CreateClient(name string) (Client, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clients, err := m.loadClientsLocked()
	if err != nil {
		return Client{}, "", err
	}
	return m.createClientLocked(name, clients)
}

func (m *Manager) GetStatus() (StatusResponse, error) {
	m.mu.Lock()
	clients, err := m.loadClientsLocked()
	if err != nil {
		m.mu.Unlock()
		return StatusResponse{}, err
	}
	running := m.interfaceRunningLocked()
	m.mu.Unlock()

	list := make([]StatusClient, 0, len(clients))
	for _, c := range clients {
		addr := strings.TrimSpace(c.Address)
		if addr == "" {
			addr = c.UUID
		}
		list = append(list, StatusClient{
			ID:        c.ID,
			Name:      c.Name,
			UUID:      c.UUID,
			Address:   addr,
			CreatedAt: c.CreatedAt,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})

	return StatusResponse{
		Running:    running,
		Interface:  m.cfg.Interface,
		ListenPort: m.cfg.ListenPort,
		Protocol:   "vless",
		Transport:  "ws+tls",
		Endpoint:   m.cfg.EndpointHost,
		Clients:    list,
	}, nil
}

func (m *Manager) GetClientConfig(clientID string) (Client, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clients, err := m.loadClientsLocked()
	if err != nil {
		return Client{}, "", err
	}

	c, ok := clients[clientID]
	if !ok {
		return Client{}, "", os.ErrNotExist
	}

	if err := m.writeClientConfigLocked(c); err != nil {
		return Client{}, "", err
	}

	raw, err := os.ReadFile(c.ConfigPath)
	if err != nil {
		return Client{}, "", fmt.Errorf("read client config: %w", err)
	}
	return c, string(raw), nil
}

func (m *Manager) StartInterface() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clients, err := m.loadClientsLocked()
	if err != nil {
		return err
	}
	if err := m.rewriteServerConfigLocked(clients); err != nil {
		return err
	}

	if m.interfaceRunningLocked() {
		return nil
	}

	return m.startInterfaceLocked()
}

func (m *Manager) startInterfaceLocked() error {
	if m.interfaceRunningLocked() {
		return nil
	}

	logFile, err := os.OpenFile(m.serverLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open sing-box log: %w", err)
	}

	cmd := exec.Command(m.cfg.SingBoxBinary, "run", "-c", m.serverConfigPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start sing-box: %w", err)
	}

	m.serverCmd = cmd
	m.logger.Printf("vless server started with sing-box (pid=%d)", cmd.Process.Pid)

	go func(cmd *exec.Cmd, outFile *os.File) {
		err := cmd.Wait()
		_ = outFile.Close()

		m.mu.Lock()
		if m.serverCmd == cmd {
			m.serverCmd = nil
		}
		m.mu.Unlock()

		if err != nil {
			m.logger.Printf("sing-box exited with error: %v", err)
			return
		}
		m.logger.Printf("sing-box exited")
	}(cmd, logFile)

	return nil
}

func (m *Manager) StopInterface() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopInterfaceLocked()
	return nil
}

func (m *Manager) stopInterfaceLocked() {
	if !m.interfaceRunningLocked() {
		return
	}

	cmd := m.serverCmd
	if cmd == nil || cmd.Process == nil {
		m.serverCmd = nil
		return
	}

	_ = cmd.Process.Signal(os.Interrupt)
	time.Sleep(350 * time.Millisecond)
	_ = cmd.Process.Kill()

	m.serverCmd = nil
	m.logger.Printf("vless server stopped")
}

func (m *Manager) ClientShareURI(c Client) string {
	return buildClientShareURI(m.cfg, c)
}

func (m *Manager) createClientLocked(name string, clients map[string]Client) (Client, string, error) {
	id := allocateClientIDLocked(name, clients)
	userUUID, err := generateUUID()
	if err != nil {
		return Client{}, "", err
	}

	configPath := filepath.Join(m.clientsDir, id+".json")
	c := Client{
		ID:         id,
		Name:       strings.TrimSpace(name),
		UUID:       userUUID,
		Address:    userUUID,
		ConfigPath: configPath,
		CreatedAt:  time.Now().UTC(),
	}
	if c.Name == "" {
		c.Name = id
	}

	clients[c.ID] = c
	if err := m.saveClientsLocked(clients); err != nil {
		return Client{}, "", err
	}
	if err := m.rewriteServerConfigLocked(clients); err != nil {
		return Client{}, "", err
	}
	if m.interfaceRunningLocked() {
		m.stopInterfaceLocked()
		if err := m.startInterfaceLocked(); err != nil {
			return Client{}, "", fmt.Errorf("reload sing-box after creating client: %w", err)
		}
	}

	raw, err := os.ReadFile(c.ConfigPath)
	if err != nil {
		return Client{}, "", fmt.Errorf("read generated client config: %w", err)
	}
	return c, string(raw), nil
}

func (m *Manager) loadClientsLocked() (map[string]Client, error) {
	clients := map[string]Client{}
	raw, err := os.ReadFile(m.statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return clients, nil
		}
		return nil, fmt.Errorf("read clients state: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return clients, nil
	}

	if err := json.Unmarshal(raw, &clients); err != nil {
		return nil, fmt.Errorf("parse clients state: %w", err)
	}

	changed := false
	for id, c := range clients {
		if strings.TrimSpace(c.ID) == "" {
			c.ID = id
			changed = true
		}
		if strings.TrimSpace(c.Name) == "" {
			c.Name = id
			changed = true
		}
		if strings.TrimSpace(c.UUID) == "" {
			u, err := generateUUID()
			if err != nil {
				return nil, err
			}
			c.UUID = u
			changed = true
		}
		if strings.TrimSpace(c.Address) == "" {
			c.Address = c.UUID
			changed = true
		}
		if strings.TrimSpace(c.ConfigPath) == "" {
			c.ConfigPath = filepath.Join(m.clientsDir, c.ID+".json")
			changed = true
		}
		if c.CreatedAt.IsZero() {
			c.CreatedAt = time.Now().UTC()
			changed = true
		}
		clients[id] = c
	}

	if changed {
		if err := m.saveClientsLocked(clients); err != nil {
			return nil, err
		}
	}

	return clients, nil
}

func (m *Manager) saveClientsLocked(clients map[string]Client) error {
	normalized := make(map[string]Client, len(clients))
	for id, c := range clients {
		c.ID = id
		if strings.TrimSpace(c.ConfigPath) == "" {
			c.ConfigPath = filepath.Join(m.clientsDir, id+".json")
		}
		normalized[id] = c
	}

	jsonBytes, err := marshalPretty(normalized)
	if err != nil {
		return fmt.Errorf("serialize clients state: %w", err)
	}
	if err := writeSecretFile(m.statePath, jsonBytes); err != nil {
		return fmt.Errorf("write clients state: %w", err)
	}
	return nil
}

func (m *Manager) rewriteServerConfigLocked(clients map[string]Client) error {
	list := make([]Client, 0, len(clients))
	changed := false
	for id, c := range clients {
		if strings.TrimSpace(c.ConfigPath) == "" {
			c.ConfigPath = filepath.Join(m.clientsDir, id+".json")
			clients[id] = c
			changed = true
		}
		if strings.TrimSpace(c.Address) == "" {
			c.Address = c.UUID
			clients[id] = c
			changed = true
		}
		list = append(list, c)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})

	payload, err := marshalPretty(buildServerConfigMap(m.cfg, list))
	if err != nil {
		return fmt.Errorf("serialize server config: %w", err)
	}
	if err := writeSecretFile(m.serverConfigPath, payload); err != nil {
		return fmt.Errorf("write server config: %w", err)
	}

	for _, c := range list {
		if err := m.writeClientConfigLocked(c); err != nil {
			return err
		}
	}

	if changed {
		if err := m.saveClientsLocked(clients); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) writeClientConfigLocked(c Client) error {
	payload, err := marshalPretty(buildClientConfigMap(m.cfg, c))
	if err != nil {
		return fmt.Errorf("serialize client config: %w", err)
	}
	if err := writeSecretFile(c.ConfigPath, payload); err != nil {
		return fmt.Errorf("write client config: %w", err)
	}
	return nil
}

func (m *Manager) interfaceRunningLocked() bool {
	return m.serverCmd != nil && m.serverCmd.Process != nil
}

func (m *Manager) ensureTLSMaterialLocked() error {
	if fileExists(m.cfg.TLSCertPath) && fileExists(m.cfg.TLSKeyPath) {
		return nil
	}

	commonName := strings.TrimSpace(m.cfg.TLSServerName)
	if commonName == "" {
		commonName = strings.TrimSpace(m.cfg.EndpointHost)
	}
	if commonName == "" {
		commonName = "localhost"
	}

	if err := generateSelfSignedCertificate(m.cfg.TLSCertPath, m.cfg.TLSKeyPath, commonName); err != nil {
		return fmt.Errorf("generate self-signed tls certificate: %w", err)
	}
	m.logger.Printf("generated self-signed TLS certificate at %s", m.cfg.TLSCertPath)
	return nil
}

func buildServerConfigMap(cfg Config, clients []Client) map[string]any {
	users := make([]map[string]string, 0, len(clients))
	for _, c := range clients {
		users = append(users, map[string]string{
			"name": c.Name,
			"uuid": c.UUID,
		})
	}

	return map[string]any{
		"log": map[string]any{
			"level":     "info",
			"timestamp": true,
		},
		"inbounds": []any{
			map[string]any{
				"type":        "vless",
				"tag":         "vless-in",
				"listen":      cfg.ListenAddress,
				"listen_port": cfg.ListenPort,
				"users":       users,
				"tls": map[string]any{
					"enabled":          true,
					"server_name":      cfg.TLSServerName,
					"certificate_path": cfg.TLSCertPath,
					"key_path":         cfg.TLSKeyPath,
				},
				"transport": map[string]any{
					"type": "ws",
					"path": cfg.WebsocketPath,
				},
			},
		},
		"outbounds": []any{
			map[string]any{
				"type": "direct",
				"tag":  "direct",
			},
			map[string]any{
				"type": "block",
				"tag":  "block",
			},
		},
	}
}

func buildClientConfigMap(cfg Config, c Client) map[string]any {
	host, port := resolveEndpointHostPort(cfg.EndpointHost, cfg.ListenPort)
	tunAddresses := splitAndTrimCSV(cfg.ClientTunCIDR)
	if len(tunAddresses) == 0 {
		tunAddresses = []string{"172.19.0.1/30"}
	}
	endpointExcludeCIDRs := routeExcludeCIDRsForHost(host)
	privateCIDRs := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	tunInbound := map[string]any{
		"type":                       "tun",
		"tag":                        "tun-in",
		"interface_name":             cfg.ClientTunName,
		"address":                    tunAddresses,
		"auto_route":                 true,
		"strict_route":               true,
		"sniff":                      true,
		"sniff_override_destination": false,
		"stack":                      "mixed",
	}
	if len(endpointExcludeCIDRs) > 0 {
		tunInbound["route_exclude_address"] = endpointExcludeCIDRs
	}

	return map[string]any{
		"log": map[string]any{
			"level": "warn",
		},
		"dns": map[string]any{
			"servers": []any{
				map[string]any{
					"type":   "udp",
					"tag":    "dns-remote",
					"server": "1.1.1.1",
					"detour": "vless-out",
				},
			},
			"final":    "dns-remote",
			"strategy": "prefer_ipv4",
		},
		"inbounds": []any{
			tunInbound,
		},
		"outbounds": []any{
			map[string]any{
				"type":        "vless",
				"tag":         "vless-out",
				"server":      host,
				"server_port": port,
				"uuid":        c.UUID,
				"tls": map[string]any{
					"enabled":     true,
					"server_name": cfg.TLSServerName,
					"insecure":    cfg.ClientInsecureTLS,
				},
				"transport": map[string]any{
					"type": "ws",
					"path": cfg.WebsocketPath,
				},
			},
			map[string]any{
				"type": "direct",
				"tag":  "direct",
			},
			map[string]any{
				"type": "block",
				"tag":  "block",
			},
		},
		"route": map[string]any{
			"auto_detect_interface": true,
			"default_domain_resolver": map[string]any{
				"server":   "dns-remote",
				"strategy": "prefer_ipv4",
			},
			"rules": []any{
				map[string]any{
					"protocol": "dns",
					"action":   "hijack-dns",
				},
				map[string]any{
					"ip_cidr":  privateCIDRs,
					"outbound": "direct",
				},
			},
			"final": "vless-out",
		},
	}
}

func buildClientShareURI(cfg Config, c Client) string {
	query := url.Values{}
	query.Set("encryption", "none")
	query.Set("security", "tls")
	query.Set("type", "ws")
	query.Set("path", cfg.WebsocketPath)
	if strings.TrimSpace(cfg.TLSServerName) != "" {
		query.Set("sni", cfg.TLSServerName)
	}
	if cfg.ClientInsecureTLS {
		query.Set("allowInsecure", "1")
	}

	host, port := resolveEndpointHostPort(cfg.EndpointHost, cfg.ListenPort)

	uri := url.URL{
		Scheme:   "vless",
		User:     url.User(c.UUID),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		RawQuery: query.Encode(),
		Fragment: c.Name,
	}
	return uri.String()
}

func resolveEndpointHostPort(rawHost string, fallbackPort int) (string, int) {
	host := strings.TrimSpace(rawHost)
	port := fallbackPort

	if host == "" {
		return "127.0.0.1", port
	}

	if parsedHost, parsedPort, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
		if p, convErr := strconv.Atoi(parsedPort); convErr == nil && p > 0 {
			port = p
		}
		return host, port
	}

	return host, port
}

func routeExcludeCIDRsForHost(host string) []string {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return nil
	}

	parsedIP := net.ParseIP(trimmedHost)
	if parsedIP == nil {
		return nil
	}
	if ipv4 := parsedIP.To4(); ipv4 != nil {
		return []string{ipv4.String() + "/32"}
	}

	return []string{parsedIP.String() + "/128"}
}

func allocateClientIDLocked(name string, clients map[string]Client) string {
	base := sanitizeClientID(name)
	if _, exists := clients[base]; !exists {
		return base
	}
	for i := 2; i < 10000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, exists := clients[candidate]; !exists {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func sanitizeClientID(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		n = "client"
	}
	n = clientIDRe.ReplaceAllString(n, "-")
	n = strings.Trim(n, "-_.")
	if n == "" {
		return "client"
	}
	return n
}

func splitAndTrimCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	), nil
}

func generateSelfSignedCertificate(certPath, keyPath, commonName string) error {
	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return err
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return err
	}

	notBefore := time.Now().Add(-1 * time.Hour)
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	if ip := net.ParseIP(commonName); ip != nil {
		tpl.IPAddresses = append(tpl.IPAddresses, ip)
	} else {
		tpl.DNSNames = append(tpl.DNSNames, commonName)
	}
	tpl.DNSNames = append(tpl.DNSNames, "localhost")
	tpl.IPAddresses = append(tpl.IPAddresses, net.ParseIP("127.0.0.1"))

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	if err := writeSecretFile(certPath, certPEM); err != nil {
		return err
	}
	if err := writeSecretFile(keyPath, keyPEM); err != nil {
		return err
	}
	return nil
}

func marshalPretty(v any) ([]byte, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		raw = append(raw, '\n')
	}
	return raw, nil
}

func writeSecretFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
