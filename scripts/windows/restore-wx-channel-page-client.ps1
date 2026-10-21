param(
    [string]$RepoRoot = ".",
    [string]$BaseURL = "http://127.0.0.1:2026",
    [string]$ProxyAddress = "127.0.0.1:2025",
    [int]$WaitSeconds = 5,
    [switch]$EmitOnly
)

$ErrorActionPreference = "Stop"

function Resolve-RepoRoot {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value) -or $Value -eq ".") {
        return (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..\..")).Path
    }

    return (Resolve-Path -LiteralPath $Value).Path
}

function Normalize-BaseURL {
    param([string]$Value)

    $trimmed = [string]$Value
    $trimmed = $trimmed.Trim()
    if ([string]::IsNullOrWhiteSpace($trimmed)) {
        return "http://127.0.0.1:2026"
    }
    return $trimmed.TrimEnd("/")
}

function Get-JSONProbe {
    param(
        [string]$URL,
        [int]$TimeoutSeconds = 5
    )

    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $URL -TimeoutSec $TimeoutSeconds
        $payload = $response.Content | ConvertFrom-Json
        return [pscustomobject]@{
            ok = $true
            url = $URL
            statusCode = [int]$response.StatusCode
            raw = $payload
            error = ""
        }
    }
    catch {
        $statusCode = 0
        if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
            $statusCode = [int]$_.Exception.Response.StatusCode
        }
        return [pscustomobject]@{
            ok = $false
            url = $URL
            statusCode = $statusCode
            raw = $null
            error = $_.Exception.Message
        }
    }
}

function Read-IntProperty {
    param(
        [object]$Object,
        [string]$Name
    )

    if ($null -eq $Object) {
        return 0
    }
    if ($Object.PSObject.Properties.Name -notcontains $Name) {
        return 0
    }
    $value = $Object.$Name
    if ($null -eq $value -or [string]::IsNullOrWhiteSpace([string]$value)) {
        return 0
    }
    return [int]$value
}

function Get-ChannelStatus {
    param([string]$ResolvedBaseURL)

    $statusURL = ($ResolvedBaseURL.TrimEnd("/") + "/api/channels/status")
    $probe = Get-JSONProbe -URL $statusURL -TimeoutSeconds 5
    $data = $null
    if ($probe.ok) {
        $data = $probe.raw
        if ($null -ne $probe.raw -and $probe.raw.PSObject.Properties.Name -contains "data") {
            $data = $probe.raw.data
        }
    }

    $clients = Read-IntProperty -Object $data -Name "clients"
    $readyClients = Read-IntProperty -Object $data -Name "ready_clients"
    $feedReadyClients = Read-IntProperty -Object $data -Name "feed_ready_clients"
    $searchReadyClients = Read-IntProperty -Object $data -Name "search_ready_clients"
    $commentReadyClients = Read-IntProperty -Object $data -Name "comment_ready_clients"

    return [pscustomobject]@{
        ok = [bool]$probe.ok
        url = $statusURL
        statusCode = [int]$probe.statusCode
        clients = $clients
        readyClients = $readyClients
        feedReadyClients = $feedReadyClients
        searchReadyClients = $searchReadyClients
        commentReadyClients = $commentReadyClients
        raw = $probe.raw
        error = [string]$probe.error
    }
}

function Get-WSHealth {
    param([string]$ResolvedBaseURL)

    $healthURL = ($ResolvedBaseURL.TrimEnd("/") + "/ws/health")
    $probe = Get-JSONProbe -URL $healthURL -TimeoutSeconds 5
    $clients = 0
    $status = ""
    if ($probe.ok -and $null -ne $probe.raw) {
        $clients = Read-IntProperty -Object $probe.raw -Name "clients"
        if ($probe.raw.PSObject.Properties.Name -contains "status") {
            $status = [string]$probe.raw.status
        }
    }

    return [pscustomobject]@{
        ok = [bool]$probe.ok
        url = $healthURL
        statusCode = [int]$probe.statusCode
        status = $status
        clients = $clients
        raw = $probe.raw
        error = [string]$probe.error
    }
}

function Get-RequiredCapabilityURLs {
    param([string]$ResolvedBaseURL)

    $baseURL = $ResolvedBaseURL.TrimEnd("/")
    return [ordered]@{
        feedSearch    = ($baseURL + "/api/channels/feed/search?keyword=__insight_capability_probe__")
        feedList      = ($baseURL + "/api/channels/contact/feed/list?username=__insight_capability_probe__")
        commentExport = ($baseURL + "/api/channels/feed/comment/export")
    }
}

