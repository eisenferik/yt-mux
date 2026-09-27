[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$temporaryParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
$fixtureRoot = Join-Path $temporaryParent ('yt-mux-install-test-' + [guid]::NewGuid().ToString('N'))
$installer = Join-Path $PSScriptRoot '..\setup\install.ps1'
$parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($installer, [ref] $null, [ref] $parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
foreach ($definition in $ast.FindAll({
    param($node)
    $node -is [Management.Automation.Language.FunctionDefinitionAst] -and
    $node.Name -in @('Assert-PathWithinInstallRoot', 'Write-JsonFile', 'Invoke-InstallTransaction', 'Write-DefaultSettings', 'Get-ExtensionOrigin')
}, $false)) {
    . ([scriptblock]::Create($definition.Extent.Text))
}

# Inject failures into real filesystem operations; never access the user's registry.
function Copy-Item {
    param([string] $LiteralPath, [string] $Destination, [switch] $Force)
    if ($LiteralPath.StartsWith($stageRoot + '\', [StringComparison]::OrdinalIgnoreCase)) {
        $script:copyCount++
        if ($script:copyCount -eq $failAt) {
            [IO.File]::WriteAllText($Destination, 'partial copy')
            throw 'Injected installation I/O failure'
        }
    }
    if ($failBackup -and $Destination.StartsWith($backupRoot + '\', [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Injected backup I/O failure'
    }
    if ($failRestore -and $LiteralPath.StartsWith($backupRoot + '\', [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Injected restore I/O failure'
    }
    Microsoft.PowerShell.Management\Copy-Item -LiteralPath $LiteralPath -Destination $Destination -Force:$Force
}

try {
    $repositoryRoot = Join-Path $PSScriptRoot '..'
    $hostSource = Get-Content -LiteralPath (Join-Path $repositoryRoot 'host\internal\host\app.go') -Raw
    if ($hostSource -notmatch 'const ExpectedOrigin = "([^"]+)"') { throw 'The host no longer declares ExpectedOrigin.' }
    $installedOrigin = Get-ExtensionOrigin -ManifestPath (Join-Path $repositoryRoot 'extension\manifest.json')
    if ($installedOrigin -ne $Matches[1]) {
        throw "Setup would allow $installedOrigin, but the host only accepts $($Matches[1])."
    }

    $files = @(
        'bin\deno.exe', 'bin\ffmpeg.exe', 'bin\ffprobe.exe', 'bin\yt-dlp.exe', 'bin\yt-mux-host.exe',
        'extension\manifest.json',
        'extension\nested\new.js', 'native\io.yt_mux.host.json'
    )
    $cases = @(@{ Name = 'success' }, @{ Name = 'backup'; Backup = $true },
        @{ Name = 'registration'; Registration = $true },
        @{ Name = 'restore'; Registration = $true; Restore = $true })
    for ($index = 1; $index -le $files.Count; $index++) {
        $cases += @{ Name = "copy-$index"; FailAt = $index }
    }
    foreach ($case in $cases) {
        $transactionRoot = Join-Path $fixtureRoot $case.Name
        $workRoot = Join-Path $transactionRoot 'tmp\setup-test'
        $stageRoot = Join-Path $workRoot 'stage'
        $backupRoot = Join-Path $workRoot 'backup'
        $failAt = if ($case.ContainsKey('FailAt')) { $case.FailAt } else { 0 }
        $failBackup = $case.ContainsKey('Backup')
        $failRestore = $case.ContainsKey('Restore')
        $failRegistration = $case.ContainsKey('Registration')
        $script:copyCount = 0
        $script:registration = 'old registration'
        foreach ($relative in $files) {
            $source = Join-Path $stageRoot $relative
            New-Item -ItemType Directory -Path (Split-Path -Parent $source) -Force | Out-Null
            [IO.File]::WriteAllText($source, "new $relative")
            if ($relative -ne 'extension\nested\new.js') {
                $target = Join-Path $transactionRoot $relative
                New-Item -ItemType Directory -Path (Split-Path -Parent $target) -Force | Out-Null
                [IO.File]::WriteAllText($target, "old $relative")
            }
        }
        $settings = Join-Path $transactionRoot 'config\settings.json'
        New-Item -ItemType Directory -Path (Split-Path -Parent $settings) -Force | Out-Null
        [IO.File]::WriteAllText($settings, '{"cookiesEnabled":true,"destination":"custom"}')
        $userFile = Join-Path $transactionRoot 'extension\notes.txt'
        [IO.File]::WriteAllText($userFile, 'user content')
        $caught = $null
        try {
            # The transaction and its path guards must use the explicit install root.
            $resolvedInstallRoot = Join-Path $fixtureRoot 'unused-outer-root'
            Invoke-InstallTransaction -InstallRoot $transactionRoot -StageRoot $stageRoot -BackupRoot $backupRoot -Register {
                $script:registration = 'new registration'
                if ($failRegistration) { throw 'Injected registration failure' }
            } -RestoreRegistration { $script:registration = 'old registration' }
        } catch { $caught = $_ }
        $success = $case.Name -eq 'success'
        if ($success -eq ($null -ne $caught)) { throw "Unexpected outcome: $($case.Name): $caught" }
        if ($failRestore) {
            if (-not $caught.Exception.Data.Contains('PreserveSetup')) { throw 'Recovery data would be deleted.' }
            if (-not (Test-Path -LiteralPath (Join-Path $workRoot 'rollback-files.json'))) { throw 'Recovery inventory is missing.' }
            foreach ($relative in $files | Where-Object { $_ -ne 'extension\nested\new.js' }) {
                if ([IO.File]::ReadAllText((Join-Path $backupRoot $relative)) -ne "old $relative") {
                    throw "Recovery backup changed: $relative"
                }
            }
        } else {
            foreach ($relative in $files) {
                $target = Join-Path $transactionRoot $relative
                if (-not $success -and $relative -eq 'extension\nested\new.js') {
                    if (Test-Path -LiteralPath $target) { throw 'New file survived rollback.' }
                    continue
                }
                $expected = if ($success) { "new $relative" } else { "old $relative" }
                if ([IO.File]::ReadAllText($target) -ne $expected) { throw "Incorrect file after $($case.Name): $relative" }
            }
        }
        $expectedRegistration = if ($success) { 'new registration' } else { 'old registration' }
        if ($script:registration -ne $expectedRegistration) { throw 'Registration was not restored.' }
        if ([IO.File]::ReadAllText($settings) -ne '{"cookiesEnabled":true,"destination":"custom"}' -or
            [IO.File]::ReadAllText($userFile) -ne 'user content') { throw 'User data changed.' }
    }

    $settingsCases = @(
        @{ Name = 'absent'; Existing = $null },
        @{ Name = 'present'; Existing = '{"destination":"D:/Videos"}' },
        @{ Name = 'unreadable'; Existing = '{invalid' }
    )
    foreach ($case in $settingsCases) {
        $caseRoot = Join-Path (Join-Path $fixtureRoot 'defaults') $case.Name
        $installed = Join-Path (Join-Path $caseRoot 'config') 'settings.json'
        $staged = Join-Path (Join-Path $caseRoot 'stage') 'settings.json'
        $destination = Join-Path (Join-Path $caseRoot 'Videos') 'yt-mux'
        New-Item -ItemType Directory -Path (Split-Path -Parent $installed) -Force | Out-Null
        New-Item -ItemType Directory -Path (Split-Path -Parent $staged) -Force | Out-Null
        if ($null -ne $case.Existing) { [IO.File]::WriteAllText($installed, $case.Existing) }

        Write-DefaultSettings -SettingsPath $installed -StagePath $staged -Destination $destination

        if ($case.Name -eq 'absent') {
            if (-not (Test-Path -LiteralPath $staged -PathType Leaf)) { throw 'Default settings were not staged.' }
            $written = Get-Content -LiteralPath $staged -Raw | ConvertFrom-Json
            if ($written.destination -ne $destination) { throw 'Staged settings name the wrong destination.' }
            if ($written.defaultPreset -ne 'archive') { throw 'Staged settings name the wrong preset.' }
            if (-not (Test-Path -LiteralPath $destination -PathType Container)) { throw 'The download folder was not created.' }
            $env:YT_MUX_SETUP_SETTINGS_PATH = $staged
            try {
                $hostCheck = & go test -C (Join-Path $repositoryRoot 'host') -count=1 -v -run '^TestSetupDefaultsLoad$' ./internal/config
            } finally {
                Remove-Item Env:\YT_MUX_SETUP_SETTINGS_PATH
            }
            if ($LASTEXITCODE -ne 0 -or -not ($hostCheck -match '^--- PASS: TestSetupDefaultsLoad')) {
                throw "The host does not load the default settings:`n$($hostCheck -join "`n")"
            }
            continue
        }
        if (Test-Path -LiteralPath $staged) { throw "Existing settings were replaced: $($case.Name)" }
        if ([IO.File]::ReadAllText($installed) -ne $case.Existing) { throw "Existing settings were rewritten: $($case.Name)" }
        if (Test-Path -LiteralPath $destination) { throw "A download folder was created beside existing settings: $($case.Name)" }
    }
} finally {
    $resolvedFixture = [IO.Path]::GetFullPath($fixtureRoot)
    if ([IO.Path]::GetDirectoryName($resolvedFixture) -ne $temporaryParent -or
        [IO.Path]::GetFileName($resolvedFixture) -notmatch '^yt-mux-install-test-[0-9a-f]{32}$') {
        throw 'Refusing to clean an unexpected fixture path.'
    }
    if (Test-Path -LiteralPath $resolvedFixture) { Remove-Item -LiteralPath $resolvedFixture -Recurse -Force }
}
Write-Host "Installer transaction checks passed ($($cases.Count) cases)."
Write-Host "Default settings checks passed ($($settingsCases.Count) cases)."
