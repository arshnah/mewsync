package mewsync

// pauseDetector infers "probably paused" from a stream of
// (wall-clock time, estimated playback position) samples, without needing a
// live "is playing" flag from the source. It keeps a short ring buffer of
// recent samples and looks at the *rate* of position increase relative to
// real time elapsed between samples: if the position is advancing much
// slower than real time for several samples in a row, playback is probably
// stalled; once the rate recovers, it's probably going again.
//
// This is a generic, source-agnostic algorithm. It's deliberately kept
// independent of any particular Connector so it can be unit tested against
// synthetic sequences and reused wherever a source can estimate a position
// but can't directly tell us whether it's paused.
type pauseDetector struct {
	samples   []pauseSample
	lowStreak int
	paused    bool
}

type pauseSample struct {
	wallMs    uint64
	elapsedMs uint64
}

const (
	pauseDetectorWindow      = 5
	pauseRateThreshold       = 0.3 // below this fraction of real time, a sample counts as "stalled"
	pauseResumeRateThreshold = 0.8 // at or above this fraction, playback counts as resumed
	pauseConfirmSamples      = 2   // consecutive stalled samples needed before flagging paused
	pauseMinSampleGapMs      = 1000
)

// Sample records one observation and returns whether playback should now be
// considered paused. wallMs is the observation's wall-clock time in
// milliseconds (e.g. time.Now().UnixMilli()); elapsedMs is the caller's best
// estimate of playback position at that moment, in milliseconds.
//
// Samples less than pauseMinSampleGapMs apart are ignored rather than fed
// into the rate calculation: back-to-back polls (a retry, a manual re-check)
// would otherwise produce a noisy, near-meaningless rate over a tiny time
// window and could misfire.
func (d *pauseDetector) Sample(wallMs, elapsedMs uint64) bool {
	if len(d.samples) > 0 {
		last := d.samples[len(d.samples)-1]
		if wallMs <= last.wallMs || wallMs-last.wallMs < pauseMinSampleGapMs {
			return d.paused
		}
	}

	d.samples = append(d.samples, pauseSample{wallMs: wallMs, elapsedMs: elapsedMs})
	if len(d.samples) > pauseDetectorWindow {
		d.samples = d.samples[len(d.samples)-pauseDetectorWindow:]
	}
	if len(d.samples) < 2 {
		return d.paused
	}

	prev := d.samples[len(d.samples)-2]
	cur := d.samples[len(d.samples)-1]
	wallDelta := cur.wallMs - prev.wallMs
	if wallDelta == 0 {
		return d.paused
	}
	var elapsedDelta uint64
	if cur.elapsedMs > prev.elapsedMs {
		elapsedDelta = cur.elapsedMs - prev.elapsedMs
	}
	rate := float64(elapsedDelta) / float64(wallDelta)

	if d.paused {
		if rate >= pauseResumeRateThreshold {
			d.paused = false
			d.lowStreak = 0
		}
		return d.paused
	}

	if rate < pauseRateThreshold {
		d.lowStreak++
	} else {
		d.lowStreak = 0
	}
	if d.lowStreak >= pauseConfirmSamples {
		d.paused = true
	}
	return d.paused
}

// reset clears all history, for when the underlying track changes and old
// samples no longer mean anything.
func (d *pauseDetector) reset() {
	d.samples = nil
	d.lowStreak = 0
	d.paused = false
}
