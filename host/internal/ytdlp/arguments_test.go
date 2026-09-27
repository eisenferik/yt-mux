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

func TestDownloadArgumentsPreserveSecurityInvariants(t *testing.T) {
	settings := config.Settings{Destination: filepath.Join(t.TempDir(), "Videos"), SubtitleLanguages: []string{"ja", "en"}}
	rawURL := "https://www.youtube.com/watch?v=abc&list=playlist"
	args := DownloadArguments(t.TempDir(), settings, config.Presets["archive"], rawURL, "")
	if !slices.Contains(args, "--no-playlist") {
		t.Fatal("--no-playlist is required")
	}
	if !slices.Contains(args, "--ignore-config") {
		t.Fatal("--ignore-config is required")
	}
	for _, forbidden := range []string{"--recode-video", "--audio-format", "--extract-audio", "--exec"} {
		if slices.Contains(args, forbidden) {
			t.Fatalf("forbidden argument generated: %s", forbidden)
		}
	}
	if args[len(args)-2] != "--" || args[len(args)-1] != rawURL {
		t.Fatalf("URL must follow the end-of-options marker: %#v", args[len(args)-2:])
	}
	if slices.Contains(args, "--cookies") {
		t.Fatal("an empty cookie path must not pass --cookies")
	}
}

func TestDownloadArgumentsIncludeTheCookieFile(t *testing.T) {
	settings := config.Settings{Destination: filepath.Join(t.TempDir(), "Videos"), SubtitleLanguages: []string{"ja"}}
	cookiePath := filepath.Join(t.TempDir(), "cookies.txt")
	args := DownloadArguments(t.TempDir(), settings, config.Presets["archive"], "https://youtu.be/abc", cookiePath)
	index := slices.Index(args, "--cookies")
	if index < 0 || args[index+1] != cookiePath {
		t.Fatalf("--cookies must precede the cookie file: %#v", args)
	}
}

func TestDownloadArgumentsEmitTemplatesTheParsersAccept(t *testing.T) {
	settings := config.Settings{Destination: filepath.Join(t.TempDir(), "Videos"), SubtitleLanguages: []string{"ja"}}
	args := DownloadArguments(t.TempDir(), settings, config.Presets["archive"], "https://youtu.be/abc", "")

	if !slices.Contains(args, "--progress") {
		t.Fatal("--print puts yt-dlp in quiet mode, which drops progress unless it is asked for")
	}

	template := stagedArgument(t, args, "--progress-template", "download:")
	assertPlaceholders(t, template,
		"%(progress.downloaded_bytes)s",
		"%(progress.total_bytes,progress.total_bytes_estimate)s",
		"%(progress.speed)s",
		"%(progress.eta)s",
		"%(info.format_id)s",
	)
	line := fillPlaceholders(template, "1024", "4096", "512.5", "6", "299")
	progress, ok := ParseProgress(line)
	if !ok {
		t.Fatalf("the parser rejected the emitted progress line: %q", line)
	}
	assertField(t, "downloaded", progress.Downloaded, int64(1024))
	assertField(t, "total", progress.Total, int64(4096))
	assertField(t, "speed", progress.Speed, 512.5)
	assertField(t, "eta", progress.ETA, 6.0)
	if progress.FormatID != "299" {
		t.Fatalf("the format did not survive the template: %q", progress.FormatID)
	}

	stage := stagedArgument(t, args, "--progress-template", "postprocess:")
	assertPlaceholders(t, stage, "%(progress.status)s")
	if status, ok := ParsePostprocess(fillPlaceholders(stage, "started")); !ok || status != "started" {
		t.Fatalf("the parser rejected the emitted postprocess line: %q", status)
	}

	announced := stagedArgument(t, args, "--print", "before_dl:")
	assertPlaceholders(t, announced, "%(filesize,filesize_approx)s")
	total, ok := ParseTotal(fillPlaceholders(announced, "8192"))
	if !ok {
		t.Fatal("the parser rejected the emitted total line")
	}
	var tracker Tracker
	tracker.SetTotal(total)
	assertField(t, "percent", tracker.Update(progress), 12.5)

	printed := stagedArgument(t, args, "--print", "after_move:")
	assertPlaceholders(t, printed, "%(filepath)s")
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

func assertPlaceholders(t *testing.T, template string, want ...string) {
	t.Helper()
	if got := placeholderPattern.FindAllString(template, -1); !slices.Equal(got, want) {
		t.Fatalf("template fields changed: got %q, want %q", got, want)
	}
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
