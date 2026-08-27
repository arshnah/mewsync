package mewsync

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	discordAPI = "https://discord.com/api/v10"
	spotifyAPI = "https://api.spotify.com/v1"
	userAgent  = "mewsync/1.0"
)

var httpClient = &http.Client{Timeout: 12 * time.Second}

// RealConnector implements Connector against the live Discord, Spotify,
// Last.fm and lyrics APIs.
type RealConnector struct {
	settings  *SettingsBox
	configDir string
	shared    *Shared

	fallbackMu        sync.Mutex
	spotifyFailStreak int
	autoFallback      bool
	lastfmProbeCount  int

	historyMu        sync.Mutex
	lastHistoryTrack string

	pauseMu        sync.Mutex
	pauseTrackID   string
	pauseFirstSeen time.Time
	pauseDetector  pauseDetector
}

// NewRealConnector builds a RealConnector. settings is consulted at call
// time to decide Spotify vs Last.fm vs MPRIS and to read Last.fm credentials.
func NewRealConnector(settings *SettingsBox, configDir string) *RealConnector {
	return &RealConnector{settings: settings, configDir: configDir}
}

// SetShared lets the connector record the estimated Last.fm playback lag
// onto the engine's tracker. Optional: without it, Last.fm falls back to
// DefaultLastfmLagMs.
func (c *RealConnector) SetShared(sh *Shared) {
	c.shared = sh
}

func doRequest(method, rawURL string, headers map[string]string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return httpClient.Do(req)
}

func (c *RealConnector) FetchSpotifyToken(discordToken string) (string, error) {
	token, err := fetchSpotifyToken(discordToken)
	if err != nil {
		c.recordSpotifyFailure()
	} else {
		c.recordSpotifySuccess()
	}
	return token, err
}

