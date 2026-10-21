[CmdletBinding()]
param([string]$RepoRoot = '', [string]$OutputPath = '', [switch]$Force)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'package-common.ps1')
$root = Resolve-PackageRepo $RepoRoot
if (-not $OutputPath) { $OutputPath = Join-Path $root ('release\insight-docker-source-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.zip') }
$output = Resolve-PackageOutput $root $OutputPath -Force:$Force
if ([IO.Path]::GetExtension($output) -ne '.zip') { throw 'Source package output must be a .zip file.' }
$stageRoot = New-PackageStage $root
$packageName = [IO.Path]::GetFileNameWithoutExtension($output)
$stage = Join-Path $stageRoot $packageName
New-Item -ItemType Directory -Path $stage | Out-Null
$archive = Join-Path $stageRoot 'package.zip'
try {
    foreach ($file in @('go.mod', 'go.sum', '.dockerignore', 'LICENSE', 'README.md', 'config.example.json', 'config.postgres.example.json')) {
        Copy-PackageFile (Join-Path $root $file) $stage $file
    }
    foreach ($dir in @('apps\api', 'apps\jobs', 'apps\scheduler', 'internal', 'migrations')) {
        $base = Join-Path $root $dir
        foreach ($file in Get-ChildItem -LiteralPath $base -File -Recurse -Force) {
            $relative = $file.FullName.Substring($root.Length + 1)
            if ($relative -match '(^|[\\/])(var|data|node_modules|dist|\.git)([\\/]|$)') { continue }
            if (($dir -eq 'migrations' -and $file.Extension -eq '.sql') -or
                ($dir -ne 'migrations' -and $file.Extension -eq '.go' -and $file.Name -notlike '*_test.go')) {
                Copy-PackageFile $file.FullName $stage $relative
            }
        }
    }
    foreach ($file in @('package.json', 'package-lock.json', 'index.html', 'vite.config.js')) {
        Copy-PackageFile (Join-Path $root "apps\web\$file") $stage "apps\web\$file"
    }
    foreach ($dir in @('src', 'public')) {
        $base = Join-Path $root "apps\web\$dir"
        if (-not (Test-Path -LiteralPath $base)) { continue }
        foreach ($file in Get-ChildItem -LiteralPath $base -File -Recurse -Force) {
            if ($file.Extension -in @('.vue', '.js', '.ts', '.json', '.css', '.scss', '.html', '.svg', '.png', '.jpg', '.jpeg', '.webp', '.ico', '.woff', '.woff2')) {
                Copy-PackageFile $file.FullName $stage $file.FullName.Substring($root.Length + 1)
            }
        }
    }
    foreach ($file in @('docker-compose.yml', 'docker-compose.offline.yml', 'Dockerfile.api', 'Dockerfile.web', 'nginx.conf', '.env.example', 'README.md', 'DEPLOYMENT.md', 'DEPLOYMENT-OFFLINE.md')) {
        Copy-PackageFile (Join-Path $root "deploy\stack\$file") $stage "deploy\stack\$file"
    }
    $composePath = Join-Path $stage 'deploy\stack\docker-compose.yml'
    $compose = [IO.File]::ReadAllText($composePath)
    if (-not $compose.Contains('target: prebuilt')) { throw 'Expected the prebuilt Web target in source Compose.' }
    [IO.File]::WriteAllText($composePath, $compose.Replace('target: prebuilt', 'target: source-built'), [Text.UTF8Encoding]::new($false))
    foreach ($file in @('start-insight-docker-stack.ps1', 'stop-insight-docker-stack.ps1', 'verify-insight-docker-stack.ps1')) {
        Copy-PackageFile (Join-Path $PSScriptRoot $file) $stage "scripts\windows\$file"
    }
    [IO.File]::WriteAllText((Join-Path $stage 'README.md'), "# wx_channel_insight Docker source package`n`nDeployment: [deploy/stack/DEPLOYMENT.md](deploy/stack/DEPLOYMENT.md).`n`nLicense: [Apache-2.0](LICENSE).`n", [Text.UTF8Encoding]::new($false))
    New-Item -ItemType Directory -Path (Join-Path $stage 'data\comment_data') -Force | Out-Null
    Write-PackageJson ([ordered]@{
        product = 'wx_channel_insight'; channel = 'docker-source'; source = (Get-PackageSource $root)
        createdAt = (Get-Date).ToUniversalTime().ToString('o'); webBuildTarget = 'source-built'
        instructions = 'deploy/stack/DEPLOYMENT.md'; license = 'Apache-2.0'
    }) (Join-Path $stage 'manifest.json')
    $zipRoot = Join-Path $stageRoot 'archive-root'
    New-Item -ItemType Directory -Path $zipRoot | Out-Null
    Move-Item -LiteralPath $stage -Destination $zipRoot
    Compress-Package $zipRoot $archive
    Publish-Package $archive $output -Force:$Force
} finally { Remove-PackageStage $root $stageRoot }
[pscustomobject]@{ status = 'built'; outputPath = $output; packageName = $packageName } | ConvertTo-Json
