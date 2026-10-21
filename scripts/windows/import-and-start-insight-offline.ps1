[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [string]$CommentDataDir = "",
    [switch]$SkipLoad,
    [switch]$Recreate
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
    $RepoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..\..")).Path
} else {
    $RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
}
$composeFile = Join-Path $RepoRoot "docker-compose.yml"
$envFile = Join-Path $RepoRoot '.env'
$imageArchive = Join-Path $RepoRoot "images\insight-runtime-images.tar"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is not available. Install Docker Desktop and retry."
}
docker info 1>$null 2>$null
if ($LASTEXITCODE -ne 0) {
    throw "Docker engine is not running. Start Docker Desktop and retry."
}
if (-not (Test-Path -LiteralPath $composeFile)) { throw "offline Compose file not found: $composeFile" }
if (-not (Test-Path -LiteralPath $imageArchive) -and -not $SkipLoad.IsPresent) {
    throw "runtime image archive not found: $imageArchive"
}

if (-not (Test-Path -LiteralPath $envFile -PathType Leaf)) { throw 'Copy .env.example to .env and configure the required secrets before starting.' }

if ([string]::IsNullOrWhiteSpace($CommentDataDir)) {
    $CommentDataDir = Join-Path $RepoRoot "data\comment_data"
}
if (-not (Test-Path -LiteralPath $CommentDataDir)) {
    New-Item -ItemType Directory -Force -Path $CommentDataDir | Out-Null
}
$env:INSIGHT_WX_CHANNEL_COMMENT_DATA_DIR = (Resolve-Path -LiteralPath $CommentDataDir).Path

& docker compose --env-file $envFile -f $composeFile config --quiet
if ($LASTEXITCODE -ne 0) { throw "offline Compose configuration is invalid" }

if (-not $SkipLoad.IsPresent) {
    Write-Host "Importing Insight runtime images..."
    & docker load --input $imageArchive
    if ($LASTEXITCODE -ne 0) { throw "docker load failed with exit code $LASTEXITCODE" }
}

$composeArgs = @("compose", "--env-file", $envFile, "-f", $composeFile, "up", "-d", "--pull", "never")
if ($Recreate.IsPresent) { $composeArgs += "--force-recreate" }
Write-Host "Starting Insight offline Docker stack..."
& docker @composeArgs
if ($LASTEXITCODE -ne 0) { throw "docker compose up failed with exit code $LASTEXITCODE" }

Write-Host ""
Write-Host "Insight is starting:"
Write-Host "  Web: http://127.0.0.1:5176/"
Write-Host "  API: http://127.0.0.1:18081/api/healthz"
Write-Host "  Check: docker compose -f `"$composeFile`" ps"
Write-Host "  Logs: docker compose -f `"$composeFile`" logs -f scheduler"
