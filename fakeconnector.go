package mewsync

import "github.com/arshnah/detsim/rt"

// FakeConnector is a scriptable Connector for detsim scenarios: every call
// can be delayed (via sched.Sleep, so it participates in the deterministic
// clock) and can be forced to fail, all driven by the scheduler's own seeded
// Rand so a fault pattern is reproducible from the seed alone.
type FakeConnector struct {
	sched *rt.Sched

	// Script, consumed in order per call kind. When empty, calls succeed with
	// sensible defaults.
	PlayerStates    []PlayerResult
	TokenFailOnce   bool
	tokenFailedOnce bool
	UnauthorizedAt  int // FetchPlayer call index (0-based) that returns ErrUnauthorized
	playerCalls     int
	Lyrics          []Line

	// LyricsByName, when set, returns lyrics keyed by track name instead of
	// the single Lyrics fallback, so tests can give different songs
	// different lyric sets.
	LyricsByName map[string][]Line

	// DelayMs, when > 0, is applied before every network call returns.
	DelayMs rt.VirtualTime

	// LyricsDelayMs, when > 0, overrides DelayMs for FetchLyrics only.
	LyricsDelayMs rt.VirtualTime

	// PatchDelayMs, when > 0, overrides DelayMs for PatchStatus only.
	PatchDelayMs rt.VirtualTime
}

type PlayerResult struct {
	State *PlayerState
	Err   error
}

func NewFakeConnector(s *rt.Sched) *FakeConnector {
	return &FakeConnector{sched: s, UnauthorizedAt: -1}
}

func (f *FakeConnector) delay() {
	if f.DelayMs > 0 {
		f.sched.Sleep(f.DelayMs)
	}
}

func (f *FakeConnector) FetchSpotifyToken(discordToken string) (string, error) {
	f.delay()
	if f.TokenFailOnce && !f.tokenFailedOnce {
		f.tokenFailedOnce = true
		return "", ErrUnauthorized
	}
	return "fake-spotify-token", nil
}

func (f *FakeConnector) FetchPlayer(spotifyToken string) (*PlayerState, error) {
	f.delay()
	idx := f.playerCalls
	f.playerCalls++

	if f.UnauthorizedAt >= 0 && idx == f.UnauthorizedAt {
		return nil, ErrUnauthorized
	}
	if idx < len(f.PlayerStates) {
		r := f.PlayerStates[idx]
		return r.State, r.Err
	}
	if len(f.PlayerStates) > 0 {
		r := f.PlayerStates[len(f.PlayerStates)-1]
		return r.State, r.Err
	}
	return nil, nil
}

func (f *FakeConnector) PatchStatus(discordToken, text, emoji string) error {
	if f.PatchDelayMs > 0 {
		f.sched.Sleep(f.PatchDelayMs)
	} else {
		f.delay()
	}
	return nil
}

func (f *FakeConnector) FetchLyrics(name, artist string) []Line {
	if f.LyricsDelayMs > 0 {
		f.sched.Sleep(f.LyricsDelayMs)
	} else {
		f.delay()
	}
	if f.LyricsByName != nil {
		return f.LyricsByName[name]
	}
	return f.Lyrics
}
