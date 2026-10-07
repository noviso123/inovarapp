$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$androidSdk = $env:ANDROID_HOME
if ([string]::IsNullOrWhiteSpace($androidSdk)) { $androidSdk = $env:ANDROID_SDK_ROOT }
if ([string]::IsNullOrWhiteSpace($androidSdk)) {
  $androidSdk = Join-Path $env:LOCALAPPDATA 'Android/Sdk'
}
if (-not (Test-Path (Join-Path $androidSdk 'platforms/android-36/android.jar'))) {
  throw "Android SDK Platform 36 não encontrado em $androidSdk. Instale-o pelo Android Studio ou SDK Manager."
}

$javaCandidates = @(
  $env:JAVA_HOME,
  $env:ANDROID_STUDIO_JDK,
  (Join-Path $env:ProgramFiles 'Android/Android Studio/jbr'),
  (Join-Path $env:LOCALAPPDATA 'Programs/Android Studio/jbr')
) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
$javaHome = $null
foreach ($candidate in $javaCandidates) {
  $java = Join-Path $candidate 'bin/java.exe'
  $releaseFile = Join-Path $candidate 'release'
  if (-not (Test-Path $java) -or -not (Test-Path $releaseFile)) { continue }
  $release = Get-Content -LiteralPath $releaseFile -Raw
  if ($release -match 'JAVA_VERSION="(?:1\.)?(2[1-9]|[3-9][0-9])(?:\.|"|\+)') {
    $javaHome = $candidate
    break
  }
}
if (-not $javaHome) {
  throw 'O build Android requer JDK 21 ou superior. Defina JAVA_HOME/ANDROID_STUDIO_JDK para um JDK compatível.'
}

$env:ANDROID_HOME = $androidSdk
$env:ANDROID_SDK_ROOT = $androidSdk
$env:JAVA_HOME = $javaHome
Push-Location $workspace
try {
  powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'build-mobile.ps1')
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao compilar e sincronizar o bundle Go/WASM para Android.' }
  & (Join-Path $workspace 'android/gradlew.bat') -p (Join-Path $workspace 'android') :app:assembleDebug --no-daemon
  if ($LASTEXITCODE -ne 0) { throw 'Gradle falhou ao gerar o APK de debug.' }

  $apk = Join-Path $workspace 'android/app/build/outputs/apk/debug/app-debug.apk'
  if (-not (Test-Path $apk)) { throw 'Gradle terminou sem gerar o APK esperado.' }
  Write-Output "APK de debug gerado: $apk"
} finally {
  Pop-Location
}
