package vpnclient

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func runtimePIDPath(configPath string) string {
	return configPath + ".pid"
}

func runtimeLogPath(configPath string) string {
	return configPath + ".log"
}

func readRuntimePID(configPath string) (int, bool) {
	raw, err := os.ReadFile(runtimePIDPath(configPath))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func writeRuntimePID(configPath string, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid")
	}
	return os.WriteFile(runtimePIDPath(configPath), []byte(strconv.Itoa(pid)+"\n"), 0o600)
}
