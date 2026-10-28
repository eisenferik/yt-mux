package ytdlp

import "strings"

type FailureReason int

const (
	FailureOther FailureReason = iota
	FailureLoginRequired
	FailureMembersOnly
)

type FailureAnalysis struct {
	Reason     FailureReason
	Diagnostic string
}

func AnalyzeFailure(lines []string) FailureAnalysis {
	diagnostic := diagnosticLine(lines)
	return FailureAnalysis{Reason: failureReason(diagnostic), Diagnostic: diagnostic}
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
