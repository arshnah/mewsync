package mewsync

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Base URLs for the three lyrics sources, overridable so tests can point
// them at an httptest server instead of the real internet.
var (
	lrcLibBaseURL  = "https://lrclib.net"
	neteaseBaseURL = "https://music.163.com"
	qqMusicBaseURL = "https://c.y.qq.com"
)

func fetchLrcLib(name, artist string) []Line {
	u := fmt.Sprintf("%s/api/get?track_name=%s&artist_name=%s",
		lrcLibBaseURL, url.QueryEscape(name), url.QueryEscape(artist))
	resp, err := doRequest("GET", u, nil, nil)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	lines, err := parseLrcLibResponse(body)
	if err != nil {
		return nil
	}
	return lines
}

// parseLrcLibResponse parses an LrcLib /api/get response body.
func parseLrcLibResponse(body []byte) ([]Line, error) {
	var payload struct {
		SyncedLyrics string `json:"syncedLyrics"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.SyncedLyrics) == "" {
		return nil, fmt.Errorf("no synced lyrics in response")
	}
	lines := parseLRC(payload.SyncedLyrics)
	if len(lines) == 0 {
		return nil, fmt.Errorf("synced lyrics did not parse into any lines")
	}
	return lines, nil
}

// fetchNetEase is a best-effort implementation of the NetEase Music lyrics
// fallback, against a public endpoint that requires no key but has an
// unstable, undocumented shape and may need adjustment if NetEase changes
// it. The HTTP calls are kept separate from response parsing
// (parseNetEaseSearchResponse, parseNetEaseLyricResponse) so the parsing
// logic itself is covered by tests against captured/hand-built response
// shapes, even though the live endpoint isn't hit in CI.
func fetchNetEase(name, artist string) []Line {
	searchURL := fmt.Sprintf(
		"%s/api/search/get?s=%s&type=1&offset=0&sub=false&limit=5",
		neteaseBaseURL, url.QueryEscape(name+"-"+artist))
	resp, err := doRequest("POST", searchURL, map[string]string{
		"Referer": "https://music.163.com",
		"Cookie":  "appver=2.0.2",
	}, nil)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	searchBody, err := readAll(resp)
	if err != nil {
		return nil
	}
	songID, ok := parseNetEaseSearchResponse(searchBody)
	if !ok {
		return nil
	}

	lyricURL := fmt.Sprintf("%s/api/song/lyric?tv=-1&kv=-1&lv=-1&os=pc&id=%d", neteaseBaseURL, songID)
	lyricResp, err := doRequest("POST", lyricURL, map[string]string{
		"Referer": "https://music.163.com",
		"Cookie":  "appver=2.0.2",
	}, nil)
	if err != nil {
		return nil
	}
	defer lyricResp.Body.Close()
	lyricBody, err := readAll(lyricResp)
	if err != nil {
		return nil
	}
	lines, err := parseNetEaseLyricResponse(lyricBody)
	if err != nil {
		return nil
	}
	return lines
}

// parseNetEaseSearchResponse pulls the first matching song ID out of a
// NetEase /api/search/get response body.
func parseNetEaseSearchResponse(body []byte) (int64, bool) {
	var search struct {
		Result struct {
			SongCount int `json:"songCount"`
			Songs     []struct {
				ID int64 `json:"id"`
			} `json:"songs"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &search); err != nil {
		return 0, false
	}
	if search.Result.SongCount == 0 || len(search.Result.Songs) == 0 {
		return 0, false
	}
	return search.Result.Songs[0].ID, true
}

