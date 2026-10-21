[CmdletBinding()]
param(
    [string]$PackageRoot = '', [string]$ConfigFile = '',
    [string]$WxChannelBaseUrl = '', [string]$WxChannelCommentDataRoot = '',
    [string]$APIAddr = '', [string]$HubUserId = '', [int]$DeviceCount = 1,
    [switch]$NoBrowser, [switch]$EmitOnly
)
$ErrorActionPreference = 'Stop'
if (-not $PackageRoot) { $PackageRoot = Join-Path $PSScriptRoot '..\..' }
$root = (Resolve-Path -LiteralPath $PackageRoot).Path
$explicitConfig = $PSBoundParameters.ContainsKey('ConfigFile')
if (-not $ConfigFile) { $ConfigFile = Join-Path $root 'insight.local.json' }
elseif (-not [IO.Path]::IsPathRooted($ConfigFile)) { $ConfigFile = Join-Path $root $ConfigFile }
$ConfigFile = [IO.Path]::GetFullPath($ConfigFile)
$readPath = $ConfigFile
if (-not (Test-Path -LiteralPath $readPath -PathType Leaf)) {
    if ($explicitConfig) { throw "Configuration file not found: $ConfigFile" }
    $readPath = Join-Path $root 'config.example.json'
}
if (-not (Test-Path -LiteralPath $readPath -PathType Leaf)) { throw "Configuration template not found: $readPath" }
if ((Get-Item -LiteralPath $readPath).Length -gt 1MB) { throw 'Configuration exceeds 1 MiB.' }
try { $config = Get-Content -LiteralPath $readPath -Raw | ConvertFrom-Json }
catch { throw "Invalid JSON configuration: $readPath" }

