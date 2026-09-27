# Contributions

Do not open issues or pull requests. For a vulnerability, use GitHub's private
vulnerability reporting instead of a public issue.

This repository is published so the source can be inspected and built. Outside
changes are not accepted. Fork the repository if you want different behavior.

## Build

Building requires Go 1.27 or later and Node.js 24 or later. From the repository
root in PowerShell:

```powershell
npm --prefix .\extension ci
./scripts/build.ps1
```

The command builds the Go host and unpacked Chrome extension into `dist`.
Third-party runtime binaries are not included. Add `-Package` to also create
the release zip, which contains only the files needed to install and run
yt-mux.

## Development checks

```powershell
./scripts/test.ps1
```

The test command runs the development checks for the host, the extension, and
setup. CI runs them through `build.ps1`, which also builds and validates the
release tree.
