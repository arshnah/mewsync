package mewsync

import (
	"sync"
	"testing"
	"time"
)

type countingConnector struct {
	mu          sync.Mutex
	playerCalls int
	patchCalls  int
}

func (c *countingConnector) FetchSpotifyToken(discordToken string) (string, error) {
	return "tok", nil
}

func (c *countingConnector) FetchPlayer(spotifyToken string) (*PlayerState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.playerCalls++
	return &PlayerState{TrackID: "song-1", Name: "One", Artist: "X", IsPlaying: true, DurationMs: 180000}, nil
}

func (c *countingConnector) PatchStatus(discordToken, text, emoji string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.patchCalls++
	return nil
}

func (c *countingConnector) FetchLyrics(name, artist string) []Line {
	return []Line{{TimeMs: 0, Text: "line"}}
}

func (c *countingConnector) snapshot() (player, patch int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.playerCalls, c.patchCalls
}

// TestRealRuntimeActuallyRunsPollerAndSender guards against the bug where
// Engine was wired to rt.Sched in production: goroutines spawned through
// Sched.Go only execute once something calls Sched.Run, which main.go never
// did, so the poller and sender silently never ran a single line in a real
// build despite compiling and appearing to serve requests. This test uses
// NewRealRuntime with no detsim involved and asserts the poller and sender
// both do real work within a real wall-clock deadline.
func TestRealRuntimeActuallyRunsPollerAndSender(t *testing.T) {
	rt := NewRealRuntime()
	conn := &countingConnector{}
	settings := NewSettingsBox(rt, Settings{
		Token:            "discord-token",
		Source:           SourceSpotify,
		SendTimeOffsetMs: 0,
		AutoClear:        true,
	})
	shared := NewShared(rt)
	e := NewEngine(rt, settings, shared, conn, 16)
	e.SpawnPoller(50 * time.Millisecond)
	defer e.Shutdown()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		e.Tick(16)
		if player, _ := conn.snapshot(); player > 0 {
			break
		}
		time.Sleep(16 * time.Millisecond)
	}

	player, _ := conn.snapshot()
	if player == 0 {
		t.Fatal("poller never called FetchPlayer within the deadline, the real runtime isn't actually driving Engine goroutines")
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		e.Tick(16)
		if _, patch := conn.snapshot(); patch > 0 {
			return
		}
		time.Sleep(16 * time.Millisecond)
	}

	t.Fatal("sender never called PatchStatus within the deadline, the real runtime isn't actually driving Engine goroutines")
}
