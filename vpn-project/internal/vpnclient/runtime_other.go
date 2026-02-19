//go:build !windows

package vpnclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var (
	runtimeMu   sync.Mutex
	runtimeProc = map[string]*exec.Cmd{}
)

func probeRuntime(ctx context.Context) (RuntimeInfo, error) {
	_ = ctx

	singBox, err := exec.LookPath("sing-box")
	if err != nil {
		return RuntimeInfo{}, fmt.Errorf("sing-box not found in PATH")
	}
	return RuntimeInfo{
		Mode:       "sing-box",
		BinaryPath: singBox,
	}, nil
}

func runUp(ctx context.Context, configPath string) (string, error) {
	info, err := probeRuntime(ctx)
	if err != nil {
		return "", err
	}
	absConfigPath, err := configAbsPath(configPath)
	if err != nil {
		return "", err
	}

	runtimeMu.Lock()
	if existing := runtimeProc[absConfigPath]; existing != nil && existing.Process != nil {
		runtimeMu.Unlock()
		return "sing-box is already running", nil
	}
	runtimeMu.Unlock()

	if pid, ok := readRuntimePID(absConfigPath); ok {
		if processExistsUnix(pid) {
			return fmt.Sprintf("sing-box is already running (pid=%d)", pid), nil
		}
		_ = os.Remove(runtimePIDPath(absConfigPath))
	}

	logFile, err := os.OpenFile(runtimeLogPath(absConfigPath), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", fmt.Errorf("open sing-box log: %w", err)
	}

	cmd := exec.Command(info.BinaryPath, "run", "-c", absConfigPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return "", fmt.Errorf("start sing-box: %w", err)
	}

	if err := writeRuntimePID(absConfigPath, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = logFile.Close()
		return "", err
	}

	runtimeMu.Lock()
	runtimeProc[absConfigPath] = cmd
	runtimeMu.Unlock()

	go func(configAbs string, processCmd *exec.Cmd, outFile *os.File) {
		_ = processCmd.Wait()
		_ = outFile.Close()
		_ = os.Remove(runtimePIDPath(configAbs))

		runtimeMu.Lock()
		if runtimeProc[configAbs] == processCmd {
			delete(runtimeProc, configAbs)
		}
		runtimeMu.Unlock()
	}(absConfigPath, cmd, logFile)

	return fmt.Sprintf("sing-box started (pid=%d)", cmd.Process.Pid), nil
}

func runDown(ctx context.Context, configPath string) (string, error) {
	_ = ctx

	absConfigPath, err := configAbsPath(configPath)
	if err != nil {
		return "", err
	}

	runtimeMu.Lock()
	cmd := runtimeProc[absConfigPath]
	if cmd != nil {
		delete(runtimeProc, absConfigPath)
	}
	runtimeMu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
		time.Sleep(250 * time.Millisecond)
		_ = cmd.Process.Kill()
	}

	if pid, ok := readRuntimePID(absConfigPath); ok {
		if processExistsUnix(pid) {
			_ = syscall.Kill(pid, syscall.SIGTERM)
			time.Sleep(250 * time.Millisecond)
			if processExistsUnix(pid) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	}

	_ = os.Remove(runtimePIDPath(absConfigPath))
	return "sing-box stopped", nil
}

func processExistsUnix(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
