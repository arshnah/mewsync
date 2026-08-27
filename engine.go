package mewsync

import "time"

// DefaultLastfmLagMs mirrors mewsic's DEFAULT_LASTFM_LAG_MS fallback.
const DefaultLastfmLagMs = 3000

type statusMsgKind int

const (
	statusUpdate statusMsgKind = iota
	statusClear
)

type statusMsg struct {
	kind  statusMsgKind
	text  string
	emoji string
}

// Engine is the Go port of mewsic's src/engine.rs Engine: a sender goroutine,
// a poller goroutine and a tick() driven from the caller, all coordinating
// through Shared under the scheduler's deterministic clock.
type Engine struct {
	rt        engineRuntime
	settings  *SettingsBox
	shared    *Shared
	conn      Connector
	sendCh    statusChan
	quit      locker
	quitFlag  bool
	SentCount int // observability hook for tests: successful PatchStatus calls
}

// NewEngine spawns the Discord-status sender goroutine, matching mewsic's
// Engine::new. sendBufCap approximates Rust's unbounded mpsc::channel; pick
// something generous enough that legitimate traffic never blocks on it.
func NewEngine(rt engineRuntime, settings *SettingsBox, shared *Shared, conn Connector, sendBufCap int) *Engine {
	e := &Engine{
		rt:       rt,
		settings: settings,
		shared:   shared,
		conn:     conn,
		sendCh:   rt.NewChan(sendBufCap),
		quit:     rt.NewMutex(),
	}

	rt.GoNamed("sender", func() {
		for {
			msg, ok := e.sendCh.RecvOK()
			if !ok {
				return
			}
			token := e.settings.Get().Token
			if token == "" {
				continue
			}
			switch msg.kind {
			case statusUpdate:
				sentAt := e.rt.Now()
				if err := e.conn.PatchStatus(token, msg.text, msg.emoji); err == nil {
					ms := uint64(e.rt.Now().Sub(sentAt).Milliseconds())
					limit := e.settings.Get().AutoOffsetLimitMs
					if limit == 0 {
						limit = 1
					}
					e.shared.WithTracker(func(t *Tracker) {
						t.AddLatency(ms, limit)
					})
					e.SentCount++
				}
			case statusClear:
				_ = e.conn.PatchStatus(token, "", "")
			}
		}
	})

	return e
}

// Tick mirrors Engine::tick: advance progress, clear sent-lines on song end,
// then decide whether a new lyric line must be pushed. Lock order is always
// Playback -> Tracker, enforced by Shared.WithBoth.
func (e *Engine) Tick(deltaMs uint64) {
	e.shared.WithBoth(func(pb *Playback, tr *Tracker) {
		if pb.IsPlaying {
			pb.SongProgress += deltaMs
		}
		if pb.Ended() {
			tr.SentLines = nil
		}
	})
	e.maybeSendLine()
}

// maybeSendLine never sends on sendCh while holding Playback or Tracker.
// The sender goroutine needs Tracker to record latency before it loops back
// to receive the next message, so a send made under the lock can deadlock
// the moment the channel fills up under a slow PatchStatus.
func (e *Engine) maybeSendLine() {
	settings := e.settings.Get()
	var pending []statusMsg

	e.shared.WithBoth(func(pb *Playback, tr *Tracker) {
		if pb.SongID != "" && pb.SongID != tr.LastSeenSong {
			tr.LastSeenSong = pb.SongID
			tr.SentLines = nil
			if settings.AutoClear {
				pending = append(pending, statusMsg{kind: statusClear})
			}
		}

		if !pb.IsPlaying || !pb.HasLyrics || pb.Ended() {
			return
		}

		var allowance uint64
		if settings.EnableAutoOffset {
			allowance = tr.AvgLatency() + 100
		} else {
			allowance = settings.SendTimeOffsetMs
		}

		offset := allowance
		if settings.Source == SourceLastfm {
			lag := DefaultLastfmLagMs
			if tr.hasLastfmLag && tr.LastfmLag != nil {
				lag = int(*tr.LastfmLag)
			}
			offset = uint64(lag) + allowance
		}
		threshold := pb.SongProgress + offset

		targetIdx := -1
		for i, line := range pb.Lyrics {
			if line.TimeMs >= threshold {
				continue
			}
			if line.Text == "" {
				continue
			}
			if i+1 < len(pb.Lyrics) && pb.Lyrics[i+1].TimeMs < threshold {
				continue
			}
			if tr.SentContains(line.TimeMs) {
				continue
			}
			if pb.CurrentLine != nil && *pb.CurrentLine == line {
				continue
			}
			targetIdx = i
			break
		}

		if targetIdx >= 0 {
			line := pb.Lyrics[targetIdx]
			pb.CurrentLine = &line
			tr.SentLines = append(tr.SentLines, line.TimeMs)
			text, emoji := BuildStatus(settings, *pb, line)
			pending = append(pending, statusMsg{kind: statusUpdate, text: text, emoji: emoji})
		}
	})

	for _, msg := range pending {
		e.sendCh.Send(msg)
	}
}