# Only explicit command-line overrides are added. File/env resolution belongs to Go.
$envVars = [ordered]@{ INSIGHT_CONFIG_FILE = $ConfigFile }
$overrides = @{
    APIAddr = 'INSIGHT_API_ADDR'; HubUserId = 'INSIGHT_DEFAULT_HUB_USER_ID'; DeviceCount = 'INSIGHT_DEFAULT_DEVICE_COUNT'
    WxChannelBaseUrl = 'INSIGHT_WX_CHANNEL_BASE_URL'; WxChannelCommentDataRoot = 'INSIGHT_WX_COMMENT_DATA_ROOT'
}
foreach ($key in $overrides.Keys) {
    if ($PSBoundParameters.ContainsKey($key)) { $envVars[$overrides[$key]] = [string]$PSBoundParameters[$key] }
}
function Get-Effective([string]$Name, [object]$FileValue, [string]$Fallback) {
    if ($envVars.Contains($Name)) { return [string]$envVars[$Name] }
    $value = [Environment]::GetEnvironmentVariable($Name, 'Process')
    if ($null -ne $value) { return $value }
    if ($null -ne $FileValue -and [string]$FileValue -ne '') { return [string]$FileValue }
    return $Fallback
}
$addr = Get-Effective 'INSIGHT_API_ADDR' $config.server.addr '127.0.0.1:18081'
if ($addr -notmatch '^(.*):(\d+)$') { throw 'API address must contain a host and port.' }
$apiPort = [int]$Matches[2]
if ($apiPort -lt 1 -or $apiPort -gt 65535) { throw 'API port must be between 1 and 65535.' }
$apiHost = $Matches[1]
if ($apiHost -in @('', '0.0.0.0', '[::]')) { $apiHost = '127.0.0.1' }
$apiURL = "http://${apiHost}:$apiPort"
$identity = Get-Effective 'INSIGHT_DEFAULT_HUB_USER_ID' $config.runtime.default_hub_user_id 'insight:operator:local'
$devices = Get-Effective 'INSIGHT_DEFAULT_DEVICE_COUNT' $config.runtime.default_device_count '1'
$browserURL = "$apiURL/"
$localOperator = Get-Effective 'INSIGHT_LOCAL_OPERATOR' $config.bridge.local_operator 'false'
if ($localOperator -match '^(true|1|yes|on)$') {
    $browserURL += "?header_hub_user_id=$([uri]::EscapeDataString($identity))&header_device_count=$devices"
}
$envVars['INSIGHT_SCHEDULER_API_BASE_URL'] = Get-Effective 'INSIGHT_SCHEDULER_API_BASE_URL' $config.scheduler.api_base_url $apiURL
if ($PSBoundParameters.ContainsKey('APIAddr')) { $envVars['INSIGHT_SCHEDULER_API_BASE_URL'] = $apiURL }
$apiExecutable = Join-Path $root 'bin\insight-api.exe'
$schedulerExecutable = Join-Path $root 'bin\insight-scheduler.exe'
if ($EmitOnly) {
    [pscustomobject]@{ packageRoot = $root; configFile = $ConfigFile; apiURL = $apiURL; webURL = "$apiURL/"
        browserURL = $browserURL; apiExecutable = $apiExecutable; schedulerExecutable = $schedulerExecutable
        browser = (-not $NoBrowser); env = $envVars } | ConvertTo-Json -Depth 5
    return
}
foreach ($file in @($apiExecutable, $schedulerExecutable, (Join-Path $root 'web\index.html'))) {
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Package is incomplete; missing $file" }
}
# Do not attach to a pre-existing API or truncate its logs.
$client = [Net.Sockets.TcpClient]::new()
try {
    $connection = $client.ConnectAsync($apiHost.Trim('[', ']'), $apiPort)
    try { $busy = $connection.Wait(1000) -and $client.Connected } catch { $busy = $false }
} finally { $client.Dispose() }
if ($busy) { throw "API address is already in use: $addr" }
if ($readPath -ne $ConfigFile) { Copy-Item -LiteralPath $readPath -Destination $ConfigFile -ErrorAction Stop }
$logRoot = Join-Path $root 'data\logs'
New-Item -ItemType Directory -Path $logRoot -Force | Out-Null
$statePath = Join-Path $root 'data\preview-runtime.json'
$previousEnv = @{}
$apiProcess = $null
$schedulerProcess = $null
try {
    foreach ($entry in $envVars.GetEnumerator()) {
        $previousEnv[$entry.Key] = [Environment]::GetEnvironmentVariable($entry.Key, 'Process')
        [Environment]::SetEnvironmentVariable($entry.Key, [string]$entry.Value, 'Process')
    }
    $apiLog = Join-Path $logRoot 'api.log'
    $apiErrorLog = Join-Path $logRoot 'api-error.log'
    $apiProcess = Start-Process -FilePath $apiExecutable -WorkingDirectory $root -WindowStyle Hidden -RedirectStandardOutput $apiLog -RedirectStandardError $apiErrorLog -PassThru
    $ready = $false
    $deadline = (Get-Date).AddSeconds(45)
    while ((Get-Date) -lt $deadline) {
        if ($apiProcess.HasExited) { throw "API exited with code $($apiProcess.ExitCode); see $apiErrorLog" }
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri "$apiURL/api/healthz" -TimeoutSec 2
            if ($response.StatusCode -eq 200) { $ready = $true; break }
        } catch { }
        Start-Sleep -Milliseconds 300
    }
    if (-not $ready) { throw "API did not become ready within 45 seconds; see $apiErrorLog" }
    $schedulerProcess = Start-Process -FilePath $schedulerExecutable -WorkingDirectory $root -WindowStyle Hidden -RedirectStandardOutput (Join-Path $logRoot 'scheduler.log') -RedirectStandardError (Join-Path $logRoot 'scheduler-error.log') -PassThru
    Start-Sleep -Milliseconds 500
    if ($apiProcess.HasExited -or $schedulerProcess.HasExited) { throw "Runtime exited during startup; see $logRoot" }
    $state = [ordered]@{
        status = 'ready'; startedAt = (Get-Date).ToUniversalTime().ToString('o'); packageRoot = $root; configFile = $ConfigFile
        apiURL = $apiURL; webURL = "$apiURL/"; browserURL = $browserURL
        apiProcessId = $apiProcess.Id; schedulerProcessId = $schedulerProcess.Id
        apiExecutable = $apiExecutable; schedulerExecutable = $schedulerExecutable
        apiStartedAt = $apiProcess.StartTime.ToUniversalTime().ToString('o')
        schedulerStartedAt = $schedulerProcess.StartTime.ToUniversalTime().ToString('o')
    }
    [IO.File]::WriteAllText($statePath, ($state | ConvertTo-Json -Depth 5), [Text.UTF8Encoding]::new($false))
    if (-not $NoBrowser) { Start-Process $browserURL }
    $state | ConvertTo-Json -Depth 5
} catch {
    foreach ($process in @($schedulerProcess, $apiProcess)) {
        if ($null -ne $process -and -not $process.HasExited) { $process.Kill() }
    }
    throw
} finally {
    foreach ($entry in $previousEnv.GetEnumerator()) { [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'Process') }
}
