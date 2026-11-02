---
page_title: "Installation and host preparation"
description: "Install the provider and prepare Linux SSH hosts with restricted network privileges."
---

# Installation and host preparation

OpenTofu and OpenSSH run on the workstation. Multipass runs on the Linux server;
the SSH user must already be authenticated with that daemon. VM-only operation
needs no privileged helper. Follow the repository README for the local provider
mirror until a registry release is indexed.

## Managed networks

The host needs `/usr/bin/python3`, NetworkManager, iproute2 and sudo. An administrator
installs the reviewed network helper once. From a trusted checkout on the server:

```sh
sudo sh scripts/install-host-helper.sh YOUR_SSH_USER
```

An extracted host-tools release bundle contains the same installer and helper:

```sh
sudo sh ./install-host-helper.sh YOUR_SSH_USER
```

The installer writes a root-owned executable to
`/usr/local/libexec/nanofaas-multipass-network` and an exact sudoers grant in
`/etc/sudoers.d/nanofaas-multipass-YOUR_SSH_USER`. It validates the sudoers syntax
before replacing either file. It rejects symlink destinations and writable
installation directories. Re-run it from a reviewed newer release to upgrade.
No daemon is installed and no packages, physical NICs or firewall rules change.

The only permitted elevated operations are `create`, `delete`, and read-only
`check`, with bounded JSON on stdin. The isolated interpreter and fixed subprocess
environment prevent caller-controlled Python modules or PATH from running as root.
The account can manage provider-owned bridges, not arbitrary NetworkManager profiles.
Network ownership is shared by trusted provider operators on the host, not isolated
per SSH account.

Verify as the SSH user:

```sh
printf '{}' | sudo -n -- /usr/local/libexec/nanofaas-multipass-network check
```

## Migrate from the development prototype

Earlier checkouts executed embedded Python with unrestricted `sudo python3`.
The provider now requires the installed helper and never falls back to that mode.
Existing owned bridge UUIDs remain usable; no VM or bridge recreation is needed.
After installation and successful verification, an administrator should back up
and remove the old Python grant. The installer intentionally does not edit other
sudoers files. Do not leave both grants enabled and assume privileges are restricted.
Keep the normal administrative recovery path while validating the migration.

The host firewall still needs to allow traffic within each selected bridge; see
[network prerequisites](../resources/network.md#host-firewall-prerequisites).

## Read-only preflight

Run the `multipass-doctor` executable from the provider release archive on your
workstation, or build it with `go build -o build/multipass-doctor ./cmd/multipass-doctor`:

```sh
multipass-doctor -host SERVER -user SSH_USER -port 22
multipass-doctor -host SERVER -user SSH_USER -port 22 -networks
```

The first command checks SSH/Linux, supported Multipass version and daemon
access. `-networks` adds QEMU, installed helper protocol/sudo access,
NetworkManager, iproute2 and bridged IPv4 filtering. It also checks root ownership,
non-writable helper directories, execution by the SSH user and NOPASSWD
authorization for the exact create/delete commands using `sudo -ll`. It makes no configuration
changes. FAIL returns exit status 1; WARN requires operator attention but does
not mean the host is unusable. A firewall warning is not a connectivity test.
Use `-identity-file`, `-known-hosts-file` and `-multipass-path` when needed;
otherwise OpenSSH configuration and the remote CLI PATH apply.

## Release verification

Download the platform ZIP, host-tools archive (when managing networks),
`SHA256SUMS`, its `.sig` signature and the public signing key from the same
[GitHub release](https://github.com/Nanofaas/terraform-provider-multipass/releases).
The dedicated signing key fingerprint is:

```text
BC1D 2E34 BCE6 C306 73A5 CAA4 FE32 DA97 9EF3 7C37
```

Verify this fingerprint independently against `packaging/public-key.asc` in the
repository. Import the public key, verify the checksum signature, and check the
downloaded files before executing the installer or binaries:

```sh
gpg --import nanofaas-multipass-signing-key.asc
gpg --verify terraform-provider-multipass_0.1.0_SHA256SUMS.sig terraform-provider-multipass_0.1.0_SHA256SUMS
sha256sum --ignore-missing -c terraform-provider-multipass_0.1.0_SHA256SUMS
```

On macOS use `shasum -a 256 -c` with the checksum entries for the files you
downloaded. A valid signature alone does not establish trust in an unknown key.
Each platform ZIP includes the provider and `multipass-doctor`; the host-tools
archive is platform-independent and runs only on supported Linux servers.
