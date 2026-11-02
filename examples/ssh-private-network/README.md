# Two VMs on a remote Linux private bridge

Install the Nanofaas provider using the repository's local mirror instructions
until a registry release exists. The workstation needs OpenTofu and OpenSSH;
Multipass runs on the server.

The Linux server needs Multipass QEMU >= 1.15, NetworkManager, Python 3 and
iproute2. The SSH account must be authenticated with Multipass and authorized
to run the installed root-owned network helper through restricted sudoers rules.
Follow the [host preparation guide](../../docs/guides/installation.md). Enroll a verified SSH
host key and configure key/agent authentication first.

```sh
tofu init
tofu plan -var='ssh_host=192.0.2.10' -var='ssh_user=ubuntu'
tofu apply -parallelism=1 -var='ssh_host=192.0.2.10' -var='ssh_user=ubuntu'
```

Choose a `subnet` that does not overlap existing server networks/routes.
Ensure the host firewall permits forwarding within `nf-lab`; see the
[firewall prerequisites](../../docs/resources/network.md#host-firewall-prerequisites),
especially on Docker hosts. Change `ssh_port` when needed; key files and ProxyJump can be configured in
OpenSSH. Names in this example are fixed, so use a separate name prefix if
running multiple copies on the same host.

The two static private addresses are `10.60.0.11` and `10.60.0.12` by default.
From the server, test `multipass exec nf-lab-first -- ping -c 2 10.60.0.12`.
The guest's default Multipass NIC remains present for management and Internet
access. The private bridge provides no additional isolation or DNS/DHCP/NAT.
Accessing private IPs from your workstation requires routing or a separate SSH
tunnel; creating and managing the VMs does not require that routing.

```sh
tofu destroy -parallelism=1 -var='ssh_host=192.0.2.10' -var='ssh_user=ubuntu'
```

Destroy removes the two owned VMs before the bridge. External bridge ports
prevent deletion. Keep the state file until cleanup has completed.
