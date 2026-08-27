package mewsync

import (
	"encoding/json"
	"testing"

	"github.com/arshnah/detsim"
	"github.com/arshnah/detsim/rt"
)

func TestDeadlockSweepCatchesLockOrderViolation(t *testing.T) {
	found := false
	for seed := int64(1); seed <= 200 && !found; seed++ {
		s := rt.NewSched(seed)
		shared := NewShared(s)

		s.GoNamed("normal-order", func() {
			shared.Playback.Lock()
			s.Sleep(1)
			shared.Tracker.Lock()
			shared.Tracker.Unlock()
			shared.Playback.Unlock()
		})
		s.GoNamed("reversed-order", func() {
			shared.Tracker.Lock()
			s.Sleep(1)
			shared.Playback.Lock()
			shared.Playback.Unlock()
			shared.Tracker.Unlock()
		})

		err := s.Run()
		s.Shutdown()
		if _, ok := err.(*rt.DeadlockError); ok {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the sweep to catch a genuine lock-order violation as a deadlock")
	}
}

func TestLyricsFetchDoesNotLeakAcrossSongChange(t *testing.T) {
	for seed := int64(1); seed <= 50; seed++ {
		s := rt.NewSched(seed)
		conn := NewFakeConnector(s)
		conn.DelayMs = 1
		conn.LyricsDelayMs = 20
		conn.PlayerStates = []PlayerResult{
			{State: &PlayerState{TrackID: "song-1", Name: "One", Artist: "X", IsPlaying: true, DurationMs: 180000}},
			{State: &PlayerState{TrackID: "song-2", Name: "Two", Artist: "X", IsPlaying: true, DurationMs: 180000}},
		}
		conn.LyricsByName = map[string][]Line{
			"One": {{TimeMs: 0, Text: "song one line"}},
			"Two": {{TimeMs: 0, Text: "song two line"}},
		}

		e, shared, _ := newTestEngine(s, conn)
		e.SpawnPoller(2)

		s.GoNamed("watchdog", func() {
			s.Sleep(30)
			e.Shutdown()
		})

		if err := s.Run(); err != nil {
			s.Shutdown()
			t.Fatalf("seed %d: %v", seed, err)
		}

		shared.WithPlayback(func(pb *Playback) {
			if pb.SongID == "" || !pb.HasLyrics {
				return
			}
			for _, line := range pb.Lyrics {
				wantOne := pb.SongID == "song-1" && line.Text != "song one line"
				wantTwo := pb.SongID == "song-2" && line.Text != "song two line"
				if wantOne || wantTwo {
					t.Fatalf("seed %d: song %q has mismatched lyric line %q", seed, pb.SongID, line.Text)
				}
			}
		})
		s.Shutdown()
	}
}

// TestSenderBackpressureDoesNotDeadlock guards a fixed bug. maybeSendLine
// used to call sendCh.Send while still holding Playback and Tracker. The
// sender goroutine needs Tracker to record latency before it loops back to
// receive the next message, so once the channel filled up under a slow
// PatchStatus, Tick's Send blocked while holding Tracker, the sender could
// never finish its bookkeeping, and the channel never drained. Fixed by
// collecting pending sends while locked and sending them after the lock is
// released. This test reproduces the exact conditions that used to deadlock
// every run: a small buffer, a slow PatchStatus, and rapid song changes.
func TestSenderBackpressureDoesNotDeadlock(t *testing.T) {
	for seed := int64(1); seed <= 50; seed++ {
		s := rt.NewSched(seed)
		conn := NewFakeConnector(s)
		conn.DelayMs = 1
		conn.PatchDelayMs = 20

		states := make([]PlayerResult, 0, 30)
		for i := 0; i < 30; i++ {
			id := "song"
			for j := 0; j <= i; j++ {
				id += "-x"
			}
			states = append(states, PlayerResult{
				State: &PlayerState{TrackID: id, Name: id, Artist: "X", IsPlaying: true, DurationMs: 180000},
			})
		}
		conn.PlayerStates = states
		conn.Lyrics = []Line{{TimeMs: 0, Text: "line"}}

		settings := NewSettingsBox(s, Settings{
			Token:            "discord-token",
			Source:           SourceSpotify,
			SendTimeOffsetMs: 0,
			AutoClear:        true,
		})
		shared := NewShared(s)
		e := NewEngine(s, settings, shared, conn, 4)
		e.SpawnPoller(1)

		s.GoNamed("ticker", func() {
			for i := 0; i < 40; i++ {
				s.Sleep(1)
				e.Tick(10)
			}
			e.Shutdown()
		})

		err := s.Run()
		s.Shutdown()
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

func TestLastfmLagNeverAppliesToWrongSongUnderConcurrentWrites(t *testing.T) {
	for seed := int64(1); seed <= 200; seed++ {
		s := rt.NewSched(seed)
		shared := NewShared(s)

		lagA := uint64(4000)
		lagB := uint64(1500)

		done := rt.NewWaitGroup(s)
		done.Add(2)

		s.GoNamed("song-a-lag", func() {
			defer done.Done()
			shared.WithTracker(func(t *Tracker) {
				v := lagA
				t.LastfmLag = &v
				t.hasLastfmLag = true
			})
		})
		s.GoNamed("song-b-lag", func() {
			defer done.Done()
			shared.WithTracker(func(t *Tracker) {
				v := lagB
				t.LastfmLag = &v
				t.hasLastfmLag = true
			})
		})

		var badLag *uint64
		var missing bool
		s.GoNamed("waiter", func() {
			done.Wait()
			shared.WithTracker(func(tr *Tracker) {
				if tr.LastfmLag == nil {
					missing = true
					return
				}
				if *tr.LastfmLag != lagA && *tr.LastfmLag != lagB {
					v := *tr.LastfmLag
					badLag = &v
				}
			})
		})

		if err := s.Run(); err != nil {
			s.Shutdown()
			t.Fatalf("seed %d: %v", seed, err)
		}
		s.Shutdown()
		if missing {
			t.Fatalf("seed %d: no lag recorded", seed)
		}
		if badLag != nil {
			t.Fatalf("seed %d: lag value %d is neither song's value, indicating a torn write", seed, *badLag)
		}
	}
}

func TestLyricsCacheSurvivesCrashBetweenWriteAndSync(t *testing.T) {
	storage := detsim.NewFaultyStorage(1, detsim.FaultProfile{})

	good := []byte(`{"source":"lrclib","lines":[{"TimeMs":0,"Text":"hello"}]}`)
	if _, err := storage.WriteAt(good, 0); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := storage.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	update := []byte(`{"source":"lrclib","lines":[{"TimeMs":0,"Text":"updated line, much longer than before"}]}`)
	if _, err := storage.WriteAt(update, 0); err != nil {
		t.Fatalf("write: %v", err)
	}
	storage.Crash()

	buf := make([]byte, storage.Size())
	n, err := storage.ReadAt(buf, 0)
	if err != nil && n == 0 {
		t.Fatalf("readat: %v", err)
	}

	var cached cachedLyrics
	parseErr := json.Unmarshal(buf[:n], &cached)

	if parseErr == nil && cached.Source == "lrclib" && len(cached.Lines) > 0 && cached.Lines[0].Text == "hello" {
		return
	}
	if parseErr != nil {
		t.Logf("finding: a crash between write and sync is caught, JSON parsing rejects the unsynced write (%v)", parseErr)
		return
	}
	t.Fatalf("finding: crash between write and sync left readable but wrong data: %+v", cached)
}
