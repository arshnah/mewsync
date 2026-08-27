package mewsync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	lastfmAPI    = "https://ws.audioscrobbler.com/2.0/"
	trustedLagMs = 10000
)

func lastfmFetchPlayer(apiKey, username string) (*PlayerState, *uint64, error) {
	u := fmt.Sprintf("%s?method=user.getrecenttracks&user=%s&api_key=%s&limit=2&format=json",
		lastfmAPI, url.QueryEscape(username), url.QueryEscape(apiKey))
	resp, err := doRequest("GET", u, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("last.fm responded %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, nil, err
	}

	if errCode, ok := payload["error"]; ok {
		msg, _ := payload["message"].(string)
		return nil, nil, fmt.Errorf("last.fm: %s (%v)", msg, errCode)
	}

	tracks := lastfmTracks(payload)
	prevUts := lastfmPreviousUts(tracks)

	state, err := lastfmParseNowPlaying(tracks)
	if err != nil {
		return nil, nil, err
	}
	if state == nil {
		return nil, nil, nil
	}

	if durationMs, ok := lastfmFetchDurationMs(apiKey, state.Artist, state.Name); ok {
		state.DurationMs = durationMs
	}
	return state, prevUts, nil
}

func lastfmTracks(payload map[string]any) []map[string]any {
	rt, ok := payload["recenttracks"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := rt["track"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func lastfmPreviousUts(tracks []map[string]any) *uint64 {
	if len(tracks) < 2 {
		return nil
	}
	date, ok := tracks[1]["date"].(map[string]any)
	if !ok {
		return nil
	}
	v, ok := asUint64(date["uts"])
	if !ok {
		return nil
	}
	return &v
}

func lastfmParseNowPlaying(tracks []map[string]any) (*PlayerState, error) {
	if len(tracks) == 0 {
		return nil, nil
	}
	track := tracks[0]

	attr, _ := track["@attr"].(map[string]any)
	nowPlaying, _ := attr["nowplaying"].(string)
	if nowPlaying != "true" {
		return nil, nil
	}

	name, _ := track["name"].(string)
	artist := lastfmTextField(track["artist"])
	if name == "" || artist == "" {
		return nil, nil
	}

	mbid, _ := track["mbid"].(string)

	return &PlayerState{
		TrackID:   fmt.Sprintf("%s|%s|%s", mbid, artist, name),
		Name:      name,
		Artist:    artist,
		IsPlaying: true,
	}, nil
}

func lastfmTextField(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if s, ok := t["#text"].(string); ok {
			return s
		}
	}
	return ""
}

func lastfmFetchDurationMs(apiKey, artist, track string) (uint64, bool) {
	u := fmt.Sprintf("%s?method=track.getInfo&artist=%s&track=%s&api_key=%s&format=json",
		lastfmAPI, url.QueryEscape(artist), url.QueryEscape(track), url.QueryEscape(apiKey))
	resp, err := doRequest("GET", u, nil, nil)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}

	var payload struct {
		Track struct {
			Duration any `json:"duration"`
		} `json:"track"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, false
	}
	raw, ok := asUint64(payload.Track.Duration)
	if !ok || raw == 0 {
		return 0, false
	}
	if raw >= 60000 {
		return raw, true
	}
	return raw * 1000, true
}

func asUint64(v any) (uint64, bool) {
	switch t := v.(type) {
	case float64:
		return uint64(t), true
	case string:
		var n uint64
		if _, err := fmt.Sscanf(t, "%d", &n); err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// lastfmMeasureLag estimates how long ago the now-playing track started,
// from the previous scrobble's end time. Anything over trustedLagMs is
// treated as untrustworthy (a pause, or a track that started before we
// began polling) and the caller should fall back to a default.
func lastfmMeasureLag(nowSecs, prevSecs uint64) (uint64, bool) {
	if nowSecs < prevSecs {
		return 0, false
	}
	lagMs := (nowSecs - prevSecs) * 1000
	if lagMs > trustedLagMs {
		return 0, false
	}
	return lagMs, true
}
