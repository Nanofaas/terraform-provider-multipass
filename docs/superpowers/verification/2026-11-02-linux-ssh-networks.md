# Linux SSH and private network verification

Upstream history and MIT attribution are preserved. The public fork is
`Nanofaas/terraform-provider-multipass`, with provider address
`registry.opentofu.org/nanofaas/multipass`. Registry enrollment and signed releases
are separate from this GitHub delivery; local mirror installation is documented.

## Automated verification

- `go test ./...`, `go vet ./...` and race checks passed.
- All nine OpenTofu examples passed formatting, initialization and validation
  against the locally built provider. SSH/network protocol schema checks passed.
- GitHub CI passed on Linux, macOS and Windows; documentation validation passed
  on Linux and Windows. Pulumi schema, SDK generation and previews passed on
  Linux and Windows. NetworkManager lifecycle corrections were subsequently
  verified by fresh Go CI on all three platforms.
- One independent whole-branch review identified three correctness issues:
  uncertain SSH launches losing VM names, bridge readback failures losing UUIDs,
  and local upload paths containing colons being rejected. Regression tests
  reproduced each failure before the fixes. Pulumi examples now use a local
  binary instead of an unpublished registry version.

## Live verification

A user-authorized Linux QEMU host running Multipass 1.17.0 and NetworkManager
1.46.0 was used over SSH. The account's Multipass certificate was authorized
using [Canonical's documented recovery procedure](https://canonical.com/multipass/docs/latest/how-to-guides/customise-multipass/authenticate-users-with-the-multipass-service/), with explicit approval and a
backup of the prior trust bundle. No private keys or passphrases were exported.

The managed-network acceptance test passed creation, deliberate deactivation,
refresh/replacement, recovery and destruction. No server reboot was performed.
The two-VM test created both Ubuntu 24.04 guests and completed cloud-init, but
private-address ping was blocked by the host's bridged IPv4 forwarding policy.
A scoped firewall exception requires separate approval; the provider deliberately
does not manage host firewall policy. Test VMs and bridge interfaces were removed.

## Implementation decisions

- The host helper uses embedded Python 3 standard-library code for JSON, address
  validation and process-wide file locking. No helper service is installed.
- NetworkManager 1.46 rejected `user.data` for bridge profiles during live tests.
  Ownership instead uses `connection.stable-id=nanofaas-multipass-<UUID>`.
- Bridge activation waits at most 30 seconds for its address, because `nmcli`
  can return before assignment. Deletion tolerates NetworkManager concurrently
  removing the owned interface. Both cases have regression coverage.
- Shared CLI validation rejects option-like VM names before execution, including
  imported `--all` and snapshot-qualified names, to prevent unrelated deletion.
- Existing upstream opt-in recovery during refresh is preserved for compatibility;
  the new network refresh performs no repairs.
- In uncertain SSH launch outcomes, the requested name stays in state after an
  absence precheck. Operators must not launch the same name concurrently from
  separate configurations.
- Private bridges do not isolate VM groups: Multipass retains its default NIC.
  Recursive SSH file transfers, DHCP/DNS/NAT and reboot verification are outside
  this delivery. There are no deferred minor review findings.

CI evidence: [Go and OpenTofu](https://github.com/Nanofaas/terraform-provider-multipass/actions/runs/41434236114),
[documentation](https://github.com/Nanofaas/terraform-provider-multipass/actions/runs/41434240284),
[Pulumi compatibility](https://github.com/Nanofaas/terraform-provider-multipass/actions/runs/41431901560).
