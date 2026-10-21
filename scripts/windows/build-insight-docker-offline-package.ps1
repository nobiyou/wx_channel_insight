[CmdletBinding()]
param(
    [string]$RepoRoot = '', [string]$OutputPath = '',
    [string]$ApiImageSource = '', [string]$WebImageSource = '',
    [string]$PostgresImageSource = 'postgres:16-alpine', [string]$RedisImageSource = 'redis:7-alpine',
    [switch]$Force
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'package-common.ps1')
$root = Resolve-PackageRepo $RepoRoot
if (-not $OutputPath) { $OutputPath = Join-Path $root ('release\insight-docker-offline-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.zip') }
$output = Resolve-PackageOutput $root $OutputPath -Force:$Force
if ([IO.Path]::GetExtension($output) -ne '.zip') { throw 'Offline package output must be a .zip file.' }
$refs = [ordered]@{ api = $ApiImageSource; web = $WebImageSource; postgres = $PostgresImageSource; redis = $RedisImageSource }
foreach ($entry in $refs.GetEnumerator()) {
    if ($entry.Value -notmatch '^[a-zA-Z0-9][a-zA-Z0-9./:_-]*:[a-zA-Z0-9_][a-zA-Z0-9_.-]*$' -or $entry.Value -like 'sha256:*') {
        throw "An explicit tagged image reference is required for $($entry.Key). Use -ApiImageSource and -WebImageSource for tested application images."
    }
}
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'docker is not available.' }
& docker info 1>$null 2>$null
if ($LASTEXITCODE -ne 0) { throw 'Docker engine is not running.' }
function Read-ImageIdentity([string]$Reference) {
    $format = '{"id":{{json .Id}},"os":{{json .Os}},"architecture":{{json .Architecture}},"revision":{{json (index .Config.Labels "org.opencontainers.image.revision")}}}'
    $raw = & docker image inspect --format $format $Reference
    if ($LASTEXITCODE -ne 0) { throw "Required local image is missing: $Reference" }
    $identity = $raw | ConvertFrom-Json
    if (-not $identity.id -or $identity.os -ne 'linux') { throw "A Linux runtime image is required: $Reference" }
    return [ordered]@{
        reference = $Reference; id = $identity.id; os = $identity.os; architecture = $identity.architecture
        revision = $(if ($identity.revision) { $identity.revision } else { 'unknown' })
    }
}
$images = [ordered]@{}
foreach ($entry in $refs.GetEnumerator()) { $images[$entry.Key] = Read-ImageIdentity $entry.Value }
if (@($images.Values | ForEach-Object { $_.architecture } | Select-Object -Unique).Count -ne 1) { throw 'All offline images must use the same architecture.' }
$stageRoot = New-PackageStage $root
$packageName = [IO.Path]::GetFileNameWithoutExtension($output)
$zipRoot = Join-Path $stageRoot 'archive-root'
$stage = Join-Path $zipRoot $packageName
New-Item -ItemType Directory -Path (Join-Path $stage 'images'), (Join-Path $stage 'data\comment_data') -Force | Out-Null
try {
    $compose = [IO.File]::ReadAllText((Join-Path $root 'deploy\stack\docker-compose.offline.yml'))
    $placeholders = @{ api = 'insight-preview/api:preview'; web = 'insight-preview/web:preview'; postgres = 'insight-preview/postgres:16-alpine'; redis = 'insight-preview/redis:7-alpine' }
    foreach ($entry in $refs.GetEnumerator()) {
        if (-not $compose.Contains($placeholders[$entry.Key])) { throw "Missing offline Compose image placeholder: $($entry.Key)" }
        $compose = $compose.Replace($placeholders[$entry.Key], $entry.Value)
    }
    [IO.File]::WriteAllText((Join-Path $stage 'docker-compose.yml'), $compose, [Text.UTF8Encoding]::new($false))
    foreach ($file in @('LICENSE', 'config.example.json', 'config.postgres.example.json')) {
        Copy-PackageFile (Join-Path $root $file) $stage $file
    }
    Copy-PackageFile (Join-Path $root 'deploy\stack\.env.example') $stage '.env.example'
    Copy-PackageFile (Join-Path $root 'deploy\stack\DEPLOYMENT-OFFLINE.md') $stage 'DEPLOYMENT.md'
    Copy-PackageFile (Join-Path $PSScriptRoot 'import-and-start-insight-offline.ps1') $stage 'scripts\windows\import-and-start-insight.ps1'
    Copy-PackageFile (Join-Path $PSScriptRoot 'stop-insight-offline.ps1') $stage 'scripts\windows\stop-insight.ps1'
    $imageRefs = @($refs.Values | ForEach-Object { [string]$_ })
    & docker save --output (Join-Path $stage 'images\insight-runtime-images.tar') @imageRefs
    if ($LASTEXITCODE -ne 0) { throw "docker save failed with exit code $LASTEXITCODE" }
    foreach ($entry in $refs.GetEnumerator()) {
        $current = Read-ImageIdentity $entry.Value
        if ($current.id -ne $images[$entry.Key].id) { throw "Image tag changed during export: $($entry.Value); retry with stable tags." }
    }
    Write-PackageJson ([ordered]@{
        product = 'wx_channel_insight'; channel = 'docker-offline'; license = 'Apache-2.0'
        createdAt = (Get-Date).ToUniversalTime().ToString('o'); packagingSource = (Get-PackageSource $root)
        images = $images; imageArchive = 'images/insight-runtime-images.tar'; instructions = 'DEPLOYMENT.md'
    }) (Join-Path $stage 'manifest.json')
    $archive = Join-Path $stageRoot 'package.zip'
    Compress-Package $zipRoot $archive
    Publish-Package $archive $output -Force:$Force
} finally { Remove-PackageStage $root $stageRoot }
[pscustomobject]@{ status = 'built'; outputPath = $output; packageBytes = (Get-Item -LiteralPath $output).Length } | ConvertTo-Json
