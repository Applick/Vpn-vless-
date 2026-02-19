//go:build !windows

package main

func ensureElevatedOrRelaunch() (bool, error) {
	return false, nil
}
