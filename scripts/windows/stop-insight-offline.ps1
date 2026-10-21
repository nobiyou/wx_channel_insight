[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [switch]$RemoveData
)

$ErrorActionPreference = "Stop"
if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
    $RepoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..\..")).Path
} else {
    $RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
}
$composeFile = Join-Path $RepoRoot "docker-compose.yml"
if (-not (Test-Path -LiteralPath $composeFile)) { throw "offline Compose file not found: $composeFile" }
$args = @("compose", "--env-file", (Join-Path $RepoRoot '.env'), "-f", $composeFile, "down")
if ($RemoveData.IsPresent) { $args += "-v" }
& docker @args
if ($LASTEXITCODE -ne 0) { throw "docker compose down failed with exit code $LASTEXITCODE" }
