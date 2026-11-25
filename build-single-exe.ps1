# OzeVPN Tek Dosya (Standalone) Masaüstü Uygulaması Derleyicisi
# - sing-box.exe ve Tor/Snowflake ikililerini SHA256 doğrulayarak tek .exe içine gömer
# - -H=windowsgui ile siyah konsol penceresi olmadan doğrudan masaüstü uygulaması olarak açılır
$ErrorActionPreference = 'Stop'

# 1. Verify bundled binary SHA256 checksums
if (Test-Path .\BINARY_CHECKSUMS.txt) {
  foreach ($line in (Get-Content .\BINARY_CHECKSUMS.txt)) {
    $t = $line.Trim()
    if (-not $t -or $t.StartsWith('#')) { continue }
    $parts = $t -split '\s+', 2
    $expected = $parts[0].ToUpper()
    $relPath = $parts[1].Trim()
    if (-not (Test-Path $relPath)) { throw "Eksik ikili dosya: $relPath" }
    $actual = (Get-FileHash $relPath -Algorithm SHA256).Hash.ToUpper()
    if ($actual -ne $expected) { throw "SHA256 uyuşmazlığı ($relPath)" }
    Write-Output "SHA256_OK: $relPath"
  }
}

$stage = ".\dist\_embed_stage"
$embedZip = ".\internal\embedded\runtime.zip"
$outExe = ".\dist\OzeVPN.exe"
$outSha = ".\dist\OzeVPN.exe.sha256"

Remove-Item $stage, $embedZip -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path "$stage\bin" | Out-Null
New-Item -ItemType Directory -Force -Path ".\dist" | Out-Null

Copy-Item .\bin\sing-box.exe "$stage\bin\sing-box.exe" -Force
Copy-Item .\bin\tor "$stage\bin\tor" -Recurse -Force

try {
  Write-Output "Gömülü çalışma zamanı arşivi hazırlanıyor..."
  Compress-Archive -Path "$stage\bin" -DestinationPath $embedZip -CompressionLevel Optimal -Force

  Write-Output "Tek dosya masaüstü uygulaması (dist\OzeVPN.exe) derleniyor..."
  go build -tags standalone -trimpath -ldflags="-H=windowsgui -s -w" -o $outExe .
  if ($LASTEXITCODE -ne 0) { throw "go build başarısız oldu" }

  $hash = (Get-FileHash $outExe -Algorithm SHA256).Hash.ToUpper()
  Set-Content -Path $outSha -Value "$hash  OzeVPN.exe"
  $sizeMB = [math]::Round((Get-Item $outExe).Length / 1MB, 1)
  Write-Output "SINGLE_EXE_READY: $outExe ($sizeMB MB) SHA256=$hash"
} finally {
  Remove-Item $stage, $embedZip -Recurse -Force -ErrorAction SilentlyContinue
}
