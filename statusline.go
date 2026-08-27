package mewsync

import (
	"fmt"
	"strings"
)

// MaxStatusLength is Discord's custom status character limit.
const MaxStatusLength = 128

// BuildStatus renders the status text and emoji for a lyric line, using the
// advanced template when enabled, otherwise the simple timestamp/label format.
func BuildStatus(settings Settings, pb Playback, line Line) (text, emoji string) {
	if settings.AdvancedEnabled {
		return renderTemplate(settings.AdvancedTemplate, pb, line), settings.AdvancedEmoji
	}

	var parts []string
	if settings.ViewTimestamp {
		parts = append(parts, fmt.Sprintf("[%s]", formatSeconds(line.TimeMs/1000)))
	}
	if settings.ViewLabel {
		parts = append(parts, "Song lyrics -")
	}
	parts = append(parts, strings.ReplaceAll(line.Text, "♪", "🎶"))
	return crop(strings.Join(parts, " "), MaxStatusLength), settings.ViewEmoji
}

func formatSeconds(totalSecs uint64) string {
	m := totalSecs / 60
	s := totalSecs % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func crop(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func lettersOnly(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\'', '"', ',', '.':
			return -1
		}
		return r
	}, s)
}

func croppedTitle(s string) string {
	s = strings.TrimSpace(s)
	dashIdx := strings.Index(s, " -")
	parenIdx := strings.Index(s, "(")
	idx := -1
	switch {
	case dashIdx >= 0 && parenIdx >= 0:
		idx = min(dashIdx, parenIdx)
	case dashIdx >= 0:
		idx = dashIdx
	case parenIdx >= 0:
		idx = parenIdx
	}
	if idx < 0 {
		return s
	}
	return strings.TrimSpace(s[:idx])
}

func renderTemplate(template string, pb Playback, line Line) string {
	song := pb.SongName
	author := pb.SongAuthor
	ts := formatSeconds(line.TimeMs / 1000)

	type tok struct{ key, val string }
	tokens := []tok{
		{"{lyrics_upper_letters_only}", lettersOnly(strings.ToUpper(line.Text))},
		{"{lyrics_lower_letters_only}", lettersOnly(strings.ToLower(line.Text))},
		{"{lyrics_letters_only}", lettersOnly(line.Text)},
		{"{song_name_upper_cropped}", croppedTitle(strings.ToUpper(song))},
		{"{song_name_lower_cropped}", croppedTitle(strings.ToLower(song))},
		{"{song_name_cropped}", croppedTitle(song)},
		{"{song_author_upper}", strings.ToUpper(author)},
		{"{song_author_lower}", strings.ToLower(author)},
		{"{lyrics_upper}", strings.ToUpper(line.Text)},
		{"{lyrics_lower}", strings.ToLower(line.Text)},
		{"{song_name_upper}", strings.ToUpper(song)},
		{"{song_name_lower}", strings.ToLower(song)},
		{"{song_name}", song},
		{"{song_author}", author},
		{"{lyrics}", line.Text},
		{"{timestamp}", ts},
	}

	out := template
	for _, t := range tokens {
		out = strings.ReplaceAll(out, t.key, t.val)
	}
	return strings.ReplaceAll(out, "♪", "🎶")
}
