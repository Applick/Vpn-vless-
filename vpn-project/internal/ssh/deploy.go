package ssh

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type ClientConfig struct {
	User            string
	Host            string
	Port            string
	PrivateKeyPath  string
	Password        string
	KnownHostsPath  string
	InsecureHostKey bool
	Timeout         time.Duration
}

func RunCommand(cfg ClientConfig, command string) (string, error) {
	if cfg.User == "" || cfg.Host == "" {
		return "", fmt.Errorf("ssh user/host is required")
	}
	if cfg.Port == "" {
		cfg.Port = "22"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}

	authMethods := make([]gossh.AuthMethod, 0, 2)
	if cfg.PrivateKeyPath != "" {
		keyBytes, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return "", fmt.Errorf("read private key: %w", err)
		}
		signer, err := gossh.ParsePrivateKey(keyBytes)
		if err != nil {
			return "", fmt.Errorf("parse private key: %w", err)
		}
		authMethods = append(authMethods, gossh.PublicKeys(signer))
	}
	if cfg.Password != "" {
		authMethods = append(authMethods, gossh.Password(cfg.Password))
	}
	if len(authMethods) == 0 {
		return "", fmt.Errorf("no auth method provided (private key or password)")
	}

	hostKeyCallback, err := buildHostKeyCallback(cfg)
	if err != nil {
		return "", err
	}

	clientCfg := &gossh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         cfg.Timeout,
	}

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	client, err := gossh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return "", fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh new session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(command)
	if err != nil {
		return string(out), fmt.Errorf("ssh exec: %w", err)
	}
	return string(out), nil
}

func buildHostKeyCallback(cfg ClientConfig) (gossh.HostKeyCallback, error) {
	if cfg.InsecureHostKey {
		return gossh.InsecureIgnoreHostKey(), nil
	}

	candidates := knownHostsCandidates(cfg.KnownHostsPath)
	existing := make([]string, 0, len(candidates))
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			existing = append(existing, p)
		}
	}
	if len(existing) == 0 {
		return nil, fmt.Errorf("ssh host key verification requires known_hosts (set KnownHostsPath or enable insecure mode explicitly)")
	}

	callback, err := knownhosts.New(existing...)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}
	return callback, nil
}

func knownHostsCandidates(explicitPath string) []string {
	paths := make([]string, 0, 3)
	if trimmed := strings.TrimSpace(explicitPath); trimmed != "" {
		paths = append(paths, trimmed)
	}

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		paths = append(paths, filepath.Join(home, ".ssh", "known_hosts"))
	}

	if userProfile := strings.TrimSpace(os.Getenv("USERPROFILE")); userProfile != "" {
		paths = append(paths, filepath.Join(userProfile, ".ssh", "known_hosts"))
	}

	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
