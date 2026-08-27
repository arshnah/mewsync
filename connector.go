package mewsync

import "errors"

// ErrUnauthorized mirrors mewsic's FetchError::Unauthorized (expired Spotify token).
var ErrUnauthorized = errors.New("unauthorized")

// PlayerState mirrors mewsic's connector.PlayerState.
type PlayerState struct {
	TrackID    string
	Name       string
	Artist     string
	IsPlaying  bool
	ProgressMs uint64
	DurationMs uint64
}

// Connector is the network boundary the poller drives. In production this
// hits Discord/Spotify/Last.fm; in tests it's a FakeConnector wired to a
// detsim rt.Sched so latency, errors and races are seeded and reproducible.
type Connector interface {
	// FetchSpotifyToken exchanges a Discord token for a Spotify access token.
	FetchSpotifyToken(discordToken string) (string, error)
	// FetchPlayer polls current playback. A nil state with nil error means
	// "nothing is currently playing".
	FetchPlayer(spotifyToken string) (*PlayerState, error)
	// PatchStatus pushes a Discord status update. Empty text/emoji clears it.
	PatchStatus(discordToken, text, emoji string) error
	// FetchLyrics returns synced lines for a track, or nil if none were found.
	FetchLyrics(name, artist string) []Line
}
