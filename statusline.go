package mewsync

import "fmt"

// BuildStatus mirrors mewsic's sync.rs `build_status`: renders the fixed
// "[m:ss] text" status line for the given lyric line. Kept deliberately
// simple (no user templates) since the port only needs to exercise the
// concurrency-critical path, not the full formatting feature set.
func BuildStatus(settings Settings, pb Playback, line Line) (text, emoji string) {
	m := line.TimeMs / 60000
	s := (line.TimeMs / 1000) % 60
	text = fmt.Sprintf("[%d:%02d] %s", m, s, line.Text)
	return text, "🎵"
}
