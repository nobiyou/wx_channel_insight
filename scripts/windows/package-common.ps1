# Shared packaging helpers. This file does not change runtime configuration.
$ErrorActionPreference = 'Stop'

function Resolve-PackageRepo([string]$Requested) {
    if ([string]::IsNullOrWhiteSpace($Requested)) { $Requested = Join-Path $PSScriptRoot '..\..' }
    return (Resolve-Path -LiteralPath $Requested).Path
}

function Resolve-PackageOutput([string]$Root, [string]$Requested, [switch]$Force) {
    $output = [IO.Path]::GetFullPath($Requested).TrimEnd('\', '/')
    $repo = [IO.Path]::GetFullPath($Root).TrimEnd('\', '/')
    $release = Join-Path $repo 'release'
    if ($output -eq $repo -or $repo.StartsWith($output + '\', [StringComparison]::OrdinalIgnoreCase) -or
        ($output.StartsWith($repo + '\', [StringComparison]::OrdinalIgnoreCase) -and
         -not $output.StartsWith($release + '\', [StringComparison]::OrdinalIgnoreCase))) {
        throw "Package output inside the repository must be below release/: $output"
    }
    $ancestor = $output
    while ($ancestor) {
        if (Test-Path -LiteralPath $ancestor) {
            if ((Get-Item -LiteralPath $ancestor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) {
                throw "Package output cannot traverse a junction or symlink: $ancestor"
            }
        }
        $ancestor = Split-Path -Parent $ancestor
    }
    if (Test-Path -LiteralPath $output) {
        if (-not $Force) { throw "Output already exists; use -Force to retain a backup and replace it: $output" }
        if (Test-Path -LiteralPath $output -PathType Container) {
            if (-not (Test-Path -LiteralPath (Join-Path $output 'manifest.json') -PathType Leaf)) {
                throw "Refusing to replace a directory without a package manifest: $output"
            }
            if (Test-Path -LiteralPath (Join-Path $output 'insight.local.json')) { throw 'Package has local configuration; choose a new output directory.' }
            foreach ($name in @('data', 'var')) {
                $state = Join-Path $output $name
                if ((Test-Path -LiteralPath $state) -and @(Get-ChildItem -LiteralPath $state -Force -Recurse -File).Count) {
                    throw "Package contains runtime data in $name; choose a new output directory."
                }
            }
        }
    }
    return $output
}

function New-PackageStage([string]$Root) {
    $release = Join-Path $Root 'release'
    New-Item -ItemType Directory -Force -Path $release | Out-Null
    if ((Get-Item -LiteralPath $release -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'release/ cannot be a junction or symlink.' }
    $stage = Join-Path $release ('.tmp-package-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $stage | Out-Null
    return $stage
}

function Remove-PackageStage([string]$Root, [string]$Stage) {
    $full = [IO.Path]::GetFullPath($Stage)
    $parent = [IO.Path]::GetFullPath((Join-Path $Root 'release'))
    if ((Split-Path -Parent $full) -ne $parent -or (Split-Path -Leaf $full) -notmatch '^\.tmp-package-[a-f0-9]{32}$') {
        throw "Unsafe staging cleanup path: $full"
    }
    if (Test-Path -LiteralPath $full) { Remove-Item -LiteralPath $full -Recurse -Force }
}

function Copy-PackageFile([string]$Source, [string]$Stage, [string]$Relative) {
    if ((Get-Item -LiteralPath $Source -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Cannot package symlink: $Source" }
    $destination = Join-Path $Stage $Relative
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $destination) | Out-Null
    Copy-Item -LiteralPath $Source -Destination $destination
}

function Write-PackageJson([object]$Value, [string]$Path) {
    [IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 12), [Text.UTF8Encoding]::new($false))
}

function Get-PackageSource([string]$Root) {
    $commit = 'unknown'
    $dirty = $null
    if ((Test-Path -LiteralPath (Join-Path $Root '.git')) -and (Get-Command git -ErrorAction SilentlyContinue)) {
        $result = & git -C $Root rev-parse HEAD 2>$null
        if ($LASTEXITCODE -eq 0) {
            $commit = [string]$result
            $status = & git -C $Root status --porcelain 2>$null
            if ($LASTEXITCODE -eq 0) { $dirty = [bool]$status }
        }
    }
    return [ordered]@{ commit = $commit; dirty = $dirty }
}

function Publish-Package([string]$Source, [string]$Output, [switch]$Force) {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Output) | Out-Null
    $backup = ''
    if (Test-Path -LiteralPath $Output) {
        if (-not $Force) { throw "Output appeared during build: $Output" }
        $backup = $Output + '.backup-' + [guid]::NewGuid().ToString('N')
        Move-Item -LiteralPath $Output -Destination $backup
    }
    try { Move-Item -LiteralPath $Source -Destination $Output -ErrorAction Stop }
    catch {
        if ($backup -and -not (Test-Path -LiteralPath $Output)) { Move-Item -LiteralPath $backup -Destination $Output }
        throw
    }
    if ($backup) { Write-Host "Previous package retained at $backup" }
}

function Compress-Package([string]$StageRoot, [string]$Archive) {
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    # .NET Framework on Windows can use backslashes in CreateFromDirectory ZIPs.
    # Emit portable entry names explicitly, including hidden templates and empty dirs.
    $base = [IO.Path]::GetFullPath($StageRoot).TrimEnd('\', '/')
    $zip = [IO.Compression.ZipFile]::Open($Archive, [IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($item in Get-ChildItem -LiteralPath $base -Force -Recurse) {
            $name = $item.FullName.Substring($base.Length + 1).Replace('\', '/')
            if ($item.PSIsContainer) { $null = $zip.CreateEntry($name + '/') }
            else { $null = [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $item.FullName, $name, [IO.Compression.CompressionLevel]::Optimal) }
        }
    } finally { $zip.Dispose() }
}
