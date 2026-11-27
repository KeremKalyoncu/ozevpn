# Code Signing Policy

This project follows strict supply-chain security practices for all published Windows binaries.

## Signing Provider

OzeVPN is applying for / intends to use free open-source Windows Authenticode code signing provided by [SignPath.io](https://signpath.io/), with the signing certificate issued by the [SignPath Foundation](https://signpath.org/).

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
