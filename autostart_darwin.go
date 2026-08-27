//go:build darwin

package mewsync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const darwinLabel = "com.mewsync.app"

// DarwinAutostart manages a launchd LaunchAgent plist.
type DarwinAutostart struct{}

// NewAutostart returns the OS-appropriate Autostart implementation.
func NewAutostart() Autostart {
	return DarwinAutostart{}
}

func darwinPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", darwinLabel+".plist"), nil
}

func (DarwinAutostart) Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := darwinPlistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>background</string>
  </array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`, darwinLabel, exe)
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	return exec.Command("launchctl", "load", path).Run()
}

func (DarwinAutostart) Disable() error {
	path, err := darwinPlistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	err = os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func (DarwinAutostart) IsEnabled() bool {
	path, err := darwinPlistPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
