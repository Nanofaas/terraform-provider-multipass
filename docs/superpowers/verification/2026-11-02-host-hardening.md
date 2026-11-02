# Host hardening and distribution verification — 2026-11-02

## Implementation and review

The installed helper replaces privileged inline Python. Its isolated interpreter,
fixed subprocess environment, canonical UUID validation, bounded input and existing
ownership checks constrain network operations. The doctor checks installation
ownership/modes, unprivileged execution, exact NOPASSWD create/delete authorization,
protocol compatibility, Multipass access, NetworkManager and firewall prerequisites.

An independent whole-branch review found no critical issues. All three important
findings were corrected: restricted-user acceptance no longer invokes privileged
Python; doctor verifies installation and lifecycle permissions; releases remain
drafts until artifact and signature verification succeeds.

Local `go test ./...`, `go vet ./...`, race tests, Python packaging tests and nine
OpenTofu example/schema checks passed. The post-review CI passed on Linux, macOS
and Windows; Linux also ran race, packaging and OpenTofu checks:
https://github.com/Nanofaas/terraform-provider-multipass/actions/runs/41443216181

The unsigned snapshot built and validated all six provider/doctor platform ZIPs,
checked host-tools contents and checksums, and validated the registry manifest:
https://github.com/Nanofaas/terraform-provider-multipass/actions/runs/41443203503

## Host installation and failure handling

Tested against the authorized Linux ARM64 host with Multipass 1.17.0,
NetworkManager 1.46.0, Python 3.12.3 and iproute2 6.1.0.
The first installation stopped before changing either target because `visudo -c`
rejected two existing root-owned sudoers files with mode 0640. Only their modes
were corrected to 0440; their grants were unchanged. Installation then succeeded.
Tests with an invalid user and invalid staged Python helper confirmed that the
existing helper and sudoers checksums were unchanged after failure.

Doctor passed all prerequisites and correctly warned about bridged IPv4 filtering
and FORWARD DROP. No permanent firewall change was made.

## VM/network acceptance and concurrency limit

The revised `TestAccSSHNetworkDrift` passed in 13.68 seconds using only owned-helper
delete/create operations. Two ordinary parallel VM acceptance attempts failed
while resizing an image (`qemu-img: Permission denied`). Logs showed overlapping
loads of the shared `multipass.qemu-img` AppArmor profile for different images.
Both failed runs cleaned up their resources and scoped firewall exceptions.

The same `TestAccSSHPrivateNetwork` passed in 93.67 seconds with actual CLI
parallelism 1: two VMs, cloud-init, static private addresses, guest-to-guest ping,
unchanged plan, bridge import and destruction. The temporary test wrapper replaced
the explicit `-parallelism=10` emitted by terraform-exec; merely setting
`TF_CLI_ARGS_apply` did not override that argument. No daemon policy was weakened.
The provider documents `tofu apply/destroy -parallelism=1` for affected hosts.

## Authorized host reboot

Before reboot both VMs had their expected private addresses, guest-to-guest ping
passed and `tofu plan -detailed-exitcode -parallelism=1` returned 0. The host
returned with a different boot ID. NetworkManager restored the same owned bridge
UUID and address without provider mutation.

Automatic guest resume did **not** pass: QEMU 10.0.3 failed loading the suspend
state with `Failed to put registers after init: Invalid argument` and exited.
Multipass still reported both guests Running, with stale default-network IPs.
The two disposable test guests were recovered through explicit
`multipass stop --force NAME` followed by serial `multipass start NAME`.
After that cold start, both configured private addresses remained correct, ping
passed and the OpenTofu plan again returned 0 with no changes. No disk files,
AppArmor profiles, physical interfaces or host boot services were edited.

This establishes bridge/configuration persistence, **not reliable automatic VM
resume** on this ARM64 host. Release documentation records that distinction.

## Post-reboot deletion regression

The first destroy removed both VMs and the NetworkManager profile, but reported
`remaining interface ownership cannot be proven; retained` while NM was removing
the bridge asynchronously. The volatile kernel alias had been lost at reboot.
The bridge subsequently disappeared; the remaining local state was reconciled.
A regression test reproduced the failure. The helper now restores the kernel
marker **after** verifying existing NM ownership and absence of attached ports,
**before** deleting the persistent ownership proof. This keeps orphan cleanup
safe without adopting foreign bridges. All tests passed after the change.
A live test cleared only the marker on a newly owned disposable bridge, then
verified read/delete/absence with the updated installed helper; it passed.

## Final host privilege migration

The old unrestricted `/usr/bin/python3` rule was backed up under a root-only
host directory and removed. `sudo -n /usr/bin/python3 -I -c 'print(0)'` now fails
with a password requirement. The installed helper doctor still passes, including
its exact NOPASSWD create/delete authorization checks.
The restricted-user `TestAccSSHNetworkDrift` was then repeated successfully
(14.88 seconds), covering create/read/delete/recreate and final cleanup after
the broad grant was gone. Independent inventory confirmed no test VMs or profiles.
Other pre-existing administrator sudo grants are outside this provider's scope;
this migration does not certify the whole account as unprivileged.

No test firewall exceptions remain. Host reboot changed the default iptables
FORWARD policy from DROP to ACCEPT; the original DROP policy was restored at
cleanup. Firewall persistence across future host boots needs administrator review;
the provider deliberately does not manage it. Both nft and legacy tables were
present after reboot. The doctor reports the selected iptables backend only.

## Signed release

Release [v0.1.0](https://github.com/Nanofaas/terraform-provider-multipass/releases/tag/v0.1.0)
was published only after artifact and signature validation:
https://github.com/Nanofaas/terraform-provider-multipass/actions/runs/41446446434
Final Linux/macOS/Windows CI also passed:
https://github.com/Nanofaas/terraform-provider-multipass/actions/runs/41446455144

All published assets were downloaded independently. Their checksums, six platform
archive layouts, host tools and registry manifest passed verification. GPG checked
the detached signature in a fresh public-key-only keyring against fingerprint
`BC1D2E34BCE6C30673A5CAA4FE32DA979EF37C37`. The released host helper matches the
installed and tested helper SHA256
`8a77d33bc0aa69569ce128c460c5e88b89dfec93079b0724c9c8815677ef44a1`.

The published darwin/arm64 ZIP was installed through a packed filesystem mirror;
OpenTofu init, validate and provider schema succeeded. The released doctor passed
against the host, retaining the expected FORWARD DROP warning. Mirror installation
is reported by OpenTofu as unauthenticated; GPG verification was performed
separately before executing the artifacts.

The dedicated private signing key and passphrase were uploaded to repository
Actions secrets only after explicit authorization. No private material is tracked.
Both required registry submissions were made through GitHub's official browser
issue forms on 2026-11-02. Public organization membership was verified.
The [key submission #6240](https://github.com/opentofu/registry/issues/6240)
passed validation and its [PR #6241](https://github.com/opentofu/registry/pull/6241)
was merged. The [provider submission #6242](https://github.com/opentofu/registry/issues/6242)
passed validation and generated [PR #6243](https://github.com/opentofu/registry/pull/6243),
which is awaiting merge. The registry versions endpoint still returned HTTP 404
at this check. Direct registry installation must be verified after approval and
publication; the signed release/local mirror remains usable meanwhile.
