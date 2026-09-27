[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string] $RepositoryRoot,

    [Parameter()]
    [string] $ReleaseRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$runtimeBinaries = @('yt-dlp.exe', 'ffmpeg.exe', 'ffprobe.exe', 'deno.exe')

$repositoryFiles = @((& git -c core.quotePath=false -C $RepositoryRoot ls-files --cached --others --exclude-standard) | Where-Object { $_ })
if ($LASTEXITCODE -ne 0) {
    throw "Could not list the files git tracks under $RepositoryRoot."
}

function Assert-MarkdownLinks {
    param(
        [Parameter(Mandatory)][string] $Path,
        [Parameter(Mandatory)][string] $Root,
        [Parameter(Mandatory)][string] $Scope
    )

    $boundary = [System.IO.Path]::GetFullPath($Root).TrimEnd('\') + '\'
    $directory = Split-Path -Parent $Path
    $name = $Path.Substring($boundary.Length)
    foreach ($match in [regex]::Matches((Get-Content -LiteralPath $Path -Raw), '\[[^\]]*\]\(([^)]+)\)')) {
        $target = $match.Groups[1].Value
        if ($target -match '^(https?:|#|mailto:|chrome:)') {
            continue
        }
        $resolved = [System.IO.Path]::GetFullPath((Join-Path $directory ($target -replace '/', '\')))
        if (-not $resolved.StartsWith($boundary, [StringComparison]::OrdinalIgnoreCase) -or -not (Test-Path -LiteralPath $resolved)) {
            throw "$name links to a file that is not in $Scope`: $target"
        }
    }
}

$dependencyPath = Join-Path $RepositoryRoot 'setup\dependencies.json'
$dependencies = Get-Content -LiteralPath $dependencyPath -Raw | ConvertFrom-Json
if ($dependencies.schemaVersion -ne 1) {
    throw 'Dependency definition must use schema version 1.'
}
foreach ($name in @('ytDlp', 'ffmpeg', 'deno')) {
    if ($null -eq $dependencies.dependencies.PSObject.Properties[$name]) {
        throw "The dependency definition is missing $name."
    }
}
foreach ($entry in $dependencies.dependencies.PSObject.Properties.Value) {
    if ([string]::IsNullOrWhiteSpace($entry.version) -or [string]::IsNullOrWhiteSpace($entry.asset)) {
        throw 'Every dependency must pin a version and asset name.'
    }
    if ($entry.url -notmatch '^https://') {
        throw "Dependency URL must use HTTPS: $($entry.url)"
    }
    if ($entry.sha256 -notmatch '^[0-9a-f]{64}$') {
        throw "Dependency SHA-256 is invalid: $($entry.asset)"
    }
    if ($entry.url -match '/latest/' -or $entry.url -match 'ffmpeg-release-') {
        throw "Dependency URL must be immutable and versioned: $($entry.url)"
    }
}

$workflowRoot = Join-Path $RepositoryRoot '.github\workflows'
$actionDefinitions = @(Get-ChildItem -LiteralPath $workflowRoot -File | Where-Object { $_.Extension -in @('.yml', '.yaml') })
$actionRoot = Join-Path $RepositoryRoot '.github\actions'
if (Test-Path -LiteralPath $actionRoot) {
    $actionDefinitions += @(Get-ChildItem -LiteralPath $actionRoot -File -Recurse | Where-Object { $_.Extension -in @('.yml', '.yaml') })
}
foreach ($actionDefinition in $actionDefinitions) {
    $definition = Get-Content -LiteralPath $actionDefinition.FullName -Raw
    foreach ($match in [regex]::Matches($definition, '(?m)^\s*(?:-\s*)?uses:\s*(\S+)')) {
        $reference = $match.Groups[1].Value
        if ($reference.StartsWith('./')) {
            continue
        }
        if ($reference -notmatch '@[0-9a-f]{40}$') {
            $name = $actionDefinition.FullName.Substring($RepositoryRoot.Length + 1)
            throw "Workflow action must be pinned to a commit SHA: $name uses $reference"
        }
    }
}

$trackedThirdPartyBinaries = @($repositoryFiles | Where-Object { [IO.Path]::GetFileName($_) -in $runtimeBinaries })
if ($trackedThirdPartyBinaries) {
    throw "Third-party runtime binaries are present in the repository: $($trackedThirdPartyBinaries -join ', ')"
}

$thirdParty = Get-Content -LiteralPath (Join-Path $RepositoryRoot 'THIRD_PARTY.md') -Raw
foreach ($binary in $runtimeBinaries) {
    if ($thirdParty -notmatch [regex]::Escape($binary)) {
        throw "THIRD_PARTY.md does not describe the installed runtime $binary."
    }
}
$goModule = Get-Content -LiteralPath (Join-Path $RepositoryRoot 'host\go.mod') -Raw
foreach ($match in [regex]::Matches($goModule, '(?m)^\s*(?:require\s+)?(\S+\.\S+/\S+)\s+v\S+')) {
    $linked = $match.Groups[1].Value
    if ($thirdParty -notmatch [regex]::Escape($linked)) {
        throw "THIRD_PARTY.md does not describe the linked Go module $linked."
    }
}

foreach ($document in $repositoryFiles | Where-Object { [IO.Path]::GetExtension($_) -eq '.md' }) {
    Assert-MarkdownLinks -Path ([IO.Path]::GetFullPath((Join-Path $RepositoryRoot $document))) -Root $RepositoryRoot -Scope 'the repository'
}

if ($ReleaseRoot) {
    $resolvedReleaseRoot = [System.IO.Path]::GetFullPath($ReleaseRoot).TrimEnd('\')

    $forbidden = Get-ChildItem -LiteralPath $resolvedReleaseRoot -File -Recurse | Where-Object {
        $_.Name -in $runtimeBinaries -or $_.Extension -in @('.pem', '.key', '.p12')
    }
    if ($forbidden) {
        throw "Forbidden files are present in the release: $($forbidden.FullName -join ', ')"
    }
    foreach ($required in @('yt-mux-host.exe', 'extension\manifest.json', 'setup\install.ps1', 'setup\uninstall.ps1', 'LICENSE', 'THIRD_PARTY.md', 'README.md')) {
        if (-not (Test-Path -LiteralPath (Join-Path $resolvedReleaseRoot $required))) {
            throw "The release is missing $required."
        }
    }
    foreach ($excluded in @('CONTRIBUTING.md', 'docs', 'host', 'scripts', 'extension\src')) {
        if (Test-Path -LiteralPath (Join-Path $resolvedReleaseRoot $excluded)) {
            throw "The release must not include $excluded."
        }
    }

    Assert-MarkdownLinks -Path (Join-Path $resolvedReleaseRoot 'README.md') -Root $resolvedReleaseRoot -Scope 'the release'
}

Write-Host 'Release policy validated.'
