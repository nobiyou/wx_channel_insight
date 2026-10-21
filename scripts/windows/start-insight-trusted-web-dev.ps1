[CmdletBinding()]
param(
    [string]$RepoRoot = "",
    [string]$APIProxyTarget = "http://127.0.0.1:18081",
    [int]$WebPort = 5177,
    [string]$HubUserId = "insight:operator:local",
    [int]$DeviceCount = 2,
    [string]$TrustedBridgeToken = "",
    [switch]$ReplaceExisting,
    [string]$ResultJsonPath = "",
    [switch]$EmitOnly
)

$ErrorActionPreference = "Stop"

function Resolve-DefaultRepoRoot {
    $path = Join-Path $PSScriptRoot "..\.."
    return (Resolve-Path -LiteralPath $path).Path
}

function Resolve-DefaultResultJsonPath {
    param(
        [string]$ResolvedRepoRoot,
        [string]$RequestedPath
    )

    if (-not [string]::IsNullOrWhiteSpace($RequestedPath)) {
        return $RequestedPath
    }

    $dir = Join-Path $ResolvedRepoRoot ".tmp-codex\trusted-web"
    return (Join-Path $dir "trusted-web-latest.json")
}

function Get-ListenerProcessIds {
    param([int]$Port)

    if ($Port -le 0) {
        return @()
    }

    $processIds = @()
    foreach ($match in @(netstat -ano -p tcp | Select-String -Pattern (":$Port\s"))) {
        $parts = ($match.Line.Trim() -split "\s+")
        if ($parts.Length -lt 5) {
            continue
        }
        if ($parts[3] -ne "LISTENING") {
            continue
        }
        if ($parts[1] -notmatch ":$Port$") {
            continue
        }
        $listenerProcessIdValue = 0
        if (-not [int]::TryParse($parts[4], [ref]$listenerProcessIdValue)) {
            continue
        }
        if ($listenerProcessIdValue -gt 0) {
            $processIds += $listenerProcessIdValue
        }
    }

    return @($processIds | Select-Object -Unique)
}

function Wait-ForHTTPJSON {
    param(
        [string]$URL,
        [int]$TimeoutSeconds = 60
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $lastBody = ""
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing $URL -TimeoutSec 3
            if ($response.StatusCode -eq 200 -and $response.Content) {
                $lastBody = $response.Content
                return $response.Content | ConvertFrom-Json
            }
        }
        catch {
            if ($_.Exception.Message) {
                $lastBody = $_.Exception.Message
            }
        }
        Start-Sleep -Milliseconds 500
    }

    throw "timed out waiting for $URL; last=$lastBody"
}

function Write-JsonResultFile {
    param(
        [psobject]$Value,
        [string]$Path
    )

    if ([string]::IsNullOrWhiteSpace($Path)) {
        return
    }

    $parent = Split-Path -Parent $Path
    if ($parent) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }

    $json = $Value | ConvertTo-Json -Depth 7
    $utf8NoBom = [System.Text.UTF8Encoding]::new($false)
    [System.IO.File]::WriteAllText($Path, $json, $utf8NoBom)
}

if (-not $RepoRoot) {
    $RepoRoot = Resolve-DefaultRepoRoot
}
else {
    $RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
}

$ResultJsonPath = Resolve-DefaultResultJsonPath -ResolvedRepoRoot $RepoRoot -RequestedPath $ResultJsonPath

$webURL = "http://127.0.0.1:$WebPort"
$envVars = [ordered]@{
    INSIGHT_API_PROXY_TARGET = $APIProxyTarget
    INSIGHT_WEB_TRUSTED_BRIDGE_TOKEN = $TrustedBridgeToken
    INSIGHT_WEB_TRUSTED_HUB_USER_ID = $HubUserId
    INSIGHT_WEB_TRUSTED_DEVICE_COUNT = "$DeviceCount"
}

if ($EmitOnly.IsPresent) {
    [pscustomobject]@{
        repoRoot = $RepoRoot
        apiProxyTarget = $APIProxyTarget
        webUrl = $webURL
        resultJsonPath = $ResultJsonPath
        env = $envVars
        step = "npm run dev -- --host 127.0.0.1 --port $WebPort"
    } | ConvertTo-Json -Depth 5
    return
}

