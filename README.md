# mewsync

mewsync keeps your Discord custom status in sync with the song you're currently playing, line by line, using synced lyrics.

Inspired by mewsic, a similar tool. mewsync is its own Go implementation, not a fork.

## What it does

mewsync watches what you're listening to and updates your Discord custom status with the current lyric line, timed to the song's playback position. Two playback sources are supported directly, plus MPRIS as a third:

- **Spotify**, through the Spotify connection your Discord account already has linked. mewsync asks Discord for a Spotify access token and polls the Spotify player endpoint for what's playing and where.
- **Last.fm**, by following your scrobbles. This covers YouTube Music (through the WebScrobbler browser extension) or any other player that scrobbles to Last.fm. Last.fm doesn't expose a live playback position, so mewsync estimates it locally from the time the track started scrobbling.
- **MPRIS**, for Linux desktop players that expose the standard `org.mpris.MediaPlayer2` interface (most Linux music players do). mewsync talks to the session bus directly with a native D-Bus client (`github.com/godbus/dbus/v5`), no external `playerctl` binary needed.

Lyrics are fetched from LrcLib first, then NetEase Music, then QQ Music, with the result cached on disk so repeat plays don't hit the network again. A manual override also works: drop a `.lrc` file at `<config dir>/lyrics-override/<artist> - <song>.lrc` and mewsync uses it before touching the network.

Status updates go to Discord's `PATCH /users/@me/settings` endpoint, timed either with a fixed offset or an offset learned from measured Discord API round-trip latency.

## Installing

```
go install mewsync/cmd/mewsync@latest
```

or build from source:

```
go build -o mewsync ./cmd/mewsync
```

Packaging manifests for Homebrew, the AUR, and winget live under `packaging/`, pointed at the real [v0.1.1 release](https://github.com/arshnah/mewsync/releases/tag/v0.1.1) with real checksums. winget only lists an x64 build since that's the only Windows architecture the release workflow actually produces; there's no arm64 Windows binary to point at.

## Getting started

```
mewsync setup
```

walks through picking a source and entering the credentials it needs: a Discord user token for Spotify, or a Last.fm API key and username for Last.fm. MPRIS needs no credentials, just a running player that exposes the standard MPRIS D-Bus interface.

Once set up:

```
mewsync            # run the terminal dashboard
mewsync web        # run with the web panel instead, at http://127.0.0.1:8999
mewsync background # run detached, for autostart
mewsync doctor     # check your configuration for common problems
mewsync history    # show recently played tracks
```

Run `mewsync` with no arguments to see the full command list.

## Configuration

Settings live as TOML at a platform config directory: `$XDG_CONFIG_HOME/mewsync` or `~/.config/mewsync` on Linux and macOS, `%APPDATA%\mewsync` on Windows, or wherever `$MEWSYNC_CONFIG_DIR` points if it's set. The Discord token is never written to that file. It's stored in the OS credential manager (Keychain, Windows Credential Manager, or the Secret Service on Linux) instead, falling back to a private, permission-restricted file if no keyring backend is available.

`mewsync settings` walks through the common options interactively. The web panel's `/api/settings` endpoint also supports reading and writing settings from a browser.

## Reliability

If Spotify stops responding for a few poll cycles in a row and Last.fm credentials are configured, mewsync temporarily switches to polling Last.fm on its own, and switches back once Spotify recovers, all without a restart. `mewsync doctor` checks whether your config directory is writable, your settings file parses, your credentials and source-specific prerequisites are in place, and whether a stale background process is still holding a PID file.

## What's simplified

A few things are intentionally cut down rather than left half-built silently:

- NetEase Music and QQ Music lyrics fallbacks call public, undocumented endpoints that could change shape without notice. The HTTP calls and response parsing are split apart (`parseNetEaseSearchResponse`, `parseNetEaseLyricResponse`, `parseQQSearchResponse`, `parseQQLyricResponse`, `parseLrcLibResponse`) so the parsing logic is covered by unit tests against realistic response bodies, and the fallback chain (LrcLib to NetEase to QQ Music) has its own end-to-end test against stub HTTP servers. What's still untested is the live shape of the real endpoints themselves, since those aren't documented and could drift.
- `mewsync update` downloads the matching release asset, verifies it against the release's checksums file when one is published, and atomically replaces the running binary on Linux and macOS (the rename works even while the old binary is executing). On Windows it doesn't attempt an in-place swap: Windows keeps a running executable's file locked, so there's no reliable way to replace it out from under itself, and `mewsync update` there just reports the release and download link instead.
- Lyric transliteration/romanization (`Transliterate`) handles Cyrillic and Greek with per-character mapping tables, and Hangul with the standard algorithmic jamo decomposition (Unicode Hangul syllables decompose into initial/medial/final components by arithmetic, then map to Revised Romanization). Han characters (Chinese hanzi, Japanese kanji) pass through unchanged: converting those to a spoken reading needs a pronunciation dictionary (and for kanji, context to disambiguate readings between multiple valid ones), not a small deterministic algorithm, so that's out of scope here.
- The Last.fm pause heuristic tracks a short ring buffer of recent poll samples (wall-clock time paired with an estimated playback position, clamped to the track's known duration) and looks at the rate of position increase across them, flagging a pause once that rate drops well below real time for two consecutive samples. That's a real signal-based check rather than the previous single-instant guess, but it still can't fully solve the underlying problem: Last.fm's API gives no live playback position, only a track name and a "still now playing" flag, so a genuinely long or ambient track with a wrong fetched duration can still trip it, and once it fires for a given now-playing entry there's no live signal that would tell it the same track resumed, only a track change resets it.
- MPRIS talks to the session bus directly using `github.com/godbus/dbus/v5`, reading `PlaybackStatus`, `Metadata` and `Position` from `org.mpris.MediaPlayer2.Player` over `org.freedesktop.DBus.Properties`. No external `playerctl` dependency needed.
- Apple Music/MusicKit and a direct YouTube Music API aren't implemented; neither has a live now-playing API usable without either owning the platform or building on top of an unofficial, unsupported integration. The `Source` type is left easy to extend if that changes.

## Development

```
go build ./...
go vet ./...
gofmt -l .
go test ./...
go test -race ./...
```

The engine's concurrency logic (Engine, Shared, SettingsBox) is written against a small `engineRuntime` interface rather than directly against goroutines, channels and mutexes. Production uses `NewRealRuntime()`: real goroutines, real `sync.Mutex`, real channels, real wall-clock time. Tests use an adapter over [detsim](https://github.com/arshnah/detsim)'s `rt.Sched`, a deterministic scheduler, so the exact same Engine code that ships is what gets swept across seeded interleavings for deadlocks and races, not a separate copy written just for testing. `real_runtime_test.go` is a plain, detsim-free test that asserts the production runtime actually drives the poller and sender goroutines end to end, guarding against a real bug this project shipped once: wiring Engine to `rt.Sched` directly in production, where a scheduler's goroutines only run once something calls `Sched.Run`, which nothing in `main.go` did, so the poller and sender silently never executed in the first published build.

The detsim sweep also caught a second real bug before it ever shipped: `maybeSendLine` used to hold both the `Playback` and `Tracker` locks across the call that sends a status update, and the sender goroutine needs the `Tracker` lock to record latency before it can loop back and drain the next message. Under a slow `PatchStatus` response and a full send buffer, `Tick`'s send blocked while still holding `Tracker`, the sender could never finish its bookkeeping, and the whole engine deadlocked permanently. `TestSenderBackpressureDoesNotDeadlock` reproduces the exact conditions that used to trigger it on every run; the fix was collecting pending sends while locked and only sending them after the lock is released.
