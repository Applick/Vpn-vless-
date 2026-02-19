package vpnclient

import "context"

type ClientConfigResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Config   string `json:"config"`
	VLESSURI string `json:"vless_uri,omitempty"`
	QRBase64 string `json:"qr_base64"`
}

type SSHConfig struct {
	User            string
	Host            string
	Port            string
	PrivateKeyPath  string
	Password        string
	KnownHostsPath  string
	InsecureHostKey bool
}

type RuntimeInfo struct {
	Mode          string
	BinaryPath    string
	SearchPaths   []string
	AutoInstalled bool
}

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Probe(ctx context.Context) (RuntimeInfo, error) {
	return probeRuntime(ctx)
}

func (r *Runner) Up(ctx context.Context, configPath string) (string, error) {
	return runUp(ctx, configPath)
}

func (r *Runner) Down(ctx context.Context, configPath string) (string, error) {
	return runDown(ctx, configPath)
}
