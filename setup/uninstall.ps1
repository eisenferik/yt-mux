[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [Parameter()]
    [string] $InstallRoot = (Join-Path $env:LOCALAPPDATA 'yt-mux')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$hostName = 'io.yt_mux.host'
$uninstallMarkerText = "$hostName uninstall in progress"
$resolvedInstallRoot = [System.IO.Path]::GetFullPath($InstallRoot)
$installedHost = Join-Path $resolvedInstallRoot 'bin\yt-mux-host.exe'
$registryPath = "HKCU:\Software\Google\Chrome\NativeMessagingHosts\$hostName"

function Get-NormalizedPath {
    param([Parameter(Mandatory)][string] $Path)
    return [System.IO.Path]::GetFullPath($Path).TrimEnd('\')
}

function Assert-SafeInstallRoot {
    param([Parameter(Mandatory)][string] $Path)
    $fullPath = Get-NormalizedPath -Path $Path
    if ($fullPath -match '^[A-Za-z]:$') {
        throw "Refusing to delete a drive root: $fullPath"
    }

    $forbidden = @(
        $env:LOCALAPPDATA,
        $env:APPDATA,
        $env:USERPROFILE,
        $env:SystemRoot,
        $env:ProgramFiles,
        ${env:ProgramFiles(x86)},
        $env:ProgramData,
        $env:TEMP
    ) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | ForEach-Object { Get-NormalizedPath -Path $_ }

    if ($forbidden -contains $fullPath) {
        throw "Refusing to delete a user or system directory: $fullPath"
    }
}

function Test-YtMuxInstallation {
    param([Parameter(Mandatory)][string] $Path)
    $markers = @(
        (Join-Path $Path 'bin\yt-mux-host.exe'),
        (Join-Path $Path "native\$hostName.json"),
        (Join-Path $Path 'extension\manifest.json')
    )
    foreach ($marker in $markers) {
        if (Test-Path -LiteralPath $marker -PathType Leaf) {
            return $true
        }
    }
    if (@(Get-SetupDirectories -Path $Path).Count -gt 0) {
        return $true
    }
    return (Test-UninstallMarker -Path (Join-Path $Path '.yt-mux-uninstalling'))
}

function Get-SetupDirectories {
    param([Parameter(Mandatory)][string] $Path)
    $temporaryDirectory = Join-Path $Path 'tmp'
    if (-not (Test-Path -LiteralPath $temporaryDirectory -PathType Container)) { return @() }
    if ((Get-Item -LiteralPath $temporaryDirectory -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { return @() }
    return @(Get-ChildItem -LiteralPath $temporaryDirectory -Directory -Force | Where-Object {
        $_.Name -cmatch '^setup-[0-9a-f]{32}$' -and -not ($_.Attributes -band [IO.FileAttributes]::ReparsePoint)
    })
}

function Test-UninstallMarker {
    param([Parameter(Mandatory)][string] $Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $false }
    if ((Get-Item -LiteralPath $Path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { return $false }
    return ([IO.File]::ReadAllText($Path) -eq $uninstallMarkerText)
}

function Test-PathUnderInstallRoot {
    param([Parameter()][string] $Path)
    if ([string]::IsNullOrWhiteSpace($Path)) {
        return $false
    }
    try {
        $fullPath = Get-NormalizedPath -Path $Path
    } catch {
        return $false
    }
    $root = Get-NormalizedPath -Path $resolvedInstallRoot
    if ($fullPath -eq $root) {
        return $true
    }
    $rootWithSeparator = $root + '\'
    return $fullPath.StartsWith($rootWithSeparator, [StringComparison]::OrdinalIgnoreCase)
}

function Remove-InstalledFiles {
    param([Parameter(Mandatory)][string] $Root)

    # Delete only files setup installs; scripts/test-setup.ps1 checks this list against the built extension.
    $applicationFiles = [ordered]@{
        'bin' = @('yt-mux-host.exe', 'yt-dlp.exe', 'ffmpeg.exe', 'ffprobe.exe', 'deno.exe')
        'config' = @('settings.json')
        'native' = @("$hostName.json")
        'extension' = @('manifest.json', 'options.html', 'options.css', 'options.js', 'permissions.js', 'presentation.js', 'protocol.js', 'service-worker.js', 'youtube-url.js', 'cookie-state.js', 'native.js', 'settings-save.js', 'cookie-collection.js')
        'extension\icons' = @('icon16.png', 'icon32.png', 'icon48.png', 'icon128.png')
        'tmp' = @()
    }
    $temporaryPatterns = @{
        'config' = '^settings-[0-9]+\.tmp$'
        'tmp' = '^cookies-[0-9a-f]{32}\.txt$'
    }
    $rootPath = Get-NormalizedPath -Path $Root
    if (-not (Test-Path -LiteralPath $rootPath -PathType Container)) { return }
    if ((Get-Item -LiteralPath $rootPath -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { return }
    # Keep identity after the normal installation markers have been deleted.
    $uninstallMarker = Join-Path $rootPath '.yt-mux-uninstalling'
    if (Test-Path -LiteralPath $uninstallMarker) {
        if (-not (Test-UninstallMarker -Path $uninstallMarker)) {
            throw "Refusing to overwrite an unrecognized uninstall marker: $uninstallMarker"
        }
    } else {
        [IO.File]::WriteAllText($uninstallMarker, $uninstallMarkerText)
    }
    $directories = @()
    foreach ($relative in $applicationFiles.Keys) {
        $directory = Join-Path $rootPath $relative
        if (-not (Test-Path -LiteralPath $directory -PathType Container)) {
            continue
        }
        # A junction here would let deletion escape the install root.
        $linked = $false
        for ($ancestor = $directory; $ancestor.Length -ge $rootPath.Length; $ancestor = Split-Path -Parent $ancestor) {
            if ((Get-Item -LiteralPath $ancestor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) {
                $linked = $true
                break
            }
        }
        if ($linked) {
            continue
        }
        $directories += $directory
        $names = @($applicationFiles[$relative])
        if ($temporaryPatterns.ContainsKey($relative)) {
            $names += @(Get-ChildItem -LiteralPath $directory -File -Force | Where-Object { $_.Name -cmatch $temporaryPatterns[$relative] } | ForEach-Object { $_.Name })
        }
        foreach ($name in $names) {
            $path = Join-Path $directory $name
            if (Test-Path -LiteralPath $path -PathType Leaf) {
                Remove-Item -LiteralPath $path -Force
            }
        }
        if ($relative -eq 'tmp') {
            $pending = [Collections.Generic.Stack[string]]::new()
            foreach ($setup in Get-SetupDirectories -Path $rootPath) {
                $pending.Push($setup.FullName)
            }
            while ($pending.Count -gt 0) {
                $setupPath = Get-NormalizedPath -Path $pending.Pop()
                if (-not $setupPath.StartsWith($directory + '\', [StringComparison]::OrdinalIgnoreCase)) {
                    throw 'Unexpected setup directory path.'
                }
                if ((Get-Item -LiteralPath $setupPath -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) {
                    continue
                }
                $directories += $setupPath
                foreach ($entry in Get-ChildItem -LiteralPath $setupPath -Force) {
                    if ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) {
                        continue
                    }
                    if ($entry.PSIsContainer) {
                        $pending.Push($entry.FullName)
                    } else {
                        Remove-Item -LiteralPath $entry.FullName -Force
                    }
                }
            }
        }
    }
    foreach ($directory in @($directories | Sort-Object Length -Descending)) {
        if (@(Get-ChildItem -LiteralPath $directory -Force).Count -eq 0) {
            [IO.Directory]::Delete($directory, $false)
        }
    }
    Remove-Item -LiteralPath $uninstallMarker -Force
    try {
        if (@(Get-ChildItem -LiteralPath $rootPath -Force).Count -eq 0) {
            [IO.Directory]::Delete($rootPath, $false)
        }
    } catch {
        # The last directory operation can fail too; keep retries recognizable.
        [IO.File]::WriteAllText($uninstallMarker, $uninstallMarkerText)
        throw
    }
}

Assert-SafeInstallRoot -Path $resolvedInstallRoot

$installPresent = Test-Path -LiteralPath $resolvedInstallRoot -PathType Container
$looksLikeInstall = $installPresent -and (Test-YtMuxInstallation -Path $resolvedInstallRoot)
$registryPresent = Test-Path -Path $registryPath
$registryValue = $null
if ($registryPresent) {
    $registryValue = (Get-Item -Path $registryPath).GetValue('')
}

if ($installPresent -and -not $looksLikeInstall) {
    throw "Refusing to delete $resolvedInstallRoot because it does not look like a yt-mux installation."
}

$removeRegistry = $false
if ($registryPresent) {
    if ([string]::IsNullOrWhiteSpace($registryValue) -or (Test-PathUnderInstallRoot -Path $registryValue)) {
        $removeRegistry = $true
    } elseif ($looksLikeInstall) {
        Write-Warning "Native messaging registry points to $registryValue, not this installation. Leaving the registry key in place."
    }
}

if (-not $looksLikeInstall -and -not $removeRegistry) {
    Write-Host "yt-mux is not installed at $resolvedInstallRoot."
    return
}

if (Test-Path -LiteralPath $installedHost -PathType Leaf) {
    try {
        $hostProbe = [System.IO.File]::Open(
            $installedHost,
            [System.IO.FileMode]::Open,
            [System.IO.FileAccess]::ReadWrite,
            [System.IO.FileShare]::None
        )
        $hostProbe.Dispose()
    } catch {
        throw 'The installed yt-mux host is in use. Finish or cancel the active operation, close the browser, and run uninstall again.'
    }
}

if (-not $PSCmdlet.ShouldProcess($resolvedInstallRoot, 'Remove the yt-mux installation')) {
    return
}

Write-Host "Uninstalling yt-mux from $resolvedInstallRoot..."

if ($removeRegistry) {
    Remove-Item -Path $registryPath -Recurse -Force
    Write-Host "Removed registry key $registryPath."
}

if ($looksLikeInstall) {
    try {
        Remove-InstalledFiles -Root $resolvedInstallRoot
    } catch {
        throw "Could not delete the yt-mux application files: $($_.Exception.Message)`nIf a file is in use, close the browser and run uninstall again."
    }
    Write-Host 'Removed the yt-mux application files.'
    if (Test-Path -LiteralPath $resolvedInstallRoot) {
        Write-Host "Files remain in $resolvedInstallRoot."
    }
}

Write-Host ''
Write-Host 'yt-mux was uninstalled successfully.' -ForegroundColor Green
Write-Host 'Remove the unpacked yt-mux extension on the browser''s extensions page if it is still listed.'
Write-Host 'Downloaded videos are not deleted.'
