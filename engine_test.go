package mewsync

import (
	"testing"

	"github.com/arshnah/detsim/rt"
)

func newTestEngine(s *rt.Sched, conn Connector) (*Engine, *Shared, *SettingsBox) {
	settings := NewSettingsBox(s, Settings{
		Token:            "discord-token",
		Source:           SourceSpotify,
		EnableAutoOffset: false,
		SendTimeOffsetMs: 200,
		AutoClear:        true,
	})
	shared := NewShared(s)
	e := NewEngine(s, settings, shared, conn, 64)
	return e, shared, settings
}

// TestNoDeadlockUnderConcurrentTickPollerSender sweeps seeds with tick(),
// the poller, and the sender all running concurrently, and asserts the
// scheduler never reports a deadlock. This is the direct check on mewsic's
// documented "lock order is always playback -> tracker" invariant: it's
// only a comment in the Rust source, unenforced by the compiler, so a
// change that violates it (e.g. an accidental swap in one call site) is
// exactly the kind of bug this catches deterministically instead of via a
// hang in production.
func TestNoDeadlockUnderConcurrentTickPollerSender(t *testing.T) {
	const trials = 500
	for seed := int64(1); seed <= trials; seed++ {
		s := rt.NewSched(seed)
		conn := NewFakeConnector(s)
		conn.DelayMs = 3
		conn.PlayerStates = []PlayerResult{
			{State: &PlayerState{TrackID: "song-1", Name: "A", Artist: "X", IsPlaying: true, ProgressMs: 0, DurationMs: 180000}},
		}
		conn.Lyrics = []Line{
			{TimeMs: 0, Text: "line one"},
			{TimeMs: 1000, Text: "line two"},
			{TimeMs: 2000, Text: "line three"},
		}

		e, _, _ := newTestEngine(s, conn)
		e.SpawnPoller(2)

		s.GoNamed("ticker", func() {
			for i := 0; i < 20; i++ {
				s.Sleep(1)
				e.Tick(50)
			}
			e.Shutdown()
		})

		err := s.Run()
		s.Shutdown()
		if err != nil {
			if _, ok := err.(*rt.DeadlockError); ok {
				t.Fatalf("seed %d: deadlock: %v", seed, err)
			}
			// Other errors (panics) still fail the sweep.
			t.Fatalf("seed %d: unexpected error: %v", seed, err)
		}
	}
}

// TestNeverSendsDuplicateLineTimestamp exercises the race between the
// poller applying a song change (which resets tracker.SentLines) and Tick
// deciding to send a line, across many random interleavings. The invariant
// under test: once a line's timestamp is recorded as sent for the current
// song, Tick must never emit it again for that same song.
func TestNeverSendsDuplicateLineTimestamp(t *testing.T) {
	const trials = 500
	for seed := int64(1); seed <= trials; seed++ {
		s := rt.NewSched(seed)
		conn := NewFakeConnector(s)
		conn.DelayMs = 2
		conn.PlayerStates = []PlayerResult{
			{State: &PlayerState{TrackID: "song-1", Name: "A", Artist: "X", IsPlaying: true, DurationMs: 180000}},
		}
		conn.Lyrics = []Line{
			{TimeMs: 0, Text: "line one"},
			{TimeMs: 500, Text: "line two"},
		}

		e, shared, _ := newTestEngine(s, conn)
		e.SpawnPoller(2)

		s.GoNamed("ticker", func() {
			for i := 0; i < 15; i++ {
				s.Sleep(1)
				e.Tick(100)
			}
			e.Shutdown()
		})

		if err := s.Run(); err != nil {
			s.Shutdown()
			t.Fatalf("seed %d: %v", seed, err)
		}

		shared.WithTracker(func(tr *Tracker) {
			seen := map[uint64]int{}
			for _, ts := range tr.SentLines {
				seen[ts]++
				if seen[ts] > 1 {
					t.Fatalf("seed %d: timestamp %dms sent twice: %v", seed, ts, tr.SentLines)
				}
			}
		})
		s.Shutdown()
	}
}

// TestTokenRefreshedAfterUnauthorized checks the poller's cached-token
// refresh path: after FetchPlayer reports ErrUnauthorized once, the poller
// must drop the cached Spotify token and successfully fetch a fresh one on
// the next cycle, under randomized scheduling and network delay.
func TestTokenRefreshedAfterUnauthorized(t *testing.T) {
	const trials = 300
	for seed := int64(1); seed <= trials; seed++ {
		s := rt.NewSched(seed)
		conn := NewFakeConnector(s)
		conn.DelayMs = 2
		conn.UnauthorizedAt = 0 // first FetchPlayer call is stale
		conn.PlayerStates = []PlayerResult{
			{}, // consumed by the unauthorized call
			{State: &PlayerState{TrackID: "song-1", Name: "A", Artist: "X", IsPlaying: true, DurationMs: 180000}},
		}

		e, shared, _ := newTestEngine(s, conn)
		e.SpawnPoller(2)

		s.GoNamed("watchdog", func() {
			s.Sleep(20)
			e.Shutdown()
		})

		if err := s.Run(); err != nil {
			s.Shutdown()
			t.Fatalf("seed %d: %v", seed, err)
		}

		var gotSong string
		shared.WithPlayback(func(pb *Playback) { gotSong = pb.SongID })
		if gotSong != "song-1" {
			t.Fatalf("seed %d: expected recovery after token refresh, got song %q", seed, gotSong)
		}
		s.Shutdown()
	}
}

// TestNoveltySearchAcrossEngineGoroutines maximizes distinct schedule shapes
// across tick/poller/sender instead of a flat seed count, per detsim's
// novelty-search pattern.
func TestNoveltySearchAcrossEngineGoroutines(t *testing.T) {
	trial := func(seed int64) (rt.Trace, error) {
		s := rt.NewSched(seed)
		conn := NewFakeConnector(s)
		conn.DelayMs = 3
		conn.PlayerStates = []PlayerResult{
			{State: &PlayerState{TrackID: "song-1", Name: "A", Artist: "X", IsPlaying: true, DurationMs: 180000}},
		}
		conn.Lyrics = []Line{{TimeMs: 0, Text: "line one"}, {TimeMs: 500, Text: "line two"}}

		e, _, _ := newTestEngine(s, conn)
		e.SpawnPoller(2)
		s.GoNamed("ticker", func() {
			for i := 0; i < 10; i++ {
				s.Sleep(1)
				e.Tick(100)
			}
			e.Shutdown()
		})

		err := s.Run()
		trace := s.Trace()
		s.Shutdown()
		return trace, err
	}

	result := rt.NoveltySearch(rt.NoveltySearchConfig{
		StartSeed: 1,
		MaxTrials: 1000,
		DryLimit:  40,
		PrefixLen: 10,
	}, trial)

	if result.FailedErr != nil {
		t.Fatalf("seed %d failed: %v", result.FailedSeed, result.FailedErr)
	}
	if result.DistinctTraces == 0 {
		t.Fatal("expected at least one distinct schedule shape")
	}
	t.Logf("ran %d trials, found %d distinct schedule shapes, stopped dry=%v",
		result.TrialsRun, result.DistinctTraces, result.StoppedDry)
}
