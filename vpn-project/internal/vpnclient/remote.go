package vpnclient

import (
	"os"
	"strings"
	"time"

	sshdeploy "vpn-project/internal/ssh"
)

const (
	remoteComposePath = "/root/vpn-project/docker-compose.yml"
	remoteServiceName = "vlessserver"
)

func StartRemoteViaSSH(cfg SSHConfig) (string, error) {
	sshCfg := sshdeploy.ClientConfig{
		User:            strings.TrimSpace(cfg.User),
		Host:            strings.TrimSpace(cfg.Host),
		Port:            strings.TrimSpace(cfg.Port),
		PrivateKeyPath:  strings.TrimSpace(cfg.PrivateKeyPath),
		Password:        cfg.Password,
		KnownHostsPath:  strings.TrimSpace(cfg.KnownHostsPath),
		InsecureHostKey: cfg.InsecureHostKey || envTruthy("VPN_SSH_INSECURE_HOST_KEY"),
		Timeout:         20 * time.Second,
	}
	return sshdeploy.RunCommand(sshCfg, remoteStartCommand())
}

func StopRemoteViaSSH(cfg SSHConfig) (string, error) {
	sshCfg := sshdeploy.ClientConfig{
		User:            strings.TrimSpace(cfg.User),
		Host:            strings.TrimSpace(cfg.Host),
		Port:            strings.TrimSpace(cfg.Port),
		PrivateKeyPath:  strings.TrimSpace(cfg.PrivateKeyPath),
		Password:        cfg.Password,
		KnownHostsPath:  strings.TrimSpace(cfg.KnownHostsPath),
		InsecureHostKey: cfg.InsecureHostKey || envTruthy("VPN_SSH_INSECURE_HOST_KEY"),
		Timeout:         20 * time.Second,
	}
	return sshdeploy.RunCommand(sshCfg, remoteStopCommand())
}

func envTruthy(key string) bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func remoteStartCommand() string {
	return "docker start " + remoteServiceName + " >/dev/null 2>&1 || docker compose -f " + remoteComposePath + " up -d " + remoteServiceName
}

func remoteStopCommand() string {
	return "docker stop " + remoteServiceName + " || true"
}
