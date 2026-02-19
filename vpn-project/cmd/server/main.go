package main

import (
	"vpn-project/internal/vpnserver"
)

func main() {
	logger := vpnserver.NewLogger()
	cfg := vpnserver.LoadConfigFromEnv()
	app := vpnserver.NewApp(cfg, logger)

	if err := app.Run(); err != nil {
		logger.Fatalf("%v", err)
	}
}
