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
