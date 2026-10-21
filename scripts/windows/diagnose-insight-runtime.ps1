[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [string]$APIBaseURL = "http://127.0.0.1:18081",
    [string]$WebBaseURL = "http://127.0.0.1:5176",
    [string]$WxChannelBaseURL = "http://127.0.0.1:2026",
    [int]$TimeoutSeconds = 3,
    [switch]$EmitOnly
)

$ErrorActionPreference = "Stop"

function Resolve-DefaultRepoRoot {
    $path = Join-Path $PSScriptRoot "..\.."
    return (Resolve-Path -LiteralPath $path).Path
}

function Normalize-BaseURL {
    param([string]$Value)
    return ([string]$Value).Trim().TrimEnd("/")
}

function Resolve-PortFromURL {
    param(
        [string]$URL,
        [int]$Fallback
    )
    try {
        $uri = [System.Uri]$URL
        if ($uri.Port -gt 0) { return [int]$uri.Port }
    }
    catch {
    }
    return $Fallback
}

function Get-ListenerState {
    param(
        [string]$Name,
        [int]$Port
    )

    $listeners = @(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
    $items = @()
    foreach ($listener in $listeners) {
        $processName = ""
        try {
            $processName = (Get-Process -Id $listener.OwningProcess -ErrorAction Stop).ProcessName
        }
        catch {
        }
        $items += [pscustomobject]@{
            localAddress = [string]$listener.LocalAddress
            localPort = [int]$listener.LocalPort
            owningProcess = [int]$listener.OwningProcess
            processName = $processName
        }
    }

    return [pscustomobject]@{
        name = $Name
        port = $Port
        listening = ($items.Count -gt 0)
        listeners = $items
    }
}

function Invoke-HTTPProbe {
    param(
        [string]$Name,
        [string]$URL,
        [int]$TimeoutSeconds
    )

    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $URL -TimeoutSec $TimeoutSeconds
        $content = [string]$response.Content
        return [pscustomobject]@{
            name = $Name
            url = $URL
            ok = $true
            statusCode = [int]$response.StatusCode
            contentPreview = $content.Substring(0, [Math]::Min(500, $content.Length))
            error = ""
        }
    }
    catch {
        $statusCode = 0
        if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
            $statusCode = [int]$_.Exception.Response.StatusCode
        }
        return [pscustomobject]@{
            name = $Name
            url = $URL
            ok = $false
            statusCode = $statusCode
            contentPreview = ""
            error = [string]$_.Exception.Message
        }
    }
}

function Resolve-ToolPath {
    param(
        [string]$Name,
        [string[]]$KnownPathPatterns = @()
    )

    $paths = @()
    $candidates = @($Name)
    if (-not $Name.EndsWith(".exe")) { $candidates += "$Name.exe" }

    foreach ($candidate in $candidates) {
        $cmd = Get-Command $candidate -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($cmd -and $cmd.Source) { $paths += [string]$cmd.Source }
    }

    foreach ($pattern in $KnownPathPatterns) {
        $matches = @(Get-ChildItem -Path $pattern -ErrorAction SilentlyContinue | Select-Object -First 3)
        foreach ($match in $matches) { $paths += [string]$match.FullName }
    }

    $uniquePaths = @($paths | Where-Object { $_ } | Select-Object -Unique)
    return [pscustomobject]@{
        name = $Name
        available = ($uniquePaths.Count -gt 0)
        paths = $uniquePaths
    }
}

function Resolve-DockerState {
    $docker = Get-Command docker -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $docker) {
        return [pscustomobject]@{
            available = $false
            engineReady = $false
            command = ""
            error = "docker command is not available"
        }
    }

    try {
        $output = & docker info 2>&1
        $exitCode = $LASTEXITCODE
        return [pscustomobject]@{
            available = $true
            engineReady = ($exitCode -eq 0)
            command = [string]$docker.Source
            error = $(if ($exitCode -eq 0) { "" } else { ConvertTo-CompactMessage -Value (($output | Out-String).Trim()) })
        }
    }
    catch {
        return [pscustomobject]@{
            available = $true
            engineReady = $false
            command = [string]$docker.Source
            error = ConvertTo-CompactMessage -Value $_.Exception.Message
        }
    }
}

function Resolve-DependencyChecks {
    return [pscustomobject]@{
        docker = Resolve-DockerState
        postgres = [pscustomobject]@{
            pgCtl = Resolve-ToolPath -Name "pg_ctl" -KnownPathPatterns @("C:\Program Files\PostgreSQL\*\bin\pg_ctl.exe")
            initdb = Resolve-ToolPath -Name "initdb" -KnownPathPatterns @("C:\Program Files\PostgreSQL\*\bin\initdb.exe")
            psql = Resolve-ToolPath -Name "psql" -KnownPathPatterns @("C:\Program Files\PostgreSQL\*\bin\psql.exe")
        }
        redis = [pscustomobject]@{
            server = Resolve-ToolPath -Name "redis-server" -KnownPathPatterns @("C:\Program Files\Redis\redis-server.exe")
            cli = Resolve-ToolPath -Name "redis-cli" -KnownPathPatterns @("C:\Program Files\Redis\redis-cli.exe")
        }
    }
}

