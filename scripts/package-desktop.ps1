$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$desktopProject = Join-Path $workspace 'cmd\web\desktop'
$distribution = Join-Path $workspace 'dist\desktop'
$package = Join-Path $distribution 'portable'
$executable = Join-Path $distribution 'bin\InovarApp.exe'
$installer = Join-Path $distribution 'bin\InovarApp Desktop-amd64-installer.exe'

$settings = @{}
foreach ($filename in @('.env.local', '.env')) {
  $path = Join-Path $workspace $filename
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { continue }
  foreach ($rawLine in Get-Content -LiteralPath $path) {
    $line = $rawLine.Trim()
    if ($line.StartsWith('export ')) { $line = $line.Substring(7).Trim() }
    if ($line -match '^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') {
      $name = $matches[1]
      $value = $matches[2].Trim()
      if ($name -eq 'VITE_SUPABASE_URL') { $name = 'SUPABASE_URL' }
      if ($name -eq 'VITE_SUPABASE_ANON_KEY') { $name = 'SUPABASE_ANON_KEY' }
      if ($name -notin @('SUPABASE_URL', 'SUPABASE_ANON_KEY') -or $settings.ContainsKey($name)) { continue }
      if ($value.Length -ge 2 -and (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'")))) {
        $value = $value.Substring(1, $value.Length - 2)
      }
      $settings[$name] = $value
    }
  }
}
if (-not $settings['SUPABASE_URL'] -or -not $settings['SUPABASE_ANON_KEY']) {
  throw 'SUPABASE_URL e SUPABASE_ANON_KEY públicos são obrigatórios para autenticar o aplicativo desktop.'
}
$apiBase = $env:GOAPP_API_BASE_URL
if ([string]::IsNullOrWhiteSpace($apiBase)) { $apiBase = 'https://inovarapp.vercel.app' }
if (-not $apiBase.StartsWith('https://', [System.StringComparison]::OrdinalIgnoreCase)) {
  throw 'GOAPP_API_BASE_URL deve usar HTTPS para a versão distribuída do desktop.'
}
$ldflags = "-X main.bundledSupabaseURL=$($settings['SUPABASE_URL']) -X main.bundledSupabaseAnonKey=$($settings['SUPABASE_ANON_KEY']) -X main.bundledAPIBaseURL=$apiBase"

$nsisBin = Join-Path ${env:ProgramFiles(x86)} 'NSIS\Bin'
if ((Test-Path (Join-Path $nsisBin 'makensis.exe')) -and -not (Get-Command makensis -ErrorAction SilentlyContinue)) {
  $env:PATH = $nsisBin + [IO.Path]::PathSeparator + $env:PATH
}

Push-Location $desktopProject
try {
  wails build -clean -platform windows/amd64 -s -skipbindings -nsis -installscope user -ldflags $ldflags
  if ($LASTEXITCODE -ne 0) { throw 'Falha ao compilar o aplicativo Wails.' }
} finally {
  Pop-Location
}
if (-not (Test-Path -LiteralPath $executable -PathType Leaf)) { throw 'Executável Wails não foi produzido.' }
if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) { throw 'Instalador NSIS do InovarApp não foi produzido.' }

New-Item -ItemType Directory -Force -Path $package | Out-Null
Copy-Item -LiteralPath $executable -Destination (Join-Path $package 'InovarApp.exe') -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'install-desktop.ps1') -Destination $package -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'uninstall-desktop.ps1') -Destination $package -Force
Copy-Item -LiteralPath (Join-Path $desktopProject 'README.md') -Destination (Join-Path $package 'LEIA-ME.md') -Force
$publicEnvironment = "SUPABASE_URL=$($settings['SUPABASE_URL'])`nSUPABASE_ANON_KEY=$($settings['SUPABASE_ANON_KEY'])`nGOAPP_API_BASE_URL=$apiBase`n"
[System.IO.File]::WriteAllText((Join-Path $package '.env.local'), $publicEnvironment, [System.Text.UTF8Encoding]::new($false))
$archive = Join-Path $distribution 'InovarApp-Windows-portable.zip'
Compress-Archive -Path (Join-Path $package '*') -DestinationPath $archive -Force
Write-Output "Pacote criado: $archive"
Write-Output "Instalador criado: $installer"
