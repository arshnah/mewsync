package mewsync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const defaultHistorySize = 50

// HistoryEntry is one played track, kept in the on-disk ring buffer.
type HistoryEntry struct {
	Name      string    `json:"name"`
	Artist    string    `json:"artist"`
	Timestamp time.Time `json:"timestamp"`
}

var historyMu sync.Mutex

func historyPath(configDir string) string {
	return filepath.Join(configDir, "history.json")
}

func readHistory(configDir string) []HistoryEntry {
	raw, err := os.ReadFile(historyPath(configDir))
	if err != nil {
		return nil
	}
	var entries []HistoryEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil
	}
	return entries
}

// AppendHistory records a played track, trimming the ring buffer to the
// configured (or default) size.
func AppendHistory(configDir, name, artist string) {
	if name == "" {
		return
	}
	historyMu.Lock()
	defer historyMu.Unlock()

	entries := readHistory(configDir)
	entries = append(entries, HistoryEntry{Name: name, Artist: artist, Timestamp: time.Now()})

	limit := defaultHistorySize
	if s, err := LoadSettings(configDir); err == nil && s.HistorySize > 0 {
		limit = s.HistorySize
	}
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return
	}
	_ = os.WriteFile(historyPath(configDir), raw, 0o600)
}

// GetHistory returns the recorded play history, oldest first.
func GetHistory(configDir string) []HistoryEntry {
	historyMu.Lock()
	defer historyMu.Unlock()
	return readHistory(configDir)
}

// PrintHistory prints the recorded play history to stdout, most recent last.
func PrintHistory() {
	entries := GetHistory(ConfigDir())
	if len(entries) == 0 {
		fmt.Println("no history yet")
		return
	}
	for _, e := range entries {
		fmt.Printf("%s  %s - %s\n", e.Timestamp.Local().Format("2006-01-02 15:04"), e.Name, e.Artist)
	}
}
