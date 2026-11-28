# Fetches and SHA-256 verifies official upstream Android tunnel cores:
#   - heiher/hev-socks5-tunnel 2.20.0 (hev-socks5-tunnel.aar)
#   - SagerNet/sing-box v1.14.6 (android-arm64 & android-amd64 -> libsingbox.so)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$cache = Join-Path $root ".core-cache"
$libsDir = Join-Path $root "app\libs"
$jniArm64 = Join-Path $root "app\src\main\jniLibs\arm64-v8a"
$jniX64 = Join-Path $root "app\src\main\jniLibs\x86_64"

New-Item -ItemType Directory -Force -Path $cache, $libsDir, $jniArm64, $jniX64 | Out-Null

$expected = @{}
foreach ($line in Get-Content (Join-Path $root "ANDROID_CHECKSUMS.txt")) {
    $t = $line.Trim()
    if (-not $t -or $t.StartsWith("#")) { continue }
    $parts = $t -split '\s+', 2
    $expected[$parts[1].Trim()] = $parts[0].ToUpper()
}

$urls = @{
    "hev-socks5-tunnel.aar" = "https://github.com/heiher/hev-socks5-tunnel/releases/download/2.20.0/hev-socks5-tunnel.aar"
    "sing-box-1.14.6-android-arm64.tar.gz" = "https://github.com/SagerNet/sing-box/releases/download/v1.14.6/sing-box-1.14.6-android-arm64.tar.gz"
    "sing-box-1.14.6-android-amd64.tar.gz" = "https://github.com/SagerNet/sing-box/releases/download/v1.14.6/sing-box-1.14.6-android-amd64.tar.gz"
}

foreach ($name in $urls.Keys) {
    $dst = Join-Path $cache $name
    if (-not (Test-Path $dst)) {
        Write-Host "Downloading $name..."
        Invoke-WebRequest -Uri $urls[$name] -OutFile $dst -UseBasicParsing
    }
    $actual = (Get-FileHash $dst -Algorithm SHA256).Hash.ToUpper()
    if ($actual -ne $expected[$name]) {
        Remove-Item $dst -Force -ErrorAction SilentlyContinue
        throw "SHA256 mismatch for ${name}: expected=$($expected[$name]) actual=$actual"
    }
    Write-Host "SHA256_OK: $name"
}

Copy-Item (Join-Path $cache "hev-socks5-tunnel.aar") (Join-Path $libsDir "hev-socks5-tunnel.aar") -Force

$extract = Join-Path $cache "extract"
Remove-Item $extract -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path "$extract\arm64", "$extract\amd64" | Out-Null

tar -xzf (Join-Path $cache "sing-box-1.14.6-android-arm64.tar.gz") -C "$extract\arm64"
tar -xzf (Join-Path $cache "sing-box-1.14.6-android-amd64.tar.gz") -C "$extract\amd64"

$armBin = Get-ChildItem "$extract\arm64" -Recurse -Filter "sing-box" | Select-Object -First 1
$x64Bin = Get-ChildItem "$extract\amd64" -Recurse -Filter "sing-box" | Select-Object -First 1
if (-not $armBin -or -not $x64Bin) { throw "sing-box binary not found inside archive" }

Copy-Item $armBin.FullName (Join-Path $jniArm64 "libsingbox.so") -Force
Copy-Item $x64Bin.FullName (Join-Path $jniX64 "libsingbox.so") -Force
Remove-Item $extract -Recurse -Force -ErrorAction SilentlyContinue

Write-Host "ANDROID_CORES_READY"
