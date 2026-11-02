---
page_title: "multipass_network Resource"
subcategory: ""
description: "Manage a persistent private Linux bridge on an SSH host."
---

# multipass_network (Resource)

Creates a persistent NetworkManager bridge on the Linux host selected by
`ssh_host`. Connect instances with their existing `networks` block and set
static guest addresses using cloud-init/Netplan. See the complete
[two-VM example](../../examples/ssh-private-network/).

```hcl
resource "multipass_network" "lab" {
  name    = "nf-lab"
  address = "10.60.0.1/24"
}
```

## Requirements

The host needs NetworkManager running, Python 3, iproute2, and Multipass QEMU
1.15 or newer for bridge discovery. The SSH user needs noninteractive sudo to
execute the embedded network helper with Python 3. This is administrative
access; grant it only to trusted infrastructure automation accounts. The helper
runs per operation and does not install a daemon or write a separate script.
No packages or authentication settings are installed/changed automatically.

Physical interfaces are not added to the bridge, and no default routes, DHCP,
DNS, NAT or firewall rules are created. Overlapping host subnets and routes are
rejected. A host-wide lock serializes this provider's network mutations.
Existing NetworkManager profiles and unrelated interfaces are never adopted.
Ownership is recorded in `connection.stable-id` as `nanofaas-multipass-<UUID>`,
which is supported by NetworkManager 1.46 bridges.

Multipass retains its default management/NAT interface. These private bridges
are additional L2 segments, **not isolation boundaries between VMs**. Guest IPs
are not automatically reachable from the OpenTofu workstation.

## Schema

### Required

- `name` (String) Linux bridge name, 1–15 letters, digits, underscores or hyphens,
  starting with a letter. Changes require replacement.
- `address` (String) RFC1918 IPv4 address and prefix for the host bridge,
  for example `10.60.0.1/24`. Prefixes /8–/30 are supported within a private block;
  network/broadcast addresses are rejected. Changes require replacement.

### Read-Only

- `id` (String) Owned NetworkManager connection UUID.
- `active` (Boolean) Bridge is up with the configured address. An inactive or
  missing runtime bridge with a retained profile plans replacement.

## Lifecycle

Profiles autoconnect after reboot. Refresh reads profile and live interface state
without repairing them. Unreachable hosts cause errors and retain resource state.
Partial creation retains the connection UUID for recovery. Deletion refuses a
bridge with attached ports: destroy/detach its VMs first. Referencing the network
name from instance resources gives OpenTofu dependency ordering, but does not
force existing VMs to recreate merely because a bridge becomes inactive.
External attachments can therefore require manual intervention during repair.

## Import

Import only a provider-owned connection UUID. Arbitrary administrator-created
bridges remain usable by instances but cannot be adopted by this resource.

```sh
tofu import multipass_network.lab afc40530-1111-4111-8111-111111111111
```
