# Third-party software

yt-mux releases contain only yt-mux's own source and compiled artifacts. The
setup script downloads the following third-party software directly from the
listed upstream distribution locations after verifying a pinned SHA-256 digest.

## yt-dlp

- Project: [yt-dlp](https://github.com/yt-dlp/yt-dlp)
- Distribution: the official `yt-dlp.exe` GitHub release asset
- License: The Unlicense, with separately licensed bundled components described
  by the upstream project
- Installed file: `bin\yt-dlp.exe`

## FFmpeg

- Project: [FFmpeg](https://ffmpeg.org/)
- Windows distribution: the release essentials build from
  [gyan.dev](https://www.gyan.dev/ffmpeg/builds/), a provider linked from the
  official FFmpeg download page, downloaded from its
  [GitHub releases](https://github.com/GyanD/codexffmpeg/releases)
- License: GNU General Public License version 3 or later for the selected build;
  see the build's included license files for exact component terms
- Installed files: `bin\ffmpeg.exe` and `bin\ffprobe.exe`

The setup process downloads the upstream archive but installs only the two
executables needed by yt-mux. yt-mux uses FFmpeg only for muxing, remuxing,
metadata, subtitles, chapters, and thumbnails. It does not request encoding.

## Deno

- Project: [Deno](https://github.com/denoland/deno)
- Distribution: the official x86-64 Windows GitHub release asset
- License: MIT
- Installed file: `bin\deno.exe`

## Go extended system library

- Project: [`golang.org/x/sys`](https://pkg.go.dev/golang.org/x/sys)
- Use: Windows Job Object, process, and access-control APIs in the native host
- License: BSD 3-Clause
- Distribution: statically linked into `yt-mux-host.exe`

Pinned versions, asset URLs, and SHA-256 values are authoritative in
[`setup/dependencies.json`](setup/dependencies.json). Third-party software is not
covered by yt-mux's MIT License.
