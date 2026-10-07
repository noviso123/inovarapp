$ErrorActionPreference = 'Stop'
$package = Split-Path -Parent $PSCommandPath
$source = Join-Path $package 'InovarApp.exe'
$publicEnvironment = Join-Path $package '.env.local'
if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
  throw 'InovarApp.exe não está junto do instalador.'
}
if (-not (Test-Path -LiteralPath $publicEnvironment -PathType Leaf)) {
  throw 'Configuração pública do Supabase não está junto do instalador.'
}

$installDir = Join-Path $env:LOCALAPPDATA 'Programs\InovarApp'
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$installedExe = Join-Path $installDir 'InovarApp.exe'
Copy-Item -LiteralPath $source -Destination $installedExe -Force
Copy-Item -LiteralPath $publicEnvironment -Destination (Join-Path $installDir '.env.local') -Force

$protocol = 'HKCU:\Software\Classes\inovarapp-desktop'
New-Item -Path $protocol -Force | Out-Null
Set-Item -LiteralPath $protocol -Value 'URL:InovarApp Desktop Protocol'
New-ItemProperty -LiteralPath $protocol -Name 'URL Protocol' -Value '' -PropertyType String -Force | Out-Null
$commandKey = Join-Path $protocol 'shell\open\command'
New-Item -Path $commandKey -Force | Out-Null
Set-Item -LiteralPath $commandKey -Value ('"' + $installedExe + '" "%1"')

$menuDir = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\InovarApp'
New-Item -ItemType Directory -Force -Path $menuDir | Out-Null
$shortcutPath = Join-Path $menuDir 'InovarApp.lnk'
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $installedExe
$shortcut.WorkingDirectory = $installDir
$shortcut.IconLocation = $installedExe
$shortcut.Save()

Write-Output "InovarApp instalado em $installDir"
Write-Output 'O protocolo OAuth inovarapp-desktop:// foi registrado para este usuário.'
