package vpnserver

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type App struct {
	cfg     Config
	logger  *log.Logger
	manager *Manager
}

func NewApp(cfg Config, logger *log.Logger) *App {
	return &App{
		cfg:     cfg,
		logger:  logger,
		manager: NewManager(cfg, logger),
	}
}

func (a *App) Run() error {
	if err := a.manager.InitState(); err != nil {
		return fmt.Errorf("state init failed: %w", err)
	}

	if a.cfg.AutoStart {
		if err := a.manager.StartInterface(); err != nil {
			a.logger.Printf("autostart failed: %v", err)
		}
	}

	server := &http.Server{
		Addr:              a.cfg.APIBind,
		Handler:           NewHTTPHandler(a.manager, a.logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if strings.TrimSpace(a.cfg.APIToken) == "" {
		a.logger.Printf("API_TOKEN is empty: non-local API calls require tokenless trusted-local access only")
	}
	if a.cfg.ClientInsecureTLS {
		a.logger.Printf("WARNING: VLESS_CLIENT_INSECURE_TLS=true (clients skip TLS certificate verification)")
	}

	a.logger.Printf("VPN manager API listening on %s", a.cfg.APIBind)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server failed: %w", err)
	}
	return nil
}
