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

func TestAnalyzeFailureNamesFailuresAnUpdateFixes(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  FailureReason
	}{
		{
			name: "an extraction failure yt-dlp did not expect",
			lines: []string{
				"ERROR: [youtube] abc: Failed to extract any player response; please report this issue on  https://github.com/yt-dlp/yt-dlp/issues?q= , filling out the appropriate issue template. Confirm you are on the latest version using  yt-dlp -U",
			},
			want: FailureOutdated,
		},
		{
			name: "formats lost to a broken n challenge",
			lines: []string{
				"[youtube] abc: Downloading player 1a2b3c4d",
				"WARNING: [youtube] abc: n challenge solving failed: Some formats may be missing. Ensure you have a supported JavaScript runtime and challenge solver script distribution installed. Review any warnings presented before this message. For more details, refer to  https://github.com/yt-dlp/yt-dlp/wiki/EJS",
				"ERROR: [youtube] abc: Requested format is not available. Use --list-formats for a list of available formats",
			},
			want: FailureOutdated,
		},
		{
			name: "formats lost to a broken signature",
			lines: []string{
				"WARNING: [youtube] abc: Signature solving failed: Some formats may be missing. Ensure you have a supported JavaScript runtime and challenge solver script distribution installed. Review any warnings presented before this message. For more details, refer to  https://github.com/yt-dlp/yt-dlp/wiki/EJS",
				"ERROR: [youtube] abc: Requested format is not available. Use --list-formats for a list of available formats",
			},
			want: FailureOutdated,
		},
		{
			name: "a format that is missing for another reason",
			lines: []string{
				"ERROR: [youtube] abc: Requested format is not available. Use --list-formats for a list of available formats",
			},
			want: FailureOther,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := AnalyzeFailure(testCase.lines).Reason; got != testCase.want {
				t.Errorf("got %d, want %d", got, testCase.want)
			}
		})
	}
}
