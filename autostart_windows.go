//go:build windows

package mewsync

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const windowsRunValueName = "mewsync"

// WindowsAutostart manages an HKCU Run registry value.
type WindowsAutostart struct{}

// NewAutostart returns the OS-appropriate Autostart implementation.
func NewAutostart() Autostart {
	return WindowsAutostart{}
}

func (WindowsAutostart) Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(windowsRunValueName, fmt.Sprintf(`"%s" background`, exe))
}

func (WindowsAutostart) Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	err = key.DeleteValue(windowsRunValueName)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

func (WindowsAutostart) IsEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	_, _, err = key.GetStringValue(windowsRunValueName)
	return err == nil
}
