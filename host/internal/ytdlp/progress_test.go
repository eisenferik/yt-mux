package ytdlp

import (
	"math"
	"testing"
)

const (
	videoTotal   = int64(7943294)
	audioTotal   = int64(166180)
	jobTotal     = videoTotal + audioTotal
	floatEpsilon = 1e-9
)

func TestParseProgressUsesNilForUnknownValues(t *testing.T) {
	progress, ok := ParseProgress("@P@NA|NA|NA|NA|NA")
	if !ok {
		t.Fatal("expected progress line")
	}
	if progress.Downloaded != nil || progress.Total != nil || progress.Speed != nil || progress.ETA != nil {
		t.Fatalf("expected nil values: %#v", progress)
	}
}

func TestParseTotalKeepsAnUnknownTotal(t *testing.T) {
	if total, ok := ParseTotal("@TOTAL@NA"); !ok || total != nil {
		t.Fatalf("an unknown total is still a total line: %#v", total)
	}
}

func TestTrackerMeasuresEveryStreamAgainstTheJobTotal(t *testing.T) {
	var tracker Tracker
	tracker.SetTotal(pointer(jobTotal))

	video := func(downloaded int64) Progress {
		return Progress{Downloaded: pointer(downloaded), Total: pointer(videoTotal), FormatID: "299"}
	}
	audio := func(downloaded int64) Progress {
		return Progress{Downloaded: pointer(downloaded), Total: pointer(audioTotal), FormatID: "251"}
	}

	first := percent(t, tracker.Update(video(1024)))
	assertPercent(t, "the first bytes of the job", first, share(1024))
	last := percent(t, tracker.Update(video(videoTotal)))
	assertPercent(t, "the end of the first stream", last, share(videoTotal))

	resumed := percent(t, tracker.Update(audio(1024)))
	if resumed < last {
		t.Fatalf("a new stream must not send the percentage backwards: %v then %v", last, resumed)
	}
	assertPercent(t, "the start of the second stream", resumed, share(videoTotal+1024))
	assertPercent(t, "the last byte of the last stream", percent(t, tracker.Update(audio(audioTotal))), 100)
}

func TestTrackerReportsNothingWithoutAJobTotal(t *testing.T) {
	var tracker Tracker
	tracker.SetTotal(nil)
	if value := tracker.Update(Progress{Downloaded: pointer(int64(1024)), FormatID: "299"}); value != nil {
		t.Fatalf("expected no percentage: %v", *value)
	}
}

func share(downloaded int64) float64 {
	return float64(downloaded) * 100 / float64(jobTotal)
}

func assertPercent(t *testing.T, stage string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > floatEpsilon {
		t.Fatalf("%s: got %v, want %v", stage, got, want)
	}
}

func percent(t *testing.T, value *float64) float64 {
	t.Helper()
	if value == nil {
		t.Fatal("expected a percentage")
	}
	return *value
}

func pointer[T any](value T) *T {
	return &value
}
