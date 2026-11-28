#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CACHE="$ROOT/.core-cache"
LIBS_DIR="$ROOT/app/libs"
JNI_ARM64="$ROOT/app/src/main/jniLibs/arm64-v8a"
JNI_X64="$ROOT/app/src/main/jniLibs/x86_64"

mkdir -p "$CACHE" "$LIBS_DIR" "$JNI_ARM64" "$JNI_X64"

declare -A URLS=(
  ["hev-socks5-tunnel.aar"]="https://github.com/heiher/hev-socks5-tunnel/releases/download/2.20.0/hev-socks5-tunnel.aar"
  ["sing-box-1.14.6-android-arm64.tar.gz"]="https://github.com/SagerNet/sing-box/releases/download/v1.14.6/sing-box-1.14.6-android-arm64.tar.gz"
  ["sing-box-1.14.6-android-amd64.tar.gz"]="https://github.com/SagerNet/sing-box/releases/download/v1.14.6/sing-box-1.14.6-android-amd64.tar.gz"
)

for name in "${!URLS[@]}"; do
  dst="$CACHE/$name"
  if [[ ! -f "$dst" ]]; then
    echo "Downloading $name..."
    curl -fsSL -o "$dst" "${URLS[$name]}"
  fi
  expected="$(awk -v f="$name" '$2 == f { print toupper($1) }' "$ROOT/ANDROID_CHECKSUMS.txt")"
  actual="$(sha256sum "$dst" | awk '{ print toupper($1) }')"
  if [[ "$actual" != "$expected" ]]; then
    rm -f "$dst"
    echo "SHA256 mismatch for $name: expected=$expected actual=$actual" >&2
    exit 1
  fi
  echo "SHA256_OK: $name"
done

cp -f "$CACHE/hev-socks5-tunnel.aar" "$LIBS_DIR/hev-socks5-tunnel.aar"

EXTRACT="$CACHE/extract"
rm -rf "$EXTRACT"
mkdir -p "$EXTRACT/arm64" "$EXTRACT/amd64"
tar -xzf "$CACHE/sing-box-1.14.6-android-arm64.tar.gz" -C "$EXTRACT/arm64"
tar -xzf "$CACHE/sing-box-1.14.6-android-amd64.tar.gz" -C "$EXTRACT/amd64"

cp -f "$(find "$EXTRACT/arm64" -name sing-box -type f | head -n1)" "$JNI_ARM64/libsingbox.so"
cp -f "$(find "$EXTRACT/amd64" -name sing-box -type f | head -n1)" "$JNI_X64/libsingbox.so"
chmod 755 "$JNI_ARM64/libsingbox.so" "$JNI_X64/libsingbox.so"
rm -rf "$EXTRACT"
echo "ANDROID_CORES_READY"
