package ytdlp

import "strings"

var loginRequiredMarkers = []string{
	"sign in",
	"--cookies",
	"not a bot",
	"confirm your age",
	"age-restricted",
	"age restricted",
	"members-only",
	"members only",
	"login required",
}

type FailureAnalysis struct {
	LoginRequired bool
	Diagnostic    string
}

func AnalyzeFailure(lines []string) FailureAnalysis {
	analysis := FailureAnalysis{Diagnostic: diagnosticLine(lines)}
	for _, line := range lines {
		if loginRequiredLine(line) {
			analysis.LoginRequired = true
			break
		}
	}
	return analysis
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

func loginRequiredLine(line string) bool {
	lowered := strings.ToLower(line)
	lowered = strings.ReplaceAll(lowered, "\u2019", "'")
	lowered = strings.ReplaceAll(lowered, "\u2018", "'")
	if !strings.Contains(lowered, "error:") && !strings.Contains(lowered, "warning:") {
		return false
	}
	for _, marker := range loginRequiredMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}
