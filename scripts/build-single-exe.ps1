# OzeVPN Tek Dosya (Standalone) Masaüstü Uygulaması Derleyicisi
# - sing-box.exe, Tor, lyrebird.exe ve GeoIP verilerini SHA256 doğrulayarak tek .exe içine gömer
# - Git commit & tarih provenance bilgilerini -ldflags ile gömer, CycloneDX SBOM üretir
# - İsteğe bağlı Authenticode kod imzalama ($env:OZEVPN_SIGN_THUMBPRINT) destekler
$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
  # 1. Verify ALL bundled runtime SHA256 checksums (sing-box, tor, lyrebird, geoip, geoip6)
  if (Test-Path .\BINARY_CHECKSUMS.txt) {
    foreach ($line in (Get-Content .\BINARY_CHECKSUMS.txt)) {
      $t = $line.Trim()
      if (-not $t -or $t.StartsWith('#')) { continue }
      $parts = $t -split '\s+', 2
      $expected = $parts[0].ToUpper()
      $relPath = $parts[1].Trim()
      if (-not (Test-Path $relPath)) { throw "Eksik ikili dosya: $relPath" }
      $actual = (Get-FileHash $relPath -Algorithm SHA256).Hash.ToUpper()
      if ($actual -ne $expected) { throw "SHA256 uyuşmazlığı ($relPath): beklenen=$expected gerçek=$actual" }
      Write-Output "SHA256_OK: $relPath"
    }
  }

  $commit = (git rev-parse --short HEAD 2>$null)
  if (-not $commit) { $commit = "unknown" }
  $buildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

  $stage = ".\dist\_embed_stage"
  $embedZip = ".\internal\embedded\runtime.zip"
  $outExe = ".\dist\OzeVPN.exe"
  $outSha = ".\dist\OzeVPN.exe.sha256"
  $outSbom = ".\dist\sbom.cdx.json"

  Remove-Item $stage, $embedZip -Recurse -Force -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Force -Path "$stage\bin" | Out-Null
  New-Item -ItemType Directory -Force -Path ".\dist" | Out-Null

  Copy-Item .\bin\sing-box.exe "$stage\bin\sing-box.exe" -Force
  Copy-Item .\bin\tor "$stage\bin\tor" -Recurse -Force

  try {
    Write-Output "Gömülü çalışma zamanı arşivi hazırlanıyor..."
    Compress-Archive -Path "$stage\bin" -DestinationPath $embedZip -CompressionLevel Optimal -Force

    Write-Output "Tek dosya masaüstü uygulaması (dist\OzeVPN.exe) derleniyor (commit=$commit)..."
    $ldflags = "-H=windowsgui -s -w -X main.buildCommit=$commit -X main.buildDate=$buildDate"
    go build -tags standalone -trimpath -ldflags="$ldflags" -o $outExe ./cmd/ozevpn
    if ($LASTEXITCODE -ne 0) { throw "go build başarısız oldu" }

    # Optional Authenticode code signing when certificate thumbprint is provided
    if ($env:OZEVPN_SIGN_THUMBPRINT) {
      $signtool = Get-Command signtool.exe -ErrorAction SilentlyContinue
      if ($signtool) {
        Write-Output "Authenticode kod imzası uygulanıyor..."
        & $signtool.Source sign /sha1 $env:OZEVPN_SIGN_THUMBPRINT /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 $outExe
        if ($LASTEXITCODE -ne 0) { throw "Authenticode imzalama başarısız" }
      }
    }

    $hash = (Get-FileHash $outExe -Algorithm SHA256).Hash.ToUpper()
    Set-Content -Path $outSha -Value "$hash  OzeVPN.exe  # commit=$commit date=$buildDate"

    # Generate CycloneDX 1.5 minimal machine-readable SBOM
    $sbom = @{
      bomFormat   = "CycloneDX"
      specVersion = "1.5"
      version     = 1
      metadata    = @{
        timestamp = $buildDate
        component = @{
          type    = "application"
          name    = "OzeVPN"
          version = "6.1.0-$commit"
          hashes  = @(@{ alg = "SHA-256"; content = $hash })
        }
      }
      components  = @(
        @{ type = "application"; name = "sing-box"; version = "1.14.4"; licenses = @(@{ license = @{ id = "GPL-3.0-or-later" } }) },
        @{ type = "application"; name = "tor"; version = "0.4.9.17"; licenses = @(@{ license = @{ id = "BSD-3-Clause" } }) },
        @{ type = "application"; name = "lyrebird-snowflake"; version = "16.0.3"; licenses = @(@{ license = @{ id = "BSD-3-Clause" } }) }
      )
    } | ConvertTo-Json -Depth 6
    Set-Content -Path $outSbom -Value $sbom -Encoding UTF8

    $sizeMB = [math]::Round((Get-Item $outExe).Length / 1MB, 1)
    Write-Output "SINGLE_EXE_READY: $outExe ($sizeMB MB) SHA256=$hash commit=$commit SBOM=$outSbom"
  } finally {
    Remove-Item $stage, $embedZip -Recurse -Force -ErrorAction SilentlyContinue
  }
} finally {
  Pop-Location
}
