$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
Push-Location $workspace
try {
  powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'build-mobile.ps1')
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao compilar e sincronizar a UI Go/WASM para iOS.' }

  npx cap ls
  if ($LASTEXITCODE -ne 0) { throw 'Não foi possível listar os plugins Capacitor instalados.' }

  if (-not (Test-Path 'ios/App/App.xcodeproj/project.pbxproj')) {
    throw 'O scaffold iOS do Capacitor não foi encontrado após a sincronização.'
  }
  if (-not (Test-Path 'ios/App/App/Info.plist')) {
    throw 'Info.plist do app iOS não foi encontrado.'
  }
  if (-not (Test-Path 'ios/App/App/App.entitlements')) {
    throw 'Entitlements de push do app iOS não foram encontrados.'
  }

  Write-Output 'Bundle Go/WASM e plugins sincronizados para iOS. Abra ios/App/App.xcodeproj em macOS/Xcode para build, assinatura e execução.'
} finally {
  Pop-Location
}
