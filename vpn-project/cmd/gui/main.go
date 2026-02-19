package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"vpn-project/internal/vpnclient"
)

const autoStartArg = "--autostart"

func main() {
	vpnApp := app.New()
	win := vpnApp.NewWindow("VPN Client")
	win.Resize(fyne.NewSize(620, 280))
	autoStartRequested := hasCLIArg(os.Args[1:], autoStartArg)

	runner := vpnclient.NewRunner()
	bootstrap := loadBootstrapConfig()
	confPath := resolveLocalConfigPath(bootstrap.BaseDir, bootstrap.LocalConfigPath)

	titleLabel := widget.NewLabel("VPN Client")
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	infoLabel := widget.NewLabel(fmt.Sprintf("Server: %s | Client: %s", bootstrap.ServerHost, bootstrap.ClientID))
	infoLabel.Wrapping = fyne.TextWrapWord
	statusLabel := widget.NewLabel("Status: ready. Press Start VPN.")
	statusLabel.Wrapping = fyne.TextWrapWord

	setStatus := func(msg string) {
		fyne.Do(func() {
			statusLabel.SetText("Status: " + msg)
		})
	}

	startBtn := widget.NewButton("Start VPN", nil)
	stopBtn := widget.NewButton("Stop VPN", nil)

	setButtons := func(startEnabled, stopEnabled bool) {
		fyne.Do(func() {
			if startEnabled {
				startBtn.Enable()
			} else {
				startBtn.Disable()
			}
			if stopEnabled {
				stopBtn.Enable()
			} else {
				stopBtn.Disable()
			}
		})
	}

	loadConfigFromAPI := func() error {
		resp, err := vpnclient.FetchClientConfigWithToken(bootstrap.ServerHost, bootstrap.ClientID, bootstrap.APIToken)
		if err != nil {
			return fmt.Errorf("%s", explainAPIError(err))
		}
		if err := saveConfigToPath(confPath, resp.Config); err != nil {
			return err
		}
		return nil
	}

	startVPN := func() {
		go func() {
			setButtons(false, false)

			relaunched, elevateErr := ensureElevatedOrRelaunch()
			if elevateErr != nil {
				setStatus("administrator elevation error: " + elevateErr.Error())
				setButtons(true, false)
				return
			}
			if relaunched {
				setStatus("requesting Administrator privileges...")
				fyne.Do(func() {
					vpnApp.Quit()
				})
				return
			}

			setStatus("checking runtime...")
			probeCtx, probeCancel := context.WithTimeout(context.Background(), 70*time.Second)
			probeInfo, probeErr := runner.Probe(probeCtx)
			probeCancel()
			if probeErr != nil {
				setStatus("runtime error: " + probeErr.Error())
				setButtons(true, false)
				return
			}

			if bootstrap.AutoFetchConfig && !localConfigReady(confPath) {
				setStatus("loading config from API...")
				if err := loadConfigFromAPI(); err != nil {
					setStatus("config load error: " + err.Error())
					setButtons(true, false)
					return
				}
			}

			if !localConfigReady(confPath) {
				setStatus("client.conf missing: " + confPath)
				setButtons(true, false)
				return
			}
			if err := normalizeConfigFile(confPath); err != nil {
				setStatus("config format error: " + err.Error())
				setButtons(true, false)
				return
			}

			setStatus("starting VPN (" + probeInfo.Mode + ")...")
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			out, err := runner.Up(ctx, confPath)
			if err != nil {
				details := strings.TrimSpace(out)
				if details != "" {
					setStatus("Start VPN error: " + err.Error() + " | " + details)
				} else {
					setStatus("Start VPN error: " + err.Error())
				}
				setButtons(true, false)
				return
			}

			setStatus("VPN started")
			setButtons(false, true)
		}()
	}
	startBtn.OnTapped = startVPN

	stopBtn.OnTapped = func() {
		go func() {
			setButtons(false, false)

			setStatus("stopping VPN...")
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			out, err := runner.Down(ctx, confPath)
			if err != nil {
				details := strings.TrimSpace(out)
				if details != "" {
					setStatus("Stop VPN error: " + err.Error() + " | " + details)
				} else {
					setStatus("Stop VPN error: " + err.Error())
				}
				setButtons(false, true)
				return
			}

			setStatus("VPN stopped")
			setButtons(true, false)
		}()
	}

	setButtons(true, false)
	content := container.NewVBox(
		titleLabel,
		infoLabel,
		container.NewGridWithColumns(2, startBtn, stopBtn),
		statusLabel,
	)

	win.SetContent(container.NewPadded(content))

	if autoStartRequested {
		setStatus("auto-start requested...")
		go func() {
			time.Sleep(350 * time.Millisecond)
			startVPN()
		}()
	}

	win.ShowAndRun()
}

