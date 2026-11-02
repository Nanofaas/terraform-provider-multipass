# Linux SSH and Private Networks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the chosen upstream provider so OpenTofu on a workstation manages VMs and persistent private bridges on a remote Linux server, and publish the fork in Nanofaas.

**Architecture:** Preserve the existing resource lifecycle code and add SSH at the shared command execution boundary. The remote host runs Multipass; the workstation needs OpenTofu, the provider and OpenSSH. Add one NetworkManager-backed bridge resource; reuse existing instance network blocks and cloud-init for static guest addresses.

**Tech Stack:** Go, existing Terraform Plugin Framework dependencies, OpenTofu, OpenSSH, Multipass QEMU, Linux iproute2 and NetworkManager.

**Spec:** [Linux SSH and networks design](../specs/2026-11-02-linux-ssh-networks-design.md).

## Global Constraints

- Upstream baseline: `todoroff/terraform-provider-multipass` at `1dc6c0a7959c87faf6540e5c7c3a48738cd68088`; preserve history, MIT license and attribution.
- Public destination: `Nanofaas/terraform-provider-multipass`; keep upstream `master` branch naming to avoid unrelated changes.
- Initial network acceptance target: Ubuntu 24.04 with Multipass QEMU >= 1.15 and an existing working NetworkManager installation.
- NetworkManager is required only for managed bridge creation. Do not install it or change physical-interface management automatically.
- Preserve upstream local mode and existing direct-daemon support; new SSH execution supports Linux servers only.
- SSH uses strict host-key verification, noninteractive key/agent authentication, no agent forwarding, and no private-key contents in HCL/state/logs.
- Private bridges and static IPv4 only: no new DHCP, DNS, NAT, port publication, overlays, or complete isolation between VM groups.
- Keep Multipass's default interface; it means private bridges are not VM security boundaries.
- No infrastructure mutations during validation, configuration, refresh or planning. No new dependencies unless existing dependencies and native tools cannot do the job.
- Never adopt unrelated resources, purge unrelated VMs, overwrite local files without upstream ownership checks, or automatically install packages.
- A real Linux test server has not been supplied. Live acceptance and reboot persistence remain unverified until actually tested on an authorized disposable target.

## Review Focus

- SSH reports `command not found`, DNS failure or host-key failure: refresh must report an error and retain VM state (Tasks 1–2).
- Empty/binary files and paths with quotes/spaces: transfer bytes exactly and preserve local-path meanings (Tasks 1–3).
- Network exists, is partly created, or gains a foreign attached port: do not adopt/delete it blindly; retain recovery identity (Tasks 4–5).
- Parallel aliases or separate applies target the same host: do not share cached results or race bridge conflict checks/mutations (Tasks 2, 4).
- A stopped VM, a dropped SSH session after launch, or a missing bridge after restart: preserve upstream power-state behavior and distinguish unavailable from absent (Tasks 2, 5–6).

## File Structure and Delivery Order

- `internal/sshclient/client.go`, `client_test.go`: shared SSH command runner; no generic transport framework.
- Existing `internal/multipasscli/client.go`, `errors.go`, new `ssh_test.go`: remote command dispatch and error classification.
- Existing `internal/provider/config.go`, `provider.go`, `provider_test.go`: optional SSH configuration and shared client wiring.
- Existing instance/upload/download resource files and new `ssh_files_test.go`: local/remote path semantics and transfer guards.
- `internal/hostnetwork/client.go`, `client_test.go`, `scripts/*.sh`: NetworkManager bridge operations and host-side mutation serialization; scripts are embedded, not installed as a daemon.
- `internal/provider/network_resource.go`, `network_resource_test.go`: OpenTofu lifecycle for `multipass_network`.
- `internal/provider/ssh_network_acc_test.go`, existing acceptance helpers, `examples/ssh-private-network/main.tf`: remote end-to-end verification and runnable example.
- Existing docs, module imports, entry point, registry metadata and workflows: Nanofaas identity, installation and CI.

Tasks 1–3 deliver remote VM management before Tasks 4–5 add managed bridges. Task 6 verifies the combined result; Task 7 publishes it. This is one plan because bridges reuse the same SSH connection and instance lifecycle.

