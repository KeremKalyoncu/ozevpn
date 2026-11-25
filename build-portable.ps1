# OzeVPN v6 Taşınabilir (Arkadaşlara Gönderilebilir) ZIP Paketleyici
# - Paketlemeden önce BINARY_CHECKSUMS.txt ile SHA256 bütünlüğünü doğrular
# - -trimpath -ldflags="-s -w" ile temiz/tekrarlanabilir derleme yapar
# - Çıktı ZIP için .sha256 doğrulama dosyası üretir
$ErrorActionPreference = 'Stop'
$outDir = ".\dist\OzeVPN-v6"
$zipPath = ".\dist\OzeVPN-v6-Portable.zip"
$shaPath = ".\dist\OzeVPN-v6-Portable.zip.sha256"

# 1. Verify bundled binary SHA256 checksums before packaging
if (Test-Path .\BINARY_CHECKSUMS.txt) {
  foreach ($line in (Get-Content .\BINARY_CHECKSUMS.txt)) {
    $t = $line.Trim()
    if (-not $t -or $t.StartsWith('#')) { continue }
    $parts = $t -split '\s+', 2
    $expected = $parts[0].ToUpper()
    $relPath = $parts[1].Trim()
    if (-not (Test-Path $relPath)) {
      throw "Eksik ikili dosya: $relPath"
    }
    $actual = (Get-FileHash $relPath -Algorithm SHA256).Hash.ToUpper()
    if ($actual -ne $expected) {
      throw "SHA256 uyuşmazlığı ($relPath): beklenen=$expected gerçek=$actual"
    }
    Write-Output "SHA256_OK: $relPath"
  }
}

Remove-Item .\dist -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path "$outDir\bin" | Out-Null

go build -trimpath -ldflags="-s -w" -o "$outDir\ozevpn.exe" .
Copy-Item .\README.md "$outDir\README.md" -Force
Copy-Item .\THIRD_PARTY_LICENSES.md "$outDir\THIRD_PARTY_LICENSES.md" -Force
if (Test-Path .\SECURITY.md) { Copy-Item .\SECURITY.md "$outDir\SECURITY.md" -Force }
Copy-Item .\bin\sing-box.exe "$outDir\bin\sing-box.exe" -Force
Copy-Item .\bin\tor "$outDir\bin\tor" -Recurse -Force
Copy-Item .\router "$outDir\router" -Recurse -Force
Copy-Item .\vps "$outDir\vps" -Recurse -Force

Compress-Archive -Path "$outDir\*" -DestinationPath $zipPath -Force
$zipHash = (Get-FileHash $zipPath -Algorithm SHA256).Hash.ToUpper()
Set-Content -Path $shaPath -Value "$zipHash  OzeVPN-v6-Portable.zip"
$sizeMB = [math]::Round((Get-Item $zipPath).Length / 1MB, 1)
Write-Output "PORTABLE_ZIP_READY: $zipPath ($sizeMB MB) SHA256=$zipHash"
