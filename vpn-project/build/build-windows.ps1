param(
    [string]$OutputDir = "dist/windows",
    [switch]$IncludeRuntimeBinary = $true,
    [string]$RuntimeBinaryPath = "",
    [string]$SingBoxZipUrl = "",
    [switch]$AutoDownloadZig = $true
)

$ErrorActionPreference = "Stop"

function Invoke-NativeCommand {
    param(
        [Parameter(Mandatory = $true)][string]$Command,
        [string[]]$Args = @(),
        [string]$ErrorHint = ""
    )

    & $Command @Args
    if ($LASTEXITCODE -ne 0) {
        if ([string]::IsNullOrWhiteSpace($ErrorHint)) {
            throw "Command failed: $Command $($Args -join ' ') (exit code $LASTEXITCODE)"
        }
        throw "$ErrorHint (exit code $LASTEXITCODE)"
    }
}

function Resolve-GoExecutable {
    $direct = Get-Command go -ErrorAction SilentlyContinue
    if ($direct -and -not [string]::IsNullOrWhiteSpace($direct.Source)) {
        return $direct.Source
    }

    if (-not [string]::IsNullOrWhiteSpace($env:GOROOT)) {
        $gorootGo = Join-Path $env:GOROOT "bin\go.exe"
        if (Test-Path $gorootGo) {
            return $gorootGo
        }
    }

    $toolchainRoot = Join-Path $env:USERPROFILE "go\pkg\mod\golang.org"
    if (Test-Path $toolchainRoot) {
        $candidates = Get-ChildItem -Path $toolchainRoot -Directory -Filter "toolchain@*" -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending
        foreach ($candidate in $candidates) {
            $goExe = Join-Path $candidate.FullName "bin\go.exe"
            if (Test-Path $goExe) {
                return $goExe
            }
        }
    }

    throw "Go toolchain not found. Install Go or add go.exe to PATH."
}

function Resolve-SingBoxZipUrl {
    param([string]$OverrideUrl)

    if (-not [string]::IsNullOrWhiteSpace($OverrideUrl)) {
        return $OverrideUrl
    }

    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/SagerNet/sing-box/releases/latest"
    $asset = $release.assets |
        Where-Object { $_.name -match "^sing-box-.*-windows-amd64\.zip$" } |
        Select-Object -First 1

    if (-not $asset -or [string]::IsNullOrWhiteSpace($asset.browser_download_url)) {
        throw "Unable to resolve sing-box windows-amd64 zip from latest release."
    }

    return $asset.browser_download_url
}

function Expand-SingBoxBinary {
    param(
        [Parameter(Mandatory = $true)][string]$ZipPath,
        [Parameter(Mandatory = $true)][string]$DestinationPath,
        [Parameter(Mandatory = $true)][string]$TempDir
    )

    $extractDir = Join-Path $TempDir "sing-box-extract"
    if (Test-Path $extractDir) {
        Remove-Item -Recurse -Force $extractDir
    }
    Expand-Archive -Path $ZipPath -DestinationPath $extractDir -Force

    $binary = Get-ChildItem -Path $extractDir -Recurse -Filter "sing-box.exe" -File | Select-Object -First 1
    if (-not $binary) {
        throw "sing-box.exe was not found in archive: $ZipPath"
    }
    Copy-Item -LiteralPath $binary.FullName -Destination $DestinationPath -Force
}

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$distDir = Join-Path $root $OutputDir
$runtimeDir = Join-Path $distDir "runtime/windows"
$defaultGoCache = Join-Path $root ".gocache"
$tmpRoot = Join-Path $root ".tmp/build-windows"
$zigLocalCache = Join-Path $root ".tmp/zig-local-cache"
$zigGlobalCache = Join-Path $root ".tmp/zig-global-cache"

New-Item -ItemType Directory -Path $distDir -Force | Out-Null
New-Item -ItemType Directory -Path $runtimeDir -Force | Out-Null
if (-not $env:GOCACHE) {
    $env:GOCACHE = $defaultGoCache
}
New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
New-Item -ItemType Directory -Path $tmpRoot -Force | Out-Null
New-Item -ItemType Directory -Path $zigLocalCache -Force | Out-Null
New-Item -ItemType Directory -Path $zigGlobalCache -Force | Out-Null

