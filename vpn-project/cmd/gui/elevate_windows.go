//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func ensureElevatedOrRelaunch() (bool, error) {
	if windows.GetCurrentProcessToken().IsElevated() {
		return false, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("resolve executable path: %w", err)
	}

	args := append([]string{}, os.Args[1:]...)
	if !hasCLIArg(args, autoStartArg) {
		args = append(args, autoStartArg)
	}

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return false, fmt.Errorf("prepare elevation verb: %w", err)
	}
	file, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return false, fmt.Errorf("prepare executable path: %w", err)
	}

	paramsRaw := buildWindowsCommandLine(args)
	var params *uint16
	if strings.TrimSpace(paramsRaw) != "" {
		params, err = windows.UTF16PtrFromString(paramsRaw)
		if err != nil {
			return false, fmt.Errorf("prepare command line: %w", err)
		}
	}

	if err := windows.ShellExecute(0, verb, file, params, nil, windows.SW_NORMAL); err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) && errno == 1223 {
			return false, errors.New("UAC confirmation was canceled")
		}
		return false, fmt.Errorf("relaunch as Administrator: %w", err)
	}

	return true, nil
}

func buildWindowsCommandLine(args []string) string {
	if len(args) == 0 {
		return ""
	}

	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, syscall.EscapeArg(arg))
	}

	return strings.Join(escaped, " ")
}
