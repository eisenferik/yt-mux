[CmdletBinding()]
param(
    [Parameter()]
    [switch] $Package
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$distributionRoot = Join-Path $repositoryRoot 'dist'
$releaseRoot = Join-Path $distributionRoot 'yt-mux'

& (Join-Path $PSScriptRoot 'test.ps1')

Push-Location (Join-Path $repositoryRoot 'host')
try {
    New-Item -ItemType Directory -Path (Join-Path $repositoryRoot 'host\build') -Force | Out-Null
    & go build -trimpath -ldflags '-s -w' -o (Join-Path $repositoryRoot 'host\build\yt-mux-host.exe') ./cmd/yt-mux-host
    if ($LASTEXITCODE -ne 0) {
        throw 'Native host build failed.'
    }
} finally {
    Pop-Location
}

if (Test-Path -LiteralPath $distributionRoot) {
    $resolvedDistribution = [System.IO.Path]::GetFullPath($distributionRoot)
    $resolvedRepository = $repositoryRoot.TrimEnd('\') + '\'
    if (-not $resolvedDistribution.StartsWith($resolvedRepository, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to clean an output path outside the repository: $resolvedDistribution"
    }
    Remove-Item -LiteralPath $distributionRoot -Recurse -Force
}
New-Item -ItemType Directory -Path $releaseRoot -Force | Out-Null

Copy-Item -LiteralPath (Join-Path $repositoryRoot 'host\build\yt-mux-host.exe') -Destination $releaseRoot
Copy-Item -LiteralPath (Join-Path $repositoryRoot 'extension\dist') -Destination (Join-Path $releaseRoot 'extension') -Recurse
Copy-Item -LiteralPath (Join-Path $repositoryRoot 'setup') -Destination (Join-Path $releaseRoot 'setup') -Recurse
foreach ($file in @('LICENSE', 'THIRD_PARTY.md', 'README.md')) {
    Copy-Item -LiteralPath (Join-Path $repositoryRoot $file) -Destination $releaseRoot
}

& (Join-Path $PSScriptRoot 'validate-release.ps1') -RepositoryRoot $repositoryRoot -ReleaseRoot $releaseRoot

if ($Package) {
    $archivePath = Join-Path $distributionRoot 'yt-mux-windows-x64.zip'
    Compress-Archive -Path (Join-Path $releaseRoot '*') -DestinationPath $archivePath -CompressionLevel Optimal
    Write-Host "Created $archivePath" -ForegroundColor Green
} else {
    Write-Host "Built $releaseRoot" -ForegroundColor Green
}
