package ytdlp

import "testing"

func TestAnalyzeFailureNamesTheReason(t *testing.T) {
	cases := []struct {
		line string
		want FailureReason
	}{
		{"ERROR: [youtube] abc: Sign in to confirm your age. This video may be inappropriate for some users.", FailureLoginRequired},
		{"ERROR: [youtube] abc: This video is available to this channel's members on level: Members Plus (or any higher level). Join this channel to get access to members-only content and other exclusive perks.", FailureMembersOnly},
		{"ERROR: [youtube] abc: Requested format is not available", FailureOther},
		{"[download] Destination: C:\\Videos\\Sign in to my channel.mkv", FailureOther},
	}
	for _, testCase := range cases {
		if got := AnalyzeFailure([]string{testCase.line}).Reason; got != testCase.want {
			t.Errorf("%q: got %d, want %d", testCase.line, got, testCase.want)
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
