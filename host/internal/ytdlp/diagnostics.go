package ytdlp

import "strings"

type FailureReason int

const (
	FailureOther FailureReason = iota
	FailureLoginRequired
	FailureMembersOnly
	FailureOutdated
)

type FailureAnalysis struct {
	Reason     FailureReason
	Diagnostic string
}

func AnalyzeFailure(lines []string) FailureAnalysis {
	diagnostic := diagnosticLine(lines)
	reason := failureReason(diagnostic)
	if reason == FailureOther && outdated(diagnostic, lines) {
		reason = FailureOutdated
	}
	return FailureAnalysis{Reason: reason, Diagnostic: diagnostic}
}

// yt-dlp asks for an update only on extraction failures it did not expect.
// A broken signature or n challenge only warns, and the formats it drops can
// leave nothing the preset selects.
func outdated(diagnostic string, lines []string) bool {
	if strings.Contains(diagnostic, "Confirm you are on the latest version") {
		return true
	}
	if !strings.Contains(diagnostic, "Requested format is not available") {
		return false
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "WARNING: [youtube]") &&
			(strings.Contains(line, "Signature solving failed") || strings.Contains(line, "n challenge solving failed")) {
			return true
		}
	}
	return false
}

func diagnosticLine(lines []string) string {
	for index := len(lines) - 1; index >= 0; index-- {
		if strings.Contains(strings.ToLower(lines[index]), "error:") {
			return lines[index]
		}
	}
	if len(lines) > 0 {
		return lines[len(lines)-1]
	}
	return ""
}

func failureReason(line string) FailureReason {
	if !strings.HasPrefix(line, "ERROR: [youtube") {
		return FailureOther
	}
	lowered := strings.ToLower(line)
	switch {
	case strings.Contains(lowered, "members-only"):
		return FailureMembersOnly
	case strings.Contains(lowered, "sign in"):
		return FailureLoginRequired
	default:
		return FailureOther
	}
}
