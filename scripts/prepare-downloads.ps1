$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$downloads = Join-Path $workspace 'public/downloads'
$desktop = Join-Path $workspace 'dist/desktop'
$desktopVersion = (Get-Content (Join-Path $workspace 'cmd/web/desktop/wails.json') -Raw | ConvertFrom-Json).info.productVersion
$androidMetadata = Get-Content (Join-Path $workspace 'android/app/build/outputs/apk/release/output-metadata.json') -Raw | ConvertFrom-Json
$androidVersion = $androidMetadata.elements[0].versionName
$androidVersionCode = $androidMetadata.elements[0].versionCode

New-Item -ItemType Directory -Force -Path $downloads | Out-Null
Copy-Item -LiteralPath (Join-Path $desktop 'bin/InovarApp Desktop-amd64-installer.exe') -Destination (Join-Path $downloads 'InovarApp-Windows-Setup.exe') -Force
Copy-Item -LiteralPath (Join-Path $desktop 'InovarApp-Windows-portable.zip') -Destination (Join-Path $downloads 'InovarApp-Windows-portable.zip') -Force

$files = @(
  @{ name = 'InovarApp-Android.apk'; platform = 'android'; version = $androidVersion; versionCode = $androidVersionCode },
  @{ name = 'InovarApp-Windows-Setup.exe'; platform = 'windows'; version = $desktopVersion },
  @{ name = 'InovarApp-Windows-portable.zip'; platform = 'windows-portable'; version = $desktopVersion }
)
foreach ($artifact in $files) {
  $path = Join-Path $downloads $artifact.name
  $file = Get-Item -LiteralPath $path
  if ($file.Length -lt 1000000) { throw "Pacote incompleto: $path" }
  $artifact.bytes = $file.Length
  $artifact.sha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
  $artifact.url = '/downloads/' + $artifact.name
}
$manifest = [ordered]@{
  release = '2026.10.07.1'
  publishedAt = [DateTimeOffset]::UtcNow.ToOffset([TimeSpan]::FromHours(-3)).ToString('o')
  apiBaseUrl = 'https://inovarapp.vercel.app'
  files = $files
  ios = @{ distribution = 'pwa'; url = 'https://inovarapp.vercel.app/'; nativeStatus = 'requires-apple-developer-signing' }
}
$json = $manifest | ConvertTo-Json -Depth 6
[IO.File]::WriteAllText((Join-Path $downloads 'release.json'), $json, [Text.UTF8Encoding]::new($false))
$checksums = ($files | ForEach-Object { $_.sha256 + '  ' + $_.name }) -join "`n"
[IO.File]::WriteAllText((Join-Path $downloads 'SHA256SUMS.txt'), $checksums + "`n", [Text.UTF8Encoding]::new($false))
Write-Output $json
