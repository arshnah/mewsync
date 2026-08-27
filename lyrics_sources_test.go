package mewsync

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arshnah/detsim/rt"
)

func TestParseLrcLibResponse(t *testing.T) {
	body := []byte(`{
		"id": 123,
		"trackName": "Test Song",
		"artistName": "Test Artist",
		"syncedLyrics": "[00:01.00]first line\n[00:02.50]second line\n"
	}`)
	lines, err := parseLrcLibResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0].TimeMs != 1000 || lines[0].Text != "first line" {
		t.Errorf("unexpected first line: %+v", lines[0])
	}
	if lines[1].TimeMs != 2500 || lines[1].Text != "second line" {
		t.Errorf("unexpected second line: %+v", lines[1])
	}
}

func TestParseLrcLibResponseEmpty(t *testing.T) {
	if _, err := parseLrcLibResponse([]byte(`{"syncedLyrics": ""}`)); err == nil {
		t.Fatal("expected error for empty synced lyrics")
	}
	if _, err := parseLrcLibResponse([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestParseNetEaseSearchResponse(t *testing.T) {
	body := []byte(`{
		"result": {
			"songs": [{"id": 4875306, "name": "Test Song"}],
			"songCount": 1
		},
		"code": 200
	}`)
	id, ok := parseNetEaseSearchResponse(body)
	if !ok || id != 4875306 {
		t.Fatalf("expected id 4875306, got %d ok=%v", id, ok)
	}

	if _, ok := parseNetEaseSearchResponse([]byte(`{"result":{"songCount":0,"songs":[]},"code":200}`)); ok {
		t.Fatal("expected no match for empty result")
	}
}

func TestParseNetEaseLyricResponse(t *testing.T) {
	body := []byte(`{
		"sgc": false,
		"sfy": false,
		"qfy": false,
		"lrc": {"version": 12, "lyric": "[00:00.00]NetEase test\n[00:03.00]second line\n"},
		"code": 200
	}`)
	lines, err := parseNetEaseLyricResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 || lines[0].Text != "NetEase test" {
		t.Fatalf("unexpected lines: %+v", lines)
	}
}

func TestParseQQSearchResponse(t *testing.T) {
	body := []byte(`{
		"code": 0,
		"count": 1,
		"data": {"song": {"itemlist": [{"mid": "003p4rE4090nCB", "name": "Test Song"}]}}
	}`)
	mid, ok := parseQQSearchResponse(body)
	if !ok || mid != "003p4rE4090nCB" {
		t.Fatalf("expected mid, got %q ok=%v", mid, ok)
	}

	if _, ok := parseQQSearchResponse([]byte(`{"count":0,"data":{"song":{"itemlist":[]}}}`)); ok {
		t.Fatal("expected no match for empty result")
	}
}

func TestParseQQLyricResponse(t *testing.T) {
	raw := "[00:00.00]QQ Music test\n[00:04.00]&amp; entities &lt;ok&gt;\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	body := []byte(fmt.Sprintf(`{"code":0,"lyric":%q}`, encoded))

	lines, err := parseQQLyricResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %+v", len(lines), lines)
	}
	if lines[0].Text != "QQ Music test" {
		t.Errorf("unexpected first line: %+v", lines[0])
	}
	if lines[1].Text != "& entities <ok>" {
		t.Errorf("expected decoded entities, got %+v", lines[1])
	}
}

func TestParseQQLyricResponseEmpty(t *testing.T) {
	if _, err := parseQQLyricResponse([]byte(`{"code":0,"lyric":""}`)); err == nil {
		t.Fatal("expected error for empty lyric")
	}
}

// TestLyricsFallbackChain stubs all three lyrics endpoints with httptest
// servers and checks the fallback order end to end: LrcLib returning nothing
// falls through to NetEase, which in turn falls through to QQ Music when it
// also has nothing.
func TestLyricsFallbackChain(t *testing.T) {
	origLrcLib, origNetease, origQQ := lrcLibBaseURL, neteaseBaseURL, qqMusicBaseURL
	defer func() {
		lrcLibBaseURL, neteaseBaseURL, qqMusicBaseURL = origLrcLib, origNetease, origQQ
	}()

	t.Run("falls through to netease when lrclib has nothing", func(t *testing.T) {
		lrcLib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"syncedLyrics": ""}`))
		}))
		defer lrcLib.Close()

		netease := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/search/get" {
				w.Write([]byte(`{"result":{"songCount":1,"songs":[{"id":42}]},"code":200}`))
				return
			}
			w.Write([]byte(`{"lrc":{"lyric":"[00:00.00]from netease\n"},"code":200}`))
		}))
		defer netease.Close()

		qq := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("qq music should not be reached when netease succeeds")
		}))
		defer qq.Close()

		lrcLibBaseURL, neteaseBaseURL, qqMusicBaseURL = lrcLib.URL, netease.URL, qq.URL

		lines := fetchLrcLib("song", "artist")
		if lines != nil {
			t.Fatalf("expected lrclib to return nothing, got %+v", lines)
		}
		lines = fetchNetEase("song", "artist")
		if len(lines) != 1 || lines[0].Text != "from netease" {
			t.Fatalf("expected netease fallback lines, got %+v", lines)
		}
	})

	t.Run("falls through to qq music when both lrclib and netease have nothing", func(t *testing.T) {
		lrcLib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer lrcLib.Close()

		netease := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"result":{"songCount":0,"songs":[]},"code":200}`))
		}))
		defer netease.Close()

		raw := "[00:00.00]from qq music\n"
		encoded := base64.StdEncoding.EncodeToString([]byte(raw))
		qq := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/splcloud/fcgi-bin/smartbox_new.fcg" {
				w.Write([]byte(`{"count":1,"data":{"song":{"itemlist":[{"mid":"abc123"}]}}}`))
				return
			}
			fmt.Fprintf(w, `{"code":0,"lyric":%q}`, encoded)
		}))
		defer qq.Close()

		lrcLibBaseURL, neteaseBaseURL, qqMusicBaseURL = lrcLib.URL, netease.URL, qq.URL

		if lines := fetchLrcLib("song", "artist"); lines != nil {
			t.Fatalf("expected lrclib to return nothing, got %+v", lines)
		}
		if lines := fetchNetEase("song", "artist"); lines != nil {
			t.Fatalf("expected netease to return nothing, got %+v", lines)
		}
		lines := fetchQQMusic("song", "artist")
		if len(lines) != 1 || lines[0].Text != "from qq music" {
			t.Fatalf("expected qq music fallback lines, got %+v", lines)
		}
	})

	t.Run("lrclib succeeds directly, no fallback needed", func(t *testing.T) {
		lrcLib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"syncedLyrics": "[00:00.00]from lrclib\n"}`))
		}))
		defer lrcLib.Close()

		lrcLibBaseURL = lrcLib.URL

		lines := fetchLrcLib("song", "artist")
		if len(lines) != 1 || lines[0].Text != "from lrclib" {
			t.Fatalf("expected lrclib lines, got %+v", lines)
		}
	})
}

// TestFetchLyricsUsesFallbackChain drives RealConnector.FetchLyrics itself
// (not just the individual fetchers) against stub servers for all three
// sources, confirming the whole chain wires together end to end and the
// result gets written to the on-disk cache.
func TestFetchLyricsUsesFallbackChain(t *testing.T) {
	origLrcLib, origNetease, origQQ := lrcLibBaseURL, neteaseBaseURL, qqMusicBaseURL
	defer func() {
		lrcLibBaseURL, neteaseBaseURL, qqMusicBaseURL = origLrcLib, origNetease, origQQ
	}()

	lrcLib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"syncedLyrics": ""}`))
	}))
	defer lrcLib.Close()

	netease := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search/get" {
			w.Write([]byte(`{"result":{"songCount":1,"songs":[{"id":1}]},"code":200}`))
			return
		}
		w.Write([]byte(`{"lrc":{"lyric":"[00:00.00]cached line\n"},"code":200}`))
	}))
	defer netease.Close()

	qq := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("qq music should not be reached when netease succeeds")
	}))
	defer qq.Close()

	lrcLibBaseURL, neteaseBaseURL, qqMusicBaseURL = lrcLib.URL, netease.URL, qq.URL

	dir := t.TempDir()
	sched := rt.NewSched(1)
	settings := NewSettingsBox(newRtRuntime(sched), Settings{})
	conn := NewRealConnector(settings, dir)

	lines := conn.FetchLyrics("song", "artist")
	if len(lines) != 1 || lines[0].Text != "cached line" {
		t.Fatalf("expected netease fallback line via FetchLyrics, got %+v", lines)
	}

	// A second call should be served from the on-disk cache, not the network:
	// point every base URL at a server that would fail the test if hit.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("FetchLyrics should have used the cache, not hit the network again")
	}))
	defer dead.Close()
	lrcLibBaseURL, neteaseBaseURL, qqMusicBaseURL = dead.URL, dead.URL, dead.URL

	cachedLines := conn.FetchLyrics("song", "artist")
	if len(cachedLines) != 1 || cachedLines[0].Text != "cached line" {
		t.Fatalf("expected cached line on second call, got %+v", cachedLines)
	}
}
