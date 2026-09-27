[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

Write-Host 'Checking Go formatting...'
$goFiles = Get-ChildItem -LiteralPath (Join-Path $repositoryRoot 'host') -Filter '*.go' -File -Recurse | ForEach-Object { $_.FullName }
$unformatted = & gofmt -l $goFiles
if ($LASTEXITCODE -ne 0) {
    throw 'gofmt failed.'
}
if ($unformatted) {
    throw "Go files need formatting:`n$($unformatted -join "`n")"
}

Write-Host 'Running Go tests...'
Push-Location (Join-Path $repositoryRoot 'host')
try {
    & go test ./...
    if ($LASTEXITCODE -ne 0) {
        throw 'Go tests failed.'
    }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) {
        throw 'Go vet failed.'
    }
} finally {
    Pop-Location
}

Write-Host 'Checking the Chrome extension...'
Push-Location (Join-Path $repositoryRoot 'extension')
try {
    & npm test
    if ($LASTEXITCODE -ne 0) {
        throw 'Extension checks failed.'
    }
} finally {
    Pop-Location
}

Write-Host 'Checking setup and release policy...'
& (Join-Path $PSScriptRoot 'test-install.ps1')
& (Join-Path $PSScriptRoot 'test-setup.ps1') -ExtensionRoot (Join-Path $repositoryRoot 'extension\dist')
& (Join-Path $PSScriptRoot 'validate-release.ps1') -RepositoryRoot $repositoryRoot

Write-Host 'All checks passed.' -ForegroundColor Green
