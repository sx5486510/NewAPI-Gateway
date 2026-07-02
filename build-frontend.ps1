param(
    [switch]$SkipInstall
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$webDir = Join-Path $root 'web'
$buildDir = Join-Path $webDir 'build'

Write-Host '========================================' -ForegroundColor Cyan
Write-Host '  NewAPI-Gateway frontend web build'
Write-Host '========================================' -ForegroundColor Cyan
Write-Host ''

Write-Host '[1/4] Checking Node.js and npm...' -ForegroundColor Yellow
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Host 'ERROR: Node.js was not found. Install Node.js first.' -ForegroundColor Red
    exit 1
}
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    Write-Host 'ERROR: npm was not found. Install npm first.' -ForegroundColor Red
    exit 1
}
Write-Host "OK: Node.js $(node --version)"
Write-Host "OK: npm $(npm --version)"
Write-Host ''

Write-Host '[2/4] Checking web directory...' -ForegroundColor Yellow
if (-not (Test-Path -LiteralPath $webDir -PathType Container)) {
    Write-Host "ERROR: web directory does not exist: $webDir" -ForegroundColor Red
    exit 1
}
Write-Host "OK: $webDir"
Write-Host ''

Push-Location $webDir
try {
    Write-Host '[3/4] Checking dependencies...' -ForegroundColor Yellow
    if ($SkipInstall) {
        Write-Host 'SKIP: dependency install was skipped.'
    }
    elseif (-not (Test-Path -LiteralPath 'node_modules' -PathType Container)) {
        Write-Host 'node_modules was not found. Running npm install...'
        npm install
        if ($LASTEXITCODE -ne 0) {
            Write-Host 'ERROR: dependency install failed.' -ForegroundColor Red
            exit 1
        }
    }
    else {
        Write-Host 'OK: node_modules exists.'
    }
    Write-Host ''

    Write-Host '[4/4] Building frontend web...' -ForegroundColor Yellow
    npm run build
    if ($LASTEXITCODE -ne 0) {
        Write-Host 'ERROR: frontend build failed.' -ForegroundColor Red
        exit 1
    }
}
finally {
    Pop-Location
}

if (-not (Test-Path -LiteralPath $buildDir -PathType Container)) {
    Write-Host "ERROR: build output was not created: $buildDir" -ForegroundColor Red
    exit 1
}

$buildBytes = (Get-ChildItem -LiteralPath $buildDir -Recurse -File | Measure-Object -Property Length -Sum).Sum
$buildSizeMb = [math]::Round(($buildBytes / 1MB), 2)

Write-Host ''
Write-Host '========================================' -ForegroundColor Cyan
Write-Host '  Frontend web build completed' -ForegroundColor Green
Write-Host '========================================' -ForegroundColor Cyan
Write-Host 'Output: web\build\'
Write-Host "Size: $buildSizeMb MB"
Write-Host ''