## Before Implementation

- [ ] Read the spec and repository AGENTS.md; select an execution branch/worktree using the execution skill without disturbing the existing checkout.
- [ ] Run `go test ./...` on the unchanged upstream code and record the baseline. Separate any pre-existing failures before writing regression tests. Module downloads are build dependencies, not system package installation; ask before installing any missing system tool.

### Task 1: Safe SSH Command Execution

**Files:** Create `internal/sshclient/client.go`, `internal/sshclient/client_test.go`.

**Interfaces:** Export `Config{Host, User, IdentityFile, KnownHostsFile string; Port int}`, `New(Config) (*Client, error)`, and `(*Client).Run(ctx context.Context, stdin []byte, argv ...string) (stdout, stderr []byte, err error)`. Construction validates configuration and finds OpenSSH; it does not connect. An absent port leaves SSH config/defaults in control. `Run` preserves `*exec.ExitError` and context error identity for callers.

- [ ] Add `TestSSHArgumentsAndStdin`: a fake SSH executable captures arguments; assert strict checking, batch mode, no forwarding and no TTY, literal remote arguments, exact stdin including zero-length and binary payloads. Exercise actual POSIX-shell decoding where available; strings include apostrophes, newlines, `$()` and backticks and must not execute their contents.
- [ ] Add `TestSSHValidationAndCancellation`: reject empty/option-like hosts, control characters, embedded `user@host`, invalid explicit ports and empty argv; permit IPv4, IPv6 and valid SSH host aliases. Assert cancellation ends the local SSH process and returns a context error; exit 255 stays identifiable as a transport failure.
- [ ] Run `go test ./internal/sshclient -v`; confirm failure from the missing implementation.
- [ ] Implement the interface using `os/exec`, a quoted remote `exec` command and the existing shell-quoting convention. Use separate pipes/buffers for stdout and stderr. Respect SSH config for omitted user, port and key settings. Never set insecure host-key options. Tests may inject the executable internally without adding a public setting.
- [ ] Run `go test -race ./internal/sshclient`; expect PASS, with platform-specific shell tests explicitly skipped only when no POSIX shell exists.
- [ ] Commit: `feat: add verified SSH command transport`.

### Task 2: Wire SSH into the Existing Provider and VM Lifecycle

**Files:** Modify `internal/provider/config.go`, `provider.go`, `provider_test.go`, `internal/multipasscli/client.go`, `errors.go`; create `internal/multipasscli/ssh_test.go`; update `docs/index.md` and `docs/guides/remote-hosts.md`.

**Interfaces:** Add `SSH *sshclient.Client` to `multipasscli.Config`; add SSH configuration attributes listed in the spec to `providerConfigModel`. Add `ssh *sshclient.Client` to `providerData` for the network client and SSH-mode path decisions; retain `hostOS` as the workstation OS. Keep all existing `multipasscli.Client` methods unchanged.

- [ ] Add `TestProviderSSHConfiguration`: `ssh_host` selects SSH, no local Multipass lookup occurs, explicit `server_address` conflicts, dependent SSH attributes without a host fail, and unknown connection values never fall back to the local daemon. Assert resources and data sources receive the same client and aliases have independent caches.
- [ ] Add `TestSSHRemoteDaemonAndErrors`: remote argv unsets `MULTIPASS_SERVER_ADDRESS` before running Multipass; no workstation environment value redirects it. Assert transport/remote-binary errors are not `ErrNotFound`, while a confirmed absent VM is. Verify timeout classification still reaches existing launch recovery and preserves partial creation identity.
- [ ] Run `go test ./internal/provider ./internal/multipasscli -run 'SSH' -v`; confirm the new tests fail.
- [ ] Update `NewClient` and `runWithStdin` to use Task 1 when configured, retaining the local code path. Apply the existing timeout before calling SSH. Treat exit 255 and remote command-not-found errors before parsing Multipass diagnostics; narrow missing-resource detection rather than interpreting every `not found` as a missing VM. Check the target is Linux read-only; never mutate it during Configure.
- [ ] Run `go test ./internal/provider ./internal/multipasscli`; expect PASS including upstream resize, deletion, server-address and alias tests. Add a stopped-VM resize case through the SSH fake if existing tests do not exercise this path.
- [ ] Document prerequisites: remote Multipass authorization, SSH known-host enrollment, key/agent use, and no workstation Multipass requirement. Commit: `feat: manage Linux Multipass hosts over SSH`.

