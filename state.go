package mewsync

import "github.com/arshnah/detsim/rt"

// Line is one synced lyric line.
type Line struct {
	TimeMs uint64
	Text   string
}

// Source mirrors mewsic's playback source setting.
type Source int

const (
	SourceSpotify Source = iota
	SourceLastfm
)

// Settings is the subset of mewsic's config that engine logic reads.
type Settings struct {
	Token            string
	Source           Source
	EnableAutoOffset bool
	SendTimeOffsetMs uint64
	AutoClear        bool
	// AutoOffsetLimitMs caps the learned auto-offset (mirrors `timing.autooffset`).
	AutoOffsetLimitMs uint64
}

// SettingsBox is a scheduler-bound RWMutex-guarded Settings, standing in for
// mewsic's `RwLock<Settings>`.
type SettingsBox struct {
	mu *rt.RWMutex
	v  Settings
}

func NewSettingsBox(s *rt.Sched, v Settings) *SettingsBox {
	return &SettingsBox{mu: rt.NewRWMutex(s), v: v}
}

func (b *SettingsBox) Get() Settings {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.v
}

func (b *SettingsBox) Set(v Settings) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.v = v
}

// Playback mirrors mewsic's `Playback` shared struct.
type Playback struct {
	SongID       string
	OldSongID    string
	SongName     string
	SongAuthor   string
	IsPlaying    bool
	SongProgress uint64
	SongDuration uint64
	HasLyrics    bool
	Lyrics       []Line
	CurrentLine  *Line
}

func (p *Playback) Ended() bool {
	return p.SongDuration > 0 && p.SongProgress >= p.SongDuration
}

// Tracker mirrors mewsic's latency/sent-line tracker.
type Tracker struct {
	LastSeenSong string
	SentLines    []uint64
	LastLatency  uint64
	latencySum   uint64
	latencyCount uint64
	LastfmLag    *uint64
	hasLastfmLag bool
}

func (t *Tracker) AddLatency(ms, limit uint64) {
	if ms > limit {
		ms = limit
	}
	t.LastLatency = ms
	t.latencySum += ms
	t.latencyCount++
}

func (t *Tracker) AvgLatency() uint64 {
	if t.latencyCount == 0 {
		return 0
	}
	return t.latencySum / t.latencyCount
}

func (t *Tracker) SentContains(timeMs uint64) bool {
	for _, v := range t.SentLines {
		if v == timeMs {
			return true
		}
	}
	return false
}

// Shared bundles every piece of state the tick/poller/sender goroutines touch
// concurrently. Lock order is always Playback -> Tracker, matching mewsic's
// documented invariant in src/engine.rs.
type Shared struct {
	Playback    *rt.Mutex
	playback    Playback
	Tracker     *rt.Mutex
	tracker     Tracker
	LyricSource *rt.Mutex
	lyricSource string
}

func NewShared(s *rt.Sched) *Shared {
	return &Shared{
		Playback:    rt.NewMutex(s),
		Tracker:     rt.NewMutex(s),
		LyricSource: rt.NewMutex(s),
	}
}

// WithPlayback runs fn with the playback lock held.
func (sh *Shared) WithPlayback(fn func(*Playback)) {
	sh.Playback.Lock()
	defer sh.Playback.Unlock()
	fn(&sh.playback)
}

// WithTracker runs fn with the tracker lock held.
func (sh *Shared) WithTracker(fn func(*Tracker)) {
	sh.Tracker.Lock()
	defer sh.Tracker.Unlock()
	fn(&sh.tracker)
}

// WithBoth acquires Playback then Tracker, in that order, and runs fn.
func (sh *Shared) WithBoth(fn func(*Playback, *Tracker)) {
	sh.Playback.Lock()
	defer sh.Playback.Unlock()
	sh.Tracker.Lock()
	defer sh.Tracker.Unlock()
	fn(&sh.playback, &sh.tracker)
}

func (sh *Shared) SetLyricSource(v string) {
	sh.LyricSource.Lock()
	defer sh.LyricSource.Unlock()
	sh.lyricSource = v
}

func (sh *Shared) GetLyricSource() string {
	sh.LyricSource.Lock()
	defer sh.LyricSource.Unlock()
	return sh.lyricSource
}
