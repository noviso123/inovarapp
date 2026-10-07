$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$signingDir = Join-Path $env:LOCALAPPDATA 'InovarApp/signing'
$keystorePath = Join-Path $signingDir 'inovarapp-android-release.p12'
$passwordPath = Join-Path $signingDir 'inovarapp-android-release-password.dpapi'
$keyAlias = 'inovarapp-release'

if (-not (Test-Path $signingDir)) {
  New-Item -ItemType Directory -Path $signingDir -Force | Out-Null
}
if ((Test-Path $keystorePath) -xor (Test-Path $passwordPath)) {
  throw "O arquivo de assinatura Android está incompleto em $signingDir. Preserve o arquivo da chave e sua senha juntos."
}

$javaCandidates = @(
  $env:JAVA_HOME,
  $env:ANDROID_STUDIO_JDK,
  (Join-Path $env:ProgramFiles 'Android/Android Studio/jbr'),
  (Join-Path $env:LOCALAPPDATA 'Programs/Android Studio/jbr')
) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
if (Test-Path (Join-Path $env:TEMP 'inovarapp-jdk21')) {
  $javaCandidates += Get-ChildItem (Join-Path $env:TEMP 'inovarapp-jdk21') -Directory | Sort-Object Name -Descending | ForEach-Object FullName
}
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
  throw 'O build Android requer JDK 21 ou superior. Instale pelo Android Studio ou defina JAVA_HOME.'
}
$env:JAVA_HOME = $javaHome
$keytool = Join-Path $javaHome 'bin/keytool.exe'
if (-not (Test-Path $keytool)) { throw "keytool não encontrado em $javaHome" }

if (-not (Test-Path $keystorePath)) {
  $randomBytes = [byte[]]::new(48)
  $randomGenerator = [System.Security.Cryptography.RandomNumberGenerator]::Create()
  try {
    $randomGenerator.GetBytes($randomBytes)
  } finally {
    $randomGenerator.Dispose()
  }
  $password = [Convert]::ToBase64String($randomBytes).Replace('+', '-').Replace('/', '_').TrimEnd('=')
  [Array]::Clear($randomBytes, 0, $randomBytes.Length)
  $securePassword = ConvertTo-SecureString $password -AsPlainText -Force
  $protectedPassword = ConvertFrom-SecureString $securePassword
  Set-Content -LiteralPath $passwordPath -Value $protectedPassword -NoNewline
  $env:INOVAR_ANDROID_KEYSTORE_PASSWORD = $password
  $env:INOVAR_ANDROID_KEY_PASSWORD = $password
  $env:INOVAR_ANDROID_KEY_ALIAS = $keyAlias
  $env:INOVAR_ANDROID_KEYSTORE = $keystorePath
  & $keytool -genkeypair -noprompt -keystore $keystorePath -storetype PKCS12 -alias $keyAlias -keyalg RSA -keysize 3072 -validity 10000 -dname 'CN=InovarApp, OU=Release, O=Inovar Refrigeracao, L=Vila Velha, ST=ES, C=BR' -storepass:env INOVAR_ANDROID_KEYSTORE_PASSWORD -keypass:env INOVAR_ANDROID_KEY_PASSWORD
  if ($LASTEXITCODE -ne 0) {
    Remove-Item -LiteralPath $passwordPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $keystorePath -Force -ErrorAction SilentlyContinue
    throw 'Não foi possível criar a chave de assinatura Android.'
  }
} else {
  $protectedPassword = Get-Content -LiteralPath $passwordPath -Raw
  $securePassword = ConvertTo-SecureString $protectedPassword
  $passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)
  try {
    $password = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer)
  } finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer)
  }
  $env:INOVAR_ANDROID_KEYSTORE_PASSWORD = $password
  $env:INOVAR_ANDROID_KEY_PASSWORD = $password
  $env:INOVAR_ANDROID_KEY_ALIAS = $keyAlias
  $env:INOVAR_ANDROID_KEYSTORE = $keystorePath
}

$env:ANDROID_HOME = if ($env:ANDROID_HOME) { $env:ANDROID_HOME } else { Join-Path $env:LOCALAPPDATA 'Android/Sdk' }
$env:ANDROID_SDK_ROOT = $env:ANDROID_HOME
if (-not (Test-Path (Join-Path $env:ANDROID_HOME 'platforms/android-36/android.jar'))) {
  throw 'Android SDK Platform 36 não foi encontrado.'
}

Push-Location $workspace
try {
  & (Join-Path $PSScriptRoot 'build-mobile.ps1')
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao compilar e sincronizar a interface para Android.' }
  & (Join-Path $workspace 'android/gradlew.bat') -p (Join-Path $workspace 'android') :app:assembleRelease --no-daemon
  if ($LASTEXITCODE -ne 0) { throw 'Gradle falhou ao gerar o APK Android de release.' }

  $apk = Join-Path $workspace 'android/app/build/outputs/apk/release/app-release.apk'
  if (-not (Test-Path $apk)) { throw 'Gradle terminou sem gerar o APK de release.' }
  $apksigner = Get-ChildItem (Join-Path $env:ANDROID_HOME 'build-tools') -Directory | Sort-Object Name -Descending | ForEach-Object { Join-Path $_.FullName 'apksigner.bat' } | Where-Object { Test-Path $_ } | Select-Object -First 1
  if (-not $apksigner) { throw 'Android apksigner não foi encontrado no SDK.' }
  & $apksigner verify --verbose $apk
  if ($LASTEXITCODE -ne 0) { throw 'A assinatura do APK não passou na verificação do Android SDK.' }

  $download = Join-Path $workspace 'public/downloads/InovarApp-Android.apk'
  Copy-Item -LiteralPath $apk -Destination $download -Force
  Write-Output "APK Android assinado e pronto para distribuição: $download"
  Write-Output "Chave privada mantida fora do repositório: $signingDir"
} finally {
  $env:INOVAR_ANDROID_KEYSTORE_PASSWORD = $null
  $env:INOVAR_ANDROID_KEY_PASSWORD = $null
  $env:INOVAR_ANDROID_KEY_ALIAS = $null
  $env:INOVAR_ANDROID_KEYSTORE = $null
  Pop-Location
}
