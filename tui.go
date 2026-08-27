package mewsync

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Snapshot is a point-in-time read of playback state, for the TUI/web panel.
type Snapshot struct {
	SongName    string
	SongAuthor  string
	IsPlaying   bool
	ProgressMs  uint64
	DurationMs  uint64
	CurrentLine string
	Source      Source
	LatencyMs   uint64
}

// GetSnapshot reads a consistent view of playback and tracker state.
func GetSnapshot(shared *Shared, settings *SettingsBox) Snapshot {
	var snap Snapshot
	snap.Source = settings.Get().Source
	shared.WithBoth(func(pb *Playback, tr *Tracker) {
		snap.SongName = pb.SongName
		snap.SongAuthor = pb.SongAuthor
		snap.IsPlaying = pb.IsPlaying
		snap.ProgressMs = pb.SongProgress
		snap.DurationMs = pb.SongDuration
		if pb.CurrentLine != nil {
			snap.CurrentLine = pb.CurrentLine.Text
		}
		snap.LatencyMs = tr.LastLatency
	})
	return snap
}

var (
	tuiAccent  = lipgloss.Color("#7dd3c0")
	tuiAccent2 = lipgloss.Color("#b98ee8")
	tuiMuted   = lipgloss.Color("#7c869c")
	tuiFg      = lipgloss.Color("#e7ebf3")

	tuiFrame = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tuiMuted).
			Padding(1, 2).
			Width(52)

	tuiTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(tuiAccent)

	tuiSubtitle = lipgloss.NewStyle().Foreground(tuiMuted)

	tuiSong = lipgloss.NewStyle().Bold(true).Foreground(tuiFg)

	tuiArtist = lipgloss.NewStyle().Foreground(tuiMuted)

	tuiLine = lipgloss.NewStyle().
		Bold(true).
		Foreground(tuiAccent2).
		Padding(0, 1)

	tuiEmptyLine = lipgloss.NewStyle().
			Italic(true).
			Foreground(tuiMuted)

	tuiBarFilled = lipgloss.NewStyle().Foreground(tuiAccent)
	tuiBarEmpty  = lipgloss.NewStyle().Foreground(tuiMuted)

	tuiFooter = lipgloss.NewStyle().Foreground(tuiMuted)
)

type tuiModel struct {
	shared   *Shared
	settings *SettingsBox
	engine   *Engine
	snap     Snapshot
	quitting bool
}

type tuiTickMsg time.Time

func tuiTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tuiTickMsg(t)
	})
}

func (m tuiModel) Init() tea.Cmd {
	return tuiTick()
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			m.engine.Shutdown()
			return m, tea.Quit
		}
	case tuiTickMsg:
		m.snap = GetSnapshot(m.shared, m.settings)
		return m, tuiTick()
	}
	return m, nil
}

func (m tuiModel) View() string {
	if m.quitting {
		return tuiSubtitle.Render("mewsync stopped.") + "\n"
	}

	var body strings.Builder

	body.WriteString(tuiTitle.Render("♪ mewsync"))
	body.WriteString("  ")
	body.WriteString(tuiSubtitle.Render(fmt.Sprintf("source: %s", m.snap.Source.String())))
	body.WriteString("\n\n")

	if m.snap.SongName == "" {
		body.WriteString(tuiEmptyLine.Render("Nothing playing yet."))
		body.WriteString("\n")
	} else {
		body.WriteString(tuiSong.Render(m.snap.SongName))
		body.WriteString("\n")
		body.WriteString(tuiArtist.Render(m.snap.SongAuthor))
		body.WriteString("\n\n")
		body.WriteString(renderProgressBar(m.snap.ProgressMs, m.snap.DurationMs, 40))
		body.WriteString("\n\n")

		if m.snap.CurrentLine != "" {
			body.WriteString(tuiLine.Render(m.snap.CurrentLine))
		} else {
			body.WriteString(tuiEmptyLine.Render("(no lyric line yet)"))
		}
		body.WriteString("\n")
	}

	body.WriteString("\n")
	body.WriteString(tuiFooter.Render(fmt.Sprintf("discord latency: %dms", m.snap.LatencyMs)))
	body.WriteString("\n")
	body.WriteString(tuiFooter.Render("press q to quit"))

	return tuiFrame.Render(body.String()) + "\n"
}

func renderProgressBar(progressMs, durationMs uint64, width int) string {
	if durationMs == 0 {
		return tuiBarEmpty.Render(strings.Repeat("─", width)) + "  " + tuiSubtitle.Render("--:-- / --:--")
	}
	filled := int(float64(progressMs) / float64(durationMs) * float64(width))
	if filled > width {
		filled = width
	}
	bar := tuiBarFilled.Render(strings.Repeat("━", filled)) + tuiBarEmpty.Render(strings.Repeat("─", width-filled))
	times := tuiSubtitle.Render(fmt.Sprintf("%s / %s", formatSeconds(progressMs/1000), formatSeconds(durationMs/1000)))
	return bar + "  " + times
}

// RunTUI starts the interactive dashboard, blocking until the user quits.
func RunTUI(shared *Shared, settings *SettingsBox, engine *Engine) error {
	m := tuiModel{shared: shared, settings: settings, engine: engine}
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
