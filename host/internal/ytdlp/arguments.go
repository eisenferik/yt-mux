package ytdlp

import (
	"path/filepath"
	"strings"

	"github.com/eisenferik/yt-mux/host/internal/config"
)

const outputTemplate = "%(uploader)s/%(upload_date>%Y%m%d)s_%(title).120B_[%(id)s]_%(format_id)s.%(ext)s"

func Executable(binDirectory string) string {
	return filepath.Join(binDirectory, "yt-dlp.exe")
}

func DownloadArguments(binDirectory string, settings config.Settings, preset config.Preset, rawURL, cookiePath string) []string {
	args := []string{
		"--ignore-config",
		"--no-playlist",
		"--windows-filenames",
		"--no-write-auto-subs",
		"--embed-subs",
		"--sub-langs", strings.Join(settings.SubtitleLanguages, ","),
		"--embed-metadata",
		"--embed-chapters",
		"--embed-thumbnail",
		"--encoding", "utf-8",
		"--newline",
		"--progress",
		"--progress-delta", "0.5",
		"--progress-template", "download:" + progressPrefix + "%(progress.downloaded_bytes)s|%(progress.total_bytes,progress.total_bytes_estimate)s|%(progress.speed)s|%(progress.eta)s|%(info.format_id)s",
		"--progress-template", "postprocess:" + postprocessPrefix + "%(progress.status)s",
		"--print", "before_dl:" + totalPrefix + "%(filesize,filesize_approx)s",
		"--print", "after_move:" + donePrefix + "%(filepath)s",
		"--no-simulate",
		"--ffmpeg-location", binDirectory,
		"-P", settings.Destination,
		"-o", outputTemplate,
		"--js-runtimes", "deno:" + filepath.Join(binDirectory, "deno.exe"),
		"-f", preset.FormatSelector,
		"-S", preset.FormatSort,
		"--merge-output-format", preset.Container,
		"--remux-video", preset.Container,
	}
	if cookiePath != "" {
		args = append(args, "--cookies", cookiePath)
	}
	args = append(args, "--", rawURL)
	return args
}