$previousEnv = @{}
foreach ($key in $envVars.Keys) {
    $previousEnv[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
    [Environment]::SetEnvironmentVariable($key, $envVars[$key], "Process")
}

try {
    $listenerPids = @(Get-ListenerProcessIds -Port $WebPort)
    if ($listenerPids.Count -gt 0) {
        if (-not $ReplaceExisting.IsPresent) {
            throw "port $WebPort is already in use by pid(s) $($listenerPids -join ', '); rerun with -ReplaceExisting or choose another -WebPort"
        }
        foreach ($listenerPid in $listenerPids) {
            Stop-Process -Id $listenerPid -Force -ErrorAction SilentlyContinue
        }
        Start-Sleep -Milliseconds 800
    }

    $logDir = Join-Path $RepoRoot ".tmp-codex"
    New-Item -ItemType Directory -Force -Path $logDir | Out-Null
    $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $stdoutLog = Join-Path $logDir "insight-trusted-web-$stamp.stdout.log"
    $stderrLog = Join-Path $logDir "insight-trusted-web-$stamp.stderr.log"

    $webProcess = Start-Process `
        -FilePath "powershell.exe" `
        -ArgumentList @("-NoProfile", "-Command", "Set-Location '$RepoRoot\apps\web'; npm run dev -- --host 127.0.0.1 --port $WebPort") `
        -WorkingDirectory (Join-Path $RepoRoot "apps\web") `
        -WindowStyle Hidden `
        -RedirectStandardOutput $stdoutLog `
        -RedirectStandardError $stderrLog `
        -PassThru

    $workspaceContext = Wait-ForHTTPJSON -URL "$webURL/api/v1/workspace/context"
    if ($workspaceContext.hubUserId -ne $HubUserId) {
        throw "trusted proxy expected hub user $HubUserId, got $($workspaceContext.hubUserId)"
    }
    $acceptedSources = @("trusted_header", "local_operator")
    if ($acceptedSources -notcontains [string]$workspaceContext.contextSource) {
        throw "trusted proxy expected contextSource trusted_header or local_operator, got $($workspaceContext.contextSource)"
    }
    $browserQueryProbe = Wait-ForHTTPJSON -URL "$webURL/api/v1/workspace/context?hub_user_id=hub:user:browser-probe&device_count=99"
    if ($browserQueryProbe.hubUserId -ne $HubUserId) {
        throw "trusted proxy query probe expected hub user $HubUserId, got $($browserQueryProbe.hubUserId)"
    }
    if ($acceptedSources -notcontains [string]$browserQueryProbe.contextSource) {
        throw "trusted proxy query probe expected contextSource trusted_header or local_operator, got $($browserQueryProbe.contextSource)"
    }
    if ([string]$browserQueryProbe.hubUserId -eq "hub:user:browser-probe") {
        throw "trusted proxy query probe leaked browser query identity into live context"
    }

    $listenerProcessIds = @(Get-ListenerProcessIds -Port $WebPort)

    $result = [pscustomobject]@{
        status = "ready"
        repoRoot = $RepoRoot
        resultJsonPath = $ResultJsonPath
        web = [pscustomobject]@{
            url = $webURL
            port = $WebPort
            processId = $webProcess.Id
            listenerProcessId = $(if ($listenerProcessIds.Count -gt 0) { [int]$listenerProcessIds[0] } else { 0 })
            listenerProcessIds = $listenerProcessIds
            stdoutLog = $stdoutLog
            stderrLog = $stderrLog
            workspaceContext = $workspaceContext
            browserQueryProbe = $browserQueryProbe
        }
        proxy = [pscustomobject]@{
            apiProxyTarget = $APIProxyTarget
            hubUserId = $HubUserId
            deviceCount = $DeviceCount
            trustedBridgeToken = $TrustedBridgeToken
        }
    }
    Write-JsonResultFile -Value $result -Path $ResultJsonPath
    $result | ConvertTo-Json -Depth 7
}
finally {
    foreach ($key in $previousEnv.Keys) {
        [Environment]::SetEnvironmentVariable($key, $previousEnv[$key], "Process")
    }
}