### Task 3: Preserve File, Cloud-init and Mount Semantics

**Files:** Modify `internal/multipasscli/client.go`, `internal/provider/file_upload_resource.go`, `file_download_resource.go`, and instance configuration only where necessary; create `internal/provider/ssh_files_test.go`; update `docs/resources/instance.md`, `file_upload.md`, `file_download.md`.

**Interfaces:** Reuse `LaunchOptions`, `TransferOptions`, `TransferCapture`, `downloadWithTar`, `fetchFileBytes`, `writeFileBytes` and existing ownership checks. Resource Configure records `data.ssh != nil`; do not replace workstation `hostOS` with Linux.

- [ ] Add `TestSSHCloudInitFile`: read a local file with spaces/quotes, assert launch receives `--cloud-init -` and its bytes; missing/unreadable files cause no remote mutation.
- [ ] Add `TestSSHFileRoundTrip`: single-file uploads use local bytes via stdin; downloads reuse the capture/write path, including empty and binary files. Assert replacement failures preserve the prior local destination, and ownership checks on destroy still hold.
- [ ] Add `TestSSHRecursiveTransferRejectedBeforeMutation`: v1 rejects recursive SSH upload/download during resource validation/planning and again before apply-side mutations, including old imported state. Local-mode recursive transfers remain supported. This is the explicitly selected minimal option permitted by the spec.
- [ ] Add `TestSSHMountUsesHostPath`: mount source is passed literally to the remote Multipass CLI without workstation existence checks; aliases and primary settings execute remotely. Verify shell metacharacters never become commands.
- [ ] Run `go test ./internal/provider ./internal/multipasscli -run 'SSH.*(CloudInit|File|Recursive|Mount)' -v`; confirm failure.
- [ ] Implement local file reads plus stdin streaming and route SSH downloads through existing byte-based download code. Guard recursive operations before staging directories, deleting paths or creating files. Preserve all upstream local-mode behavior.
- [ ] Run `go test ./internal/provider ./internal/multipasscli`; expect PASS. Document SSH recursion limitation, host-relative mounts and cloud-init contents potentially appearing in state. Commit: `fix: preserve local file semantics with remote Multipass`.

### Task 4: Persistent Bridge Lifecycle on the Linux Host

**Files:** Create `internal/hostnetwork/client.go`, `client_test.go` and minimal embedded `internal/hostnetwork/scripts/create.sh`, `delete.sh` as needed for locked compound operations.

**Interfaces:** `Bridge{ID, Name, Address string; Active bool}`, `New(*sshclient.Client) *Client`, `Create(ctx context.Context, name, address string) (*Bridge, error)`, `Read(ctx context.Context, id string) (*Bridge, error)`, `Delete(ctx context.Context, id string) error`, and `ErrNotFound`. Create returns a non-nil bridge with UUID on post-creation activation/readback failures. Ownership marker: NetworkManager `user.data` key `org.nanofaas.multipass.owner` with value `terraform-provider-multipass`.

- [ ] Add `TestBridgeValidationAndConflicts`: reject names beyond 15 bytes or outside `[a-zA-Z][a-zA-Z0-9_-]*`, non-private IPv4, network/broadcast host addresses, prefixes outside /8–/30, existing interface/profile names, and overlap with host interface subnets or non-default routes. Accept `nf-test`, `10.60.0.1/24`. Use `net/netip`; do not treat a default route as overlap with every subnet.
- [ ] Add `TestBridgeOwnershipAndFailureRecovery`: unrelated profiles/interfaces survive; attached ports prevent deletion; absent owned profiles allow idempotent cleanup only when the remaining interface is also provably owned. Unknown SSH/permission errors are not absence. Activation failure returns the created UUID. A missing NetworkManager/sudo permission produces a prerequisite diagnostic before mutations.
- [ ] Add `TestBridgeConcurrentCreation`: two independent callers on one host cannot both pass the same name or overlapping-subnet check and create conflicting bridges. Use a host-side `flock` around the conflict check plus mutation, not merely a Go mutex; validate and test the embedded script's actual control flow with fake host commands.
- [ ] Run `go test ./internal/hostnetwork -v`; confirm failure.
- [ ] Implement read-only queries with locale-stable `nmcli --terse` and `ip -j`; create a persistent bridge with manual IPv4, no gateway/default route, IPv6 disabled, no physical ports and the ownership marker. Privileged mutations use `sudo -n`; use a fixed root-owned lock under `/run/lock`. Handle command quoting and exit status using Task 1. Keep any scripts static and pass validated values as arguments, not interpolated source.
- [ ] Implement deletion under the same lock, rechecking ownership and attached ports immediately before deleting only the selected profile/interface. Recover partial mutations conservatively; never use global network reset commands.
- [ ] Run `go test -race ./internal/hostnetwork`; expect PASS. Commit: `feat: manage persistent private bridges on Linux hosts`.

