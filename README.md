# yt-mux

yt-mux is not a download manager that happens to live in the browser. It is one
action with no life of its own: no GUI, tray process, local server, queue, or
injected page control, and no native process while idle.

yt-mux adds one local action to YouTube on Windows 11 in a Chromium-based
browser: save the active video at the best quality offered by YouTube without
re-encoding it.

The toolbar action or the default `Alt+Shift+D` shortcut starts a download.
Click the toolbar action again to cancel it. Right-click the action to choose a
preset. If the shortcut is unavailable, assign one on the browser's extension
shortcuts page, such as `chrome://extensions/shortcuts`.

## Requirements

- Windows 11 x64
- A Chromium-based browser built on Chromium 116 or later
- PowerShell 7 or Windows PowerShell 5.1 for setup

## Verify a release

Download the release zip and its matching `SHA256SUMS.txt`, then calculate the
archive's SHA-256 digest before extracting it:

```powershell
Get-FileHash .\yt-mux-windows-x64.zip -Algorithm SHA256
```

The digest must match `SHA256SUMS.txt` from the same release.

## Install

1. [Verify](#verify-a-release) and extract the release.
2. Open PowerShell in the extracted release directory and run:

   ```powershell
   .\setup\install.ps1
   ```

3. Open the browser's extensions page, such as `chrome://extensions`, enable
   Developer mode, choose **Load unpacked**, and select `%LOCALAPPDATA%\yt-mux\extension`.
4. Open a YouTube video and click the yt-mux toolbar action.

Downloads are saved to the Windows Videos folder under `yt-mux` by default.
The first install requests Japanese and English subtitles. To change the
destination, preset, subtitle languages, cookie access, or notifications, open
**Details** for yt-mux on the browser's extensions page, then choose
**Extension options**.

Setup installs yt-mux for the current user, downloads the exact dependency
assets listed in [`setup/dependencies.json`](setup/dependencies.json), verifies
their SHA-256 digests, and registers `io.yt_mux.host` under HKCU. Administrator
access is not required.

## Update

[Verify](#verify-a-release) and extract the new release. Finish or cancel any
active download, close the browser, then run `.\setup\install.ps1` from the new
release directory. Settings are preserved and the extension does not need to be
loaded again.

If an update fails, setup attempts to restore the previous installation.

## Uninstall

Finish or cancel any active download, close the browser, then run
`.\setup\uninstall.ps1` from a yt-mux release or repository directory. Remove
the unpacked yt-mux extension on the browser's extensions page.

Uninstall removes the native messaging registry key and application files from
`%LOCALAPPDATA%\yt-mux`, keeping downloaded videos and files you added. It does
not turn off the browser's Developer mode.

## Presets

| Preset | Stream policy | Final container |
| --- | --- | --- |
| Archive | Highest resolution, frame rate, and HDR preference | MKV |
| Archive / VP9 | Archive policy with VP9 and Opus preference | MKV |
| Compatible MP4 | Select H.264/AVC video and AAC audio at the source | MP4 |

The toolbar action uses Archive unless you change the default in Extension
options. Compatible MP4 fails when compatible source streams are unavailable. It
never falls back to transcoding.

## Privacy and security

Each download is limited to one validated YouTube HTTPS URL. Cookie access is
off by default. Once enabled in extension options, every download uses your
YouTube sign-in until it is turned off there. Closing the browser or
disconnecting the native port stops the download process tree.

`yt-mux-host.exe` is not Authenticode-signed. Windows SmartScreen and some
antivirus products may display a warning. Follow
[Verify a release](#verify-a-release) before installing it.

## License

yt-mux is licensed under the [MIT License](LICENSE). Downloaded dependencies
retain their own licenses; see [`THIRD_PARTY.md`](THIRD_PARTY.md).
