# Code Signing Policy

This project follows strict supply-chain security practices for all published Windows binaries.

## Signing Provider

OzeVPN signs Android releases with a dedicated 4096-bit RSA release keystore (SHA-256 fingerprint `05:F7:B3:D9:28:4F:B0:2B:7D:FD:E8:4A:34:5B:78:46:67:5E:93:81:6E:DD:0C:3B:17:98:44:B1:90:D7:4C:45`) and signs Windows CI releases with keyless Sigstore Cosign + SLSA Build Provenance. For future Windows Authenticode signing, OzeVPN intends to reapply to the free open-source [SignPath Foundation](https://signpath.org/) program via [SignPath.io](https://signpath.io/) once community visibility requirements are met.

In addition to Authenticode signing, all GitHub releases provide:
- SHA-256 checksums (`OzeVPN.exe.sha256`) verified by [`scripts/install.ps1`](scripts/install.ps1)
- CycloneDX Software Bill of Materials (`sbom.cdx.json`)
- GitHub-hosted [Sigstore SLSA Build Provenance](https://github.com/KeremKalyoncu/ozevpn/actions/workflows/release.yml) attestations

## Project Team & Roles

All privileged roles enforce Multi-Factor Authentication (MFA) on both GitHub and signing services.

| Role | Members | Responsibilities |
| :--- | :--- | :--- |
| **Committers / Authors** | `@KeremKalyoncu` (Kerem Kalyoncu) | Author, review, and merge code changes on protected `main` branch. |
| **Reviewers** | `@KeremKalyoncu` | Review pull requests, verify CI/CodeQL/staticcheck/REUSE gates before release. |
| **Approvers** | `@KeremKalyoncu` | Explicitly review and approve each release signing request before publication. |

## Build & Release Origin

- Releases are built from tagged commits on the public [`KeremKalyoncu/ozevpn`](https://github.com/KeremKalyoncu/ozevpn) repository.
- Automated verification runs on official GitHub-hosted runners (`windows-latest` / `ubuntu-latest`) via [`.github/workflows/ci.yml`](.github/workflows/ci.yml), [`.github/workflows/codeql.yml`](.github/workflows/codeql.yml), and [`.github/workflows/release.yml`](.github/workflows/release.yml).
- Self-hosted runners are not used for signing workflows.
- Signing applies strictly to binaries built from this repository's open-source GPL-3.0-or-later codebase.

## Privacy Policy

OzeVPN collects **zero** user telemetry, analytics, or personal data. During CI signing, only public build metadata (commit SHA, artifact hash, build timestamp) is transmitted to the signing service. See [PRIVACY.md](PRIVACY.md) for the complete application privacy policy.

## Vulnerability Reporting

Security issues or suspected signing/supply-chain anomalies must be reported privately via [SECURITY.md](SECURITY.md) and [GitHub Private Vulnerability Reporting](https://github.com/KeremKalyoncu/ozevpn/security/advisories/new).
