//go:build !windows

package main

// Bluetooth ports on Unix are identified by their port names or metadata.
func bluetoothPortNames() map[string]bool { return nil }
