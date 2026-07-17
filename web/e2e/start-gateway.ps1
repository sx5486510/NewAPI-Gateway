$ErrorActionPreference = 'Stop'

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("newapi-cpa-e2e-" + [System.Guid]::NewGuid().ToString('N'))
$tempRoot = [System.IO.Path]::GetFullPath($tempRoot)
$tempBase = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())

function Test-SafeE2ETempPath {
  param([string]$Path)

  $resolved = [System.IO.Path]::GetFullPath($Path)
  $name = [System.IO.Path]::GetFileName($resolved.TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar))
  return $resolved.StartsWith($tempBase, [System.StringComparison]::OrdinalIgnoreCase) -and $name.StartsWith('newapi-cpa-e2e-', [System.StringComparison]::Ordinal)
}

if (-not (Test-SafeE2ETempPath $tempRoot)) {
  throw "refusing unsafe e2e temp path: $tempRoot"
}

New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $tempRoot 'upload') -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $tempRoot 'cpa-runtime') -Force | Out-Null

try {
  Push-Location $repoRoot
  try {
    Push-Location (Join-Path $repoRoot 'web')
    try {
      npm run build
    } finally {
      Pop-Location
    }

    $exePath = Join-Path $tempRoot 'newapi-gateway.exe'
    $compiler = @('gcc', 'clang', 'clang-cl', 'cl') |
      ForEach-Object { Get-Command $_ -ErrorAction SilentlyContinue } |
      Select-Object -First 1
    if ($null -eq $compiler) {
      throw "Task 10 E2E requires CGO_ENABLED=1 and a C compiler (gcc, clang, clang-cl, or cl) for go-sqlite3; no compiler was found on PATH."
    }
    $env:CGO_ENABLED = '1'
    go build -o $exePath .
  } finally {
    Pop-Location
  }

  $env:PORT = '3031'
  $env:GIN_MODE = 'release'
  $env:SESSION_SECRET = 'cpa-e2e-session-secret'
  $env:SQL_DRIVER = 'sqlite'
  $env:SQL_DSN = ''
  $env:SQLITE_PATH = Join-Path $tempRoot 'gateway-e2e.db'
  $env:UPLOAD_PATH = Join-Path $tempRoot 'upload'
  $env:CPA_RUNTIME_DIR = Join-Path $tempRoot 'cpa-runtime'
  $env:REDIS_CONN_STRING = ''
  $env:HTTPS_ENABLED = ''

  Push-Location $tempRoot
  try {
    & $exePath
  } finally {
    Pop-Location
  }
} finally {
  if (Test-SafeE2ETempPath $tempRoot -and (Test-Path $tempRoot)) {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force
  }
}
