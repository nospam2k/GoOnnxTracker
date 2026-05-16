# Windows Build Script for GoOnnxTracker
# This script downloads ONNX Runtime and builds the Windows executable

param(
    [string]$Version = "dev",
    [switch]$Help
)

if ($Help) {
    Write-Host @"
GoOnnxTracker Windows Build Script

Usage:
  .\build-windows.ps1 [options]

Options:
  -Version <version>  Build version (default: dev)
  -Help              Show this help message

Example:
  .\build-windows.ps1 -Version "1.0.0"
"@
    exit 0
}

$ErrorActionPreference = "Stop"

Write-Host "GoOnnxTracker Windows Build Script" -ForegroundColor Cyan
Write-Host "Version: $Version`n" -ForegroundColor Cyan

# Paths — script lives in build/, project root is one level up
$projectRoot = Split-Path $PSScriptRoot -Parent
$buildDir = $PSScriptRoot
Push-Location $projectRoot
try {

# Configuration
$ONNX_VERSION = "1.25.1"
$ONNX_URL = "https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VERSION}/onnxruntime-win-x64-${ONNX_VERSION}.zip"
$ONNX_DIR = "onnxruntime"

# Refresh PATH to pick up any tools installed in previous runs
$env:PATH = [System.Environment]::GetEnvironmentVariable("PATH", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("PATH", "User")

# Check if Go is installed, install via winget if not
Write-Host "Checking Go installation..." -ForegroundColor Yellow
if (!(Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "Go not found. Installing via winget..." -ForegroundColor Yellow
    winget install GoLang.Go --accept-source-agreements --accept-package-agreements
    $env:PATH = [System.Environment]::GetEnvironmentVariable("PATH", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("PATH", "User")
    if (!(Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Host "ERROR: Go installed but not found in PATH. Please restart your shell and re-run." -ForegroundColor Red
        exit 1
    }
}
$goVersion = go version
Write-Host "✓ $goVersion" -ForegroundColor Green

# Download ONNX Runtime if not present
if (!(Test-Path $ONNX_DIR)) {
    Write-Host "`nDownloading ONNX Runtime $ONNX_VERSION..." -ForegroundColor Yellow
    try {
        Invoke-WebRequest -Uri $ONNX_URL -OutFile "onnxruntime.zip" -ErrorAction Stop
        Write-Host "✓ Downloaded" -ForegroundColor Green

        Write-Host "Extracting..." -ForegroundColor Yellow
        Expand-Archive -Path "onnxruntime.zip" -DestinationPath "." -ErrorAction Stop
        Write-Host "✓ Extracted" -ForegroundColor Green

        # Rename extracted folder
        $folderName = "onnxruntime-win-x64-${ONNX_VERSION}"
        if (Test-Path $folderName) {
            Move-Item $folderName $ONNX_DIR -ErrorAction Stop
        } else {
            $found = Get-ChildItem -Filter "onnxruntime*" -Directory | Select-Object -First 1
            if ($found) {
                Move-Item $found.FullName $ONNX_DIR -ErrorAction Stop
            } else {
                Write-Host "ERROR: Could not find extracted ONNX Runtime folder" -ForegroundColor Red
                exit 1
            }
        }

        Remove-Item "onnxruntime.zip" -ErrorAction SilentlyContinue
        Write-Host "✓ ONNX Runtime ready at: $ONNX_DIR" -ForegroundColor Green
    } catch {
        Write-Host "ERROR: Failed to download/extract ONNX Runtime: $_" -ForegroundColor Red
        exit 1
    }
} else {
    Write-Host "✓ ONNX Runtime already present at: $ONNX_DIR" -ForegroundColor Green
}

# Verify ONNX Runtime structure
Write-Host "`nVerifying ONNX Runtime..." -ForegroundColor Yellow
if (!(Test-Path "$ONNX_DIR/lib") -or !(Test-Path "$ONNX_DIR/include")) {
    Write-Host "ERROR: Invalid ONNX Runtime structure (missing lib or include)" -ForegroundColor Red
    exit 1
}
Write-Host "✓ ONNX Runtime structure valid" -ForegroundColor Green

# Check if gcc is installed (required for CGo), install MSYS2 + MinGW-w64 via winget if not
Write-Host "`nChecking GCC installation..." -ForegroundColor Yellow
$msys2GccPath = "C:\msys64\mingw64\bin"
if (Test-Path "$msys2GccPath\gcc.exe") {
    $env:PATH = "$msys2GccPath;" + $env:PATH
}
if (!(Get-Command gcc -ErrorAction SilentlyContinue)) {
    Write-Host "GCC not found. Installing MSYS2 + MinGW-w64 via winget..." -ForegroundColor Yellow
    winget install --id MSYS2.MSYS2 --accept-source-agreements --accept-package-agreements
    Write-Host "Installing MinGW-w64 GCC via pacman..." -ForegroundColor Yellow
    & "C:\msys64\usr\bin\pacman.exe" -S --noconfirm mingw-w64-x86_64-gcc
    $env:PATH = "$msys2GccPath;" + $env:PATH
    if (!(Get-Command gcc -ErrorAction SilentlyContinue)) {
        Write-Host "ERROR: GCC not found after MSYS2 install. Please restart your shell and re-run." -ForegroundColor Red
        exit 1
    }
}
Write-Host "✓ $(gcc --version | Select-Object -First 1)" -ForegroundColor Green

# Set environment variables
Write-Host "`nSetting up build environment..." -ForegroundColor Yellow
$onnx_path = (Resolve-Path $ONNX_DIR).Path -replace '\\', '/'
$env:ONNX_RUNTIME_PATH = $onnx_path
$env:CGO_LDFLAGS = "-L$onnx_path/lib -lonnxruntime"
$env:CGO_CPPFLAGS = "-I$onnx_path/include"
Write-Host "✓ Environment variables set" -ForegroundColor Green

# Install rsrc if needed
Write-Host "`nChecking rsrc installation..." -ForegroundColor Yellow
$gopath = go env GOPATH
$rsrcPath = Join-Path $gopath "bin\rsrc.exe"

if (!(Test-Path $rsrcPath)) {
    Write-Host "Installing rsrc..." -ForegroundColor Yellow
    try {
        go install github.com/akavel/rsrc@latest
        Write-Host "✓ rsrc installed" -ForegroundColor Green
    } catch {
        Write-Host "ERROR: Failed to install rsrc: $_" -ForegroundColor Red
        exit 1
    }
} else {
    Write-Host "✓ rsrc already installed" -ForegroundColor Green
}

# Generate resource file
Write-Host "`nGenerating Windows resource file..." -ForegroundColor Yellow
try {
    & $rsrcPath -arch amd64 -ico app.ico -o app.syso
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Failed to generate app.syso" -ForegroundColor Red
        exit 1
    }
    Write-Host "✓ app.syso generated" -ForegroundColor Green
} catch {
    Write-Host "ERROR: Failed to run rsrc: $_" -ForegroundColor Red
    exit 1
}

# Build
Write-Host "`nBuilding GoOnnxTracker..." -ForegroundColor Yellow
try {
    $ldflags = "-H windowsgui -X main.version=$Version"
    go build -ldflags $ldflags -o (Join-Path $buildDir "gotracker.exe") .
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Build failed" -ForegroundColor Red
        exit 1
    }
    Write-Host "✓ Build successful" -ForegroundColor Green
} catch {
    Write-Host "ERROR: Build error: $_" -ForegroundColor Red
    exit 1
}

# Summary
Write-Host "`n✓ Build Complete!" -ForegroundColor Green
Write-Host "`nBuild artifacts:" -ForegroundColor Cyan
Get-Item (Join-Path $buildDir "gotracker.exe") -ErrorAction SilentlyContinue | ForEach-Object {
    $size = [math]::Round($_.Length / 1MB, 2)
    Write-Host "  build\$($_.Name) ($size MB)"
}
} finally {
    Pop-Location
}
