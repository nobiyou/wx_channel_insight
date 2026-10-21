[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [string]$ComposeFile = "",
    [switch]$RemoveVolumes
)

$ErrorActionPreference = "Stop"

function Resolve-DefaultRepoRoot {
    $path = Join-Path $PSScriptRoot "..\.."
    return (Resolve-Path -LiteralPath $path).Path
}

if (-not $RepoRoot) {
    $RepoRoot = Resolve-DefaultRepoRoot
} else {
    $RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
}

if (-not $ComposeFile) {
    $ComposeFile = Join-Path $RepoRoot "deploy\stack\docker-compose.yml"
}
$ComposeFile = (Resolve-Path -LiteralPath $ComposeFile).Path

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is not available."
}

$composeArgs = @("compose", "-f", $ComposeFile, "down")
if ($RemoveVolumes) {
    $composeArgs += "-v"
}

Write-Host "Stopping Insight Docker stack: $ComposeFile"
& docker @composeArgs
if ($LASTEXITCODE -ne 0) {
    throw "docker compose down failed with exit code $LASTEXITCODE"
}

Write-Host "Stopped."