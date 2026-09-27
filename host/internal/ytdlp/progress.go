package ytdlp

import (
	"math"
	"strconv"
	"strings"
)

const (
	progressPrefix    = "@P@"
	donePrefix        = "@DONE@"
	totalPrefix       = "@TOTAL@"
	postprocessPrefix = "@PP@"
)

type Progress struct {
	Downloaded *int64
	Total      *int64
	Speed      *float64
	ETA        *float64
	FormatID   string
}

// yt-dlp reports each selected stream as its own download, so a fraction of
// the whole job needs the job total from before the first stream and a base
// that grows as streams complete.
type Tracker struct {
	total       *int64
	base        int64
	format      string
	streamTotal *int64
}

func (t *Tracker) SetTotal(total *int64) {
	if total != nil && *total > 0 {
		t.total = total
	}
}

func (t *Tracker) Update(progress Progress) *float64 {
	if progress.FormatID != t.format {
		if t.streamTotal != nil {
			t.base += *t.streamTotal
		}
		t.format = progress.FormatID
		t.streamTotal = nil
	}
	if progress.Total != nil {
		t.streamTotal = progress.Total
	}
	if t.total == nil || progress.Downloaded == nil {
		return nil
	}
	value := math.Min(100, float64(t.base+*progress.Downloaded)*100/float64(*t.total))
	return &value
}

func ParseProgress(line string) (Progress, bool) {
	if !strings.HasPrefix(line, progressPrefix) {
		return Progress{}, false
	}
	parts := strings.Split(strings.TrimPrefix(line, progressPrefix), "|")
	if len(parts) != 5 {
		return Progress{}, false
	}
	return Progress{
		Downloaded: parseInteger(parts[0]),
		Total:      parseInteger(parts[1]),
		Speed:      parseFloat(parts[2]),
		ETA:        parseFloat(parts[3]),
		FormatID:   strings.TrimSpace(parts[4]),
	}, true
}

func ParseTotal(line string) (*int64, bool) {
	if !strings.HasPrefix(line, totalPrefix) {
		return nil, false
	}
	return parseInteger(strings.TrimPrefix(line, totalPrefix)), true
}

func ParsePostprocess(line string) (string, bool) {
	if !strings.HasPrefix(line, postprocessPrefix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, postprocessPrefix)), true
}

func ParseDone(line string) (string, bool) {
	if !strings.HasPrefix(line, donePrefix) {
		return "", false
	}
	path := strings.TrimSpace(strings.TrimPrefix(line, donePrefix))
	return path, path != ""
}

func parseInteger(value string) *int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return nil
	}
	return &parsed
}

func parseFloat(value string) *float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return nil
	}
	return &parsed
}