func fetchSpotifyToken(discordToken string) (string, error) {
	resp, err := doRequest("GET", discordAPI+"/users/@me/connections", map[string]string{
		"Authorization": discordToken,
	}, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discord connections responded %d", resp.StatusCode)
	}

	var connections []struct {
		Type        string `json:"type"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&connections); err != nil {
		return "", err
	}
	for _, conn := range connections {
		if conn.Type == "spotify" && conn.AccessToken != "" {
			return conn.AccessToken, nil
		}
	}
	return "", fmt.Errorf("no Spotify account connected to Discord")
}

// recordSpotifyFailure counts a failed Spotify token/player fetch. After
// three in a row, if Last.fm credentials are on hand, it flips Settings.Source
// to Last.fm so the poller keeps working without a restart. This is a
// heuristic: it can't tell a truly dead Spotify connection from a blip, so
// it just reacts to a short streak of failures.
func (c *RealConnector) recordSpotifyFailure() {
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()

	c.spotifyFailStreak++
	if c.spotifyFailStreak < 3 || c.autoFallback {
		return
	}
	settings := c.settings.Get()
	if settings.Source != SourceSpotify || settings.LastfmAPIKey == "" || settings.LastfmUsername == "" {
		return
	}
	settings.Source = SourceLastfm
	c.settings.Set(settings)
	c.autoFallback = true
	_ = Write("spotify unreachable after 3 poll cycles, falling back to last.fm")
}

func (c *RealConnector) recordSpotifySuccess() {
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()
	c.spotifyFailStreak = 0
	if !c.autoFallback {
		return
	}
	settings := c.settings.Get()
	settings.Source = SourceSpotify
	c.settings.Set(settings)
	c.autoFallback = false
	_ = Write("spotify recovered, switching back from last.fm fallback")
}

func (c *RealConnector) inAutoFallback() bool {
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()
	return c.autoFallback
}

func (c *RealConnector) FetchPlayer(spotifyToken string) (*PlayerState, error) {
	settings := c.settings.Get()

	var state *PlayerState
	var err error
	switch settings.Source {
	case SourceLastfm:
		state, err = c.fetchLastfmPlayer(settings)
		if c.inAutoFallback() {
			c.probeSpotifyRecovery(settings)
		}
	case SourceMPRIS:
		state, err = fetchMPRISPlayer()
	default:
		state, err = fetchSpotifyPlayer(spotifyToken)
		if err != nil {
			c.recordSpotifyFailure()
		} else {
			c.recordSpotifySuccess()
		}
	}

	if err == nil && state != nil {
		c.recordHistory(state)
	}
	return state, err
}

// probeSpotifyRecovery is called occasionally while polling Last.fm as an
// automatic fallback, to notice when Spotify comes back. It only runs every
// few polls so the fallback path isn't paying for an extra HTTP round trip
// on every single cycle.
func (c *RealConnector) probeSpotifyRecovery(settings Settings) {
	c.fallbackMu.Lock()
	c.lastfmProbeCount++
	shouldProbe := c.lastfmProbeCount%5 == 0
	c.fallbackMu.Unlock()
	if !shouldProbe || settings.Token == "" {
		return
	}
	if token, err := fetchSpotifyToken(settings.Token); err == nil {
		if _, err := fetchSpotifyPlayer(token); err == nil {
			c.recordSpotifySuccess()
		}
	}
}

func fetchSpotifyPlayer(spotifyToken string) (*PlayerState, error) {
	resp, err := doRequest("GET", spotifyAPI+"/me/player", map[string]string{
		"Authorization": "Bearer " + spotifyToken,
	}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify player responded %d", resp.StatusCode)
	}

	var payload struct {
		IsPlaying  bool   `json:"is_playing"`
		ProgressMs uint64 `json:"progress_ms"`
		Item       *struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			DurationMs uint64 `json:"duration_ms"`
			Artists    []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Item == nil {
		return nil, nil
	}

	artist := ""
	if len(payload.Item.Artists) > 0 {
		artist = payload.Item.Artists[0].Name
	}

	return &PlayerState{
		TrackID:    payload.Item.ID,
		Name:       payload.Item.Name,
		Artist:     artist,
		IsPlaying:  payload.IsPlaying,
		ProgressMs: payload.ProgressMs,
		DurationMs: payload.Item.DurationMs,
	}, nil
}

func (c *RealConnector) fetchLastfmPlayer(settings Settings) (*PlayerState, error) {
	if settings.LastfmAPIKey == "" || settings.LastfmUsername == "" {
		return nil, fmt.Errorf("last.fm api key or username not configured")
	}
	state, prevUts, err := lastfmFetchPlayer(settings.LastfmAPIKey, settings.LastfmUsername)
	if err != nil {
		return nil, err
	}
	if state == nil {
		c.pauseMu.Lock()
		c.pauseTrackID = ""
		c.pauseMu.Unlock()
		return nil, nil
	}
	if prevUts != nil && c.shared != nil {
		nowSecs := uint64(time.Now().Unix())
		if lag, ok := lastfmMeasureLag(nowSecs, *prevUts); ok {
			c.shared.WithTracker(func(t *Tracker) {
				t.LastfmLag = &lag
				t.hasLastfmLag = true
			})
		}
	}
	c.applyLastfmPauseHeuristic(state)
	return state, nil
}

// applyLastfmPauseHeuristic flags a probable pause using pauseDetector: it
// tracks, since the track was first seen, an estimated position (real
// elapsed time, clamped to the track's known duration) and feeds it to the
// detector's rate-based ring buffer. Once the estimate hits the duration
// ceiling it stops advancing, which the detector reads as playback stalling
// (rate drops to 0) and, after two consecutive polls confirm it, flags as
// paused.
//
// This still can't distinguish a real pause from a track whose fetched
// duration is wrong (a live version, an extended mix), because Last.fm's API
// gives no live playback position to check against, only a track name and a
// "still now playing" flag. And because the estimate is clamped rather than
// frozen by an external "actually resumed" signal, once this fires for a
// given now-playing entry it can't un-fire until Last.fm reports a different
// track: there's nothing in the API that would tell us the same track
// genuinely resumed. It's a real signal-based heuristic, not a guess, but
// it's still a heuristic.
func (c *RealConnector) applyLastfmPauseHeuristic(state *PlayerState) {
	c.pauseMu.Lock()
	defer c.pauseMu.Unlock()

	if state.TrackID != c.pauseTrackID {
		c.pauseTrackID = state.TrackID
		c.pauseFirstSeen = time.Now()
		c.pauseDetector.reset()
		return
	}
	if state.DurationMs == 0 {
		return
	}

	elapsedMs := uint64(time.Since(c.pauseFirstSeen).Milliseconds())
	if elapsedMs > state.DurationMs {
		elapsedMs = state.DurationMs
	}
	wallMs := uint64(time.Now().UnixMilli())
	if c.pauseDetector.Sample(wallMs, elapsedMs) {
		state.IsPlaying = false
	}
}

func (c *RealConnector) recordHistory(state *PlayerState) {
	if state.TrackID == "" {
		return
	}
	c.historyMu.Lock()
	changed := state.TrackID != c.lastHistoryTrack
	c.lastHistoryTrack = state.TrackID
	c.historyMu.Unlock()
	if !changed {
		return
	}
	AppendHistory(c.configDir, state.Name, state.Artist)
}

func (c *RealConnector) PatchStatus(discordToken, text, emoji string) error {
	var expiresAt any
	if text != "" {
		expiresAt = time.Now().UTC().Add(60 * time.Second).Format(time.RFC3339)
	}
	var emojiName any
	if emoji != "" {
		emojiName = emoji
	}

	body, err := json.Marshal(map[string]any{
		"custom_status": map[string]any{
			"text":       text,
			"emoji_id":   nil,
			"emoji_name": emojiName,
			"expires_at": expiresAt,
		},
	})
	if err != nil {
		return err
	}

	resp, err := doRequest("PATCH", discordAPI+"/users/@me/settings", map[string]string{
		"Authorization": discordToken,
		"Content-Type":  "application/json",
	}, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord settings patch responded %d", resp.StatusCode)
	}
	return nil
}

// ValidateToken checks a Discord token against /users/@me.
func ValidateToken(token string) bool {
	resp, err := doRequest("GET", discordAPI+"/users/@me", map[string]string{
		"Authorization": token,
	}, nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *RealConnector) FetchLyrics(name, artist string) []Line {
	if lines, ok := readLyricsOverride(c.configDir, name, artist); ok {
		return applyTranslitIfEnabled(c.settings.Get(), lines)
	}

	if cached, ok := readLyricsCache(c.configDir, name, artist); ok {
		return applyTranslitIfEnabled(c.settings.Get(), cached)
	}

	if lines := fetchLrcLib(name, artist); lines != nil {
		writeLyricsCache(c.configDir, name, artist, "lrclib", lines)
		return applyTranslitIfEnabled(c.settings.Get(), lines)
	}
	if lines := fetchNetEase(name, artist); lines != nil {
		writeLyricsCache(c.configDir, name, artist, "netease", lines)
		return applyTranslitIfEnabled(c.settings.Get(), lines)
	}
	if lines := fetchQQMusic(name, artist); lines != nil {
		writeLyricsCache(c.configDir, name, artist, "qqmusic", lines)
		return applyTranslitIfEnabled(c.settings.Get(), lines)
	}
	return nil
}

func readLyricsOverride(configDir, name, artist string) ([]Line, bool) {
	path := filepath.Join(configDir, "lyrics-override", fmt.Sprintf("%s - %s.lrc", artist, name))
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	lines := parseLRC(string(raw))
	if len(lines) == 0 {
		return nil, false
	}
	return lines, true
}

func applyTranslitIfEnabled(settings Settings, lines []Line) []Line {
	if !settings.Translit || len(lines) == 0 {
		return lines
	}
	out := make([]Line, len(lines))
	for i, l := range lines {
		out[i] = Line{TimeMs: l.TimeMs, Text: Transliterate(l.Text)}
	}
	return out
}

func lyricsCacheDir(configDir string) string {
	return filepath.Join(configDir, "lyrics-cache")
}

func lyricsCacheKey(name, artist string) string {
	h := sha1.Sum([]byte(name + "\x00" + artist))
	return hex.EncodeToString(h[:])
}

type cachedLyrics struct {
	Source string `json:"source"`
	Lines  []Line `json:"lines"`
}

func readLyricsCache(configDir, name, artist string) ([]Line, bool) {
	path := filepath.Join(lyricsCacheDir(configDir), lyricsCacheKey(name, artist)+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var c cachedLyrics
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, false
	}
	if len(c.Lines) == 0 {
		return nil, false
	}
	return c.Lines, true
}

func writeLyricsCache(configDir, name, artist, source string, lines []Line) {
	dir := lyricsCacheDir(configDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	raw, err := json.Marshal(cachedLyrics{Source: source, Lines: lines})
	if err != nil {
		return
	}
	path := filepath.Join(dir, lyricsCacheKey(name, artist)+".json")
	_ = os.WriteFile(path, raw, 0o600)
}