Push-Location $root
try {
    $goExe = Resolve-GoExecutable

    $env:TEMP = $tmpRoot
    $env:TMP = $tmpRoot
    if (-not $env:ZIG_LOCAL_CACHE_DIR) {
        $env:ZIG_LOCAL_CACHE_DIR = $zigLocalCache
    }
    if (-not $env:ZIG_GLOBAL_CACHE_DIR) {
        $env:ZIG_GLOBAL_CACHE_DIR = $zigGlobalCache
    }

    Write-Host "Using Go: $goExe"

    Write-Host "[1/6] Downloading modules..."
    Invoke-NativeCommand -Command $goExe -Args @("mod", "download") -ErrorHint "go mod download failed"

    Write-Host "[2/6] Checking gcc (Fyne requires CGO)..."
    $gcc = Get-Command gcc -ErrorAction SilentlyContinue
    if ($gcc) {
        $env:CC = "gcc"
    } else {
        $zigVersion = "0.14.0"
        $zigDir = Join-Path $root ".tools/zig-$zigVersion"
        $zigExe = Join-Path $zigDir "zig.exe"
        $zigZip = Join-Path $root ".tools/zig-windows-x86_64-$zigVersion.zip"
        $zigUrl = "https://ziglang.org/download/$zigVersion/zig-windows-x86_64-$zigVersion.zip"

        if (-not (Test-Path $zigExe)) {
            if (-not $AutoDownloadZig) {
                throw "gcc not found and zig is not available. Install gcc or run with -AutoDownloadZig."
            }
            Write-Host "      gcc not found, downloading portable zig..."
            New-Item -ItemType Directory -Path (Join-Path $root ".tools") -Force | Out-Null
            Invoke-WebRequest -Uri $zigUrl -OutFile $zigZip
            Expand-Archive -Path $zigZip -DestinationPath (Join-Path $root ".tools") -Force
            $expanded = Join-Path $root ".tools/zig-windows-x86_64-$zigVersion"
            if (Test-Path $zigDir) {
                Remove-Item -Recurse -Force $zigDir
            }
            Rename-Item -Path $expanded -NewName "zig-$zigVersion"
        }
        $env:CC = "$zigExe cc"
    }

    Write-Host "[3/6] Building vpnclient.exe..."
    $env:CGO_ENABLED = "1"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    Invoke-NativeCommand -Command $goExe -Args @("build", "-o", (Join-Path $distDir "vpnclient.exe"), "./cmd/gui") -ErrorHint "go build for vpnclient.exe failed"

    Write-Host "[4/6] Preparing sing-box runtime binary..."
    if ($IncludeRuntimeBinary) {
        $runtimeExe = Join-Path $runtimeDir "sing-box.exe"
        if ($RuntimeBinaryPath -and (Test-Path $RuntimeBinaryPath)) {
            Copy-Item -LiteralPath $RuntimeBinaryPath -Destination $runtimeExe -Force
        } else {
            $zipUrl = Resolve-SingBoxZipUrl -OverrideUrl $SingBoxZipUrl
            $zipPath = Join-Path $tmpRoot "sing-box-windows-amd64.zip"
            Invoke-WebRequest -Uri $zipUrl -OutFile $zipPath
            Expand-SingBoxBinary -ZipPath $zipPath -DestinationPath $runtimeExe -TempDir $tmpRoot
        }
    }

    Write-Host "[5/6] Copying docs..."
    Get-ChildItem -LiteralPath $root -Filter "*.md" | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $distDir $_.Name) -Force
    }
    $settingsPath = Join-Path $root "client.settings.json"
    if (Test-Path $settingsPath) {
        Copy-Item -LiteralPath $settingsPath -Destination (Join-Path $distDir "client.settings.json") -Force
    }

    Write-Host "[6/6] Creating zip..."
    $zipPath = Join-Path $root "dist/vpnclient-windows-amd64.zip"
    if (Test-Path $zipPath) {
        Remove-Item $zipPath -Force
    }
    Compress-Archive -Path (Join-Path $distDir "*") -DestinationPath $zipPath

    Write-Host "Done."
    Write-Host " - $distDir"
    Write-Host " - $zipPath"
}
finally {
    Pop-Location
}
