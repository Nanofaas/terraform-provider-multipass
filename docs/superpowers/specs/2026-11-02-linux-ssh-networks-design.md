# Nanofaas Multipass provider: Linux hosts and private bridges

Status: proposed for approval; implementation has not started.

## Intent and agreed scope

Extend `todoroff/terraform-provider-multipass`, retaining its history, MIT
license, attribution, existing resource implementation, and tests. Publish the
result as the public GitHub repository `Nanofaas/terraform-provider-multipass`.
The user explicitly selected this upstream as the base.

Manage Multipass VMs on Linux servers reachable by IP. SSH is acceptable where
needed. Create private bridges on one host and connect VMs with static IPv4
addresses. The user explicitly deferred complete isolation between VM groups.
Keep the upstream local execution mode; new remote-host support targets Linux.

## Approaches and decisions

1. Reuse the upstream provider rather than write a new provider. Selected by
   the user; this preserves VM lifecycle, resizing, import and cloud-init work.
2. For remote execution, run the host's Multipass CLI over SSH. Recommended
   implementation choice: it reaches both the daemon and guest networks locally
   on that host and also permits bridge administration. Reusing the upstream
   direct daemon connection would still require TLS setup and guest routing,
   and cannot create host bridges. No new direct-daemon feature is planned.
3. Use persistent Linux bridges with static guest addresses. Selected scope;
   full Docker-style IPAM, DNS, NAT, port publication and isolation are deferred.

Implementation proposal: use existing OpenSSH on the workstation and
NetworkManager (`nmcli`) on hosts where managed bridge resources are requested.
NetworkManager supplies persistent bridge lifecycle and avoids custom boot
scripts. VM-only hosts do not need NetworkManager. Do not install it, replace a
host's current network manager, or change physical-interface management
automatically. Hosts without a suitable existing NetworkManager setup can use
externally prepared bridges; another persistence backend is outside v1.
This prerequisite is a builder proposal, not a user requirement already agreed.

## Remote transport

Add optional `ssh_host`, `ssh_user`, `ssh_port`, `ssh_identity_file`, and
`ssh_known_hosts_file` provider settings. `ssh_host` enables remote execution;
otherwise retain upstream behavior. Allow SSH configuration and agent keys;
never store private-key contents or passwords in provider configuration.
Reject contradictory `ssh_host` and explicit `server_address` configuration.
In SSH mode, select the host's local Multipass socket, ignoring inherited
`MULTIPASS_SERVER_ADDRESS` values on either side.

Use OpenSSH in noninteractive mode with strict host-key verification and no
agent forwarding. Execute properly quoted argument lists through the remote
POSIX shell, preserve stdin and separate stdout/stderr, propagate deadlines and
cancellation, and validate host/user/port/path inputs. An SSH connection failure
must never be classified as a missing VM. No commands that modify infrastructure
run during provider configuration, refresh, validation, or planning.

The remote user must already be authorized to run Multipass. Bridge operations
may use noninteractive `sudo -n`; failures explain the required privileges.
Multipass commands themselves run as the authenticated SSH user.

## Preserve path and resource semantics

All resources and data sources use the same configured client. Provider aliases
remain independent. Local cloud-init files are read locally and sent via stdin
in SSH mode. Inline cloud-init already follows that pattern upstream.

Local upload sources and download destinations retain their documented meaning.
Audit every transfer path before enabling SSH: stream single-file transfers;
either safely support directory streaming or reject unsupported recursive SSH
transfers before any mutation with a clear diagnostic. Do not reinterpret local
paths as server paths. Existing local-mode transfers remain supported.
Mount sources refer to the Multipass host filesystem in SSH mode and must be
documented and validated accordingly. CLI aliases and primary-instance settings
belong to the remote CLI user. Workstation OS-dependent file handling must remain
distinct from the remote host OS.

## Managed bridge resource

Add `multipass_network` with required `name` (Linux bridge interface name) and
`address` (host IPv4 address/prefix). The resource exposes its bridge name and a
stable NetworkManager connection UUID as its ID. Require SSH mode for this new
resource so local workstation networking cannot be changed accidentally.

Create one persistent, active NetworkManager bridge profile without physical
ports, DHCP, DNS, NAT or default routes. Disable automatic IPv6 on this bridge.
Validate names, usable private IPv4 addresses/prefixes, and conflicts with
existing host networks before mutation. Mark the profile as provider-owned;
refuse to overwrite a pre-existing interface or unrelated connection.

Read the actual connection and live interface configuration. Detect missing
profiles and drift; unavailable hosts produce errors, never silent state removal.
Name or address changes require replacement. Refuse deletion while the bridge
has attached ports, remove only its owned profile/interface, and make deletion
of an already-absent owned resource idempotent. Preserve a created UUID in state
on partial failures so retry/destroy can recover. Serialize conflicting host
mutations or use atomic conflict detection to handle parallel applies safely.
Import accepts a provider-owned UUID; v1 does not take ownership of arbitrary
administrator-managed bridges.

Existing `multipass_instance.networks` references the bridge name, providing
OpenTofu dependency ordering. Static guest addresses use the existing
`cloud_init` plus Netplan pattern and explicit MAC addresses; include a complete
two-VM example using `cidrhost` and a shared subnet variable. The provider does
not allocate addresses dynamically or promise global duplicate-IP detection.
Changing instance network configuration keeps upstream replacement semantics.

## Limits and safety properties

The extra bridge is a separate L2 segment, not a security boundary for the VM.
Multipass retains its default management/NAT interface, so guests may communicate
through that shared network. No cross-host overlay or firewall isolation is
promised. Connectivity from the workstation to guest IPs still needs routing or
an SSH tunnel; provider operations themselves only need SSH to the host.

Do not reconfigure physical interfaces, flush firewall rules, purge unrelated
VMs, adopt unowned networks, or install packages automatically. Initial network
acceptance target: Ubuntu 24.04 with Multipass QEMU >= 1.15 and an existing
working NetworkManager installation. Other compatible Linux hosts may work but
are not claimed tested without evidence.

## Code, documentation and publication scope

Touch the shared CLI execution/transfer code, provider configuration and data,
new network resource/client helpers, registration, tests/fakes, examples,
reference documentation, README and AGENTS/llms guidance. Update the Go module,
provider address, release metadata and CI for Nanofaas while preserving upstream
attribution and any deliberate references to upstream migration-test versions.

Create a public GitHub fork where supported, keep an `upstream` remote, and push
reviewed commits to Nanofaas. GitHub publication is already authorized. Registry
listing and signing credentials are separate from creating the GitHub repository;
document local installation and do not claim registry availability prematurely.

## Verification

Run upstream unit tests first, then add focused tests for SSH argument quoting,
stdin/binary transfers, failures/timeouts, configuration validation, alias
independence, and bridge lifecycle/ownership/drift/partial failures. Run Go tests,
race checks, vet, build, HCL formatting/validation and schema/document checks
appropriate to the changes. Use OpenTofu for acceptance tests.

A disposable Linux server is needed for real SSH/Multipass/network acceptance:
create a bridge and two VMs, verify static-IP communication, check a no-change
plan and refresh/import, and destroy only test resources. Verify bridge
persistence after a reboot only on a disposable target with authorization.
No target has been supplied yet: report live acceptance as unverified until it
is actually performed. Do not create VMs on an unrelated host as a substitute.

## Builder discretion

Exact helper names, test fixtures, diagnostic wording and connection ownership
metadata may follow upstream conventions. Prefer the standard library and
installed OpenSSH over new dependencies. Do not add an agent/service of our own
or a generic plugin architecture for networking backends.
