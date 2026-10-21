[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [string]$ComposeFile = "",
    [string]$APIBaseURL = "http://127.0.0.1:18081",
    [string]$WebBaseURL = "http://127.0.0.1:5176",
    [string]$HubUserId = "insight:operator:local",
    [int]$DeviceCount = 1,
    [string]$TrustedBridgeToken = "",
    [string]$ApiContainer = "wx_channel_insight_stack_api",
    [string]$ResultJsonPath = "",
    [switch]$SkipOperatorLoop,
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

function Resolve-URLPort {
    param([string]$URL, [int]$Fallback)
    try {
        $uri = [System.Uri]$URL
        if ($uri.Port -gt 0) {
            return [int]$uri.Port
        }
    }
    catch {
    }
    return $Fallback
}

function Resolve-DefaultResultJsonPath {
    param([string]$ResolvedRepoRoot, [string]$RequestedPath)
    if (-not [string]::IsNullOrWhiteSpace($RequestedPath)) {
        return $RequestedPath
    }
    return (Join-Path $ResolvedRepoRoot ".tmp-codex\live-stack-proof-latest.json")
}

function Write-JsonResultFile {
    param([object]$Value, [string]$Path)
    $parent = Split-Path -Parent $Path
    if ($parent) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $utf8NoBom = [System.Text.UTF8Encoding]::new($false)
    [System.IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 20), $utf8NoBom)
}

function Convert-ResponseBodyToJson {
    param([string]$Content)
    if ([string]::IsNullOrWhiteSpace($Content)) {
        return $null
    }
    try {
        return $Content | ConvertFrom-Json
    }
    catch {
        return $null
    }
}

function Get-HTTPErrorBody {
    param([object]$Response)
    if ($null -eq $Response) {
        return ""
    }
    try {
        $stream = $Response.GetResponseStream()
        if ($null -eq $stream) {
            return ""
        }
        $reader = [System.IO.StreamReader]::new($stream)
        try {
            return $reader.ReadToEnd()
        }
        finally {
            $reader.Dispose()
            $stream.Dispose()
        }
    }
    catch {
        return ""
    }
}

function Invoke-HTTPJSON {
    param(
        [string]$Name,
        [string]$URL,
        [hashtable]$Headers = @{},
        [int]$TimeoutSeconds = 90
    )

    $startedAt = Get-Date
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $URL -Headers $Headers -TimeoutSec $TimeoutSeconds
        $content = [string]$response.Content
        return [pscustomobject]@{
            name       = $Name
            url        = $URL
            statusCode = [int]$response.StatusCode
            body       = Convert-ResponseBodyToJson -Content $content
            bytes      = $content.Length
            durationMs = [int]((Get-Date) - $startedAt).TotalMilliseconds
            error      = ""
        }
    }
    catch {
        $statusCode = 0
        $content = ""
        if ($null -ne $_.Exception.Response) {
            try { $statusCode = [int]$_.Exception.Response.StatusCode } catch { $statusCode = 0 }
            $content = Get-HTTPErrorBody -Response $_.Exception.Response
        }
        return [pscustomobject]@{
            name       = $Name
            url        = $URL
            statusCode = $statusCode
            body       = Convert-ResponseBodyToJson -Content $content
            bytes      = $content.Length
            durationMs = [int]((Get-Date) - $startedAt).TotalMilliseconds
            error      = [string]$_.Exception.Message
        }
    }
}

function Invoke-HTTPText {
    param([string]$Name, [string]$URL, [int]$TimeoutSeconds = 30)
    $startedAt = Get-Date
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $URL -TimeoutSec $TimeoutSeconds
        return [pscustomobject]@{
            name       = $Name
            url        = $URL
            statusCode = [int]$response.StatusCode
            bytes      = ([string]$response.Content).Length
            durationMs = [int]((Get-Date) - $startedAt).TotalMilliseconds
            error      = ""
        }
    }
    catch {
        return [pscustomobject]@{
            name       = $Name
            url        = $URL
            statusCode = 0
            bytes      = 0
            durationMs = [int]((Get-Date) - $startedAt).TotalMilliseconds
            error      = [string]$_.Exception.Message
        }
    }
}

function Assert-HTTPJSON {
    param([object]$Probe, [string]$Label)
    if ([int]$Probe.statusCode -ne 200 -or $null -eq $Probe.body) {
        throw "$Label expected HTTP 200 JSON, got status=$($Probe.statusCode) error=$($Probe.error)"
    }
}

function Assert-Condition {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        throw $Message
    }
}

function Invoke-Docker {
    param([string[]]$Arguments)
    $startedAt = Get-Date
    $output = @(& docker @Arguments 2>&1 | ForEach-Object { [string]$_ })
    $exitCode = $LASTEXITCODE
    return [pscustomobject]@{
        arguments  = $Arguments
        exitCode   = $exitCode
        durationMs = [int]((Get-Date) - $startedAt).TotalMilliseconds
        output     = (($output -join "`n").Trim())
    }
}