// parseNetEaseLyricResponse parses a NetEase /api/song/lyric response body.
func parseNetEaseLyricResponse(body []byte) ([]Line, error) {
	var lyricPayload struct {
		Lrc struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	if err := json.Unmarshal(body, &lyricPayload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(lyricPayload.Lrc.Lyric) == "" {
		return nil, fmt.Errorf("no lyric in response")
	}
	lines := parseLRC(lyricPayload.Lrc.Lyric)
	if len(lines) == 0 {
		return nil, fmt.Errorf("lyric did not parse into any lines")
	}
	return lines, nil
}

// fetchQQMusic is a best-effort implementation of the QQ Music lyrics
// fallback, same caveats as fetchNetEase: undocumented public endpoints that
// may change shape without notice. Parsing is split out the same way, into
// parseQQSearchResponse and parseQQLyricResponse.
func fetchQQMusic(name, artist string) []Line {
	searchURL := fmt.Sprintf(
		"%s/splcloud/fcgi-bin/smartbox_new.fcg?inCharset=utf-8&outCharset=utf-8&format=json&key=%s",
		qqMusicBaseURL, url.QueryEscape(name+"-"+artist))
	resp, err := doRequest("GET", searchURL, map[string]string{
		"Referer": "http://y.qq.com/portal/player.html",
	}, nil)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	searchBody, err := readAll(resp)
	if err != nil {
		return nil
	}
	mid, ok := parseQQSearchResponse(searchBody)
	if !ok {
		return nil
	}

	lyricURL := fmt.Sprintf(
		"%s/lyric/fcgi-bin/fcg_query_lyric_new.fcg?g_tk=5381&format=json&inCharset=utf-8&outCharset=utf-8&songmid=%s",
		qqMusicBaseURL, mid)
	lyricResp, err := doRequest("GET", lyricURL, map[string]string{
		"Referer": "http://y.qq.com/portal/player.html",
	}, nil)
	if err != nil {
		return nil
	}
	defer lyricResp.Body.Close()
	lyricBody, err := readAll(lyricResp)
	if err != nil {
		return nil
	}
	lines, err := parseQQLyricResponse(lyricBody)
	if err != nil {
		return nil
	}
	return lines
}

// parseQQSearchResponse pulls the first matching song mid out of a QQ Music
// smartbox search response body.
func parseQQSearchResponse(body []byte) (string, bool) {
	var search struct {
		Count int `json:"count"`
		Data  struct {
			Song struct {
				ItemList []struct {
					Mid string `json:"mid"`
				} `json:"itemlist"`
			} `json:"song"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &search); err != nil {
		return "", false
	}
	if search.Count == 0 || len(search.Data.Song.ItemList) == 0 {
		return "", false
	}
	mid := search.Data.Song.ItemList[0].Mid
	if mid == "" {
		return "", false
	}
	return mid, true
}

// parseQQLyricResponse parses a QQ Music fcg_query_lyric_new response body.
// The lyric field is base64-encoded LRC text with HTML entities escaped.
func parseQQLyricResponse(body []byte) ([]Line, error) {
	var lyricPayload struct {
		Lyric string `json:"lyric"`
	}
	if err := json.Unmarshal(body, &lyricPayload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(lyricPayload.Lyric) == "" {
		return nil, fmt.Errorf("no lyric in response")
	}
	decoded, err := decodeBase64Loose(lyricPayload.Lyric)
	if err != nil {
		return nil, err
	}
	lines := parseLRC(decodeHTMLEntities(decoded))
	if len(lines) == 0 {
		return nil, fmt.Errorf("lyric did not parse into any lines")
	}
	return lines, nil
}

func decodeBase64Loose(s string) (string, error) {
	s = strings.TrimSpace(s)
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	dst, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(dst), nil
}

func parseLRC(text string) []Line {
	var out []Line
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		times, body := splitTimestamps(line)
		body = strings.TrimSpace(body)
		if len(times) == 0 || body == "" {
			continue
		}
		for _, t := range times {
			out = append(out, Line{TimeMs: t, Text: body})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TimeMs < out[j].TimeMs })
	return out
}

func splitTimestamps(line string) ([]uint64, string) {
	var times []uint64
	var body strings.Builder
	rest := line
	for {
		pos := strings.IndexByte(rest, '[')
		if pos < 0 {
			body.WriteString(rest)
			break
		}
		body.WriteString(rest[:pos])
		after := rest[pos+1:]
		end := strings.IndexByte(after, ']')
		if end < 0 {
			body.WriteByte('[')
			rest = rest[pos+1:]
			continue
		}
		if ms, ok := parseLRCTime(after[:end]); ok {
			times = append(times, ms)
			rest = after[end+1:]
			continue
		}
		body.WriteByte('[')
		rest = rest[pos+1:]
	}
	return times, body.String()
}

func parseLRCTime(tag string) (uint64, bool) {
	minStr, secStr, ok := strings.Cut(tag, ":")
	if !ok {
		return 0, false
	}
	minutes, err := strconv.ParseUint(strings.TrimSpace(minStr), 10, 64)
	if err != nil {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(secStr), 64)
	if err != nil {
		return 0, false
	}
	return minutes*60000 + uint64(seconds*1000+0.5), true
}

func decodeHTMLEntities(s string) string {
	named := map[string]string{
		"amp": "&", "lt": "<", "gt": ">", "quot": "\"", "apos": "'", "nbsp": " ",
	}
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '&' {
			if semi := strings.IndexByte(s[i+1:], ';'); semi >= 0 {
				entity := s[i+1 : i+1+semi]
				if v, ok := named[entity]; ok {
					out.WriteString(v)
					i += semi + 2
					continue
				}
				if strings.HasPrefix(entity, "#") {
					if cp, err := strconv.Atoi(entity[1:]); err == nil {
						out.WriteRune(rune(cp))
						i += semi + 2
						continue
					}
				}
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func readAll(resp *http.Response) ([]byte, error) {
	return io.ReadAll(resp.Body)
}
