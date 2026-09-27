package ytdlp

import "testing"

func TestAnalyzeFailureRecognizesLoginMarkers(t *testing.T) {
	positives := []string{
		"ERROR: [youtube] abc: Sign in to confirm you’re not a bot. Use --cookies-from-browser or --cookies for the authentication.",
		"ERROR: [youtube] abc: Sign in to confirm your age. This video may be inappropriate for some users.",
		"ERROR: [youtube] abc: This video is available to this channel's members. Join this channel to get access to members-only content.",
		"WARNING: [youtube] abc: Login required",
		"ERROR: requested format is unavailable; age-restricted video",
	}
	for _, line := range positives {
		if !AnalyzeFailure([]string{line}).LoginRequired {
			t.Errorf("expected login required: %q", line)
		}
	}

	negatives := []string{
		"ERROR: [youtube] abc: Requested format is not available",
		"[download] Destination: C:\\Videos\\Sign in to my channel.mkv",
		"ERROR: unable to extract yt initial player response",
		"WARNING: nsig extraction failed",
	}
	for _, line := range negatives {
		if AnalyzeFailure([]string{line}).LoginRequired {
			t.Errorf("did not expect login required: %q", line)
		}
	}
}

func TestAnalyzeFailureDiagnosticPrefersTheLastErrorLine(t *testing.T) {
	lines := []string{
		"[youtube] abc: Downloading webpage",
		"ERROR: unable to extract yt initial player response",
		"[info] abc: Downloading 1 format(s): 315+251",
		"ERROR: unable to download video data: HTTP Error 403: Forbidden",
	}
	if got := AnalyzeFailure(lines).Diagnostic; got != lines[3] {
		t.Errorf("expected the last error line, got %q", got)
	}
}

func TestAnalyzeFailureDiagnosticFallsBackToTheLastLine(t *testing.T) {
	lines := []string{
		"[youtube] abc: Downloading webpage",
		"[Merger] Merging formats into \"abc.mkv\"",
	}
	if got := AnalyzeFailure(lines).Diagnostic; got != lines[1] {
		t.Errorf("expected the last line, got %q", got)
	}
}

func TestAnalyzeFailureWithoutOutput(t *testing.T) {
	analysis := AnalyzeFailure(nil)
	if analysis.Diagnostic != "" || analysis.LoginRequired {
		t.Errorf("expected an empty analysis, got %+v", analysis)
	}
}
