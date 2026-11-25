# OzeVPN v6 Taşınabilir (Arkadaşlara Gönderilebilir) ZIP Paketleyici
$ErrorActionPreference = 'Stop'
$outDir = ".\dist\OzeVPN-v6"
$zipPath = ".\dist\OzeVPN-v6-Portable.zip"

Remove-Item .\dist -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path "$outDir\bin" | Out-Null

go build -ldflags="-s -w" -o "$outDir\ozevpn.exe" .
Copy-Item .\README.md "$outDir\README.md" -Force
Copy-Item .\bin\sing-box.exe "$outDir\bin\sing-box.exe" -Force
Copy-Item .\bin\tor "$outDir\bin\tor" -Recurse -Force
Copy-Item .\router "$outDir\router" -Recurse -Force
Copy-Item .\vps "$outDir\vps" -Recurse -Force

Compress-Archive -Path "$outDir\*" -DestinationPath $zipPath -Force
$sizeMB = [math]::Round((Get-Item $zipPath).Length / 1MB, 1)
Write-Output "PORTABLE_ZIP_READY: $zipPath ($sizeMB MB)"
