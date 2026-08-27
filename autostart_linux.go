//go:build linux

package mewsync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const linuxUnitName = "mewsync.service"

// LinuxAutostart manages a systemd user unit.
type LinuxAutostart struct{}

// NewAutostart returns the OS-appropriate Autostart implementation.
func NewAutostart() Autostart {
	return LinuxAutostart{}
}

func linuxUnitDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func linuxUnitPath() (string, error) {
	dir, err := linuxUnitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, linuxUnitName), nil
}

func (LinuxAutostart) Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := linuxUnitDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=mewsync

[Service]
ExecStart=%s background
Restart=on-failure

[Install]
WantedBy=default.target
`, exe)
	path, err := linuxUnitPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return exec.Command("systemctl", "--user", "enable", linuxUnitName).Run()
}

func (LinuxAutostart) Disable() error {
	_ = exec.Command("systemctl", "--user", "disable", linuxUnitName).Run()
	path, err := linuxUnitPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func (LinuxAutostart) IsEnabled() bool {
	path, err := linuxUnitPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