function Get-RouteCapabilityProbe {
    param(
        [string]$Name,
        [string]$Method,
        [string]$URL,
        [string]$Body = "",
        [int]$TimeoutSeconds = 5
    )

    $statusCode = 0
    $errorText = ""
    try {
        $request = @{
            UseBasicParsing = $true
            Uri             = $URL
            Method          = $Method
            TimeoutSec      = $TimeoutSeconds
        }
        if (-not [string]::IsNullOrWhiteSpace($Body)) {
            $request["Body"] = $Body
            $request["ContentType"] = "application/json"
        }
        $response = Invoke-WebRequest @request
        $statusCode = [int]$response.StatusCode
    }
    catch {
        $errorText = $_.Exception.Message
        if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
            $statusCode = [int]$_.Exception.Response.StatusCode
        }
    }

    $routePresent = $true
    if ($statusCode -eq 0 -or $statusCode -eq 404 -or $statusCode -eq 405) {
        $routePresent = $false
    }
    return [pscustomobject]@{
        name = $Name
        method = $Method
        url = $URL
        statusCode = $statusCode
        routePresent = [bool]$routePresent
        error = $errorText
    }
}

function Get-WXChannelCapabilityStatus {
    param([string]$ResolvedBaseURL)

    $urls = Get-RequiredCapabilityURLs -ResolvedBaseURL $ResolvedBaseURL
    $probes = @()
    $probes += Get-RouteCapabilityProbe -Name "feedSearch" -Method "GET" -URL $urls.feedSearch
    $probes += Get-RouteCapabilityProbe -Name "feedList" -Method "GET" -URL $urls.feedList
    $probes += Get-RouteCapabilityProbe `
        -Name "commentExport" `
        -Method "POST" `
        -URL $urls.commentExport `
        -Body '{"object_id":"__insight_capability_probe__","nonce_id":"__insight_capability_probe__","title":"Insight capability probe"}'

    $missing = @()
    foreach ($probe in $probes) {
        if (-not $probe.routePresent) {
            $missing += $probe.name
        }
    }

    return [pscustomobject]@{
        ok = ($missing.Count -eq 0)
        requiredCapabilityURLs = $urls
        missing = @($missing)
        probes = @($probes)
    }
}

function Get-WindowsUserProxy {
    try {
        $settings = Get-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings" -ErrorAction Stop
        return [pscustomobject]@{
            ok = $true
            enabled = ([int]$settings.ProxyEnable -ne 0)
            server = [string]$settings.ProxyServer
            override = [string]$settings.ProxyOverride
            error = ""
        }
    }
    catch {
        return [pscustomobject]@{
            ok = $false
            enabled = $false
            server = ""
            override = ""
            error = $_.Exception.Message
        }
    }
}

function Get-PrimaryProxyAddress {
    param([string]$Server)

    $raw = [string]$Server
    $raw = $raw.Trim()
    if ([string]::IsNullOrWhiteSpace($raw)) {
        return ""
    }

    $firstFallback = ""
    foreach ($part in $raw.Split(";")) {
        $trimmed = $part.Trim()
        if ([string]::IsNullOrWhiteSpace($trimmed)) {
            continue
        }
        if ($trimmed.Contains("=")) {
            $pieces = $trimmed.Split("=", 2)
            $scheme = $pieces[0].Trim().ToLowerInvariant()
            $endpoint = $pieces[1].Trim()
            if ([string]::IsNullOrWhiteSpace($firstFallback)) {
                $firstFallback = $endpoint
            }
            if ($scheme -eq "http" -or $scheme -eq "https") {
                return $endpoint
            }
            continue
        }
        if ([string]::IsNullOrWhiteSpace($firstFallback)) {
            $firstFallback = $trimmed
        }
    }
    return $firstFallback
}

function Test-SameProxyAddress {
    param(
        [string]$Left,
        [string]$Right
    )

    $leftValue = ([string]$Left).Trim().ToLowerInvariant().Replace("localhost", "127.0.0.1")
    $rightValue = ([string]$Right).Trim().ToLowerInvariant().Replace("localhost", "127.0.0.1")
    return $leftValue -eq $rightValue
}

function Build-Recommendation {
    param(
        [pscustomobject]$Status,
        [pscustomobject]$Health,
        [pscustomobject]$Capabilities,
        [pscustomobject]$Proxy,
        [string]$ExpectedProxy
    )

    if (-not $Status.ok -and -not $Health.ok) {
        return "wx_channel_data_plane_unreachable:start_wx_channel_and_verify_2026_api"
    }
    if ($null -ne $Capabilities -and -not $Capabilities.ok) {
        $missing = ($Capabilities.missing -join ",")
        return "wx_channel_data_plane_incompatible:missing=$missing;run=E:\netdisk\wx-down\wx_channel_new\wx_channel\scripts\switch-insight-data-plane-admin.cmd"
    }
    if ($Status.readyClients -gt 0 -or $Status.searchReadyClients -gt 0 -or $Status.commentReadyClients -gt 0) {
        return "wx_channel_data_plane_ready:rerun_insight_operator"
    }

    $currentProxy = ""
    if ($Proxy.ok -and $Proxy.enabled) {
        $currentProxy = Get-PrimaryProxyAddress -Server $Proxy.server
    }
    if (-not [string]::IsNullOrWhiteSpace($currentProxy) -and -not (Test-SameProxyAddress -Left $currentProxy -Right $ExpectedProxy)) {
        return "wx_channel_proxy_mismatch:current=$currentProxy;expected=$ExpectedProxy;verify=/api/channels/status ready/search_ready/comment_ready"
    }

    if ($Status.clients -gt 0 -or $Health.clients -gt 0) {
        return "wx_channel_client_connected_but_not_ready:open_target_video_or_search_in_standalone_client_and_wait_for_api_ready"
    }
    return "wx_channel_no_standalone_client_connected:ensure_client_runs_through_wx_channel_proxy"
}

