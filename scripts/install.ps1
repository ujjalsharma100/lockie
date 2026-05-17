# Install lockie from GitHub Releases (pre-built binary).
# Usage:
#   iwr -useb https://raw.githubusercontent.com/ujjalsharma100/lockie/main/scripts/install.ps1 | iex
#   $env:LOCKIE_VERSION = "v0.1.0"; iwr ... | iex

$ErrorActionPreference = "Stop"

$Repo = if ($env:LOCKIE_REPO) { $env:LOCKIE_REPO } else { "ujjalsharma100/lockie" }
$Force = $false
if ($args -contains "--force" -or $args -contains "-f") { $Force = $true }

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default {
        Write-Error "install.ps1: unsupported architecture: $($env:PROCESSOR_ARCHITECTURE) (supported: amd64)"
    }
}

if (-not $env:LOCKIE_VERSION) {
    $api = "https://api.github.com/repos/$Repo/releases/latest"
    $release = Invoke-RestMethod -Uri $api -Headers @{ "User-Agent" = "lockie-installer" }
    $Tag = $release.tag_name
    $Version = $Tag -replace '^v', ''
} else {
    $Tag = if ($env:LOCKIE_VERSION -match '^v') { $env:LOCKIE_VERSION } else { "v$($env:LOCKIE_VERSION)" }
    $Version = $Tag -replace '^v', ''
}

if (-not $Force -and (Get-Command lockie -ErrorAction SilentlyContinue)) {
    $existing = (Get-Command lockie).Source
    Write-Error "install.ps1: lockie already on PATH at $existing. Re-run with -Force or remove it first."
}

$Archive = "lockie_${Version}_windows_${Arch}.zip"
$Base = "https://github.com/$Repo/releases/download/$Tag"
$Url = "$Base/$Archive"

$InstallRoot = if ($env:LOCKIE_INSTALL_DIR) {
    $env:LOCKIE_INSTALL_DIR
} else {
    Join-Path $env:LOCALAPPDATA "Programs\lockie"
}
New-Item -ItemType Directory -Force -Path $InstallRoot | Out-Null
$Dest = Join-Path $InstallRoot "lockie.exe"

$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("lockie-install-" + [guid]::NewGuid().ToString())
New-Item -ItemType Directory -Force -Path $Tmp | Out-Null

try {
    Write-Host "Downloading $Url"
    $ZipPath = Join-Path $Tmp $Archive
    Invoke-WebRequest -Uri $Url -OutFile $ZipPath -UseBasicParsing

    $ChecksumsUrl = "$Base/checksums.txt"
    try {
        $ChecksumsPath = Join-Path $Tmp "checksums.txt"
        Invoke-WebRequest -Uri $ChecksumsUrl -OutFile $ChecksumsPath -UseBasicParsing
        $line = Get-Content $ChecksumsPath | Where-Object { $_ -match " $([regex]::Escape($Archive))`$" } | Select-Object -First 1
        if ($line) {
            $expected = ($line -split '\s+', 2)[0].ToLower()
            $actual = (Get-FileHash -Algorithm SHA256 -Path $ZipPath).Hash.ToLower()
            if ($actual -ne $expected) {
                throw "checksum mismatch for $Archive`n  expected: $expected`n  actual:   $actual"
            }
        }
    } catch {
        Write-Warning "install.ps1: could not verify checksum ($($_.Exception.Message))"
    }

    Expand-Archive -Path $ZipPath -DestinationPath $Tmp -Force
    $Bin = Join-Path $Tmp "lockie.exe"
    if (-not (Test-Path $Bin)) {
        throw "archive did not contain lockie.exe"
    }
    Copy-Item -Path $Bin -Destination $Dest -Force
} finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallRoot*") {
    [Environment]::SetEnvironmentVariable("Path", "$InstallRoot;$UserPath", "User")
    $env:Path = "$InstallRoot;$env:Path"
    Write-Host ""
    Write-Host "Added $InstallRoot to your user PATH (open a new terminal if lockie is not found)."
}

Write-Host ""
Write-Host "lockie $Version installed at $Dest"
Write-Host "Next steps:"
Write-Host "  lockie version"
Write-Host "  lockie install cursor --scope user"
Write-Host "  lockie install claude-code --scope user"
Write-Host "  lockie status"