function Get-ListCount {
    param([object]$Value)
    if ($null -eq $Value) { return 0 }
    return @($Value).Count
}

function Get-EndpointSummary {
    param([object]$Probe)
    return [ordered]@{
        name       = $Probe.name
        url        = $Probe.url
        statusCode = $Probe.statusCode
        bytes      = $Probe.bytes
        durationMs = $Probe.durationMs
    }
}

function Get-RuntimeSummary {
    param([object]$Status)
    return [ordered]@{
        runtimeProfile       = $Status.runtimeProfile
        hubBridgeMode        = $Status.hubBridgeMode
        demoContextEnabled   = $Status.demoContextEnabled
        liveRuntimeClass     = $Status.liveRuntimeClass
        liveRuntimeProofState = $Status.liveRuntimeProofState
        launchReadiness      = [ordered]@{
            totalCount   = $Status.launchReadiness.totalCount
            metCount     = $Status.launchReadiness.metCount
            unknownCount = $Status.launchReadiness.unknownCount
            blockedCount = $Status.launchReadiness.blockedCount
            gates        = @($Status.launchReadiness.gates | ForEach-Object {
                [ordered]@{ key = $_.key; status = $_.status }
            })
        }
    }
}

if (-not $RepoRoot) {
    $RepoRoot = Resolve-DefaultRepoRoot
}
else {
    $RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
}

if (-not $ComposeFile) {
    $ComposeFile = Join-Path $RepoRoot "deploy\stack\docker-compose.yml"
}
$ComposeFile = (Resolve-Path -LiteralPath $ComposeFile).Path
$APIBaseURL = Normalize-BaseURL -Value $APIBaseURL
$WebBaseURL = Normalize-BaseURL -Value $WebBaseURL
$ResultJsonPath = Resolve-DefaultResultJsonPath -ResolvedRepoRoot $RepoRoot -RequestedPath $ResultJsonPath
$apiPort = Resolve-URLPort -URL $APIBaseURL -Fallback 18081
$webPort = Resolve-URLPort -URL $WebBaseURL -Fallback 5176
$proofContainerPath = "/tmp/insight-live-stack-proof.json"

$apiHeaders = @{
    "X-Hub-User-ID" = $HubUserId
    "X-Hub-Device-Count" = "$DeviceCount"
}
if (-not [string]::IsNullOrWhiteSpace($TrustedBridgeToken)) {
    $apiHeaders["X-Insight-Bridge-Token"] = $TrustedBridgeToken
}

if ($EmitOnly.IsPresent) {
    [pscustomobject]@{
        mode             = "live-docker-stack-verification"
        repoRoot         = $RepoRoot
        composeFile      = $ComposeFile
        apiBaseURL       = $APIBaseURL
        webBaseURL       = $WebBaseURL
        apiContainer     = $ApiContainer
        hubUserId        = $HubUserId
        deviceCount      = $DeviceCount
        resultJsonPath   = $ResultJsonPath
        proofOutputPath  = "/app/var/trusted-runtime-evidence.json"
        operatorLoop     = (-not $SkipOperatorLoop.IsPresent)
        saveCommand      = "docker exec $ApiContainer /app/insight-jobs save-trusted-runtime-evidence --proof-path $proofContainerPath --output-path /app/var/trusted-runtime-evidence.json"
    } | ConvertTo-Json -Depth 8
    return
}

$status = "success"
$failure = ""
$proofSaved = $false
$steps = @()
$summary = $null

