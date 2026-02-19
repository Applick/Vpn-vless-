package vpnserver

import (
	"io"
	"log"
	"os"
)

func NewLogger() *log.Logger {
	logWriters := []io.Writer{os.Stdout}
	if err := os.MkdirAll("/var/log", 0o755); err == nil {
		f, err := os.OpenFile("/var/log/vpn-api.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			logWriters = append(logWriters, f)
		}
	}
	return log.New(io.MultiWriter(logWriters...), "[vpn-server] ", log.LstdFlags|log.LUTC)
}
