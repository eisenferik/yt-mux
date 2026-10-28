package ytdlp

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/eisenferik/yt-mux/host/internal/config"
)

var placeholderPattern = regexp.MustCompile(`%\([^)]*\)s`)

func TestDownloadArgumentsPassTheCookieFileOnlyWhenThereIsOne(t *testing.T) {
	settings := config.Settings{Destination: filepath.Join(t.TempDir(), "Videos"), SubtitleLanguages: []string{"ja"}}
	cookiePath := filepath.Join(t.TempDir(), "cookies.txt")
	args := DownloadArguments(t.TempDir(), settings, config.Presets["archive"], "https://youtu.be/abc", cookiePath)
	index := slices.Index(args, "--cookies")
	if index < 0 || args[index+1] != cookiePath {
		t.Fatalf("--cookies must precede the cookie file: %#v", args)
	}
	args = DownloadArguments(t.TempDir(), settings, config.Presets["archive"], "https://youtu.be/abc", "")
	if slices.Contains(args, "--cookies") {
		t.Fatal("an empty cookie path must not pass --cookies")
	}
}

func TestDownloadArgumentsFollowTheSubtitleLanguages(t *testing.T) {
	settings := config.Settings{Destination: filepath.Join(t.TempDir(), "Videos"), SubtitleLanguages: []string{"ja", "en"}}
	args := DownloadArguments(t.TempDir(), settings, config.Presets["archive"], "https://youtu.be/abc", "")
	index := slices.Index(args, "--sub-langs")
	if !slices.Contains(args, "--embed-subs") || index < 0 || args[index+1] != "ja,en" {
		t.Fatalf("the chosen languages must be embedded: %#v", args)
	}

	settings.SubtitleLanguages = []string{}
	args = DownloadArguments(t.TempDir(), settings, config.Presets["archive"], "https://youtu.be/abc", "")
	if slices.Contains(args, "--embed-subs") || slices.Contains(args, "--sub-langs") {
		t.Fatalf("no languages must mean no subtitles: %#v", args)
	}
}

func TestDownloadArgumentsEmitTemplatesTheParsersAccept(t *testing.T) {
	settings := config.Settings{Destination: filepath.Join(t.TempDir(), "Videos"), SubtitleLanguages: []string{"ja"}}
	args := DownloadArguments(t.TempDir(), settings, config.Presets["archive"], "https://youtu.be/abc", "")

	template := stagedArgument(t, args, "--progress-template", "download:")
	line := fillPlaceholders(template, "1024", "4096", "299")
	progress, ok := ParseProgress(line)
	if !ok {
		t.Fatalf("the parser rejected the emitted progress line: %q", line)
	}
	assertField(t, "downloaded", progress.Downloaded, int64(1024))
	assertField(t, "total", progress.Total, int64(4096))
	if progress.FormatID != "299" {
		t.Fatalf("the format did not survive the template: %q", progress.FormatID)
	}

	stage := stagedArgument(t, args, "--progress-template", "postprocess:")
	if status, ok := ParsePostprocess(fillPlaceholders(stage, "started")); !ok || status != "started" {
		t.Fatalf("the parser rejected the emitted postprocess line: %q", status)
	}

	announced := stagedArgument(t, args, "--print", "before_dl:")
	total, ok := ParseTotal(fillPlaceholders(announced, "8192"))
	if !ok {
		t.Fatal("the parser rejected the emitted total line")
	}
	var tracker Tracker
	tracker.SetTotal(total)
	assertField(t, "percent", tracker.Update(progress), 12.5)

	printed := stagedArgument(t, args, "--print", "after_move:")
	final := fillPlaceholders(printed, `C:\Videos\clip.mkv`)
	path, ok := ParseDone(final)
	if !ok {
		t.Fatalf("the parser rejected the emitted done line: %q", final)
	}
	if path != `C:\Videos\clip.mkv` {
		t.Fatalf("unexpected final path: %q", path)
	}
}

func stagedArgument(t *testing.T, args []string, name, stage string) string {
	t.Helper()
	for index, argument := range args {
		if argument != name || index+1 >= len(args) {
			continue
		}
		if body, ok := strings.CutPrefix(args[index+1], stage); ok {
			return body
		}
	}
	t.Fatalf("%s is missing a %s template", name, stage)
	return ""
}

func fillPlaceholders(template string, values ...string) string {
	index := 0
	return placeholderPattern.ReplaceAllStringFunc(template, func(string) string {
		if index >= len(values) {
			return ""
		}
		value := values[index]
		index++
		return value
	})
}

func assertField[T int64 | float64](t *testing.T, name string, got *T, want T) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s was not parsed from the emitted template", name)
	}
	if *got != want {
		t.Fatalf("%s did not survive the template: got %v, want %v", name, *got, want)
	}
}

func TestDownloadArgumentsCarryEveryShippedPreset(t *testing.T) {
	for name, preset := range config.Presets {
		t.Run(name, func(t *testing.T) {
			settings := config.Settings{Destination: filepath.Join(t.TempDir(), "Videos"), SubtitleLanguages: []string{"ja"}}
			args := DownloadArguments(t.TempDir(), settings, preset, "https://youtu.be/abc", "")
			for _, pair := range [][2]string{
				{"-f", preset.FormatSelector},
				{"-S", preset.FormatSort},
				{"--merge-output-format", preset.Container},
				{"--remux-video", preset.Container},
			} {
				index := slices.Index(args, pair[0])
				if index < 0 || args[index+1] != pair[1] {
					t.Fatalf("%s must be followed by %q: %#v", pair[0], pair[1], args)
				}
			}
		})
	}
}
