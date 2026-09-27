# Technical design

## Purpose and invariants

yt-mux uses YouTube itself as the user interface. A toolbar click or
`Alt+Shift+D` saves only the active video by selecting the best applicable
YouTube delivery streams and preserving their codecs. The result is not the
uploader's original file; it is a lossless copy, mux, or remux of streams served
by YouTube.

The product has no desktop GUI, tray process, clipboard monitor, local server,
scheduled task, queue manager, or injected page control. Native process count is
zero while idle. It targets Windows 11 and Chromium-based browsers and does not
depend on YouTube's DOM.

## Architecture and lifetime

```text
YouTube tab
  -> Chrome MV3 service worker
  -> Chrome Native Messaging over framed JSON
  -> yt-mux-host.exe
  -> Windows Job Object
  -> yt-dlp.exe
       -> deno.exe
       -> ffmpeg.exe / ffprobe.exe
```

One download runs per tab, over one native port, in one host process, and it
owns one `yt-dlp` process tree.

Clicking the toolbar action on a tab that is already downloading cancels that
download rather than starting a second one.

Nothing yt-mux starts outlives the download it belongs to. When `yt-dlp`
finishes, the user cancels, the port closes, or the browser exits, every
descendant dies with it.

The extension has no popup and no content script.

## Trust boundary

The host runs nothing the extension supplies. The extension names a preset;
the host itself holds what each preset passes to `yt-dlp`, and finds the
runtimes beside its own executable. The host creates the executable directly
and never starts a shell. The URL reaches `yt-dlp` as one
argument after the `--` end-of-options marker.

A URL is accepted only as a YouTube address that names a single video.

The host talks to one extension, and that identity is fixed rather than
assigned when it is installed.

Cookies are refused unless the host's own settings say they are enabled.

## Configuration

Settings hold what the user chose: destination, default preset, subtitle
languages, cookie opt-in, and notifications. A rejected setting leaves the
previous one standing.

Settings govern exactly one notification, the one for a finished download.
Every other notification is unconditional.

yt-mux holds the cookie permission only while the setting says it is on. A
save whose outcome is unknown leaves the permission in place.

The folder dialog belongs to the host. A folder the user picks changes
nothing until the settings are saved.

## Download policy

There are three presets and no others.

**Archive** takes the best streams YouTube has, whatever their codecs, into
Matroska. Best means resolution first, then framerate, then HDR, ahead of any
preference between codecs.

**Archive / VP9** is the same, except that it prefers VP9 and Opus.

**Compatible MP4** takes only AVC and AAC, into MP4. When YouTube serves no
such pair, the download fails rather than transcoding.

Every preset puts the subtitles the user asked for, the metadata, the
chapters, and the thumbnail into the file itself. Automatic captions are
never taken. yt-dlp reads no configuration of its own.

## Failures

Every error response carries a `kind` from a closed vocabulary, and the
extension maps that vocabulary exhaustively. A failure the user can fix is
reported with the remedy. A failure inside yt-mux is reported as one generic
failure.

## Cookies

Cookies are off by default. The permission is requested at the action that
needs it and covers only YouTube and Google.

The host writes them to a file it creates, never to one that already exists,
under an access list that admits only its owner. The file is gone when the
child exits. At startup, the host attempts to remove cookie files whose names
match its own convention and whose last write was more than 24 hours ago. It
does not check whether a file is in use.

## Installation model

Nothing updates itself. yt-mux and every runtime it drives change only when
the user installs a new release.

A release carries what a user needs to install and run yt-mux, and nothing
more. The runtimes are downloaded rather than redistributed, each pinned to an
exact version, URL, and SHA-256 that setup verifies before the binary is ever
run.

yt-mux installs under the user's own profile at `%LOCALAPPDATA%\yt-mux` and
registers `HKCU\Software\Google\Chrome\NativeMessagingHosts\io.yt_mux.host`.
Administrator rights are never required.

An update will not begin while the host is running. It preserves
`settings.json`. The extension keeps its path, so the browser does not need to
load it again. Its identity comes from the manifest key.

Uninstall deletes the files yt-mux installed, out of a directory it has
identified as a yt-mux installation, and nothing else. Videos and anything
else the user put there survive, including when the destination folder is
inside the installation.

User-facing installation, update, and removal instructions are in
[`README.md`](../README.md). Runtime provenance and licensing are in
[`THIRD_PARTY.md`](../THIRD_PARTY.md); exact versions, URLs, and digests are
in [`setup/dependencies.json`](../setup/dependencies.json).
