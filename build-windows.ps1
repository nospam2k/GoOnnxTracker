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

# Configuration
$ONNX_VERSION = "1.25.1"
$ONNX_URL = "https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VERSION}/onnxruntime-win-x64-${ONNX_VERSION}.zip"
$ONNX_DIR = "onnxruntime"

# Check if Go is installed
Write-Host "Checking Go installation..." -ForegroundColor Yellow
$goVersion = go version 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Go is not installed or not in PATH" -ForegroundColor Red
    exit 1
}
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
$rsrcPath = Join-Path $gopath "bin" "rsrc.exe"

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
    go build -ldflags $ldflags -o gotracker.exe .
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Build failed" -ForegroundColor Red
        exit 1
    }
    Write-Host "✓ Build successful" -ForegroundColor Green
} catch {
    Write-Host "ERROR: Build error: $_" -ForegroundColor Red
    exit 1
}

# Copy DLL
Write-Host "`nCopying runtime DLL..." -ForegroundColor Yellow
try {
    Copy-Item "$onnx_path/lib/onnxruntime.dll" -Destination "onnxruntime.dll" -Force
    Write-Host "✓ onnxruntime.dll copied" -ForegroundColor Green
} catch {
    Write-Host "ERROR: Failed to copy DLL: $_" -ForegroundColor Red
    exit 1
}

# Summary
Write-Host "`n✓ Build Complete!" -ForegroundColor Green
Write-Host "`nBuild artifacts:" -ForegroundColor Cyan
Get-Item "gotracker.exe", "onnxruntime.dll" -ErrorAction SilentlyContinue | ForEach-Object {
    $size = [math]::Round($_.Length / 1MB, 2)
    Write-Host "  $($_.Name) ($size MB)"
}

Write-Host "`nTo run the app, ensure both gotracker.exe and onnxruntime.dll are in the same directory."