func explainAPIError(err error) string {
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)

	switch {
	case strings.Contains(msg, "401"):
		return msg + " (API token invalid or missing)"
	case strings.Contains(msg, "404"):
		return msg + " (client ID not found on server)"
	case strings.Contains(lower, "dial tcp"),
		strings.Contains(lower, "connectex"),
		strings.Contains(lower, "i/o timeout"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "tls"):
		return msg + " (cannot reach VPN API server)"
	default:
		return msg
	}
}

func saveConfigToPath(confPath, config string) error {
	if strings.TrimSpace(confPath) == "" {
		return errors.New("local config path is empty")
	}
	if strings.TrimSpace(config) == "" {
		return errors.New("config is empty")
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(confPath, []byte(config), 0o600)
}

func localConfigReady(confPath string) bool {
	info, err := os.Stat(confPath)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() > 0
}

func normalizeConfigFile(confPath string) error {
	raw, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}
	if len(raw) >= 2 {
		isUTF16LE := raw[0] == 0xFF && raw[1] == 0xFE
		isUTF16BE := raw[0] == 0xFE && raw[1] == 0xFF
		if isUTF16LE || isUTF16BE {
			return errors.New("client.conf is UTF-16, please save it as UTF-8")
		}
	}
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		if err := os.WriteFile(confPath, raw[3:], 0o600); err != nil {
			return err
		}
	}
	return nil
}

type bootstrapConfig struct {
	BaseDir         string
	ServerHost      string
	ClientID        string
	APIToken        string
	LocalConfigPath string
	AutoFetchConfig bool
}

type bootstrapConfigFile struct {
	ServerHost      string `json:"server_host"`
	ClientID        string `json:"client_id"`
	APIToken        string `json:"api_token"`
	LocalConfigPath string `json:"local_config_path"`
	AutoFetchConfig *bool  `json:"auto_fetch_config,omitempty"`
}

func loadBootstrapConfig() bootstrapConfig {
	baseDir := localExecutableDir()
	if strings.TrimSpace(baseDir) == "" {
		baseDir, _ = os.Getwd()
	}

	cfg := bootstrapConfig{
		BaseDir:         baseDir,
		ServerHost:      "127.0.0.1",
		ClientID:        "default-client",
		LocalConfigPath: filepath.Join(baseDir, "client.conf"),
		AutoFetchConfig: true,
	}

	for _, candidate := range bootstrapConfigCandidates(baseDir) {
		fileCfg, err := readBootstrapConfigFile(candidate)
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(fileCfg.ServerHost); v != "" {
			cfg.ServerHost = v
		}
		if v := strings.TrimSpace(fileCfg.ClientID); v != "" {
			cfg.ClientID = v
		}
		if v := strings.TrimSpace(fileCfg.APIToken); v != "" {
			cfg.APIToken = v
		}
		if v := strings.TrimSpace(fileCfg.LocalConfigPath); v != "" {
			cfg.LocalConfigPath = v
		}
		if fileCfg.AutoFetchConfig != nil {
			cfg.AutoFetchConfig = *fileCfg.AutoFetchConfig
		}
		break
	}

	if v := strings.TrimSpace(os.Getenv("VPN_SERVER_HOST")); v != "" {
		cfg.ServerHost = v
	}
	if v := strings.TrimSpace(os.Getenv("VPN_CLIENT_ID")); v != "" {
		cfg.ClientID = v
	}
	if v := strings.TrimSpace(os.Getenv("VPN_API_TOKEN")); v != "" {
		cfg.APIToken = v
	}
	if v := strings.TrimSpace(os.Getenv("VPN_LOCAL_CONF_PATH")); v != "" {
		cfg.LocalConfigPath = v
	}
	if v, ok := parseBoolEnv("VPN_AUTO_FETCH_CONFIG"); ok {
		cfg.AutoFetchConfig = v
	}

	cfg.LocalConfigPath = resolveLocalConfigPath(baseDir, cfg.LocalConfigPath)
	return cfg
}

func bootstrapConfigCandidates(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, "client.settings.json"),
		"client.settings.json",
	}
}

func readBootstrapConfigFile(path string) (bootstrapConfigFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return bootstrapConfigFile{}, err
	}
	var cfg bootstrapConfigFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return bootstrapConfigFile{}, err
	}
	return cfg, nil
}

func parseBoolEnv(key string) (bool, bool) {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true, true
	case "0", "false", "no", "n", "off":
		return false, true
	default:
		return false, false
	}
}

func hasCLIArg(args []string, want string) bool {
	trimmedWant := strings.TrimSpace(want)
	if trimmedWant == "" {
		return false
	}
	for _, arg := range args {
		if strings.TrimSpace(arg) == trimmedWant {
			return true
		}
	}
	return false
}

func localExecutableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func resolveLocalConfigPath(baseDir, rawPath string) string {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		if strings.TrimSpace(baseDir) == "" {
			return "client.conf"
		}
		return filepath.Join(baseDir, "client.conf")
	}
	if filepath.IsAbs(path) {
		return path
	}
	if strings.TrimSpace(baseDir) == "" {
		return path
	}
	return filepath.Join(baseDir, path)
}
