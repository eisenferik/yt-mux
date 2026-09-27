[CmdletBinding()]
param(
    [Parameter()]
    [string] $ReleaseRoot = (Split-Path -Parent $PSScriptRoot),

    [Parameter()]
    [string] $InstallRoot = (Join-Path $env:LOCALAPPDATA 'yt-mux')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$hostName = 'io.yt_mux.host'
$resolvedReleaseRoot = (Resolve-Path -LiteralPath $ReleaseRoot).Path
$resolvedInstallRoot = [System.IO.Path]::GetFullPath($InstallRoot)
$hostSource = Join-Path $resolvedReleaseRoot 'yt-mux-host.exe'
$extensionSource = Join-Path $resolvedReleaseRoot 'extension'
$dependencyDefinition = Join-Path $PSScriptRoot 'dependencies.json'

if (-not (Test-Path -LiteralPath $hostSource -PathType Leaf)) {
    throw "The release does not contain yt-mux-host.exe."
}
if (-not (Test-Path -LiteralPath (Join-Path $extensionSource 'manifest.json') -PathType Leaf)) {
    throw "The release does not contain the built extension."
}

$binDirectory = Join-Path $resolvedInstallRoot 'bin'
$configDirectory = Join-Path $resolvedInstallRoot 'config'
$nativeDirectory = Join-Path $resolvedInstallRoot 'native'
$temporaryDirectory = Join-Path $resolvedInstallRoot 'tmp'
$installedExtension = Join-Path $resolvedInstallRoot 'extension'
$installedHost = Join-Path $binDirectory 'yt-mux-host.exe'
$isUpdate = Test-Path -LiteralPath $installedHost -PathType Leaf

if ($isUpdate) {
    try {
        $hostProbe = [System.IO.File]::Open(
            $installedHost,
            [System.IO.FileMode]::Open,
            [System.IO.FileAccess]::ReadWrite,
            [System.IO.FileShare]::None
        )
        $hostProbe.Dispose()
    } catch {
        throw 'The installed yt-mux host is in use. Finish or cancel the active operation, close the browser, and run setup again.'
    }
    Write-Host 'Updating the existing yt-mux installation...'
} else {
    Write-Host 'Installing yt-mux...'
}

$setupID = [Guid]::NewGuid().ToString('N')
$workDirectory = Join-Path $temporaryDirectory "setup-$setupID"
New-Item -ItemType Directory -Path $workDirectory -Force | Out-Null

function Assert-PathWithinInstallRoot {
    param(
        [Parameter(Mandatory)][string] $Path,
        [Parameter(Mandatory)][string] $Root
    )
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $rootWithSeparator = $Root.TrimEnd('\') + '\'
    if (-not $fullPath.StartsWith($rootWithSeparator, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify a path outside the yt-mux install directory: $fullPath"
    }
}

function Save-VerifiedAsset {
    param(
        [Parameter(Mandatory)] $Definition,
        [Parameter(Mandatory)][string] $OutputPath
    )
    Assert-PathWithinInstallRoot -Path $OutputPath -Root $resolvedInstallRoot
    Write-Host "Downloading $($Definition.asset) ($($Definition.version))..."
    # Invoke-WebRequest slows down whenever it shows progress; curl shows it at full speed.
    $curl = Join-Path $env:SystemRoot 'System32\curl.exe'
    & {
        # Windows PowerShell may surface curl's progress on stderr as errors.
        $ErrorActionPreference = 'Continue'
        $null = & $curl --fail --location --output $OutputPath $Definition.url
    }
    if ($LASTEXITCODE -ne 0) {
        throw "Downloading $($Definition.asset) failed (curl exit code $LASTEXITCODE)."
    }
    $actualHash = (Get-FileHash -LiteralPath $OutputPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $expectedHash = ([string] $Definition.sha256).ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        Remove-Item -LiteralPath $OutputPath -Force
        throw "SHA-256 verification failed for $($Definition.asset)."
    }
}

function Write-JsonFile {
    param(
        [Parameter(Mandatory)][string] $Path,
        [Parameter(Mandatory)] $Value,
        [Parameter()][int] $Depth = 8
    )
    $json = ($Value | ConvertTo-Json -Depth $Depth) + [Environment]::NewLine
    [System.IO.File]::WriteAllText($Path, $json, [System.Text.UTF8Encoding]::new($false))
}

function Get-ExtensionOrigin {
    param([Parameter(Mandatory)][string] $ManifestPath)
    $key = (Get-Content -LiteralPath $ManifestPath -Raw | ConvertFrom-Json).key
    $sha256 = [System.Security.Cryptography.SHA256]::Create()
    try {
        $digest = $sha256.ComputeHash([Convert]::FromBase64String($key))
    } finally {
        $sha256.Dispose()
    }
    $id = -join ($digest[0..15] | ForEach-Object { [char](97 + ($_ -shr 4)); [char](97 + ($_ -band 15)) })
    return "chrome-extension://$id/"
}

function Write-DefaultSettings {
    param(
        [Parameter(Mandatory)][string] $SettingsPath,
        [Parameter(Mandatory)][string] $StagePath,
        [Parameter(Mandatory)][string] $Destination
    )

    if (Test-Path -LiteralPath $SettingsPath -PathType Leaf) {
        return
    }
    New-Item -ItemType Directory -Path $Destination -Force | Out-Null
    $defaults = [ordered]@{
        destination = $Destination
        defaultPreset = 'archive'
        subtitleLanguages = @('ja', 'en')
        cookiesEnabled = $false
        notifications = $true
    }
    Write-JsonFile -Path $StagePath -Value $defaults -Depth 4
}

function Save-RuntimesToStage {
    param(
        [Parameter(Mandatory)] $Lock,
        [Parameter(Mandatory)][string] $WorkDirectory,
        [Parameter(Mandatory)][string] $BinDirectory
    )

    $ytDlpDownload = Join-Path $WorkDirectory $Lock.dependencies.ytDlp.asset
    Save-VerifiedAsset -Definition $Lock.dependencies.ytDlp -OutputPath $ytDlpDownload
    Move-Item -LiteralPath $ytDlpDownload -Destination (Join-Path $BinDirectory 'yt-dlp.exe') -Force

    $denoDownload = Join-Path $WorkDirectory $Lock.dependencies.deno.asset
    Save-VerifiedAsset -Definition $Lock.dependencies.deno -OutputPath $denoDownload
    $denoExtract = Join-Path $WorkDirectory 'deno'
    New-Item -ItemType Directory -Path $denoExtract -Force | Out-Null
    Expand-Archive -LiteralPath $denoDownload -DestinationPath $denoExtract -Force
    $denoExecutable = Get-ChildItem -LiteralPath $denoExtract -Filter 'deno.exe' -File -Recurse | Select-Object -First 1
    if ($null -eq $denoExecutable) {
        throw "The verified Deno archive did not contain deno.exe."
    }
    Move-Item -LiteralPath $denoExecutable.FullName -Destination (Join-Path $BinDirectory 'deno.exe') -Force

    $ffmpegDownload = Join-Path $WorkDirectory $Lock.dependencies.ffmpeg.asset
    Save-VerifiedAsset -Definition $Lock.dependencies.ffmpeg -OutputPath $ffmpegDownload
    $ffmpegExtract = Join-Path $WorkDirectory 'ffmpeg'
    New-Item -ItemType Directory -Path $ffmpegExtract -Force | Out-Null
    Expand-Archive -LiteralPath $ffmpegDownload -DestinationPath $ffmpegExtract -Force
    foreach ($executableName in @('ffmpeg.exe', 'ffprobe.exe')) {
        $executable = Get-ChildItem -LiteralPath $ffmpegExtract -Filter $executableName -File -Recurse | Select-Object -First 1
        if ($null -eq $executable) {
            throw "The verified FFmpeg archive did not contain $executableName."
        }
        Move-Item -LiteralPath $executable.FullName -Destination (Join-Path $BinDirectory $executableName) -Force
    }
}

function Invoke-InstallTransaction {
    param(
        [Parameter(Mandatory)][string] $InstallRoot,
        [Parameter(Mandatory)][string] $StageRoot,
        [Parameter(Mandatory)][string] $BackupRoot,
        [Parameter(Mandatory)][scriptblock] $Register,
        [Parameter(Mandatory)][scriptblock] $RestoreRegistration
    )

    $entries = @(foreach ($file in Get-ChildItem -LiteralPath $StageRoot -File -Recurse -Force) {
        $relative = $file.FullName.Substring($StageRoot.Length + 1)
        $target = Join-Path $InstallRoot $relative
        $backup = Join-Path $BackupRoot $relative
        Assert-PathWithinInstallRoot -Path $target -Root $InstallRoot
        Assert-PathWithinInstallRoot -Path $backup -Root $InstallRoot
        if (Test-Path -LiteralPath $target -PathType Container) {
            throw "An installation file is occupied by a directory: $target"
        }
        $existed = Test-Path -LiteralPath $target -PathType Leaf
        if ($existed) {
            New-Item -ItemType Directory -Path (Split-Path -Parent $backup) -Force | Out-Null
            Copy-Item -LiteralPath $target -Destination $backup -Force
        }
        [pscustomobject]@{ Source = $file.FullName; Target = $target; Backup = $backup; Existed = $existed }
    })
    Write-JsonFile -Path (Join-Path (Split-Path -Parent $BackupRoot) 'rollback-files.json') -Value @($entries)
    $attempted = [System.Collections.Generic.List[object]]::new()
    $registrationAttempted = $false
    try {
        foreach ($entry in $entries) {
            New-Item -ItemType Directory -Path (Split-Path -Parent $entry.Target) -Force | Out-Null
            # A failed copy may already have truncated the destination.
            $attempted.Add($entry)
            Copy-Item -LiteralPath $entry.Source -Destination $entry.Target -Force
        }
        $registrationAttempted = $true
        & $Register
    } catch {
        $installFailure = $_
        $restoreFailures = [System.Collections.Generic.List[string]]::new()
        if ($registrationAttempted) {
            try { & $RestoreRegistration } catch { $restoreFailures.Add($_.Exception.Message) }
        }
        for ($index = $attempted.Count - 1; $index -ge 0; $index--) {
            $entry = $attempted[$index]
            try {
                if ($entry.Existed) {
                    Copy-Item -LiteralPath $entry.Backup -Destination $entry.Target -Force
                } elseif (Test-Path -LiteralPath $entry.Target) {
                    Assert-PathWithinInstallRoot -Path $entry.Target -Root $InstallRoot
                    Remove-Item -LiteralPath $entry.Target -Force
                }
            } catch { $restoreFailures.Add($_.Exception.Message) }
        }
        if ($restoreFailures.Count -gt 0) {
            $failure = [System.Exception]::new("Setup failed: $($installFailure.Exception.Message) Rollback was incomplete. Recovery files are retained at $(Split-Path -Parent $BackupRoot). Restore errors: $($restoreFailures -join '; ')", $installFailure.Exception)
            $failure.Data['PreserveSetup'] = $true
            throw $failure
        }
        throw $installFailure
    }
}

$preserveSetup = $false
try {
    $lock = Get-Content -LiteralPath $dependencyDefinition -Raw | ConvertFrom-Json
    if ($lock.schemaVersion -ne 1) {
        throw "Unsupported dependency definition version."
    }

    $stageRoot = Join-Path $workDirectory 'stage'
    foreach ($name in @('bin', 'extension', 'config', 'native')) {
        New-Item -ItemType Directory -Path (Join-Path $stageRoot $name) -Force | Out-Null
    }
    Save-RuntimesToStage -Lock $lock -WorkDirectory $workDirectory -BinDirectory (Join-Path $stageRoot 'bin')
    Copy-Item -LiteralPath $hostSource -Destination (Join-Path $stageRoot 'bin\yt-mux-host.exe') -Force
    Get-ChildItem -LiteralPath $extensionSource -Force | Copy-Item -Destination (Join-Path $stageRoot 'extension') -Recurse -Force

    $videoFolder = [Environment]::GetFolderPath([Environment+SpecialFolder]::MyVideos)
    if ([string]::IsNullOrWhiteSpace($videoFolder)) {
        $videoFolder = Join-Path $env:USERPROFILE 'Videos'
    }
    $stagedSettings = Join-Path (Join-Path $stageRoot 'config') 'settings.json'
    Write-DefaultSettings -SettingsPath (Join-Path $configDirectory 'settings.json') -StagePath $stagedSettings -Destination (Join-Path $videoFolder 'yt-mux')

    $nativeManifestPath = Join-Path $nativeDirectory "$hostName.json"
    $nativeManifest = [ordered]@{
        name = $hostName
        description = 'yt-mux native messaging host'
        path = $installedHost
        type = 'stdio'
        allowed_origins = @(Get-ExtensionOrigin -ManifestPath (Join-Path $extensionSource 'manifest.json'))
    }
    Write-JsonFile -Path (Join-Path $stageRoot "native\$hostName.json") -Value $nativeManifest -Depth 4

    $registrySubkey = "Software\Google\Chrome\NativeMessagingHosts\$hostName"
    $registryPath = "HKCU:\$registrySubkey"
    $registryExisted = Test-Path -LiteralPath $registryPath
    $registryValue = $null
    $registryKind = $null
    if ($registryExisted) {
        $registryKey = Get-Item -LiteralPath $registryPath
        try {
            if ($registryKey.GetValueNames() -contains '') {
                $registryValue = $registryKey.GetValue('', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
                $registryKind = $registryKey.GetValueKind('').ToString()
            }
        } finally { $registryKey.Close() }
    }
    Write-JsonFile -Path (Join-Path $workDirectory 'rollback-registry.json') -Value @{
        Path = $registryPath; Existed = $registryExisted; Value = $registryValue; Kind = $registryKind
    }
    Invoke-InstallTransaction -InstallRoot $resolvedInstallRoot -StageRoot $stageRoot -BackupRoot (Join-Path $workDirectory 'backup') -Register {
        if (-not $registryExisted) { New-Item -Path $registryPath -Force | Out-Null }
        Set-Item -Path $registryPath -Value $nativeManifestPath
    } -RestoreRegistration {
        if (-not $registryExisted) {
            if (Test-Path -LiteralPath $registryPath) { Remove-Item -LiteralPath $registryPath -Force }
        } else {
            $registryKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($registrySubkey, $true)
            try {
                if ($null -ne $registryKind) {
                    $registryKey.SetValue('', $registryValue, [Microsoft.Win32.RegistryValueKind] $registryKind)
                } else {
                    $registryKey.DeleteValue('', $false)
                }
            } finally { if ($null -ne $registryKey) { $registryKey.Close() } }
        }
    }

    Write-Host ''
    if ($isUpdate) {
        Write-Host 'yt-mux was updated successfully.' -ForegroundColor Green
        Write-Host 'Restart the browser to load the updated extension.'
    } else {
        Write-Host 'yt-mux was installed successfully.' -ForegroundColor Green
        Write-Host "Load this unpacked extension in the browser: $installedExtension"
    }
} catch {
    $preserveSetup = $_.Exception.Data.Contains('PreserveSetup')
    throw
} finally {
    if (-not $preserveSetup -and (Test-Path -LiteralPath $workDirectory)) {
        Assert-PathWithinInstallRoot -Path $workDirectory -Root $resolvedInstallRoot
        try { Remove-Item -LiteralPath $workDirectory -Recurse -Force } catch {
            Write-Warning "Could not remove setup temporary files at ${workDirectory}: $($_.Exception.Message)"
        }
    }
}
