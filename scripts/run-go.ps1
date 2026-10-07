$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot

Push-Location $workspace
try {
  & (Join-Path $PSScriptRoot 'build-web.ps1')
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

  if ($env:PORT) {
    $listenPort = $env:PORT
  } else {
    $listenPort = '8080'
  }
  Write-Host "Iniciando app Go em http://localhost:$listenPort"
  go run ./cmd/web
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
  Pop-Location
}
