$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$previousAPIBase = $env:GOAPP_API_BASE_URL
$apiBase = $previousAPIBase
if ([string]::IsNullOrWhiteSpace($apiBase)) {
  $apiBase = 'https://inovarapp.vercel.app'
}
if (-not [string]::IsNullOrWhiteSpace($apiBase)) {
  if (-not $apiBase.StartsWith('https://', [System.StringComparison]::OrdinalIgnoreCase)) {
    throw 'GOAPP_API_BASE_URL deve usar HTTPS.'
  }
  $probeUrl = $apiBase.TrimEnd('/') + '/api/whatsapp'
  $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
  if ($null -eq $curl) { throw 'curl.exe é necessário para validar o backend Go configurado.' }
  $probeHeaders = & $curl.Source -sS -D - -o NUL --max-time 20 $probeUrl 2>&1
  if ($LASTEXITCODE -ne 0) { throw "Não foi possível confirmar o dispatcher Go em $probeUrl." }
  $contentType = [regex]::Match(($probeHeaders -join "`n"), '(?im)^Content-Type:\s*([^\r\n]+)').Groups[1].Value.Trim()
  if ($contentType -notmatch '^application/json(?:\s*;|$)') { throw "O endpoint móvel respondeu '$contentType' em $probeUrl; esperava JSON da API Go." }
}

$env:GOAPP_API_BASE_URL = $apiBase
Push-Location $workspace
try {
  powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'build-web.ps1')
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao compilar a UI Go/WASM.' }
  go run ./cmd/web/export -out mobile/www
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao exportar a interface Go para o WebView móvel.' }
  $contactsBridgeOutput = Join-Path $workspace 'mobile/www/native-contacts.js'
  & (Join-Path $workspace 'node_modules/.bin/esbuild.cmd') (Join-Path $workspace 'src/platform/nativeContactsBridge.ts') --bundle --format=esm "--outfile=$contactsBridgeOutput"
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao compilar a ponte nativa de contatos.' }
  node (Join-Path $PSScriptRoot 'inject-mobile-contacts-bridge.mjs') (Join-Path $workspace 'mobile/www/index.html')
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao ligar a ponte nativa de contatos ao app exportado.' }
  npx cap sync
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao sincronizar os recursos dos apps Android e iOS.' }
} finally {
  $env:GOAPP_API_BASE_URL = $previousAPIBase
  Pop-Location
}
