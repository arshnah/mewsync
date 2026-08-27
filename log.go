package mewsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxLogSizeBytes = 5 * 1024 * 1024

var (
	logMu   sync.Mutex
	logPath string
)

// SetLogDir points the package logger at dir/mewsic.log for the rest of the
// process's life.
func SetLogDir(dir string) {
	logMu.Lock()
	defer logMu.Unlock()
	logPath = filepath.Join(dir, "mewsic.log")
}

// Write appends a timestamped line to the log file, truncating it first if
// it has grown past a few MB. It is a no-op until SetLogDir has been called.
func Write(msg string) error {
	logMu.Lock()
	defer logMu.Unlock()
	if logPath == "" {
		return nil
	}

	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogSizeBytes {
		_ = os.Remove(logPath)
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	line := fmt.Sprintf("%s %s\n", time.Now().UTC().Format(time.RFC3339), msg)
	_, err = f.WriteString(line)
	return err
}