$resolvedRepoRoot = Resolve-RepoRoot -Value $RepoRoot
$resolvedBaseURL = Normalize-BaseURL -Value $BaseURL
$resolvedProxyAddress = [string]$ProxyAddress
$resolvedWaitSeconds = [Math]::Max(0, $WaitSeconds)

if ($EmitOnly.IsPresent) {
    [pscustomobject]@{
        repoRoot = $resolvedRepoRoot
        mode = "read-only-data-plane-diagnostic"
        baseURL = $resolvedBaseURL
        statusURL = ($resolvedBaseURL.TrimEnd("/") + "/api/channels/status")
        wsHealthURL = ($resolvedBaseURL.TrimEnd("/") + "/ws/health")
        requiredCapabilityURLs = Get-RequiredCapabilityURLs -ResolvedBaseURL $resolvedBaseURL
        requiredCapabilityAdminHint = "Run E:\netdisk\wx-down\wx_channel_new\wx_channel\scripts\switch-insight-data-plane-admin.cmd as administrator if these routes return 404/405."
        expectedProxyAddress = $resolvedProxyAddress
        waitSeconds = $resolvedWaitSeconds
        launchesBrowser = $false
        mutatesProxySettings = $false
    } | ConvertTo-Json -Depth 6
    return
}

$statusBefore = Get-ChannelStatus -ResolvedBaseURL $resolvedBaseURL
$healthBefore = Get-WSHealth -ResolvedBaseURL $resolvedBaseURL
$capabilitiesBefore = Get-WXChannelCapabilityStatus -ResolvedBaseURL $resolvedBaseURL

$statusAfter = $statusBefore
$healthAfter = $healthBefore
$capabilitiesAfter = $capabilitiesBefore
$deadline = (Get-Date).AddSeconds($resolvedWaitSeconds)
while ((Get-Date) -lt $deadline) {
    if ($capabilitiesAfter.ok -and $statusAfter.ok -and ($statusAfter.readyClients -gt 0 -or $statusAfter.searchReadyClients -gt 0 -or $statusAfter.commentReadyClients -gt 0)) {
        break
    }
    Start-Sleep -Seconds 1
    $statusAfter = Get-ChannelStatus -ResolvedBaseURL $resolvedBaseURL
    $healthAfter = Get-WSHealth -ResolvedBaseURL $resolvedBaseURL
    $capabilitiesAfter = Get-WXChannelCapabilityStatus -ResolvedBaseURL $resolvedBaseURL
}

$proxy = Get-WindowsUserProxy
$state = "blocked"
if (-not $statusAfter.ok -and -not $healthAfter.ok) {
    $state = "unreachable"
}
elseif (-not $capabilitiesAfter.ok) {
    $state = "incompatible"
}
elseif ($statusAfter.ok -and ($statusAfter.readyClients -gt 0 -or $statusAfter.searchReadyClients -gt 0 -or $statusAfter.commentReadyClients -gt 0)) {
    $state = "ready"
}

[pscustomobject]@{
    status = $state
    repoRoot = $resolvedRepoRoot
    mode = "read-only-data-plane-diagnostic"
    baseURL = $resolvedBaseURL
    statusURL = ($resolvedBaseURL.TrimEnd("/") + "/api/channels/status")
    wsHealthURL = ($resolvedBaseURL.TrimEnd("/") + "/ws/health")
    expectedProxyAddress = $resolvedProxyAddress
    checkedAt = [System.DateTimeOffset]::Now.ToString("o")
    statusBefore = $statusBefore
    statusAfter = $statusAfter
    healthBefore = $healthBefore
    healthAfter = $healthAfter
    capabilitiesBefore = $capabilitiesBefore
    capabilitiesAfter = $capabilitiesAfter
    windowsUserProxy = $proxy
    launchesBrowser = $false
    mutatesProxySettings = $false
    recommendedAction = Build-Recommendation -Status $statusAfter -Health $healthAfter -Capabilities $capabilitiesAfter -Proxy $proxy -ExpectedProxy $resolvedProxyAddress
} | ConvertTo-Json -Depth 8
