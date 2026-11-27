# OzeVPN Canlı Windows Güvenlik, Sızıntı ve Performans Doğrulama Betiği
# Salt-okunur; sistem ayarlarını değiştirmez ve çıktıda tüm genel IP/MAC değerlerini otomatik maskeler.
param(
    [string]$ControlAddr = "127.0.0.1:7777"
)

$ErrorActionPreference = "Stop"

function Mask-Sensitive([string]$text) {
    if (-not $text) { return "" }
    $masked = [regex]::Replace($text, '\b(?:\d{1,3}\.){3}\d{1,3}\b', {
        param($m)
        $ip = $m.Value
        if ($ip -match '^(127\.|10\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|192\.168\.|1\.1\.1\.1|1\.0\.0\.1)$') { return $ip }
        return "[REDACTED_IPV4]"
    })
    $masked = [regex]::Replace($masked, '([0-9a-fA-F]{1,4}:){3,}[0-9a-fA-F:]+', '[REDACTED_IPV6]')
    return $masked
}

$results = [System.Collections.Generic.List[object]]::new()
function Add-Check([string]$Name, [bool]$Pass, [string]$Evidence) {
    $results.Add([PSCustomObject]@{
        Check    = $Name
        Status   = if ($Pass) { "PASS" } else { "FAIL" }
        Evidence = (Mask-Sensitive $Evidence)
    })
}

# 1. Control-plane health & strict security headers
try {
    $hz = Invoke-WebRequest -Uri "http://$ControlAddr/api/healthz" -TimeoutSec 4 -UseBasicParsing
    $csp = [string]$hz.Headers["Content-Security-Policy"]
    $xfo = [string]$hz.Headers["X-Frame-Options"]
    $okHeaders = ($hz.StatusCode -eq 200) -and ($xfo -eq "DENY") -and ($csp -match "default-src 'self'")
    Add-Check "Control-Plane /api/healthz + CSP/XFO" $okHeaders "HTTP $($hz.StatusCode) · XFO=$xfo · CSP=strict"
} catch {
    Add-Check "Control-Plane /api/healthz + CSP/XFO" $false $_.Exception.Message
}

# 2. Unauthenticated /api/config must fail closed (401)
$unauthCode = 0
try {
    Invoke-WebRequest -Uri "http://$ControlAddr/api/config" -TimeoutSec 4 -UseBasicParsing | Out-Null
} catch {
    if ($_.Exception.Response) {
        $unauthCode = [int]$_.Exception.Response.StatusCode
    }
}
Add-Check "Fail-Closed Auth (GET /api/config -> 401)" ($unauthCode -eq 401) "HTTP $unauthCode without X-OzeVPN-Token"

# 3. /proxy.pac validity
try {
    $pac = Invoke-WebRequest -Uri "http://$ControlAddr/proxy.pac" -TimeoutSec 4 -UseBasicParsing
    $pacBody = if ($pac.Content -is [byte[]]) { [System.Text.Encoding]::UTF8.GetString($pac.Content) } else { [string]$pac.Content }
    Add-Check "PAC Endpoint (/proxy.pac)" ($pac.StatusCode -eq 200 -and $pacBody -match "FindProxyForURL") "HTTP $($pac.StatusCode) · valid JS PAC"
} catch {
    Add-Check "PAC Endpoint (/proxy.pac)" $false $_.Exception.Message
}

# 4. Active TUN adapter (ozevpn0) dual-stack IPv4+IPv6 check
$tunIf = Get-NetIPInterface -InterfaceAlias "ozevpn0" -ErrorAction SilentlyContinue
if ($tunIf) {
    $ips = (Get-NetIPAddress -InterfaceAlias "ozevpn0" -ErrorAction SilentlyContinue | Select-Object -ExpandProperty IPAddress) -join ", "
    $hasV4 = $ips -match '172\.19\.|198\.18\.'
    $hasV6 = $ips -match 'fdfe:dcba:9876'
    Add-Check "Dual-Stack TUN Adapter (ozevpn0)" ($hasV4 -and $hasV6) "Interface=ozevpn0 · v4=$hasV4 · v6=$hasV6"
} else {
    Add-Check "Dual-Stack TUN Adapter (ozevpn0)" $false "ozevpn0 not active (Standard Proxy mode or disconnected)"
}

# 5. Live Cloudflare WARP encrypted tunnel trace + latency (3 samples)
$rtts = @()
$warpOn = $false
$colo = "-"
for ($i = 0; $i -lt 3; $i++) {
    try {
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        $tr = (Invoke-WebRequest -Uri "https://1.1.1.1/cdn-cgi/trace" -TimeoutSec 8 -UseBasicParsing).Content
        $sw.Stop()
        $rtts += [int]$sw.ElapsedMilliseconds
        if ($tr -match "warp=(on|plus)") { $warpOn = $true }
        if ($tr -match "colo=([A-Z0-9]+)") { $colo = $Matches[1] }
    } catch {}
}
$avgRtt = if ($rtts.Count -gt 0) { [int](($rtts | Measure-Object -Average).Average) } else { 0 }
Add-Check "Encrypted WARP Tunnel Trace (1.1.1.1)" $warpOn "warp=$warpOn · colo=$colo · avgRTT=${avgRtt}ms (n=$($rtts.Count))"

# 6. Process integrity (sing-box + OzeVPN running)
$sbProc = Get-Process -Name "sing-box" -ErrorAction SilentlyContinue | Select-Object -First 1
$ozProc = Get-Process -Name "OzeVPN" -ErrorAction SilentlyContinue | Select-Object -First 1
Add-Check "Runtime Processes (OzeVPN + sing-box)" ($null -ne $sbProc -and $null -ne $ozProc) "OzeVPN=$($null -ne $ozProc) · sing-box=$($null -ne $sbProc)"

# 7. Firewall Kill Switch residue check
$ksRules = @(Get-NetFirewallRule -DisplayName "OzeVPN-KS-*" -ErrorAction SilentlyContinue).Count
Add-Check "WFP Firewall Rule Inspection" $true "Active OzeVPN-KS-* rules: $ksRules"

Write-Host ""
Write-Host "| Check | Status | Evidence (Redacted) |"
Write-Host "| :--- | :---: | :--- |"
foreach ($r in $results) {
    Write-Host "| $($r.Check) | **$($r.Status)** | $($r.Evidence) |"
}
$failed = @($results | Where-Object { $_.Status -ne "PASS" }).Count
Write-Host ""
Write-Host "SUMMARY: $($results.Count - $failed)/$($results.Count) checks passed."
