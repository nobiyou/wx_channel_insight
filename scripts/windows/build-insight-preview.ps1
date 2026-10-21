[CmdletBinding()]
param([string]$RepoRoot = '', [string]$OutputPath = '', [switch]$Force)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'package-common.ps1')
$root = Resolve-PackageRepo $RepoRoot
if (-not $OutputPath) { $OutputPath = Join-Path $root ('release\insight-windows-' + (Get-Date -Format 'yyyyMMdd-HHmmss')) }
$output = Resolve-PackageOutput $root $OutputPath -Force:$Force
$targetOS = & go env GOOS
if ($LASTEXITCODE -ne 0 -or [string]$targetOS -ne 'windows') { throw 'Windows packages require go env GOOS=windows.' }
function Invoke-Checked([string]$Command, [string[]]$Arguments, [string]$WorkingDirectory) {
    Push-Location -LiteralPath $WorkingDirectory
    try {
        & $Command @Arguments
        if ($LASTEXITCODE -ne 0) { throw "$Command failed with exit code $LASTEXITCODE" }
    } finally { Pop-Location }
}
$stageRoot = New-PackageStage $root
$stage = Join-Path $stageRoot 'package'
New-Item -ItemType Directory -Path (Join-Path $stage 'bin'), (Join-Path $stage 'web'), (Join-Path $stage 'data\comment_data') -Force | Out-Null
try {
    Invoke-Checked 'npm' @('run', 'build') (Join-Path $root 'apps\web')
    foreach ($app in @('api', 'scheduler', 'jobs')) {
        Invoke-Checked 'go' @('build', '-trimpath', '-ldflags=-s -w', '-o', (Join-Path $stage "bin\insight-$app.exe"), "./apps/$app") $root
    }
    Copy-Item -Path (Join-Path $root 'apps\web\dist\*') -Destination (Join-Path $stage 'web') -Recurse
    foreach ($file in @('start-insight-preview.ps1', 'stop-insight-preview.ps1', 'Start-Insight-Preview.cmd', 'Stop-Insight-Preview.cmd')) {
        Copy-PackageFile (Join-Path $PSScriptRoot $file) $stage "scripts\windows\$file"
    }
    foreach ($name in @('config.example.json', 'config.postgres.example.json')) {
        $config = Get-Content -Raw -LiteralPath (Join-Path $root $name) | ConvertFrom-Json
        $config.server.web_static_root = 'web'
        $config.storage.data_dir = 'data'
        if ($name -eq 'config.example.json') { $config.storage.sqlite_path = 'data/insight.db' }
        $config.runtime | Add-Member -NotePropertyName jobs_state_path -NotePropertyValue 'data/jobs-runtime-state.json' -Force
        $config.wx_channel | Add-Member -NotePropertyName comment_data_root -NotePropertyValue 'data/comment_data' -Force
        Write-PackageJson $config (Join-Path $stage $name)
    }
    Copy-PackageFile (Join-Path $root 'LICENSE') $stage 'LICENSE'
    Copy-PackageFile (Join-Path $root 'deploy\windows\README.md') $stage 'README.md'
    foreach ($file in Get-ChildItem -LiteralPath (Join-Path $root 'migrations') -Filter '*.sql' -Recurse -File) {
        Copy-PackageFile $file.FullName $stage $file.FullName.Substring($root.Length + 1)
    }
    Write-PackageJson ([ordered]@{
        product = 'wx_channel_insight'; channel = 'windows'; source = (Get-PackageSource $root)
        createdAt = (Get-Date).ToUniversalTime().ToString('o'); license = 'Apache-2.0'
        config = 'insight.local.json'; configTemplate = 'config.example.json'; storage = 'sqlite'
        start = 'scripts/windows/Start-Insight-Preview.cmd'; stop = 'scripts/windows/Stop-Insight-Preview.cmd'
    }) (Join-Path $stage 'manifest.json')
    # Revalidate immediately before publish so runtime data added during a build is protected.
    $null = Resolve-PackageOutput $root $output -Force:$Force
    Publish-Package $stage $output -Force:$Force
} finally { Remove-PackageStage $root $stageRoot }
[pscustomobject]@{ status = 'built'; outputPath = $output; manifestPath = (Join-Path $output 'manifest.json') } | ConvertTo-Json
