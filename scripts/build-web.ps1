$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$output = Join-Path $workspace 'web\app.wasm'
$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH

try {
  $env:GOOS = 'js'
  $env:GOARCH = 'wasm'
  Push-Location $workspace

  # Use the current packaged assets, then embed this build's WASM for Wails.
  $packagedAssets = Join-Path $workspace 'core\ui\webapp\static\web'
  New-Item -ItemType Directory -Force -Path (Join-Path $workspace 'web') | Out-Null
  Get-ChildItem -LiteralPath $packagedAssets -File | Where-Object Name -NE 'app.wasm' | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $workspace 'web') -Force
  }
  $desktopBuildDir = Join-Path $workspace 'dist\desktop'
  New-Item -ItemType Directory -Force -Path $desktopBuildDir | Out-Null
  Copy-Item -LiteralPath (Join-Path $workspace 'public\icon-512.png') -Destination (Join-Path $desktopBuildDir 'appicon.png') -Force
  go build -o $output ./cmd/web
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao compilar a interface WebAssembly.' }
  Copy-Item -LiteralPath $output -Destination (Join-Path $packagedAssets 'app.wasm') -Force
} finally {
  if ($null -eq $previousGoos) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGoos }
  if ($null -eq $previousGoarch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGoarch }
  Pop-Location -ErrorAction SilentlyContinue
}