try {
    $compose = Invoke-Docker -Arguments @("compose", "--profile", "jobs", "-f", $ComposeFile, "ps")
    $steps += [ordered]@{ name = "docker-stack-status"; exitCode = $compose.exitCode; output = $compose.output }
    Assert-Condition ($compose.exitCode -eq 0) "docker compose ps failed: $($compose.output)"

    $apiHealth = Invoke-HTTPJSON -Name "api-health" -URL "$APIBaseURL/api/healthz"
    $steps += [ordered]@{ name = "api-health"; exitCode = $(if($apiHealth.statusCode -eq 200){0}else{1}); output = ($apiHealth | ConvertTo-Json -Depth 6) }
    Assert-HTTPJSON -Probe $apiHealth -Label "API health"

    $webHealth = Invoke-HTTPText -Name "web-root" -URL "$WebBaseURL/"
    $steps += [ordered]@{ name = "web-root"; exitCode = $(if($webHealth.statusCode -eq 200){0}else{1}); output = ($webHealth | ConvertTo-Json -Depth 5) }
    Assert-Condition ($webHealth.statusCode -eq 200) "Web root expected HTTP 200, got $($webHealth.statusCode)"

    if (-not $SkipOperatorLoop.IsPresent) {
        $operator = Invoke-Docker -Arguments @(
            "compose", "--profile", "jobs", "-f", $ComposeFile, "run", "--rm", "--no-deps", "jobs",
            "run-operator-loop", "--interval", "1s", "--iterations", "1",
            "--sources", "wx_comment_export,wx_video_search,wx_feed_list",
            "--hub-user-id", $HubUserId, "--device-count", "$DeviceCount", "--strict"
        )
        $steps += [ordered]@{ name = "run-operator-loop"; exitCode = $operator.exitCode; output = $operator.output }
        Assert-Condition ($operator.exitCode -eq 0) "current stack operator loop failed: $($operator.output)"
    }

    $runtimeProbe = Invoke-HTTPJSON -Name "check-runtime-status" -URL "$APIBaseURL/api/v1/runtime/status" -Headers $apiHeaders
    $apiContextProbe = Invoke-HTTPJSON -Name "check-runtime-trusted-context" -URL "$APIBaseURL/api/v1/workspace/context" -Headers $apiHeaders
    $apiBrowserProbe = Invoke-HTTPJSON -Name "check-runtime-browser-query" -URL "$APIBaseURL/api/v1/workspace/context?hub_user_id=hub:user:browser-probe&device_count=99"
    $webContextProbe = Invoke-HTTPJSON -Name "check-trusted-web-context" -URL "$WebBaseURL/api/v1/workspace/context" -Headers $apiHeaders
    $webBrowserProbe = Invoke-HTTPJSON -Name "check-trusted-web-browser-query" -URL "$WebBaseURL/api/v1/workspace/context?hub_user_id=hub:user:browser-probe&device_count=99"
    Assert-HTTPJSON -Probe $runtimeProbe -Label "runtime status"
    Assert-HTTPJSON -Probe $apiContextProbe -Label "API trusted context"
    Assert-HTTPJSON -Probe $webContextProbe -Label "Web trusted context"

    Assert-Condition ([string]$apiContextProbe.body.hubUserId -eq $HubUserId) "API trusted context user mismatch: $($apiContextProbe.body.hubUserId)"
    Assert-Condition ([string]$apiContextProbe.body.contextSource -in @("trusted_header", "local_operator")) "API trusted context source is not trusted: $($apiContextProbe.body.contextSource)"
    Assert-Condition ([string]$webContextProbe.body.hubUserId -eq $HubUserId) "Web trusted context user mismatch: $($webContextProbe.body.hubUserId)"
    Assert-Condition ([string]$webContextProbe.body.contextSource -in @("trusted_header", "local_operator")) "Web trusted context source is not trusted: $($webContextProbe.body.contextSource)"

    $runtimeStatus = $runtimeProbe.body
    $proofResults = @(
        [ordered]@{ name = "check-runtime-status"; exitCode = 0; output = ($runtimeStatus | ConvertTo-Json -Depth 20) },
        [ordered]@{ name = "check-runtime-trusted-context"; exitCode = 0; output = ($apiContextProbe.body | ConvertTo-Json -Depth 10) },
        [ordered]@{ name = "check-runtime-browser-query-blocked"; exitCode = 0; output = ([ordered]@{ statusCode = $apiBrowserProbe.statusCode; json = $apiBrowserProbe.body } | ConvertTo-Json -Depth 10) },
        [ordered]@{ name = "check-trusted-web-context"; exitCode = 0; output = ($webContextProbe.body | ConvertTo-Json -Depth 10) },
        [ordered]@{ name = "check-trusted-web-browser-query-isolated"; exitCode = 0; output = ($webBrowserProbe.body | ConvertTo-Json -Depth 10) }
    )

    $dashboardProbe = Invoke-HTTPJSON -Name "dashboard" -URL "$APIBaseURL/api/v1/dashboard" -Headers $apiHeaders
    $hotVideosProbe = Invoke-HTTPJSON -Name "hot-videos" -URL "$APIBaseURL/api/v1/read-models/hot-videos" -Headers $apiHeaders
    $hotTopicsProbe = Invoke-HTTPJSON -Name "hot-topics" -URL "$APIBaseURL/api/v1/read-models/hot-topics" -Headers $apiHeaders
    $accountRecommendationsProbe = Invoke-HTTPJSON -Name "account-recommendations" -URL "$APIBaseURL/api/v1/read-models/account-recommendations?limit=6" -Headers $apiHeaders
    $videoRecommendationsProbe = Invoke-HTTPJSON -Name "video-recommendations" -URL "$APIBaseURL/api/v1/read-models/video-recommendations?limit=6" -Headers $apiHeaders
    $reportProbe = Invoke-HTTPJSON -Name "report-summary" -URL "$APIBaseURL/api/v1/read-models/report-summary" -Headers $apiHeaders
    foreach ($probe in @($dashboardProbe, $hotVideosProbe, $hotTopicsProbe, $accountRecommendationsProbe, $videoRecommendationsProbe, $reportProbe)) {
        Assert-HTTPJSON -Probe $probe -Label $probe.name
        $steps += [ordered]@{ name = "check-live-$($probe.name)"; exitCode = 0; output = ((Get-EndpointSummary -Probe $probe) | ConvertTo-Json -Depth 6) }
    }

    $proof = [ordered]@{
        status = "success"
        hubUserId = $HubUserId
        apiPort = $apiPort
        webPort = $webPort
        results = $proofResults
    }
    Write-JsonResultFile -Value $proof -Path $ResultJsonPath

    $copy = Invoke-Docker -Arguments @("cp", $ResultJsonPath, "${ApiContainer}:$proofContainerPath")
    $steps += [ordered]@{ name = "copy-live-proof-into-api-container"; exitCode = $copy.exitCode; output = $copy.output }
    Assert-Condition ($copy.exitCode -eq 0) "copying live proof into API container failed: $($copy.output)"

    $save = Invoke-Docker -Arguments @(
        "exec", $ApiContainer, "/app/insight-jobs", "save-trusted-runtime-evidence",
        "--proof-path", $proofContainerPath,
        "--output-path", "/app/var/trusted-runtime-evidence.json"
    )
    $steps += [ordered]@{ name = "save-trusted-runtime-evidence"; exitCode = $save.exitCode; output = $save.output }
    Assert-Condition ($save.exitCode -eq 0) "saving trusted runtime evidence failed: $($save.output)"
    $proofSaved = $true

    $runtimeAfterProof = Invoke-HTTPJSON -Name "check-runtime-status-after-proof" -URL "$APIBaseURL/api/v1/runtime/status" -Headers $apiHeaders
    Assert-HTTPJSON -Probe $runtimeAfterProof -Label "runtime status after proof"
    $after = $runtimeAfterProof.body
    Assert-Condition ([string]$after.liveRuntimeProofState -eq "verified") "live runtime proof state expected verified, got $($after.liveRuntimeProofState)"
    Assert-Condition ([int]$after.launchReadiness.totalCount -ge 8) "launch readiness expected at least 8 gates, got $($after.launchReadiness.totalCount)"
    Assert-Condition ([int]$after.launchReadiness.unknownCount -eq 0 -and [int]$after.launchReadiness.blockedCount -eq 0) "launch readiness not fully met: met=$($after.launchReadiness.metCount) unknown=$($after.launchReadiness.unknownCount) blocked=$($after.launchReadiness.blockedCount)"
    Assert-Condition ([int]$after.launchReadiness.metCount -eq [int]$after.launchReadiness.totalCount) "launch readiness met count mismatch"
    $steps += [ordered]@{ name = "check-runtime-status-after-proof"; exitCode = 0; output = ($after | ConvertTo-Json -Depth 20) }

    $summary = [ordered]@{
        api = Get-EndpointSummary -Probe $apiHealth
        web = Get-EndpointSummary -Probe $webHealth
        dashboard = Get-EndpointSummary -Probe $dashboardProbe
        hotVideos = Get-ListCount -Value $hotVideosProbe.body.items
        hotTopics = Get-ListCount -Value $hotTopicsProbe.body.items
        accountRecommendations = Get-ListCount -Value $accountRecommendationsProbe.body.items
        videoRecommendations = Get-ListCount -Value $videoRecommendationsProbe.body.items
        runtime = Get-RuntimeSummary -Status $after
        proofOutputPath = "/app/var/trusted-runtime-evidence.json"
    }
}
catch {
    $status = "failed"
    $failure = [string]$_.Exception.Message
}
finally {
    $cleanup = Invoke-Docker -Arguments @("exec", "--user", "0", $ApiContainer, "rm", "-f", $proofContainerPath)
    if ($cleanup.exitCode -ne 0 -and $status -eq "success") {
        $status = "failed"
        $failure = "temporary proof cleanup failed: $($cleanup.output)"
    }
}

$final = [ordered]@{
    status = $status
    failure = $failure
    mode = "live-docker-stack-verification"
    repoRoot = $RepoRoot
    composeFile = $ComposeFile
    apiBaseURL = $APIBaseURL
    webBaseURL = $WebBaseURL
    hubUserId = $HubUserId
    deviceCount = $DeviceCount
    resultJsonPath = $ResultJsonPath
    proofSaved = $proofSaved
    summary = $summary
    steps = $steps
}
Write-JsonResultFile -Value $final -Path $ResultJsonPath
$final | ConvertTo-Json -Depth 20
if ($status -ne "success") {
    exit 1
}
