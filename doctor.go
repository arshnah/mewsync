package mewsync

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func checkLine(ok bool, label string) {
	status := "FAIL"
	if ok {
		status = "OK"
	}
	fmt.Printf("[%s] %s\n", status, label)
}

// RunDoctor prints a pass/fail diagnostic for common configuration problems.
func RunDoctor() {
	dir := ConfigDir()

	info, err := os.Stat(dir)
	dirOK := err == nil && info.IsDir()
	checkLine(dirOK, fmt.Sprintf("config dir exists (%s)", dir))

	writable := false
	if dirOK {
		probe := filepath.Join(dir, ".write-test")
		if werr := os.WriteFile(probe, []byte("x"), 0o600); werr == nil {
			writable = true
			_ = os.Remove(probe)
		}
	}
	checkLine(writable, "config dir is writable")

	settings, loadErr := LoadSettings(dir)
	checkLine(loadErr == nil, "settings.toml parses")

	hasToken := settings.Token != ""
	checkLine(hasToken, "credential store has a Discord token")
	if hasToken {
		checkLine(ValidateToken(settings.Token), "Discord token is valid")
	}

	switch settings.Source {
	case SourceSpotify:
		checkLine(hasToken, "spotify source: Discord token present")
	case SourceLastfm:
		checkLine(settings.LastfmAPIKey != "" && settings.LastfmUsername != "",
			"lastfm source: api key and username configured")
		if settings.LastfmAPIKey != "" && settings.LastfmUsername != "" {
			state, _, err := lastfmFetchPlayer(settings.LastfmAPIKey, settings.LastfmUsername)
			if err == nil && state == nil {
				fmt.Println("[INFO] nothing is currently scrobbling on Last.fm." +
					" If you're listening via YouTube Music or a browser player," +
					" make sure the WebScrobbler extension (or your player's own Last.fm integration) is installed and scrobbling.")
			}
		}
	case SourceMPRIS:
		checkLine(mprisAvailable(), "mpris source: a D-Bus session and an active player are reachable")
	}

	checkLine(NewAutostart().IsEnabled() == settings.AutoStart,
		"autostart registration matches settings")

	pidPath := filepath.Join(dir, "mewsync.background.pid")
	if raw, err := os.ReadFile(pidPath); err == nil {
		checkLine(pidLooksAlive(string(raw)), "background pid file points at a live process")
	} else {
		checkLine(true, "no stale background pid file")
	}
}

// pidLooksAlive checks a PID file's process with signal 0. This is a
// unix-style liveness check; on Windows it will report false for a live
// process too, which just means the doctor check is best-effort there.
func pidLooksAlive(raw string) bool {
	pid, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
