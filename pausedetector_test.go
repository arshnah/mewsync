package mewsync

import "testing"

// TestPauseDetectorNormalPlayback feeds position samples that advance at
// real time (1x); the detector should never flag a pause.
func TestPauseDetectorNormalPlayback(t *testing.T) {
	var d pauseDetector
	wall := uint64(0)
	elapsed := uint64(0)
	for i := 0; i < 20; i++ {
		wall += 5000
		elapsed += 5000
		if d.Sample(wall, elapsed) {
			t.Fatalf("sample %d: falsely flagged paused during normal playback", i)
		}
	}
}

// TestPauseDetectorGenuinePause simulates position advancing normally, then
// stalling for several polls (a real pause), then resuming. The detector
// should freeze (flag paused) during the stall and clear the flag once
// advancement resumes.
func TestPauseDetectorGenuinePause(t *testing.T) {
	var d pauseDetector
	wall := uint64(0)
	elapsed := uint64(0)

	// normal playback for a while
	for i := 0; i < 5; i++ {
		wall += 5000
		elapsed += 5000
		if d.Sample(wall, elapsed) {
			t.Fatalf("falsely flagged paused before the pause window (sample %d)", i)
		}
	}

	// genuine pause: wall clock keeps advancing, position doesn't
	pausedAt := -1
	for i := 0; i < 5; i++ {
		wall += 5000
		if d.Sample(wall, elapsed) {
			pausedAt = i
			break
		}
	}
	if pausedAt < 0 {
		t.Fatal("expected pause to be detected during the stall")
	}
	// should need more than one stalled sample (hysteresis)
	if pausedAt == 0 {
		t.Fatal("expected at least 2 consecutive stalled samples before flagging paused")
	}

	// stays flagged while still stalled
	wall += 5000
	if !d.Sample(wall, elapsed) {
		t.Fatal("expected pause flag to persist while still stalled")
	}

	// resume: position starts advancing again at real time
	resumed := false
	for i := 0; i < 5; i++ {
		wall += 5000
		elapsed += 5000
		if !d.Sample(wall, elapsed) {
			resumed = true
			break
		}
	}
	if !resumed {
		t.Fatal("expected pause flag to clear once playback resumed")
	}
}

// TestPauseDetectorJitterNotMisdetected simulates rapid re-polls (small gaps
// between samples, as from a retry or a manual re-check) interleaved with
// normal polling. The tiny-gap samples should be ignored rather than treated
// as stalls, so normal playback is never misdetected as paused.
func TestPauseDetectorJitterNotMisdetected(t *testing.T) {
	var d pauseDetector
	wall := uint64(0)
	elapsed := uint64(0)

	for i := 0; i < 20; i++ {
		wall += 5000
		elapsed += 5000
		if d.Sample(wall, elapsed) {
			t.Fatalf("sample %d: falsely flagged paused", i)
		}
		// a rapid re-poll a few hundred ms later, position barely moved
		jitterWall := wall + 200
		jitterElapsed := elapsed + 200
		if d.Sample(jitterWall, jitterElapsed) {
			t.Fatalf("jitter sample %d: falsely flagged paused", i)
		}
	}
}

func TestPauseDetectorReset(t *testing.T) {
	var d pauseDetector
	wall := uint64(0)
	elapsed := uint64(0)
	for i := 0; i < 5; i++ {
		wall += 5000
		d.Sample(wall, elapsed) // elapsed never advances: this would eventually flag paused
	}
	if !d.paused {
		t.Fatal("expected detector to be flagged paused before reset")
	}
	d.reset()
	if d.paused || len(d.samples) != 0 || d.lowStreak != 0 {
		t.Fatal("expected reset to clear all state")
	}
}
