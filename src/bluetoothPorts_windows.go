//go:build windows

package main

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

func bluetoothPortNames() map[string]bool {
	ports := make(map[string]bool)
	// SERIALCOMM maps kernel device names to COM ports. The arbiter maps COM
	// ports to PnP identities; consult both in case one source is incomplete.
	for _, path := range []string{
		`HARDWARE\DEVICEMAP\SERIALCOMM`,
		`SYSTEM\CurrentControlSet\Control\COM Name Arbiter\Devices`,
	} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, err := key.ReadValueNames(0)
		if err == nil {
			for _, name := range names {
				value, _, err := key.GetStringValue(name)
				if err != nil {
					continue
				}
				if isBluetoothDevicePath(name) {
					ports[strings.ToUpper(value)] = true
				} else if isBluetoothDevicePath(value) {
					ports[strings.ToUpper(name)] = true
				}
			}
		}
		key.Close()
	}
	return ports
}
