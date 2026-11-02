# Host Hardening and Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver reduced host privileges, read-only preflight diagnostics and an installable first release; complete authorized live checks.
**Architecture:** Reuse the Python bridge helper and shared Go SSH runner; install a fixed helper instead of passing privileged code. Extend existing release automation.
**Tech Stack:** Go, Python standard library, OpenSSH, sudoers, NetworkManager, GoReleaser/GPG.
**Spec:** ../specs/2026-11-02-host-hardening-design.md

## Global Constraints

- No arbitrary Python sudo fallback, configurable privileged executable or new service.
- Fixed helper path `/usr/local/libexec/nanofaas-multipass-network`; protocol version 1.
- Exact create/delete/check sudo commands; no network mutations in preflight.
- Preserve existing bridge identities and VM-only workflows.
- No private signing material in Git/logs. Release v0.1.0, Linux/macOS/Windows amd64/arm64.
- No reboot before explicit approval of the concrete host test.

## Review Focus

- User-controlled PATH/PYTHONPATH/cwd must not execute privileged code (task 1 tests).
- Unexpected actions, malformed/oversized JSON and invalid UUIDs fail before commands (task 1 tests).
- Missing or incompatible helper gives useful failure while VM-only access still works (tasks 1–2 tests).
- Failed bootstrap or sudoers validation must not revoke working access (task 1 installer and live validation).
- Release artifacts must contain runnable provider/doctor, host tools, matching checksums/signature (task 3 artifact checks).

### Task 1: Restrict bridge privileges

Files: internal/hostnetwork/client.go, scripts/bridge.py, their tests; scripts/install-host-helper.sh; network documentation.
Interfaces: fixed helper path; `check` returns protocol_version and checks; every response includes protocol_version=1. Read/Create/Delete Go API unchanged.
- [x] Add failing tests for fixed remote commands, protocol mismatch and input/environment boundaries.
- [x] Implement isolated helper dispatcher, sanitized command environment and client invocation.
- [x] Add root-owned atomic installation and exact sudoers rules, validate before replacement.
- [x] Run hostnetwork tests and full Go suite; document installation/migration.

### Task 2: Read-only doctor

Files: cmd/multipass-doctor/main.go, internal/preflight/check.go and tests, hostnetwork check tests, README.
Interfaces: `preflight.Run(ctx, ssh, binary, networks) []Check`; Check{Name, Status, Detail}; doctor exit 1 on FAIL, 0 otherwise.
- [x] Add failing fake-SSH tests for missing auth/helper, non-Linux host, protocol mismatch, firewall warning and VM-only success.
- [x] Implement standalone read-only command and helper health report with no mutations.
- [x] Run tests/vet/build and verify CLI usage, errors and successful local fixtures.

### Task 3: Signed distribution

Files: .goreleaser.yml, .github/workflows/release.yml, scripts/package-host-tools.py, packaging/public-key.asc, docs/guides/installation.md.
- [x] Add snapshot dispatch and package provider, doctor and host tools; preserve the registry manifest format.
- [x] Verify snapshot artifacts in CI; create isolated dedicated signing key and repository secrets.
- [x] Review whole branch before merge/release; fix important findings with regressions.
- [x] Publish v0.1.0; download artifacts, verify signature/checksums, run released doctor/provider with a local mirror.
- [ ] Submit registry forms if possible; otherwise prepare exact public metadata and report the external step.

### Task 4: Host migration and completion

- [x] Inventory active users/VMs/containers; prepare concrete reboot test and request approval only for the restart.
- [x] Install reviewed helper; run preflight and remote network/VM acceptance with previously authorized temporary scoped firewall rule.
- [x] If approved, reboot with disposable resources, verify persistent network/static IPs and refresh/plan, then destroy all test resources and rule.
- [x] Backup/remove only named broad Python sudo rule after narrow helper verification; prove Python sudo denied and owned bridge create/read/delete succeeds.
- [x] Update verification record, publish documentation and verify clean remote/local state.

Registry forms are the only pending external step; browser session was stopped. Release, host migration and checks are complete, with host reboot limitations documented in the verification record.
