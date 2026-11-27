<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/banner-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/banner-light.svg">
  <img alt="OzeVPN — Pure local hybrid VPN" src="assets/banner-dark.svg" width="100%">
</picture>

<br/>
<br/>

**[Türkçe](README.md) · English**

[![CI](https://github.com/KeremKalyoncu/ozevpn/actions/workflows/ci.yml/badge.svg)](https://github.com/KeremKalyoncu/ozevpn/actions/workflows/ci.yml)
[![CodeQL](https://github.com/KeremKalyoncu/ozevpn/actions/workflows/codeql.yml/badge.svg)](https://github.com/KeremKalyoncu/ozevpn/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/KeremKalyoncu/ozevpn/badge)](https://scorecard.dev/viewer/?uri=github.com/KeremKalyoncu/ozevpn)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=KeremKalyoncu_ozevpn&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=KeremKalyoncu_ozevpn)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=KeremKalyoncu_ozevpn&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=KeremKalyoncu_ozevpn)
[![REUSE status](https://api.reuse.software/badge/github.com/KeremKalyoncu/ozevpn)](https://api.reuse.software/info/github.com/KeremKalyoncu/ozevpn)
[![SLSA Provenance](https://github.com/KeremKalyoncu/ozevpn/actions/workflows/release.yml/badge.svg)](https://github.com/KeremKalyoncu/ozevpn/actions/workflows/release.yml)
[![Release](https://img.shields.io/github/v/release/KeremKalyoncu/ozevpn?color=0EA5E9)](https://github.com/KeremKalyoncu/ozevpn/releases)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-22C55E)](LICENSE)

**Zero third-party Go/npm dependencies. Single-file Windows desktop VPN (`.exe`) combining native Cloudflare WARP WireGuard (`TCP+UDP`), EU Tor + Snowflake, smart split tunneling, LAN proxy sharing, live throughput telemetry, and a WFP Kill Switch.**

[Quick Start](#quick-start) · [Architecture](#architecture) · [Security](#security--privacy) · [Supply Chain](#verification--supply-chain) · [Contributing](CONTRIBUTING.md)

</div>

---

## Live Preview

<div align="center">
  <img src="docs/screenshots/demo.gif" alt="OzeVPN Signal Cockpit Demo" width="90%" />
</div>

| Dark Theme (Obsidian Cockpit) | Light Theme (Cool Porcelain) |
| :---: | :---: |
| ![OzeVPN Dark Theme](docs/screenshots/signal-cockpit-dark.png) | ![OzeVPN Light Theme](docs/screenshots/signal-cockpit-light.png) |

---

## Why OzeVPN?

Named after Turkish **"Öz"** *(core, pure, unadorned)* and **Oze** *(an unobstructed natural pass)*, OzeVPN is a local, telemetry-free desktop tunnel controller built with pure Go stdlib and a `<120 KB` embedded Vanilla Web UI — no Electron, no accounts, no cloud backend.

| Capability | Typical Free VPNs | OzeVPN v6.1 |
| :--- | :--- | :--- |
| **External Dependencies** | Hundreds of npm / Electron / SDK packages | **`0` external Go/npm modules** (Go stdlib + `<120 KB` Vanilla UI) |
| **Gaming UDP (Roblox / Discord Voice)** | Often TCP-only SOCKS proxies | **Native pure-Go Cloudflare WARP WireGuard (full L3 TCP+UDP, `MTU 1280`, `25s` keepalive)** |
| **Smart Split Tunneling** | Routes everything through slow nodes | **Direct bypass** for banking, `.gov`, and 4K/Steam CDNs + EU Tor for geo-restricted TCP |
| **Home Wi-Fi / Phone / TV Sharing** | Requires router flashing | **1-click LAN Proxy Sharing** (`192.168.x.x:10808`, opt-in, disabled by default) |
| **Real-Time Telemetry** | Static or fake counters | **Live throughput (`↓/↑ B/s` & session total)** via authenticated loopback `sing-box` API |
| **Leak & Crash Protection** | Silent leaks on crash | **Dual-stack IPv6 TUN + DoH detour + Windows Firewall WFP fail-closed Kill Switch** |
| **Privacy & Accounts** | Accounts, emails, analytics SDKs | **Zero accounts, zero telemetry**, Windows **DPAPI + NTFS DACL** secret isolation |

---

## Architecture

```mermaid
flowchart LR
    subgraph User["Local Windows Host"]
        UI["Desktop Panel (Edge/Chrome --app)\nWebRTC non-proxied UDP disabled"]
        TRAY["Win32 System Tray\nWM_POWERBROADCAST Wake Recovery"]
        CFG["ozevpn.json (DPAPI + NTFS DACL)\n.ozevpn_token (256-bit, NTFS DACL)"]
    end

    subgraph Control["Control Plane (127.0.0.1:7777)"]
        API["Go HTTP API (internal/server)\n256-bit X-OzeVPN-Token + CSRF + Rate Limit"]
    end

    subgraph Data["Data Plane (sing-box 1.14+)"]
        PROXY["mixed-in :10808 (Loopback or Opt-in LAN)"]
        TUN["tun-in ozevpn0 (IPv4+IPv6, MTU 1280/1400)\n+ WFP Fail-Closed Kill Switch (Admin)"]
        DNS["DoH 1.1.1.1 (Tunnel Detour)\n+ 28 Ad/Tracker Reject Rules"]
    end

    subgraph Exit["Exit Layers"]
        WARP["Native WARP WireGuard\n(Full L3 TCP + UDP, 25s Keepalive)"]
        TOR["Tor 0.4.9.17 + Snowflake\n(EU TCP Exit + Watchdog)"]
        CUST["Custom Server\n(VLESS+TLS/Reality | WG+PQ PSK)"]
        DIR["Direct Line (Local & 4K CDN Bypass)"]
    end

    UI -->|"HttpOnly Cookie + X-OzeVPN-Token"| API
    TRAY -.-> API
    API --> CFG
    API -.->|"sing-box check + Traffic Stream"| TUN
    PROXY --> DNS
    TUN --> DNS
    DNS --> WARP
    DNS --> TOR
    DNS --> CUST
    DNS --> DIR
```

---

## Security & Privacy

- **Local IPC Auth & CSRF:** Per-session 256-bit `crypto/rand` token (`HttpOnly; SameSite=Strict` + mandatory `X-OzeVPN-Token` on mutations), loopback-only `Host` check (DNS rebinding defense), `Origin`/`Sec-Fetch-Site` CSRF guard, rate & concurrency limits.
- **Secret Isolation:** Custom server credentials and native WARP WireGuard private keys are encrypted via Windows **DPAPI** (`CryptProtectData`) and restricted with owner-only **NTFS DACL**. Active `sing-box-active.json` is deleted on disconnect.
- **Protocol Hardening:** Rejects plaintext `vless://`, remote plaintext `socks5://`, `allowInsecure=1`, and `tls.insecure: true`; supports WireGuard Post-Quantum `PresharedKey`; masks secrets in error logs (`RedactSecrets`).
- **Leak Protection & Kill Switch:** Dual-stack `IPv4 + IPv6` TUN, tunnel-detoured DoH DNS, `ipconfig /flushdns` on state changes, WebRTC non-proxied UDP restriction, and optional Windows Firewall (`blockoutbound`) Kill Switch in Admin `Oyun TUN` mode.

See [SECURITY.md](SECURITY.md), [PRIVACY.md](PRIVACY.md), and [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) for full details.

---

## Verification & Supply Chain

- **CI Gates (`.github/workflows/ci.yml`):** SHA-pinned Actions, `gofmt`, `go vet`, `staticcheck`, `node --check`, `go test -race -cover ./...` (14 packages + 36-combination official `sing-box check` matrix), fuzzing, `govulncheck`, secret scan, and a **live Windows `ozevpn.exe` HTTP/security smoke test**.
- **Static & Supply-Chain Security:** `CodeQL` SAST (`Go` + `JavaScript`), Linux Foundation **OpenSSF Scorecard**, `Dependabot`, `5/5` bundled runtime SHA-256 pinning (`BINARY_CHECKSUMS.txt`), and **CycloneDX 1.5 SBOM**.
- **Cryptographic Provenance (SLSA Attestation):**
  ```powershell
  gh attestation verify OzeVPN-core.exe -R KeremKalyoncu/ozevpn
  ```

---

## Quick Start

### Option 1: One-Line PowerShell Installer (SHA-256 Verified)
```powershell
irm https://raw.githubusercontent.com/KeremKalyoncu/ozevpn/main/scripts/install.ps1 | iex
```

### Option 2: Portable Single-EXE Download
1. Download `OzeVPN.exe` from [Releases](https://github.com/KeremKalyoncu/ozevpn/releases) and double-click.
2. Click **Oyun TUN** in the top bar for full-system IPv4+IPv6+UDP tunneling and Kill Switch.
3. Clean uninstall anytime: `.\OzeVPN.exe -uninstall`.

### Build from Source (Windows `amd64`, Go 1.22+ / 1.26+)
```powershell
go run ./cmd/ozevpn
gofmt -l . && go vet ./... && go test -race -cover ./...
.\scripts\build-single-exe.ps1
```

---

## License
Licensed under the **GNU General Public License v3.0 (GPL-3.0)** — see [LICENSE](LICENSE) and [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).
