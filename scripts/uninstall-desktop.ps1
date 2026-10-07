$ErrorActionPreference = 'Stop'
$installDir = Join-Path $env:LOCALAPPDATA 'Programs\InovarApp'
$menuDir = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\InovarApp'
if (Get-Process -Name InovarApp -ErrorAction SilentlyContinue) {
  throw 'Feche o InovarApp antes de remover o aplicativo.'
}
Remove-Item -LiteralPath 'HKCU:\Software\Classes\inovarapp-desktop' -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $menuDir -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $installDir -Recurse -Force -ErrorAction SilentlyContinue
Write-Output 'InovarApp e o protocolo OAuth foram removidos deste usuário.'
