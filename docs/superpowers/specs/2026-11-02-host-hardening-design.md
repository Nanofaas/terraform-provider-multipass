# Host hardening and first release

The user approved all feasible follow-up work: replace unrestricted Python sudo,
provide read-only prerequisite checks, verify reboot persistence when authorized,
and publish installable signed artifacts and register the provider.

Use the existing bridge helper as an installed, root-owned executable at
`/usr/local/libexec/nanofaas-multipass-network`. Its isolated Python interpreter,
fixed subprocess environment, bounded JSON input and strict operation/UUID
validation form the privilege boundary. Sudoers grants only exact `create`,
`delete`, and read-only `check` commands. Read remains unprivileged. No arbitrary
helper path, shell, Python source or automatic fallback to Python sudo is allowed.
A root-run installer installs the same reviewed source and narrow sudoers entry.
Existing VM-only use requires no helper installation. Protocol version 1 prevents
silent incompatible helper/provider combinations. Network ownership stays intact.

Provide a standalone `multipass-doctor` command, using the existing SSH runner,
with host/user/port/key/known-hosts and optional managed-network checks. Report
Linux, Multipass version/authentication, QEMU driver, helper protocol/permissions,
NetworkManager/iproute2, and bridged firewall filtering. Findings are PASS, FAIL
or WARN; firewall policy is observed, not changed. No provisioner or background
service is added.

Keep the existing GoReleaser workflow, add a tested snapshot mode, package the
provider and doctor for Linux/macOS/Windows amd64/arm64, and attach the host tools,
manifest, checksums and signature. Use a dedicated project signing key, isolated
from personal keys; protect private material outside Git and configure repository
secrets without printing it. First Nanofaas release is v0.1.0. Registry submission
must follow the current official issue-form process, with truthful status if a
human/browser/account step blocks submission or acceptance.

Alternatives considered: continuing arbitrary Python sudo fails least privilege;
a root daemon adds unnecessary operational complexity. A fixed installed helper
is the approach already proposed and authorized. Firewall management, isolation,
DHCP, NAT and automatic package installation remain excluded.

Live migration: install and verify the helper, test resources, then revoke only
the previously added broad Python sudo entry, after preserving a root-only backup.
Verify that generic Python sudo is denied and helper operations still work. Any
reboot needs explicit approval after inventory and a concrete disposable test
configuration. Do not silently reboot an active user session.
