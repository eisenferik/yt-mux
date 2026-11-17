[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$fixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ('yt-mux-update-test-' + [guid]::NewGuid().ToString('N'))
$updater = Join-Path $fixtureRoot 'update.ps1'

function Invoke-RestMethod { [pscustomobject] @{ tag_name = 'v1.0.0' } }
# A function called from update.ps1 resolves $script: to update.ps1's scope, so the count is held by reference.
$downloads = [ref] 0
function Invoke-WebRequest { $downloads.Value++; throw 'Injected download stop' }

try {
    New-Item -ItemType Directory -Path (Join-Path $fixtureRoot 'extension') | Out-Null
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot '..\setup\update.ps1') -Destination $updater
    Set-Content -LiteralPath (Join-Path $fixtureRoot 'extension\manifest.json') -Value '{"version":"1.0.0"}'

    & ([scriptblock]::Create((Get-Content -LiteralPath $updater -Raw))) 6>&1 | Out-Null
    if ($downloads.Value -ne 1) {
        throw 'The install command did not reinstall the installed release.'
    }

    $downloads.Value = 0
    & $updater -FromNotification 6>&1 | Out-Null
    if ($downloads.Value -ne 0) {
        throw 'The update from the notification reinstalled the installed release.'
    }
} finally {
    Remove-Item -LiteralPath $fixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host 'Update script checks passed (2 cases).'
