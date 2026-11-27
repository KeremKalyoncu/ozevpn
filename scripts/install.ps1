# OzeVPN Tek Satır Kurulum Betiği (SHA-256 Doğrulamalı)
# Kullanım:
#   irm https://raw.githubusercontent.com/KeremKalyoncu/ozevpn/main/scripts/install.ps1 | iex
$ErrorActionPreference = 'Stop'

$repo = 'KeremKalyoncu/ozevpn'
$installDir = Join-Path $env:LOCALAPPDATA 'OzeVPN'
$exePath = Join-Path $installDir 'OzeVPN.exe'
$tmpExe = Join-Path $env:TEMP ("OzeVPN-download-" + [guid]::NewGuid().ToString('N') + ".exe")
$tmpSha = $tmpExe + '.sha256'

Write-Host "OzeVPN son sürüm bilgisi GitHub Releases üzerinden alınıyor..." -ForegroundColor Cyan
$release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ 'User-Agent' = 'OzeVPN-Installer' }
$exeAsset = $release.assets | Where-Object { $_.name -eq 'OzeVPN.exe' } | Select-Object -First 1
$shaAsset = $release.assets | Where-Object { $_.name -eq 'OzeVPN.exe.sha256' } | Select-Object -First 1

if (-not $exeAsset -or -not $shaAsset) {
  throw "Release ($($release.tag_name)) içinde OzeVPN.exe veya OzeVPN.exe.sha256 bulunamadı."
}

try {
  Write-Host "İndiriliyor: OzeVPN $($release.tag_name)..." -ForegroundColor Cyan
  Invoke-WebRequest -Uri $exeAsset.browser_download_url -OutFile $tmpExe -UseBasicParsing
  Invoke-WebRequest -Uri $shaAsset.browser_download_url -OutFile $tmpSha -UseBasicParsing

  $expected = ((Get-Content $tmpSha -Raw).Trim() -split '\s+')[0].ToUpper()
  $actual = (Get-FileHash $tmpExe -Algorithm SHA256).Hash.ToUpper()
  if ($actual -ne $expected) {
    throw "SHA-256 bütünlük doğrulaması BAŞARISIZ! Beklenen=$expected Gerçek=$actual"
  }
  Write-Host "SHA-256 doğrulandı: $actual" -ForegroundColor Green

  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  Move-Item -Force $tmpExe $exePath

  # Masaüstü kısayolu oluştur
  $desktop = [Environment]::GetFolderPath('Desktop')
  $lnkPath = Join-Path $desktop 'OzeVPN.lnk'
  $shell = New-Object -ComObject WScript.Shell
  $sc = $shell.CreateShortcut($lnkPath)
  $sc.TargetPath = $exePath
  $sc.WorkingDirectory = $installDir
  $sc.Description = 'OzeVPN — Yerel Hibrit Masaüstü VPN'
  $sc.Save()

  Write-Host "`nOzeVPN $($release.tag_name) başarıyla kuruldu!" -ForegroundColor Green
  Write-Host "Konum: $exePath"
  Write-Host "Kısayol: $lnkPath"
  Write-Host "Kaldırmak için: `"$exePath`" -uninstall" -ForegroundColor DarkGray
} finally {
  Remove-Item -Force -ErrorAction SilentlyContinue $tmpExe, $tmpSha
}
