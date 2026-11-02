#!/usr/bin/env bash
# Validate examples with this checkout's binary, without contacting a VM host.
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$repo"
platform="$(go env GOOS)_$(go env GOARCH)"
provider_dir="$work/mirror/registry.opentofu.org/nanofaas/multipass/0.1.0/$platform"
mkdir -p "$provider_dir"
go build -ldflags '-X main.version=0.1.0' -o "$provider_dir/terraform-provider-multipass_v0.1.0" ./cmd/terraform-provider-multipass
python3 - "$work" <<'PY'
import json, pathlib, sys
work=pathlib.Path(sys.argv[1])
(work/'local.tfrc').write_text('provider_installation {\n filesystem_mirror {\n path = '+json.dumps(str(work/'mirror'))+'\n include = ["registry.opentofu.org/nanofaas/multipass"]\n }\n}\n')
PY
export TF_CLI_CONFIG_FILE="$work/local.tfrc" TF_IN_AUTOMATION=1 TF_INPUT=0
unset TF_REATTACH_PROVIDERS TF_DATA_DIR TF_CLI_ARGS TF_LOG TF_LOG_PATH
cp -R examples "$work/examples"
tofu fmt -check -recursive examples
for main in "$work"/examples/*/main.tf; do
  dir=$(dirname "$main")
  tofu -chdir="$dir" init -backend=false -no-color > "$work/init.log" 2>&1 || { cat "$work/init.log"; exit 1; }
  tofu -chdir="$dir" validate -no-color
 done
tofu -chdir="$work/examples/ssh-private-network" providers schema -json > "$work/schema.json"
python3 - "$work/schema.json" <<'PY'
import json,sys
s=json.load(open(sys.argv[1]))['provider_schemas']['registry.opentofu.org/nanofaas/multipass']
assert 'ssh_host' in s['provider']['block']['attributes']
assert {'id','name','address','active'} <= s['resource_schemas']['multipass_network']['block']['attributes'].keys()
print('Nanofaas SSH and managed network schema verified')
PY
