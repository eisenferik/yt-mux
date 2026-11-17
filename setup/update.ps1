param([switch] $FromNotification)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$unreachable = 'Could not reach GitHub. Check the network connection and try again.'
Write-Host 'Checking the latest yt-mux release...'
try {
    try {
        $tag = (Invoke-RestMethod -Uri 'https://api.github.com/repos/eisenferik/yt-mux/releases/latest').tag_name
    } catch {
        Write-Host $unreachable
        return
    }
    # Without the switch, the same command also repairs a broken installation.
    if ($FromNotification) {
        $installed = [version] (Get-Content -LiteralPath (Join-Path $PSScriptRoot 'extension\manifest.json') -Raw | ConvertFrom-Json).version
        if ([version] $tag.TrimStart('v') -le $installed) {
            Write-Host 'No newer version of yt-mux is out yet. Try updating again later.'
            return
        }
    }

    $workDirectory = Join-Path ([IO.Path]::GetTempPath()) "yt-mux-$([Guid]::NewGuid().ToString('N'))"
    New-Item -ItemType Directory -Path $workDirectory -Force | Out-Null
    try {
        $archive = Join-Path $workDirectory 'yt-mux-windows-x64.zip'
        try {
            Invoke-WebRequest -Uri "https://github.com/eisenferik/yt-mux/releases/download/$tag/yt-mux-windows-x64.zip" -OutFile $archive -UseBasicParsing
        } catch {
            Write-Host $unreachable
            return
        }
        $releaseRoot = Join-Path $workDirectory 'release'
        Expand-Archive -LiteralPath $archive -DestinationPath $releaseRoot
        # Calling install.ps1 directly fails when the execution policy is left at its default.
        $installer = (Join-Path $releaseRoot 'setup\install.ps1') -replace "'", "''"
        & (Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe') -NoProfile -ExecutionPolicy Bypass -Command "try { & '$installer' } catch { Write-Host `$_.Exception.Message; exit 1 }"
        if ($FromNotification -and $LASTEXITCODE -eq 0) {
            Write-Host 'Try saving the video again.'
        }
    } finally {
        Remove-Item -LiteralPath $workDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
} finally {
    if ($FromNotification) {
        Write-Host 'You can close this window.'
    }
}
