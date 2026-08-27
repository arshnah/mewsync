package mewsync

// Autostart registers or unregisters mewsync to launch on login. Each OS
// gets its own implementation in autostart_<os>.go.
type Autostart interface {
	Enable() error
	Disable() error
	IsEnabled() bool
}
