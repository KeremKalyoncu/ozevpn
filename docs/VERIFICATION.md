# Live Windows Field Verification Report

This document records repeatable, real-world Windows host verification results for **OzeVPN** (distinct from CI unit tests and static analysis badges). All public IP addresses and hardware identifiers are redacted.

## How to Reproduce Locally

While OzeVPN is running (ideally in Administrator **Oyun TUN** mode), run the read-only verification harness from the repository root:

```powershell
.\tools\verify-live.ps1
```

The script makes **zero** system mutations and automatically redacts public IPv4/IPv6 addresses.

## Latest Live Windows Run (`2026-11-28`, Windows 11 `amd64`, Admin `Oyun TUN`)

| Check | Status | Verified Evidence (Redacted) |
| :--- | :---: | :--- |
| **Control-Plane `/api/healthz` + Strict Headers** | **PASS** | `HTTP 200` · `X-Frame-Options: DENY` · `CSP: default-src 'self'` |
| **Fail-Closed Auth (`GET /api/config`)** | **PASS** | `HTTP 401` without `X-OzeVPN-Token` / session cookie |
| **PAC Endpoint (`/proxy.pac`)** | **PASS** | `HTTP 200` · valid `FindProxyForURL` JS payload |
| **Dual-Stack TUN Adapter (`ozevpn0`)** | **PASS** | `Interface=ozevpn0` · `IPv4=True` (`172.19.0.1/30`) · `IPv6=True` (`fdfe:dcba:9876::1/126`) |
| **Encrypted WARP WireGuard Tunnel Trace** | **PASS** | `https://1.1.1.1/cdn-cgi/trace` · `warp=on` · `colo=AMS` · `avgRTT=131ms` (`n=3`) |
| **Runtime Process Integrity** | **PASS** | `OzeVPN.exe` + `sing-box.exe` live > 3 hours uninterrupted (`0` watchdog crashes) |
| **WFP Firewall Residue Inspection** | **PASS** | Clean rule state (`0` orphaned `OzeVPN-KS-*` rules when Kill Switch disabled) |

**Result:** `7 / 7` checks passed.

## Scope & Limitations

- This verification confirms real-world behavior on a Windows 11 `amd64` residential connection; it is **not** a third-party penetration test.
- Standard (non-Admin) mode uses Windows WinINET proxy (`127.0.0.1:10808`) and does not capture raw UDP or non-proxy-aware processes; full system IPv4+IPv6+UDP tunneling and the WFP Kill Switch require Administrator **Oyun TUN** mode.
- External exit networks (Cloudflare WARP / Tor) are third-party infrastructures; users requiring full end-to-end exit ownership can import their own WireGuard (with Post-Quantum `PresharedKey`) or `VLESS+TLS/Reality` VPS via the `custom` profile (see [`vps/setup-vps.sh`](../vps/setup-vps.sh)).
