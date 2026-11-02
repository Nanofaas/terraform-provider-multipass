# Nanofaas OpenTofu Multipass Provider

<p align="center">
  <img width="600"  alt="terraform-provider-multipass-heading-image" src="https://github.com/user-attachments/assets/61f462de-90b1-4352-80a7-ab6bb140e175" />
</p>

[Terraform](https://www.terraform.io/) provider for [Canonical Multipass](https://canonical.com/multipass), implemented with the modern Terraform Plugin Framework. It exposes rich instance lifecycle management, alias automation, and data sources for images and networks while favoring performance and clear diagnostics.

## Features

- Provider configuration for CLI discovery, command timeouts, default images, and cached `multipass` metadata.
- Remote Multipass daemons through `server_address`, with provider aliases for managing multiple hosts.
- `multipass_instance` resource with CPU/memory/disk sizing, multiple networks, host mounts, and inline or file-based cloud-init.
- Configurable resizing: CPU, memory, and disk growth update the existing VM by default; `resize_policy = "replace"` rebuilds it instead.
- `multipass_snapshot` resource for managing named snapshots (create/list/delete/import).
- `multipass_alias` resource for ergonomic host shortcuts into instances.
- `multipass_file_upload` and `multipass_file_download` resources for Terraform-managed file transfers without provisioners.
- Data sources for images, networks, instances, and snapshots to compose dynamic plans.
- Parser-backed CLI abstraction with detailed diagnostics.
- Pulumi examples and bridge compatibility checks for TypeScript and Python.

## Getting Started

This is a fork of [todoroff/terraform-provider-multipass](https://github.com/todoroff/terraform-provider-multipass), preserving its MIT license and history. Nanofaas adds Linux SSH execution and persistent private bridge resources. The original local/daemon modes remain available.

Run OpenTofu on your workstation; the provider runs Multipass **on the Linux server over SSH**. The workstation needs OpenTofu and OpenSSH. The server needs an authenticated Multipass CLI user. Managed networks additionally need NetworkManager, Python 3, iproute2 and the [root-owned network helper with restricted sudoers rules](docs/guides/installation.md). Strict SSH host-key verification is always enabled.

The Nanofaas address is `registry.opentofu.org/nanofaas/multipass`. **Registry registration is not yet complete:** build and install from this checkout using a local mirror:

```sh
provider_build="$(pwd)/build"
provider_platform="$(go env GOOS)_$(go env GOARCH)"
provider_dir="$provider_build/mirror/registry.opentofu.org/nanofaas/multipass/0.1.0/$provider_platform"
mkdir -p "$provider_dir"
go build -ldflags '-X main.version=0.1.0' -o "$provider_dir/terraform-provider-multipass_v0.1.0" ./cmd/terraform-provider-multipass
cat > "$provider_build/local.tfrc" <<EOF
provider_installation {
  filesystem_mirror {
    path = "$provider_build/mirror"
    include = ["registry.opentofu.org/nanofaas/multipass"]
  }
}
EOF
export TF_CLI_CONFIG_FILE="$provider_build/local.tfrc"
cd examples/ssh-private-network
tofu init
tofu plan -var='ssh_host=192.0.2.10' -var='ssh_user=ubuntu'
```

The [two-VM example](examples/ssh-private-network/) creates a private bridge and static guest addresses. Choose a non-overlapping subnet and verified host key before applying. These bridges are additional L2 segments: Multipass retains its default NIC, so they do not provide complete VM isolation.

```hcl
provider "multipass" {
  ssh_host = "192.0.2.10"
  ssh_user = "ubuntu"
  ssh_port = 22
}
```

Optional `ssh_identity_file` and `ssh_known_hosts_file` refer to files on the workstation; SSH agent and configuration are supported. In SSH mode, `multipass_path` is remote, cloud-init/upload files are local, and mount sources are server paths. Recursive SSH transfers are explicitly unsupported; single files are supported.

## Pulumi

Use the provider through Pulumi's [Any Terraform Provider](https://www.pulumi.com/docs/iac/concepts/providers/any-terraform-provider/) bridge. Build the local provider before running the examples; Nanofaas registry installation is pending:

```sh
go build -o build/terraform-provider-multipass ./cmd/terraform-provider-multipass
cd examples/pulumi/typescript # or examples/pulumi/python
pulumi install
```

Start with the [TypeScript or Python examples](examples/pulumi/). The [Pulumi guide](docs/guides/pulumi.md) covers local builds (including Windows), configuration, imports, and lifecycle details such as replacement ordering and the generated `ipv4s` output.

## Provider Configuration

| Attribute        | Type   | Description                                                                 |
| ---------------- | ------ | --------------------------------------------------------------------------- |
| `multipass_path` | String | Optional explicit path to the `multipass` binary. Defaults to PATH lookup. |
| `server_address` | String | Optional daemon address, such as `host:50051`. Overrides `MULTIPASS_SERVER_ADDRESS` for this provider; otherwise inherits the environment or the CLI's local default. |
| `command_timeout`| Int    | Timeout in seconds for CLI calls (default 600).                             |
| `default_image`  | String | Fallback image alias/name when resources omit `image`.                      |

## Remote hosts

Set `server_address` on each provider configuration to manage instances on different Multipass hosts:

```hcl
provider "multipass" {
  alias          = "remote"
  server_address = "192.0.2.10:50051"
}

resource "multipass_instance" "remote_dev" {
  provider = multipass.remote
  name     = "remote-dev"
  image    = "lts"
}
```

The Multipass CLI still runs on the Terraform machine. Configure the remote daemon's endpoint, establish TLS trust, and authenticate that CLI user with it before applying. Multipass 1.16 needs the remote CA in its application-specific trust bundle. See the [remote-host guide](docs/guides/remote-hosts.md) for the tested SSH tunnel setup, environment precedence, and networking requirements, or the [two-host example](examples/remote-hosts/).

## Resources

- `multipass_network`: persistent private Linux bridges over SSH ([reference](docs/resources/network.md)).

- `multipass_instance`: manages VM lifecycle. Supports optional `networks` and `mounts` nested blocks, cloud-init file references, and auto-recovery semantics.
- `multipass_alias`: creates host aliases executing commands inside instances.
- `multipass_file_upload`: provision-style file or directory uploads backed by `multipass transfer`, an alternative to Terraform provisioners.
- `multipass_file_download`: pull files or directories from Multipass instances back to the host with Terraform-managed lifecycles.

## Instance resize policy

`multipass_instance.resize_policy` defaults to `"in_place"`. A size change stops and resizes a running VM, then restarts it; a stopped VM remains stopped. In-place disk shrink is rejected. Removing a sizing argument keeps its current allocation.

This changes the previous default of replacing the VM for every size change. Set `resize_policy = "replace"` to retain that behavior, including replacement with a smaller disk. Changing only the policy does not restart or replace the instance. Image, name, network, and cloud-init changes still force replacement.

See the [instance resource documentation](docs/resources/instance.md#resizing) for downtime, failure recovery, and guest filesystem expansion details.

## Data Sources

- `multipass_images`: enumerates images/blueprints from `multipass find`, with filters for name, alias, kind, and text query.
- `multipass_networks`: lists bridgable host networks.
- `multipass_instance`: inspects an existing instance for read-only data.
- `multipass_snapshots`: returns snapshots for a target instance with optional name filtering.

## Examples

See `examples/README.md` for scenario overviews. Highlights:

- `examples/basic`: minimal single instance + alias.
- `examples/dev-lab`: multi-instance dev tier (db/api/web) with aliases and outputs.
- `examples/bridged-workstation`: demonstrates bridged networking, host mounts, and working-directory aliases.
- `examples/cloud-init-lab`: shows cloud-init provisioning with supporting YAML.

Each Terraform directory is self-contained. Configure the local mirror above before running `tofu init`; Nanofaas registry downloads are not yet available. The [Pulumi examples](examples/pulumi/) use `pulumi install`.

## Development

For local hacking you can still build from source via:

```bash
go build ./cmd/terraform-provider-multipass
```

### Tests

Run unit tests and static checks without creating VMs:

```bash
go test ./...
go vet ./...
```

Acceptance tests use real Multipass instances and remove their test resources afterward. They require a running Multipass daemon and Terraform or OpenTofu. Set the provider namespace because the test configurations use `registry.opentofu.org/nanofaas/multipass` explicitly.

```powershell
$env:TF_ACC = "1"
$env:TF_ACC_PROVIDER_NAMESPACE = "nanofaas"
$env:TF_ACC_PROVIDER_HOST = "registry.opentofu.org"
$env:TF_ACC_TERRAFORM_PATH = (Get-Command tofu).Source # or terraform
go test ./internal/multipasscli ./internal/provider -p 1 -run '^TestAcc' -count=1 -v -timeout 30m
```

On Linux/macOS, set the same environment variables before running the Go command. Leave `TF_ACC` unset for unit tests. The suite covers scoped VM deletion, instance replacement, both resize policies and power-state preservation, cloud-init failure and retry, computed uploads, download contents and ownership, aliases, data sources, and snapshots. Run only resize acceptance tests with `go test ./internal/provider -run '^TestAccInstanceResource_resize' -count=1 -v -timeout 30m` after setting the same environment variables.

### Documentation

Documentation is maintained by hand under `docs/`. Resource and data-source filenames omit the `multipass_` prefix. Registry pages include YAML frontmatter, and guides set `page_title`. Keep links in the READMEs, `AGENTS.md`, and `llms.txt` aligned when renaming pages.

Run the same validation used by CI with PowerShell and Go, plus Terraform or OpenTofu on PATH:

```powershell
./scripts/validate-docs.ps1
# To use OpenTofu instead:
./scripts/validate-docs.ps1 -TerraformPath tofu
```

On Linux/macOS, invoke it with `pwsh -File scripts/validate-docs.ps1`. The script checks local Markdown links, builds the provider from `cmd/`, exports its schema, and runs pinned `tfplugindocs` 0.25.0 validation. That tool requires Go 1.25.8 or newer; Go can download the required toolchain automatically. It needs no Multipass daemon and does not create VMs. Temporary CLI settings and files are isolated under `build/` and cleaned up afterward.

Reference page paths now use names such as `docs/resources/instance.md` and `docs/data-sources/instance.md`. Existing release documentation keeps its previous paths; update links to `latest` when the next release publishes these renamed pages. Use the [Registry preview tool](https://registry.terraform.io/tools/doc-preview) to review rendering before a release.

### CI & Releases

- CI runs on GitHub Actions (`.github/workflows/ci.yml`) and executes `go test ./...` across a small matrix of Go versions and OSes.
- Documentation validation runs in `.github/workflows/docs.yml` on Linux and Windows, including documentation-only changes.
- Pulumi compatibility runs separately in `.github/workflows/pulumi.yml`: schema checks, SDK generation, TypeScript checking, and previews with a fake Multipass CLI on Linux and Windows. See [how to run it locally](docs/guides/pulumi.md#compatibility-checks).
- Tagged releases (`X.Y.Z`) trigger GoReleaser (`.goreleaser.yml`) via `.github/workflows/release.yml`, which builds cross-platform artifacts suitable for attaching to GitHub Releases and publishing to the Terraform/OpenTofu registries.

## License

This project is licensed under the [MIT License](LICENSE).

## Remote acceptance tests

Only use a disposable, authorized Linux host. Verify its host key and authenticate
its SSH user with Multipass first. No tests install packages or change access
policies. Network tests require noninteractive sudo and an unused subnet.

```sh
export TF_ACC=1 TF_ACC_PROVIDER_NAMESPACE=nanofaas TF_ACC_PROVIDER_HOST=registry.opentofu.org
export TF_ACC_TERRAFORM_PATH="$(command -v tofu)"
export MULTIPASS_ACC_SSH_HOST=192.0.2.10 MULTIPASS_ACC_SSH_USER=ubuntu MULTIPASS_ACC_SSH_PORT=22
export MULTIPASS_ACC_SUBNET=10.60.0.0/24
go test ./internal/provider -run '^TestAccSSH' -count=1 -v -timeout 40m
```

Ordinary `go test ./...` does not create VMs. `bash scripts/validate-opentofu.sh`
builds a temporary mirror and validates all examples/schema without connecting
to a VM host. A GitHub repository is not a registry release: signed releases
require the repository's `GPG_PRIVATE_KEY` and `PASSPHRASE` secrets and separate
OpenTofu registry registration. No signing credentials are included in this fork.
