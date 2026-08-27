package mewsync

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/pelletier/go-toml/v2"
)

// ConfigDir returns the platform config directory for mewsync, honoring the
// MEWSYNC_CONFIG_DIR override.
func ConfigDir() string {
	if v := os.Getenv("MEWSYNC_CONFIG_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("APPDATA"); v != "" {
			return filepath.Join(v, "mewsync")
		}
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "mewsync")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "mewsync")
	}
	return "."
}

type fileLastfm struct {
	APIKey   string `toml:"api_key"`
	Username string `toml:"username"`
}

type fileAdvanced struct {
	Enabled  bool   `toml:"enabled"`
	Emoji    string `toml:"emoji"`
	Template string `toml:"template"`
}

type fileView struct {
	Timestamp bool         `toml:"timestamp"`
	Label     bool         `toml:"label"`
	Emoji     string       `toml:"emoji"`
	AutoClear bool         `toml:"auto_clear"`
	Advanced  fileAdvanced `toml:"advanced"`
}

type fileTiming struct {
	SendTimeOffsetMs  uint64 `toml:"send_time_offset_ms"`
	EnableAutoOffset  bool   `toml:"enable_autooffset"`
	AutoOffsetSamples int    `toml:"autooffset_samples"`
	AutoOffsetLimitMs uint64 `toml:"autooffset_limit_ms"`
}

type fileUpdate struct {
	AutoStart bool `toml:"auto_start"`
}

type fileLyrics struct {
	Translit bool `toml:"translit"`
}

type fileHistory struct {
	Size int `toml:"size"`
}

// FileSettings is the on-disk TOML shape. It never carries the Discord
// token: that goes through credential.go instead.
type FileSettings struct {
	Source  string      `toml:"source"`
	Lastfm  fileLastfm  `toml:"lastfm"`
	View    fileView    `toml:"view"`
	Timing  fileTiming  `toml:"timing"`
	Update  fileUpdate  `toml:"update"`
	Lyrics  fileLyrics  `toml:"lyrics"`
	History fileHistory `toml:"history"`
}

// DefaultFileSettings returns the settings a fresh install should start with.
func DefaultFileSettings() FileSettings {
	return FileSettings{
		Source: SourceSpotify.String(),
		View: fileView{
			Timestamp: true,
			Label:     true,
			AutoClear: true,
			Advanced: fileAdvanced{
				Template: "[{timestamp}] [{lyrics}]",
			},
		},
		Timing: fileTiming{
			SendTimeOffsetMs:  500,
			EnableAutoOffset:  true,
			AutoOffsetSamples: 3,
			AutoOffsetLimitMs: 2000,
		},
		History: fileHistory{Size: 50},
	}
}

func settingsPath(dir string) string {
	return filepath.Join(dir, "settings.toml")
}

// LoadSettings reads settings.toml from dir (defaulting when absent or
// unparsable) and merges in the Discord token from the credential store.
func LoadSettings(dir string) (Settings, error) {
	fs := DefaultFileSettings()
	if raw, err := os.ReadFile(settingsPath(dir)); err == nil {
		var parsed FileSettings
		if err := toml.Unmarshal(raw, &parsed); err == nil {
			fs = parsed
		}
	}

	settings := fileToRuntime(fs)
	if tok, ok := LoadToken(dir); ok {
		settings.Token = tok
	}
	return settings, nil
}

// SaveSettings writes settings to settings.toml (without the token) and
// stores or clears the token in the credential store.
func SaveSettings(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	if s.Token == "" {
		_ = DeleteToken(dir)
	} else if err := StoreToken(dir, s.Token); err != nil {
		if writeErr := Write("credential store unavailable, token not persisted: " + err.Error()); writeErr != nil {
			_ = writeErr
		}
	}

	fs := runtimeToFile(s)
	raw, err := toml.Marshal(fs)
	if err != nil {
		return err
	}
	path := settingsPath(dir)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func fileToRuntime(fs FileSettings) Settings {
	source, ok := ParseSource(fs.Source)
	if !ok {
		source = SourceSpotify
	}
	return Settings{
		Source:            source,
		LastfmAPIKey:      fs.Lastfm.APIKey,
		LastfmUsername:    fs.Lastfm.Username,
		ViewTimestamp:     fs.View.Timestamp,
		ViewLabel:         fs.View.Label,
		ViewEmoji:         fs.View.Emoji,
		AutoClear:         fs.View.AutoClear,
		AdvancedEnabled:   fs.View.Advanced.Enabled,
		AdvancedEmoji:     fs.View.Advanced.Emoji,
		AdvancedTemplate:  fs.View.Advanced.Template,
		SendTimeOffsetMs:  fs.Timing.SendTimeOffsetMs,
		EnableAutoOffset:  fs.Timing.EnableAutoOffset,
		AutoOffsetSamples: fs.Timing.AutoOffsetSamples,
		AutoOffsetLimitMs: fs.Timing.AutoOffsetLimitMs,
		AutoStart:         fs.Update.AutoStart,
		Translit:          fs.Lyrics.Translit,
		HistorySize:       fs.History.Size,
	}
}

func runtimeToFile(s Settings) FileSettings {
	return FileSettings{
		Source: s.Source.String(),
		Lastfm: fileLastfm{
			APIKey:   s.LastfmAPIKey,
			Username: s.LastfmUsername,
		},
		View: fileView{
			Timestamp: s.ViewTimestamp,
			Label:     s.ViewLabel,
			Emoji:     s.ViewEmoji,
			AutoClear: s.AutoClear,
			Advanced: fileAdvanced{
				Enabled:  s.AdvancedEnabled,
				Emoji:    s.AdvancedEmoji,
				Template: s.AdvancedTemplate,
			},
		},
		Timing: fileTiming{
			SendTimeOffsetMs:  s.SendTimeOffsetMs,
			EnableAutoOffset:  s.EnableAutoOffset,
			AutoOffsetSamples: s.AutoOffsetSamples,
			AutoOffsetLimitMs: s.AutoOffsetLimitMs,
		},
		Update: fileUpdate{
			AutoStart: s.AutoStart,
		},
		Lyrics:  fileLyrics{Translit: s.Translit},
		History: fileHistory{Size: s.HistorySize},
	}
}