### Task 5: Expose `multipass_network` to OpenTofu

**Files:** Create `internal/provider/network_resource.go`, `network_resource_test.go`, `docs/resources/network.md`; modify `internal/provider/provider.go`, `config.go` if needed and `resource_test_helpers_test.go` only as required.

**Interfaces:** `NewNetworkResource() resource.Resource`; attributes `name` and `address` required, `id` computed UUID, `active` computed bool. Implement Resource, ResourceWithConfigure, ResourceWithImportState and ResourceWithModifyPlan. Use Task 4 client and existing schema/state test helpers.

- [ ] Add `TestNetworkRequiresSSHAndValidation`: no SSH configuration fails before mutation; invalid/unknown input behavior is deterministic. Resource registration and protocol schema include `multipass_network`.
- [ ] Add `TestNetworkLifecycle`: create records canonical actual state, refresh reads configuration, a missing owned profile removes state only after safe orphan assessment, connection failure preserves state, and partial create saves UUID despite diagnostics. Assert no mutations from Read/ModifyPlan.
- [ ] Add `TestNetworkReplacementAndImport`: name/address changes require replacement; a confirmed missing/inactive live bridge with its owned profile retained plans replacement to restore it. Import refreshes only a provider-owned UUID and rejects unrelated connections. Destroy refuses an attached port and succeeds after dependent VMs are gone.
- [ ] Run `go test ./internal/provider -run 'TestNetwork' -v`; confirm failure.
- [ ] Implement using Task 4 with each operation bounded by the existing provider command timeout. Preserve actual observed drift in state and request replacement where needed; do not silently claim desired values or repair during Read. Register the resource. Dependents use `multipass_network.lab.name` in existing instance network blocks; no new attachment abstraction.
- [ ] Run `go test ./internal/provider ./internal/hostnetwork`; expect PASS. Document import, replacement, privileges, persistence and lack of isolation. Commit: `feat: expose managed bridge resources to OpenTofu`.

### Task 6: Integrated Example and Verification

**Files:** Create `examples/ssh-private-network/main.tf`, `README.md`, `internal/provider/ssh_network_acc_test.go`; modify `internal/provider/acc_test.go` only to share safe remote configuration helpers; update `examples/README.md`, `docs/README.md`.

**Interfaces:** Gate live tests on `TF_ACC=1` plus `MULTIPASS_ACC_SSH_HOST`; optional user/port/key/known-host variables map to provider SSH settings. New SSH acceptance prechecks and destroy checks must use the remote client, never the local defaults in existing helpers.

