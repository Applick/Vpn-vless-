//go:build windows

package vpnclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const singBoxRuntimeEnv = "VPN_RUNTIME_DIR"

var (
	runtimeMu   sync.Mutex
	runtimeProc = map[string]*exec.Cmd{}
)

func probeRuntime(ctx context.Context) (RuntimeInfo, error) {
	_ = ctx

	searchDirs := runtimeSearchDirsWindows()
	if binary := findExecutableInDirs([]string{"sing-box.exe", "sing-box"}, searchDirs); binary != "" {
		return RuntimeInfo{
			Mode:        "sing-box",
			BinaryPath:  binary,
			SearchPaths: searchDirs,
		}, nil
	}

	if binary, err := exec.LookPath("sing-box.exe"); err == nil {
		return RuntimeInfo{
			Mode:        "sing-box",
			BinaryPath:  binary,
			SearchPaths: searchDirs,
		}, nil
	}
	if binary, err := exec.LookPath("sing-box"); err == nil {
		return RuntimeInfo{
			Mode:        "sing-box",
			BinaryPath:  binary,
			SearchPaths: searchDirs,
		}, nil
	}

	return RuntimeInfo{SearchPaths: searchDirs}, fmt.Errorf("sing-box runtime not found (checked local runtime folders, PATH and Program Files)")
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
		if processExistsWindows(pid) {
			return fmt.Sprintf("sing-box is already running (pid=%d)", pid), nil
		}
		_ = os.Remove(runtimePIDPath(absConfigPath))
	}

	logFile, err := os.OpenFile(runtimeLogPath(absConfigPath), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", fmt.Errorf("open sing-box log: %w", err)
	}

	cmd := exec.Command(info.BinaryPath, "run", "-c", absConfigPath)
	cmd.Dir = filepath.Dir(info.BinaryPath)
	cmd.Env = withPathPrefix(os.Environ(), cmd.Dir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

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

	time.Sleep(1800 * time.Millisecond)
	if !processExistsWindows(cmd.Process.Pid) {
		_ = os.Remove(runtimePIDPath(absConfigPath))
		runtimeMu.Lock()
		delete(runtimeProc, absConfigPath)
		runtimeMu.Unlock()
		logTail := strings.TrimSpace(readTailLines(runtimeLogPath(absConfigPath), 20))
		if logTail != "" {
			return logTail, fmt.Errorf("sing-box exited right after start")
		}
		return "", fmt.Errorf("sing-box exited right after start")
	}

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
	absConfigPath, err := configAbsPath(configPath)
	if err != nil {
		return "", err
	}

	var details []string

	runtimeMu.Lock()
	cmd := runtimeProc[absConfigPath]
	if cmd != nil {
		delete(runtimeProc, absConfigPath)
	}
	runtimeMu.Unlock()

	if cmd != nil && cmd.Process != nil {
		taskkillOut, killErr := killWindowsProcess(ctx, cmd.Process.Pid)
		if strings.TrimSpace(taskkillOut) != "" {
			details = append(details, strings.TrimSpace(taskkillOut))
		}
		if killErr != nil {
			return strings.Join(details, " | "), killErr
		}
	}

	if pid, ok := readRuntimePID(absConfigPath); ok {
		taskkillOut, killErr := killWindowsProcess(ctx, pid)
		if strings.TrimSpace(taskkillOut) != "" {
			details = append(details, strings.TrimSpace(taskkillOut))
		}
		if killErr != nil {
			return strings.Join(details, " | "), killErr
		}
	}

	_ = os.Remove(runtimePIDPath(absConfigPath))
	if len(details) == 0 {
		details = append(details, "sing-box stopped")
	}
	return strings.Join(details, " | "), nil
}

func runtimeSearchDirsWindows() []string {
	exeDir := executableDir()
	cwd, _ := os.Getwd()
	envRuntimeDir := strings.TrimSpace(os.Getenv(singBoxRuntimeEnv))

	programFiles := strings.TrimSpace(os.Getenv("ProgramFiles"))
	programFilesX86 := strings.TrimSpace(os.Getenv("ProgramFiles(x86)"))

	dirs := []string{
		envRuntimeDir,
		filepath.Join(exeDir, "runtime", "windows"),
		filepath.Join(exeDir, "runtime"),
		filepath.Join(cwd, "runtime", "windows"),
		filepath.Join(cwd, "runtime"),
	}
	if programFiles != "" {
		dirs = append(dirs, filepath.Join(programFiles, "sing-box"))
	}
	if programFilesX86 != "" {
		dirs = append(dirs, filepath.Join(programFilesX86, "sing-box"))
	}
	dirs = append(dirs, filepath.SplitList(os.Getenv("PATH"))...)
	return dedupePaths(dirs)
}

func killWindowsProcess(ctx context.Context, pid int) (string, error) {
	if pid <= 0 {
		return "", nil
	}
	out, err := runCommandCombined(ctx, "", "taskkill.exe", "/PID", strconv.Itoa(pid), "/T")
	if err == nil {
		return out, nil
	}

	lower := strings.ToLower(strings.TrimSpace(out + " " + err.Error()))
	if strings.Contains(lower, "not found") ||
		strings.Contains(lower, "no running instance") ||
		strings.Contains(lower, "cannot find") ||
		strings.Contains(lower, "не найден") {
		return out, nil
	}

	if !processExistsWindows(pid) {
		return out, nil
	}

	forcedOut, forcedErr := runCommandCombined(ctx, "", "taskkill.exe", "/PID", strconv.Itoa(pid), "/F", "/T")
	combined := strings.TrimSpace(strings.TrimSpace(out) + "\n" + strings.TrimSpace(forcedOut))
	if forcedErr == nil {
		return combined, nil
	}

	forcedLower := strings.ToLower(strings.TrimSpace(forcedOut + " " + forcedErr.Error()))
	if strings.Contains(forcedLower, "not found") ||
		strings.Contains(forcedLower, "no running instance") ||
		strings.Contains(forcedLower, "cannot find") ||
		strings.Contains(forcedLower, "не найден") {
		return combined, nil
	}
	return combined, forcedErr
}

func processExistsWindows(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := runCommandCombined(context.Background(), "", "tasklist.exe", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	if err != nil {
		return false
	}
	trimmed := strings.TrimSpace(strings.ToLower(out))
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "no tasks are running") {
		return false
	}
	return strings.Contains(trimmed, fmt.Sprintf(",\"%d\"", pid)) || strings.Contains(trimmed, "\"sing-box.exe\"")
}

func readTailLines(path string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	start := 0
	if len(lines) > maxLines {
		start = len(lines) - maxLines
	}
	return strings.Join(lines[start:], "\n")
}
