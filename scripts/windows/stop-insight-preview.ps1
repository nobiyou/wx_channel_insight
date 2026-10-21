[CmdletBinding()]
param([string]$PackageRoot = "")

$ErrorActionPreference = "Stop"
if ([string]::IsNullOrWhiteSpace($PackageRoot)) {
    $PackageRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..\..")).Path
} else {
    $PackageRoot = (Resolve-Path -LiteralPath $PackageRoot).Path
}

$statePath = Join-Path $PackageRoot "data\preview-runtime.json"
if (-not (Test-Path -LiteralPath $statePath)) {
    Write-Output "Insight preview is not running (state file not found)."
    return
}

$state = Get-Content -Raw -LiteralPath $statePath | ConvertFrom-Json
$stopped = @()
foreach ($entry in @(
    [pscustomobject]@{ role = "scheduler"; id = [int]$state.schedulerProcessId; path = [string]$state.schedulerExecutable },
    [pscustomobject]@{ role = "api"; id = [int]$state.apiProcessId; path = [string]$state.apiExecutable }
)) {
    if ($entry.id -le 0) { continue }
    $process = Get-Process -Id $entry.id -ErrorAction SilentlyContinue
    if ($null -eq $process) { continue }
    $processPath = ""
    try { $processPath = [string]$process.Path } catch { }
    $expected = Join-Path $PackageRoot "bin\insight-$($entry.role).exe"
    if (-not $processPath -or ([IO.Path]::GetFullPath($entry.path) -ne $expected) -or
        ([IO.Path]::GetFullPath($processPath) -ne $expected)) {
        Write-Warning "skip $($entry.role) pid $($entry.id): executable path changed"
        continue
    }
    $startedAt = $state.($entry.role + 'StartedAt')
    $sameStart = $false
    if ($startedAt) {
        try {
            if ($startedAt -is [DateTime]) { $recordedTime = $startedAt }
            else { $recordedTime = [DateTime]::Parse([string]$startedAt, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::RoundtripKind) }
            $sameStart = $process.StartTime.ToUniversalTime().Ticks -eq $recordedTime.ToUniversalTime().Ticks
        } catch { $sameStart = $false }
    }
    if (-not $sameStart) {
        Write-Warning "skip $($entry.role) pid $($entry.id): process start time could not be verified"
        continue
    }
    Stop-Process -Id $entry.id -Force
    $stopped += "$($entry.role):$($entry.id)"
}

Remove-Item -LiteralPath $statePath -Force -ErrorAction SilentlyContinue
Write-Output ("Stopped Insight preview processes: " + $(if ($stopped.Count) { $stopped -join ", " } else { "none" }))