- [ ] Add `TestAccSSHPrivateNetwork`: create one unique bridge and two `24.04` VMs with explicit distinct MACs and cloud-init static IPs from `cidrhost`; wait for cloud-init, ping the private IP from the other VM, assert a no-change plan, test network import and confirm scoped cleanup. Reject an overlapping test subnet before creating resources. Preserve/report state on failures so cleanup remains possible.
- [ ] Add `TestAccSSHNetworkDrift`: on a disposable test bridge without attached VMs, disable/remove only its owned runtime device, refresh, assert a repair plan, apply and verify recovery. No host reboot in automated default tests.
- [ ] Write the example with `ssh_host`, a shared subnet variable, `multipass_network`, two instances using its name, `mode = "manual"`, explicit MACs, Netplan via cloud-init and `wait_for_cloud_init = true`. Explain how to reach guests and which addresses are stable.
- [ ] Run `gofmt` on changed Go files, `go test ./...`, `go test -race ./internal/...`, `go vet ./...`, and `go build ./cmd/terraform-provider-multipass`; expect success. Run `tofu fmt -check -recursive examples`, correcting any existing formatting necessary for that check.
- [ ] Build to a temporary local provider mirror, use a temporary OpenTofu CLI config and isolated data directory, then run `tofu init -backend=false`, `tofu validate` and `tofu providers schema -json` for the example. Assert the network and SSH schema fields exist. Do not change global CLI config or a real infrastructure state.
- [ ] If an authorized Linux target is supplied, run only the new SSH acceptance suite with `TF_ACC_TERRAFORM_PATH` pointing at OpenTofu. Verify bridge reboot persistence separately only on an explicitly disposable/reboot-authorized server. Otherwise record these tests as NOT RUN, not passing.
- [ ] Commit: `test: verify SSH VM and private network workflows`.

### Task 7: Nanofaas Identity, CI and Public Fork

**Files:** Modify `go.mod`, repository-local Go imports, `cmd/terraform-provider-multipass/main.go`, `.github/workflows/ci.yml`, `docs.yml`, release configuration only as necessary, README, AGENTS.md, llms.txt, docs and examples. Preserve `LICENSE`.

**Interfaces:** Go module `github.com/Nanofaas/terraform-provider-multipass`; target provider address `registry.opentofu.org/nanofaas/multipass`. Local mirrors/dev instructions work before registry registration. Keep deliberate upstream addresses in migration tests that actually test an upstream release; do not blindly replace them.

- [ ] Update module/imports, provider source, acceptance test source, examples and docs consistently. Preserve attribution and explain how the fork extends upstream. Search `rg -n 'todoroff|registry\.terraform\.io|registry\.opentofu\.org'` and classify each remaining old reference rather than replacing historical links.
- [ ] Configure CI to run Go checks and OpenTofu schema/example validation using an available pinned OpenTofu release. Verify the current setup action interface from official docs before changing it. Keep baseline local-mode platform coverage. Do not make live SSH tests use unavailable CI secrets or public hosts.
- [ ] Inspect the inherited signing workflow: document required signing secrets, do not copy upstream credentials, trigger release tags or claim registry publication. Registry enrollment is separate from this GitHub delivery.
- [ ] Re-run `go test ./...`, `go vet ./...`, build, schema/example checks after namespace changes. Run the existing documentation validator with OpenTofu if PowerShell is available; otherwise use CI for that check and report its actual result. Review the diff for accidental state, credentials, host details and unsupported claims.
- [ ] Commit: `chore: prepare Nanofaas provider distribution`.
- [ ] Create the public GitHub fork with `gh repo fork todoroff/terraform-provider-multipass --org Nanofaas --clone=false` if no destination exists; rename the local original remote to `upstream`, set Nanofaas as `origin`, and push only the reviewed branch/commits. If fork creation is unavailable, use a public repository retaining upstream history and attribution.
- [ ] Verify GitHub visibility, upstream relationship where applicable, remote commit SHA and CI results. Report the repo link, supported scope, executed tests and remaining live/registry checks. If publication fails, retain local work and report the exact blocker; do not call the project published.

## Self-Review and Handoff

Spec coverage: transport and host selection (1–2), file semantics (3), bridges and ownership (4–5), VM dependencies/static addresses (6), Linux prerequisites and operational limits (all docs tasks), attribution/publication (7). Review-focus cases have named tests above. Interface names and provider attribute names are consistent across tasks.

Recommended execution: native in this session, one task at a time, then independent whole-branch review. Transport, file handling and bridge lifecycle share state/error semantics, so sequential implementation reduces coordination overhead. Subagent-driven execution with a fresh implementer/reviewer per task is the alternative if the user prefers it.

Pending before live acceptance: server address, SSH identity/user and authorization for disposable test VMs/networks; confirmation that NetworkManager is already usable for private bridges. No secrets should be pasted into chat. This does not prevent implementing and unit-testing the provider.
