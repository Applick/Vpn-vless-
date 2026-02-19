package vpnclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runCommandCombined(ctx context.Context, workDir, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if workDir != "" {
		cmd.Dir = workDir
		cmd.Env = withPathPrefix(os.Environ(), workDir)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w", command, strings.Join(args, " "), err)
	}
	return string(out), nil
}

func withPathPrefix(env []string, prefix string) []string {
	if strings.TrimSpace(prefix) == "" {
		return env
	}

	pathKey := "PATH="
	pathValue := os.Getenv("PATH")
	newPath := prefix + string(os.PathListSeparator) + pathValue
	updated := make([]string, 0, len(env)+1)
	found := false

	for _, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			updated = append(updated, pathKey+newPath)
			found = true
			continue
		}
		updated = append(updated, e)
	}
	if !found {
		updated = append(updated, pathKey+newPath)
	}
	return updated
}

func configAbsPath(configPath string) (string, error) {
	if strings.TrimSpace(configPath) == "" {
		return "", fmt.Errorf("config path is empty")
	}
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func dedupePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func findExecutableInDirs(names []string, dirs []string) string {
	for _, dir := range dirs {
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate
			}
		}
	}
	return ""
}
