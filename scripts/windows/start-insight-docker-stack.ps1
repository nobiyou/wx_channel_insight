[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [string]$ComposeFile = "",
    [string]$WxChannelBaseUrl = "http://host.docker.internal:2026",
    [string]$WxChannelCommentDataDir = "",
    [string]$HubUserId = "insight:operator:local",
    [int]$DeviceCount = 1,
    [switch]$Build,
    [switch]$Recreate,
    [switch]$SkipPortCheck,
    [switch]$EmitOnly
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
$ComposeEnvFile = Join-Path (Split-Path -Parent $ComposeFile) ".env"

$composeArgs = @(
    "compose",
    $(if (Test-Path -LiteralPath $ComposeEnvFile) { "--env-file" } else { $null }),
    $(if (Test-Path -LiteralPath $ComposeEnvFile) { $ComposeEnvFile } else { $null }),
    "-f", $ComposeFile,
    "up",
    "-d"
)
$composeArgs = @($composeArgs | Where-Object { $null -ne $_ -and $_ -ne "" })
if ($Build -or $Recreate) {
    $composeArgs += "--build"
}
if ($Recreate) {
    $composeArgs += "--force-recreate"
}

$mirror = $env:INSIGHT_DOCKER_MIRROR
$stackEnv = [ordered]@{}
# Compose owns defaults and .env parsing. Only explicit parameters override it.
if ($PSBoundParameters.ContainsKey('WxChannelBaseUrl')) { $stackEnv.INSIGHT_WX_CHANNEL_BASE_URL = $WxChannelBaseUrl }
if ($PSBoundParameters.ContainsKey('HubUserId')) {
    $stackEnv.INSIGHT_DEFAULT_HUB_USER_ID = $HubUserId
    $stackEnv.INSIGHT_SCHEDULER_HUB_USER_ID = $HubUserId
}
if ($PSBoundParameters.ContainsKey('DeviceCount')) {
    $stackEnv.INSIGHT_DEFAULT_DEVICE_COUNT = "$DeviceCount"
    $stackEnv.INSIGHT_SCHEDULER_DEVICE_COUNT = "$DeviceCount"
}
if ($mirror) {
    $stackEnv.INSIGHT_BUILD_GO_IMAGE = "$mirror/golang:1.25-alpine"
    $stackEnv.INSIGHT_BUILD_RUNTIME_IMAGE = "$mirror/alpine:3.21"
    $stackEnv.INSIGHT_BUILD_NODE_IMAGE = "$mirror/node:22-alpine"
    $stackEnv.INSIGHT_BUILD_NGINX_IMAGE = "$mirror/nginx:1.27-alpine"
    $stackEnv.INSIGHT_POSTGRES_IMAGE = "$mirror/postgres:16-alpine"
    $stackEnv.INSIGHT_REDIS_IMAGE = "$mirror/redis:7-alpine"
}
if (-not [string]::IsNullOrWhiteSpace($WxChannelCommentDataDir)) {
    $resolvedCommentDataDir = Resolve-Path -LiteralPath $WxChannelCommentDataDir -ErrorAction SilentlyContinue
    if ($null -eq $resolvedCommentDataDir) {
        if (-not $EmitOnly) { New-Item -ItemType Directory -Force -Path $WxChannelCommentDataDir | Out-Null }
        $stackEnv.INSIGHT_WX_CHANNEL_COMMENT_DATA_DIR = [IO.Path]::GetFullPath($WxChannelCommentDataDir)
    } else {
        $stackEnv.INSIGHT_WX_CHANNEL_COMMENT_DATA_DIR = $resolvedCommentDataDir.Path
    }
}

if ($EmitOnly.IsPresent) {
    $browserURL = "http://127.0.0.1:5176/"
    [pscustomobject]@{
        repoRoot = $RepoRoot
        composeFile = $ComposeFile
        composeEnvFile = $(if (Test-Path -LiteralPath $ComposeEnvFile) { $ComposeEnvFile } else { $null })
        wxChannelBaseUrl = $WxChannelBaseUrl
        wxChannelCommentDataDir = $WxChannelCommentDataDir
        hubUserId = $HubUserId
        deviceCount = $DeviceCount
        build = $Build.IsPresent
        recreate = $Recreate.IsPresent
        skipPortCheck = $SkipPortCheck.IsPresent
        composeCommand = "docker"
        composeArguments = $composeArgs
        env = $stackEnv
        ports = [pscustomobject]@{
            web = 5176
            api = 18081
            postgres = 15432
            redis = 16379
        }
        checks = [pscustomobject]@{
            webURL = "http://127.0.0.1:5176/"
            browserURL = $browserURL
            apiHealthURL = "http://127.0.0.1:18081/api/healthz"
            edgeWSURL = "ws://127.0.0.1:18081/ws/client"
        }
    } | ConvertTo-Json -Depth 6
    return
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is not available. Install Docker Desktop and retry."
}

docker info 1>$null 2>$null
if ($LASTEXITCODE -ne 0) {
    throw "Docker engine is not running. Start Docker Desktop and retry."
}

if (-not $SkipPortCheck) {
    foreach ($port in @(5176, 18081, 15432, 16379)) {
        $listener = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($listener) {
            Write-Warning "Port $port is already in use by PID $($listener.OwningProcess). Stack may fail to bind; stop the process or use -SkipPortCheck."
        }
    }
}

$previousEnv = @{}
foreach ($entry in $stackEnv.GetEnumerator()) {
    $previousEnv[$entry.Key] = [Environment]::GetEnvironmentVariable($entry.Key, 'Process')
    [Environment]::SetEnvironmentVariable([string]$entry.Key, [string]$entry.Value, "Process")
}
Write-Host "Starting Insight Docker stack..."
Write-Host "  compose: $ComposeFile"
Write-Host "  settings: Compose environment with explicit script overrides"
if ($stackEnv.Contains("INSIGHT_WX_CHANNEL_COMMENT_DATA_DIR")) {
    Write-Host "  comment data: $($stackEnv.INSIGHT_WX_CHANNEL_COMMENT_DATA_DIR)"
}
try {
    $configArgs = @('compose')
    if (Test-Path -LiteralPath $ComposeEnvFile) { $configArgs += @('--env-file', $ComposeEnvFile) }
    & docker @configArgs -f $ComposeFile config --quiet
    if ($LASTEXITCODE -ne 0) { throw 'Docker Compose configuration is invalid; prepare deploy/stack/.env first.' }
    & docker @composeArgs
    if ($LASTEXITCODE -ne 0) { throw "docker compose up failed with exit code $LASTEXITCODE" }
} finally {
    foreach ($entry in $previousEnv.GetEnumerator()) { [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'Process') }
}

Write-Host ""
Write-Host "Stack is up (or still starting). Useful checks:"
Write-Host "  Web UI : http://127.0.0.1:5176/"
Write-Host "  API    : http://127.0.0.1:18081/api/healthz"
Write-Host "  Edge WS: ws://127.0.0.1:18081/ws/client"
Write-Host "  Postgres: 127.0.0.1:15432  Redis: 127.0.0.1:16379"
Write-Host "  Scheduler logs: docker compose -f `"$ComposeFile`" logs -f scheduler"
Write-Host ""
Write-Host "Optional jobs one-shot:"
Write-Host "  docker compose -f `"$ComposeFile`" run --rm jobs status"
Write-Host ""
Write-Host "Stop:"
Write-Host "  powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows\stop-insight-docker-stack.ps1 -RepoRoot ."
