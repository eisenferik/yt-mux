[CmdletBinding()]
param(
    [string] $ExtensionRoot = (Join-Path $PSScriptRoot '..\extension\dist')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$uninstallScript = Join-Path $repositoryRoot 'setup\uninstall.ps1'
if (-not (Test-Path -LiteralPath (Join-Path $ExtensionRoot 'manifest.json') -PathType Leaf)) {
    throw 'Setup tests require a built extension. Run npm --prefix extension run build, or run scripts/test.ps1.'
}
$extensionRoot = (Resolve-Path -LiteralPath $ExtensionRoot).Path
$temporaryParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
$fixtureRoot = Join-Path $temporaryParent ('yt-mux-setup-test-' + [guid]::NewGuid().ToString('N'))

function Get-FixtureInstallPath {
    param([Parameter(Mandatory)][string] $Path)

    $resolved = [IO.Path]::GetFullPath($Path)
    if (-not $resolved.StartsWith($fixtureRoot + '\', [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Unexpected test installation path.'
    }
    return $resolved
}

# Exercise the real file cleanup without touching the native messaging registry.
$parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($uninstallScript, [ref] $null, [ref] $parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
foreach ($definition in $ast.FindAll({
    param($node)
    $node -is [Management.Automation.Language.FunctionDefinitionAst] -and
    $node.Name -in @('Get-NormalizedPath', 'Get-SetupDirectories', 'Remove-InstalledFiles', 'Test-YtMuxInstallation', 'Test-UninstallMarker')
}, $false)) {
    . ([scriptblock]::Create($definition.Extent.Text))
}

try {
    $hostName = 'io.yt_mux.host'
    $uninstallMarkerText = "$hostName uninstall in progress"
    $dependencies = Get-Content -LiteralPath (Join-Path $repositoryRoot 'setup\dependencies.json') -Raw | ConvertFrom-Json
    $installedFiles = @(
        'bin\yt-mux-host.exe', 'bin\yt-dlp.exe', 'bin\ffmpeg.exe', 'bin\ffprobe.exe', 'bin\deno.exe',
        'config\settings.json', 'config\settings-123456.tmp',
        'native\io.yt_mux.host.json', 'tmp\cookies-0123456789abcdef0123456789abcdef.txt'
    )
    $installedFiles += @(Get-ChildItem -LiteralPath $extensionRoot -File -Recurse | ForEach-Object {
        'extension\' + $_.FullName.Substring($extensionRoot.Length + 1)
    })

    $cases = @(
        @{ Name = 'clean-install'; UserFiles = $false; InvalidSettings = $false; InternalDestination = $false; InterruptedSetup = $false; FirstInstall = $false },
        @{ Name = 'with-user-files'; UserFiles = $true; InvalidSettings = $false; InternalDestination = $false; InterruptedSetup = $false; FirstInstall = $false },
        @{ Name = 'internal-destination'; UserFiles = $true; InvalidSettings = $false; InternalDestination = $true; InterruptedSetup = $false; FirstInstall = $false },
        @{ Name = 'unreadable-settings'; UserFiles = $false; InvalidSettings = $true; InternalDestination = $false; InterruptedSetup = $false; FirstInstall = $false },
        @{ Name = 'interrupted-setup'; UserFiles = $false; InvalidSettings = $false; InternalDestination = $false; InterruptedSetup = $true; FirstInstall = $false },
        @{ Name = 'interrupted-first-install'; UserFiles = $false; InvalidSettings = $false; InternalDestination = $false; InterruptedSetup = $true; FirstInstall = $true },
        @{ Name = 'interrupted-first-install-user-files'; UserFiles = $true; InvalidSettings = $false; InternalDestination = $false; InterruptedSetup = $true; FirstInstall = $true }
    )
    foreach ($case in $cases) {
        $installRoot = Join-Path $fixtureRoot $case.Name
        $userFiles = @(
            'saved-video.mkv', 'downloads\channel\video.mp4', 'bin\video.mkv',
            'extension\icons\notes.txt', 'tmp\notes.txt', 'tmp\setup-abc123\yt-dlp.exe',
            'tmp\setup-0123456789abcdef0123456789abcdef-backup\notes.txt',
            'tmp\Setup-0123456789ABCDEF0123456789abcdef\notes.txt',
            'tmp\cookies-ABCDEF0123456789ABCDEF0123456789.txt'
        )
        $files = @()
        if (-not $case.FirstInstall) {
            $files += $installedFiles
        }
        if ($case.UserFiles) {
            $files += $userFiles
        }
        $setupRoot = Join-Path $installRoot ('tmp\setup-' + [guid]::NewGuid().ToString('N'))
        if ($case.InterruptedSetup) {
            foreach ($relative in @(
                $dependencies.dependencies.ytDlp.asset,
                $dependencies.dependencies.deno.asset,
                $dependencies.dependencies.ffmpeg.asset,
                'deno\deno.exe', 'ffmpeg\build\bin\ffmpeg.exe', 'ffmpeg\build\bin\ffprobe.exe'
            )) {
                $path = Join-Path $setupRoot $relative
                New-Item -ItemType Directory -Path (Split-Path -Parent $path) -Force | Out-Null
                Set-Content -LiteralPath $path -Value 'interrupted setup artifact' -Encoding UTF8
            }
        }
        foreach ($relative in $files) {
            $path = Join-Path $installRoot $relative
            New-Item -ItemType Directory -Path (Split-Path -Parent $path) -Force | Out-Null
            Set-Content -LiteralPath $path -Value 'fixture content' -Encoding UTF8
        }
        if (-not $case.FirstInstall) {
            $settingsPath = Join-Path $installRoot 'config\settings.json'
            if ($case.InvalidSettings) {
                Set-Content -LiteralPath $settingsPath -Value '{invalid' -Encoding UTF8
            } else {
                $destination = if ($case.InternalDestination) { Join-Path $installRoot 'downloads' } else { Join-Path $fixtureRoot 'videos' }
                @{ destination = $destination } | ConvertTo-Json | Set-Content -LiteralPath $settingsPath -Encoding UTF8
            }
        }

        & $uninstallScript -InstallRoot $installRoot -WhatIf -WarningAction SilentlyContinue
        Remove-InstalledFiles -Root (Get-FixtureInstallPath -Path $installRoot)
        if ($case.InterruptedSetup -and (Test-Path -LiteralPath $setupRoot)) {
            throw 'Interrupted setup artifacts were not removed.'
        }
        foreach ($relative in $installedFiles) {
            if (Test-Path -LiteralPath (Join-Path $installRoot $relative)) {
                throw "Application file was not removed: $relative"
            }
        }
        if ($case.UserFiles) {
            foreach ($relative in $userFiles) {
                if ((Get-Content -LiteralPath (Join-Path $installRoot $relative) -Raw).Trim() -ne 'fixture content') {
                    throw "User file was changed or deleted: $relative"
                }
            }
        } elseif (Test-Path -LiteralPath $installRoot) {
            throw 'An installation containing no user files should be removed completely.'
        }
    }

    foreach ($lockedRelative in @('extension\service-worker.js', 'tmp\cookies-0123456789abcdef0123456789abcdef.txt', '.yt-mux-uninstalling')) {
        $installRoot = Join-Path $fixtureRoot ('retry-' + [guid]::NewGuid().ToString('N'))
        foreach ($relative in $installedFiles) {
            $path = Join-Path $installRoot $relative
            New-Item -ItemType Directory -Path (Split-Path -Parent $path) -Force | Out-Null
            [IO.File]::WriteAllText($path, 'fixture content')
        }
        $userFile = Join-Path $installRoot 'saved-video.mkv'
        [IO.File]::WriteAllText($userFile, 'user content')
        $markerPath = Join-Path $installRoot '.yt-mux-uninstalling'
        if ($lockedRelative -eq '.yt-mux-uninstalling') {
            [IO.File]::WriteAllText($markerPath, $uninstallMarkerText)
        }
        $lockedFile = [IO.File]::Open((Join-Path $installRoot $lockedRelative), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
        $failed = $false
        try {
            try { Remove-InstalledFiles -Root $installRoot } catch { $failed = $true }
            if (-not $failed) { throw "Expected a locked-file failure: $lockedRelative" }
            foreach ($relative in @('bin\yt-mux-host.exe', 'native\io.yt_mux.host.json', 'extension\manifest.json')) {
                if (Test-Path -LiteralPath (Join-Path $installRoot $relative)) { throw 'Failure did not reach the former retry gap.' }
            }
            if (-not (Test-YtMuxInstallation -Path $installRoot)) { throw 'Interrupted uninstall is not recognized.' }
            # Exercise the real entry-point safety checks without changing registry state.
            & $uninstallScript -InstallRoot $installRoot -WhatIf -WarningAction SilentlyContinue
        } finally { $lockedFile.Dispose() }
        Remove-InstalledFiles -Root $installRoot
        foreach ($relative in $installedFiles + @('.yt-mux-uninstalling')) {
            if (Test-Path -LiteralPath (Join-Path $installRoot $relative)) { throw "Retry left an application file: $relative" }
        }
        if ([IO.File]::ReadAllText($userFile) -ne 'user content') { throw 'Retry changed a user file.' }
    }

    $installRoot = Join-Path $fixtureRoot 'retry-final-directory'
    New-Item -ItemType Directory -Path (Join-Path $installRoot 'bin') -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $installRoot 'bin\yt-mux-host.exe'), 'fixture content')
    & {
        # Fail after the progress marker was removed, just before root cleanup.
        function Get-ChildItem {
            param([string] $LiteralPath, [switch] $Force, [switch] $File, [switch] $Directory)
            if ($LiteralPath -eq $installRoot -and -not (Test-Path -LiteralPath (Join-Path $installRoot '.yt-mux-uninstalling'))) {
                throw 'Injected final directory I/O failure'
            }
            Microsoft.PowerShell.Management\Get-ChildItem @PSBoundParameters
        }
        $failed = $false
        try { Remove-InstalledFiles -Root $installRoot } catch { $failed = $true }
        if (-not $failed) { throw 'Expected a final directory failure.' }
    }
    if (-not (Test-YtMuxInstallation -Path $installRoot)) { throw 'Final directory failure lost uninstall identity.' }
    & $uninstallScript -InstallRoot $installRoot -WhatIf -WarningAction SilentlyContinue
    Remove-InstalledFiles -Root $installRoot
    if (Test-Path -LiteralPath $installRoot) { throw 'Final directory cleanup could not be retried.' }

    $unknownRoot = Join-Path $fixtureRoot 'unknown-directory'
    New-Item -ItemType Directory -Path $unknownRoot -Force | Out-Null
    $unknownMarker = Join-Path $unknownRoot '.yt-mux-uninstalling'
    [IO.File]::WriteAllText($unknownMarker, 'user content')
    if (Test-YtMuxInstallation -Path $unknownRoot) { throw 'An unrelated file was accepted as uninstall state.' }
    $rejected = $false
    try { & $uninstallScript -InstallRoot $unknownRoot -WhatIf } catch { $rejected = $true }
    if (-not $rejected -or [IO.File]::ReadAllText($unknownMarker) -ne 'user content') {
        throw 'An unrelated directory was not protected.'
    }

    $linkedTemporaryRoot = Join-Path $fixtureRoot 'linked-tmp'
    $linkedTemporaryShared = Join-Path $fixtureRoot 'linked-tmp-shared'
    $linkedTemporaryArtifact = Join-Path $linkedTemporaryShared ('setup-' + [guid]::NewGuid().ToString('N') + '\yt-dlp.exe')
    New-Item -ItemType Directory -Path $linkedTemporaryRoot, (Split-Path -Parent $linkedTemporaryArtifact) -Force | Out-Null
    New-Item -ItemType Junction -Path (Join-Path $linkedTemporaryRoot 'tmp') -Target $linkedTemporaryShared | Out-Null

    $linkedSetupRoot = Join-Path $fixtureRoot 'linked-setup-root'
    $linkedSetupShared = Join-Path $fixtureRoot 'linked-setup-root-shared'
    New-Item -ItemType Directory -Path (Join-Path $linkedSetupRoot 'tmp'), $linkedSetupShared -Force | Out-Null
    New-Item -ItemType Junction -Path (Join-Path $linkedSetupRoot ('tmp\setup-' + [guid]::NewGuid().ToString('N'))) -Target $linkedSetupShared | Out-Null

    $similarNameRoot = Join-Path $fixtureRoot 'similar-setup-name'
    $similarNameArtifact = Join-Path $similarNameRoot 'tmp\setup-abc123\yt-dlp.exe'
    New-Item -ItemType Directory -Path (Split-Path -Parent $similarNameArtifact) -Force | Out-Null

    $unrelatedSetups = @(
        @{ Root = $linkedTemporaryRoot; Artifact = $linkedTemporaryArtifact },
        @{ Root = $linkedSetupRoot; Artifact = (Join-Path $linkedSetupShared 'yt-dlp.exe') },
        @{ Root = $similarNameRoot; Artifact = $similarNameArtifact }
    )
    foreach ($unrelated in $unrelatedSetups) {
        [IO.File]::WriteAllText($unrelated.Artifact, 'user content')
        if (Test-YtMuxInstallation -Path $unrelated.Root) { throw "An unrelated setup directory was accepted: $($unrelated.Root)" }
        $rejected = $false
        try { & $uninstallScript -InstallRoot $unrelated.Root -WhatIf } catch { $rejected = $true }
        if (-not $rejected -or [IO.File]::ReadAllText($unrelated.Artifact) -ne 'user content') {
            throw "An unrelated setup directory was not protected: $($unrelated.Root)"
        }
    }

    $installRoot = Join-Path $fixtureRoot 'linked-setup'
    $setupRoot = Join-Path $installRoot ('tmp\setup-' + [guid]::NewGuid().ToString('N'))
    $sharedRoot = Join-Path $fixtureRoot 'shared-files'
    New-Item -ItemType Directory -Path $setupRoot, $sharedRoot -Force | Out-Null
    $sharedFile = Join-Path $sharedRoot 'yt-dlp.exe'
    Set-Content -LiteralPath $sharedFile -Value 'shared file' -Encoding UTF8
    $link = Join-Path $setupRoot 'deno'
    New-Item -ItemType Junction -Path $link -Target $sharedRoot | Out-Null
    Remove-InstalledFiles -Root (Get-FixtureInstallPath -Path $installRoot)
    if (-not (Test-Path -LiteralPath $link) -or (Get-Content -LiteralPath $sharedFile -Raw).Trim() -ne 'shared file') {
        throw 'Setup cleanup followed or removed a directory junction.'
    }
} finally {
    $resolvedFixture = [IO.Path]::GetFullPath($fixtureRoot)
    if ([IO.Path]::GetDirectoryName($resolvedFixture) -ne $temporaryParent -or
        [IO.Path]::GetFileName($resolvedFixture) -notmatch '^yt-mux-setup-test-[0-9a-f]{32}$') {
        throw 'Refusing to clean an unexpected fixture path.'
    }
    if (Test-Path -LiteralPath $resolvedFixture) {
        Remove-Item -LiteralPath $resolvedFixture -Recurse -Force
    }
}

Write-Host 'Setup file cleanup checks passed.'
