[CmdletBinding()]
param([string]$RepoRoot = '', [string]$OutputPath = '')
$ErrorActionPreference = 'Stop'
if (-not $RepoRoot) { $RepoRoot = Join-Path $PSScriptRoot '..\..' }
$RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
if (-not $OutputPath) { $OutputPath = Join-Path $RepoRoot 'bin\wx_channel_insight_jobs.exe' }
$OutputPath = [IO.Path]::GetFullPath($OutputPath)
New-Item -ItemType Directory -Path (Split-Path -Parent $OutputPath) -Force | Out-Null
$temporary = $OutputPath + '.building-' + [guid]::NewGuid().ToString('N') + '.exe'
Push-Location -LiteralPath $RepoRoot
try {
    & go build -o $temporary ./apps/jobs
    if ($LASTEXITCODE -ne 0) { throw "go build jobs failed with exit code $LASTEXITCODE; existing executable was preserved." }
    if (-not (Test-Path -LiteralPath $temporary -PathType Leaf)) { throw 'go build did not produce the jobs executable.' }
    Move-Item -LiteralPath $temporary -Destination $OutputPath -Force
} finally {
    Pop-Location
    if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Force }
}
[pscustomobject]@{ repoRoot = $RepoRoot; outputPath = $OutputPath; built = $true } | ConvertTo-Json