// SpawnPoller mirrors Engine::spawn_poller: a background goroutine polling
// every pollEveryMs, applying fetched state and refreshing lyrics on song
// change. All I/O goes through Connector so tests can inject faults.
func (e *Engine) SpawnPoller(pollEvery time.Duration) {
	e.rt.GoNamed("poller", func() {
		var spotifyToken string
		haveToken := false

		for {
			e.quit.Lock()
			done := e.quitFlag
			e.quit.Unlock()
			if done {
				return
			}

			e.rt.Sleep(pollEvery)

			settings := e.settings.Get()
			switch settings.Source {
			case SourceSpotify:
				if settings.Token == "" {
					continue
				}
				if !haveToken {
					tok, err := e.conn.FetchSpotifyToken(settings.Token)
					if err != nil {
						continue
					}
					spotifyToken = tok
					haveToken = true
				}
				state, err := e.conn.FetchPlayer(spotifyToken)
				if err == ErrUnauthorized {
					haveToken = false
					continue
				}
				if err != nil {
					continue
				}
				if state == nil {
					e.shared.WithPlayback(func(pb *Playback) { pb.IsPlaying = false })
					continue
				}
				songChanged := e.applyState(state, &state.ProgressMs)
				e.syncLyrics(songChanged)
			case SourceLastfm:
				// Last.fm has no playback position; progress is left to Tick.
				state, err := e.conn.FetchPlayer("")
				if err != nil {
					continue
				}
				if state == nil {
					e.shared.WithPlayback(func(pb *Playback) { pb.IsPlaying = false })
					continue
				}
				songChanged := e.applyState(state, nil)
				e.syncLyrics(songChanged)
			case SourceMPRIS:
				state, err := e.conn.FetchPlayer("")
				if err != nil {
					continue
				}
				if state == nil {
					e.shared.WithPlayback(func(pb *Playback) { pb.IsPlaying = false })
					continue
				}
				songChanged := e.applyState(state, &state.ProgressMs)
				e.syncLyrics(songChanged)
			}
		}
	})
}

func (e *Engine) applyState(state *PlayerState, progressMs *uint64) bool {
	var songChanged bool
	e.shared.WithPlayback(func(pb *Playback) {
		songChanged = pb.SongID != state.TrackID
		pb.IsPlaying = state.IsPlaying
		pb.SongDuration = state.DurationMs
		if progressMs != nil {
			pb.SongProgress = *progressMs
		}
		if songChanged {
			pb.OldSongID = pb.SongID
			pb.SongID = state.TrackID
			pb.SongName = state.Name
			pb.SongAuthor = state.Artist
			pb.Lyrics = nil
			pb.CurrentLine = nil
			pb.HasLyrics = false
			if progressMs == nil {
				pb.SongProgress = 0
			}
		}
	})
	return songChanged
}

func (e *Engine) syncLyrics(songChanged bool) {
	var hasLyrics bool
	e.shared.WithPlayback(func(pb *Playback) { hasLyrics = pb.HasLyrics })
	if !songChanged && hasLyrics {
		return
	}

	var name, artist string
	e.shared.WithPlayback(func(pb *Playback) {
		name, artist = pb.SongName, pb.SongAuthor
	})
	if name == "" {
		return
	}

	lines := e.conn.FetchLyrics(name, artist)
	e.shared.WithPlayback(func(pb *Playback) {
		if lines != nil {
			pb.Lyrics = lines
			pb.HasLyrics = true
			pb.CurrentLine = nil
		} else {
			pb.HasLyrics = false
		}
	})
	if lines != nil {
		e.shared.SetLyricSource("fake")
	} else {
		e.shared.SetLyricSource("none")
	}
}

// Shutdown mirrors Engine::shutdown: signals the poller to stop and closes
// the send channel so the sender goroutine exits too.
func (e *Engine) Shutdown() {
	e.quit.Lock()
	e.quitFlag = true
	e.quit.Unlock()
	e.sendCh.Close()
}