function Resolve-RuntimeBlockers {
    param(
        [object[]]$Ports,
        [object[]]$Probes
    )

    $items = @()
    foreach ($portName in @("wx-channel", "insight-api")) {
        $state = $Ports | Where-Object { $_.name -eq $portName } | Select-Object -First 1
        if ($state -and -not $state.listening) { $items += "$portName-port-not-listening:$($state.port)" }
    }

    foreach ($probe in @($Probes | Where-Object { -not $_.ok })) {
        $items += "$($probe.name)-probe-failed:$($probe.error)"
    }
    return $items
}

function ConvertTo-CompactMessage {
    param(
        [string]$Value,
        [int]$MaxLength = 300
    )

    $text = ([string]$Value) -replace "\s+", " "
    if ($text.Length -le $MaxLength) { return $text }
    return $text.Substring(0, $MaxLength) + "..."
}

function Resolve-OverallState {
    param([object[]]$Probes)

    $wx = $Probes | Where-Object { $_.name -eq "wx-channel-status" } | Select-Object -First 1
    $api = $Probes | Where-Object { $_.name -eq "insight-api-health" } | Select-Object -First 1
    $web = $Probes | Where-Object { $_.name -eq "insight-web-runtime-proxy" } | Select-Object -First 1

    if ($api -and -not $api.ok) { return "blocked-insight-api" }
    if ($web -and -not $web.ok) { return "blocked-web-proxy" }
    if ($wx -and -not $wx.ok) { return "blocked-wx-channel" }
    return "ready"
}

function Resolve-CurrentEvidence {
    param(
        [object[]]$Ports,
        [object[]]$Probes
    )

    $readyPorts = @($Ports | Where-Object { $_.listening })
    $failedProbes = @($Probes | Where-Object { -not $_.ok })
    return [pscustomobject]@{
        ready = ($failedProbes.Count -eq 0)
        listeningPorts = @($readyPorts | ForEach-Object { "$($_.name):$($_.port)" })
        failedProbes = @($failedProbes | ForEach-Object { "$($_.name):$($_.error)" })
    }
}

if (-not $RepoRoot) {
    $RepoRoot = Resolve-DefaultRepoRoot
}
else {
    $RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
}

$APIBaseURL = Normalize-BaseURL $APIBaseURL
$WebBaseURL = Normalize-BaseURL $WebBaseURL
$WxChannelBaseURL = Normalize-BaseURL $WxChannelBaseURL

$apiPort = Resolve-PortFromURL -URL $APIBaseURL -Fallback 18081
$webPort = Resolve-PortFromURL -URL $WebBaseURL -Fallback 5176
$wxPort = Resolve-PortFromURL -URL $WxChannelBaseURL -Fallback 2026

$plannedURLs = [pscustomobject]@{
    wxChannelStatus = "$WxChannelBaseURL/api/channels/status"
    insightAPIHealth = "$APIBaseURL/api/healthz"
    insightRuntimeStatus = "$APIBaseURL/api/v1/runtime/status"
    insightWebRuntimeProxy = "$WebBaseURL/api/v1/runtime/status"
}

if ($EmitOnly.IsPresent) {
    [pscustomobject]@{
        repoRoot = $RepoRoot
        apiBaseURL = $APIBaseURL
        webBaseURL = $WebBaseURL
        wxChannelBaseURL = $WxChannelBaseURL
        timeoutSeconds = $TimeoutSeconds
        plannedURLs = $plannedURLs
        ports = [pscustomobject]@{
            wxProxy = 2025
            wxChannel = $wxPort
            insightWeb = $webPort
            insightAPI = $apiPort
            postgres = 15432
            redis = 16379
        }
        plannedDependencyChecks = [pscustomobject]@{
            docker = "docker info"
            postgresTools = @("pg_ctl", "initdb", "psql")
            redisTools = @("redis-server", "redis-cli")
        }
        mode = "read-only-diagnostic"
    } | ConvertTo-Json -Depth 5
    return
}

$probes = @(
    (Invoke-HTTPProbe -Name "wx-channel-status" -URL $plannedURLs.wxChannelStatus -TimeoutSeconds $TimeoutSeconds),
    (Invoke-HTTPProbe -Name "insight-api-health" -URL $plannedURLs.insightAPIHealth -TimeoutSeconds $TimeoutSeconds),
    (Invoke-HTTPProbe -Name "insight-runtime-status" -URL $plannedURLs.insightRuntimeStatus -TimeoutSeconds $TimeoutSeconds),
    (Invoke-HTTPProbe -Name "insight-web-runtime-proxy" -URL $plannedURLs.insightWebRuntimeProxy -TimeoutSeconds $TimeoutSeconds)
)

$ports = @(
    (Get-ListenerState -Name "wx-proxy" -Port 2025),
    (Get-ListenerState -Name "wx-channel" -Port $wxPort),
    (Get-ListenerState -Name "insight-web" -Port $webPort),
    (Get-ListenerState -Name "insight-api" -Port $apiPort),
    (Get-ListenerState -Name "postgres" -Port 15432),
    (Get-ListenerState -Name "redis" -Port 16379)
)
$dependencyChecks = Resolve-DependencyChecks

[pscustomobject]@{
    repoRoot = $RepoRoot
    state = Resolve-OverallState -Probes $probes
    apiBaseURL = $APIBaseURL
    webBaseURL = $WebBaseURL
    wxChannelBaseURL = $WxChannelBaseURL
    ports = $ports
    probes = $probes
    dependencies = $dependencyChecks
    blockers = Resolve-RuntimeBlockers -Ports $ports -Probes $probes
    currentEvidence = Resolve-CurrentEvidence -Ports $ports -Probes $probes
} | ConvertTo-Json -Depth 8
