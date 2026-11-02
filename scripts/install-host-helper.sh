#!/bin/sh
# Run as an administrator from a trusted checkout or extracted host-tools archive.
set -eu
PATH=/usr/sbin:/usr/bin:/sbin:/bin
export PATH
if [ "$(id -u)" -ne 0 ]; then echo 'Run this installer as root.' >&2; exit 1; fi
if [ "$#" -ne 1 ]; then echo "Usage: $0 SSH_USER" >&2; exit 1; fi
user=$1
/usr/bin/python3 -I -c 'import re,sys; sys.exit(0 if re.fullmatch(r"[a-z_][a-z0-9_-]*[$]?",sys.argv[1]) else 1)' "$user" || { echo 'Invalid SSH user name.' >&2; exit 1; }
getent passwd "$user" >/dev/null || { echo 'SSH user does not exist.' >&2; exit 1; }
visudo -c >/dev/null
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
source=$script_dir/nanofaas-multipass-network
if [ ! -f "$source" ]; then source=$script_dir/../internal/hostnetwork/scripts/bridge.py; fi
[ -f "$source" ] || { echo 'Network helper source is missing.' >&2; exit 1; }
helper=/usr/local/libexec/nanofaas-multipass-network
sudoers=/etc/sudoers.d/nanofaas-multipass-$user
# Refuse writable ancestors and symlinks instead of silently changing their ownership.
/usr/bin/python3 -I - <<'PY'
import pathlib,stat
for raw in ('/usr/local/libexec','/etc/sudoers.d'):
 p=pathlib.Path(raw)
 for entry in (p,*p.parents):
  if entry.is_symlink(): raise SystemExit('Unsafe symbolic link: '+str(entry))
  if entry.exists():
   mode=entry.stat()
   if mode.st_uid!=0 or mode.st_mode & 0o022: raise SystemExit('Unsafe directory ownership/mode: '+str(entry))
PY
[ ! -L "$helper" ] && [ ! -L "$sudoers" ] || { echo 'Refusing a symbolic link destination.' >&2; exit 1; }
install -d -o root -g root -m 0755 /usr/local/libexec /etc/sudoers.d
helper_tmp=$(mktemp /usr/local/libexec/.nanofaas-helper.XXXXXX)
sudo_tmp=$(mktemp /etc/sudoers.d/.nanofaas-rule.XXXXXX)
trap 'rm -f "$helper_tmp" "$sudo_tmp"' EXIT HUP INT TERM
install -o root -g root -m 0755 "$source" "$helper_tmp"
/usr/bin/python3 -I "$helper_tmp" --version
printf '%s ALL=(root) NOPASSWD: %s create, %s delete, %s check\n' "$user" "$helper" "$helper" "$helper" > "$sudo_tmp"
chmod 0440 "$sudo_tmp"
visudo -cf "$sudo_tmp"
mv -f "$helper_tmp" "$helper"
mv -f "$sudo_tmp" "$sudoers"
echo "Installed $helper and $sudoers"
echo 'Existing broad sudo rules were not changed; revoke obsolete grants after verification.'
