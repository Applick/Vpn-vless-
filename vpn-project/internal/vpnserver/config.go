package vpnserver

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	StateDir          string
	Interface         string
	ListenAddress     string
	ListenPort        int
	EndpointHost      string
	WebsocketPath     string
	TLSServerName     string
	TLSCertPath       string
	TLSKeyPath        string
	ClientTunName     string
	ClientTunCIDR     string
	ClientInsecureTLS bool
	SingBoxBinary     string
	APIBind           string
	APIToken          string
	AutoStart         bool
}

func LoadConfigFromEnv() Config {
	stateDir := firstNonEmpty(
		strings.TrimSpace(os.Getenv("VLESS_STATE_DIR")),
		strings.TrimSpace(os.Getenv("WG_DIR")),
		"/etc/vpn",
	)

	endpoint := firstNonEmpty(
		strings.TrimSpace(os.Getenv("VLESS_ENDPOINT")),
		strings.TrimSpace(os.Getenv("WG_ENDPOINT")),
		"127.0.0.1",
	)

	return Config{
		StateDir:          stateDir,
		Interface:         "vless",
		ListenAddress:     envOrDefault("VLESS_LISTEN_ADDRESS", "::"),
		ListenPort:        envInt("VLESS_LISTEN_PORT", envInt("WG_LISTEN_PORT", 443)),
		EndpointHost:      endpoint,
		WebsocketPath:     normalizeWebsocketPath(envOrDefault("VLESS_WS_PATH", "/vpn")),
		TLSServerName:     envOrDefault("VLESS_TLS_SERVER_NAME", endpoint),
		TLSCertPath:       envOrDefault("VLESS_TLS_CERT_PATH", "/etc/vpn/tls/server.crt"),
		TLSKeyPath:        envOrDefault("VLESS_TLS_KEY_PATH", "/etc/vpn/tls/server.key"),
		ClientTunName:     envOrDefault("VLESS_CLIENT_TUN_NAME", "sb-tun"),
		ClientTunCIDR:     envOrDefault("VLESS_CLIENT_TUN_CIDR", "172.19.0.1/30"),
		ClientInsecureTLS: envBool("VLESS_CLIENT_INSECURE_TLS", true),
		SingBoxBinary:     envOrDefault("SING_BOX_BIN", "sing-box"),
		APIBind:           envOrDefault("API_BIND", "127.0.0.1:8080"),
		APIToken:          strings.TrimSpace(os.Getenv("API_TOKEN")),
		AutoStart:         envBool("VLESS_AUTOSTART", envBool("WG_AUTOSTART", true)),
	}
}

func envOrDefault(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func normalizeWebsocketPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "/vpn"
	}
	if !strings.HasPrefix(trimmed, "/") {
		return "/" + trimmed
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
