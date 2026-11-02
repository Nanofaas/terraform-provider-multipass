#!/usr/bin/python3 -I
"""Root-owned network helper; only validated, provider-owned NM bridges change."""
import fcntl
import ipaddress
import json
import os
import re
import stat
import subprocess
import sys
import time
import uuid
from pathlib import Path

LOCK_PATH = "/run/lock/nanofaas-multipass-networks.lock"
OWNER = "nanofaas-multipass-"
ALIAS = "nanofaas-multipass:"
PRIVATE = tuple(ipaddress.ip_network(n) for n in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"))


def validate(name, address):
    if not re.fullmatch(r"[a-zA-Z][a-zA-Z0-9_-]{0,14}", name):
        raise ValueError("bridge name must be 1–15 letters, digits, underscores or hyphens, starting with a letter")
    ip = ipaddress.ip_interface(address)
    if ip.version != 4 or not 8 <= ip.network.prefixlen <= 30 or not any(ip.network.subnet_of(n) for n in PRIVATE):
        raise ValueError("address must be a private IPv4 host/prefix (/8–/30)")
    if ip.ip in (ip.network.network_address, ip.network.broadcast_address):
        raise ValueError("network and broadcast addresses are not usable host addresses")
    return ip


class Host:
    def run(self, *args):
        result = subprocess.run(args, text=True, capture_output=True, timeout=60,
                                env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL": "C", "LANG": "C"})
        if result.returncode:
            raise RuntimeError(f"{args[0]} failed ({result.returncode}): {result.stderr.strip()}")
        return result.stdout.strip()

    def links(self):
        return json.loads(self.run("ip", "-j", "-d", "address", "show"))

    def profiles(self):
        return self.run("nmcli", "-g", "UUID", "connection", "show").splitlines()

    def profile(self, ident):
        values = self.run("nmcli", "-g", "connection.id,connection.interface-name,ipv4.addresses,connection.type,connection.stable-id",
                          "connection", "show", "uuid", ident).splitlines()
        if len(values) != 5:
            raise RuntimeError("unexpected NetworkManager bridge profile output")
        _, name, address, kind, owner = values
        if kind != "bridge" or owner != OWNER + ident:
            raise RuntimeError("connection is not owned by this provider")
        validate(name, address)
        return {"id": ident, "name": name, "address": address, "active": False}

    def owned_link(self, link, ident):
        if link.get("linkinfo", {}).get("info_kind") != "bridge":
            return False
        alias = link.get("ifalias", "")
        if alias:
            return alias == ALIAS + ident
        # A partially created bridge may not yet have received its kernel alias.
        return self.run("nmcli", "-g", "GENERAL.CON-UUID", "device", "show", link["ifname"]) == ident

    def read(self, ident):
        links = self.links()
        if ident not in self.profiles():
            owned = [x for x in links if x.get("ifalias") == ALIAS + ident]
            if not owned:
                return None
            if len(owned) != 1 or owned[0].get("linkinfo", {}).get("info_kind") != "bridge":
                raise RuntimeError("ambiguous orphaned bridge ownership")
            link = owned[0]
            addresses = [f'{x["local"]}/{x["prefixlen"]}' for x in link.get("addr_info", []) if x["family"] == "inet"]
            return {"id": ident, "name": link["ifname"], "address": addresses[0] if addresses else "", "active": False}
        result = self.profile(ident)
        link = next((x for x in links if x["ifname"] == result["name"]), None)
        if link is not None:
            if not self.owned_link(link, ident):
                raise RuntimeError("bridge interface is not owned by this connection")
            addresses = [f'{x["local"]}/{x["prefixlen"]}' for x in link.get("addr_info", []) if x["family"] == "inet"]
            result["active"] = "UP" in link.get("flags", []) and addresses == [result["address"]]
        return result

    def check_conflicts(self, name, network):
        links = self.links()
        if any(x["ifname"] == name for x in links):
            raise RuntimeError("bridge interface already exists; refusing to adopt it")
        subnets = []
        for link in links:
            for addr in link.get("addr_info", []):
                if addr["family"] == "inet":
                    subnets.append(ipaddress.ip_interface(f'{addr["local"]}/{addr["prefixlen"]}').network)
        for route in json.loads(self.run("ip", "-j", "-4", "route", "show", "table", "all")):
            dst = route.get("dst", "default")
            if dst not in ("default", "0.0.0.0/0"):
                subnets.append(ipaddress.ip_network(dst, strict=False))
        for ident in self.profiles():
            interface = self.run("nmcli", "-g", "connection.interface-name", "connection", "show", "uuid", ident)
            if interface == name:
                raise RuntimeError("a connection for this interface already exists")
            addresses = self.run("nmcli", "-g", "ipv4.addresses", "connection", "show", "uuid", ident)
            for addr in addresses.split(","):
                if addr.strip():
                    subnets.append(ipaddress.ip_interface(addr.strip()).network)
        if any(network.overlaps(n) for n in subnets):
            raise RuntimeError("requested subnet overlaps an existing address, route or connection")

    def create(self, req):
        ip = validate(req["name"], req["address"])
        if req["id"] in self.profiles():
            raise RuntimeError("connection UUID already exists")
        self.check_conflicts(req["name"], ip.network)
        # The add can succeed even if its response or later queries are lost.
        self.pending = dict(req, active=False)
        self.run("nmcli", "connection", "add", "type", "bridge", "ifname", req["name"],
                 "con-name", "nanofaas-multipass-" + req["name"], "connection.uuid", req["id"],
                 "connection.autoconnect", "yes", "bridge.stp", "no", "ipv4.method", "manual",
                 "ipv4.addresses", str(ip), "ipv4.never-default", "yes", "ipv6.method", "disabled", "connection.stable-id", OWNER + req["id"])
        self.run("nmcli", "connection", "up", "uuid", req["id"])
        self.run("ip", "link", "set", "dev", req["name"], "alias", ALIAS + req["id"])
        # nmcli can return while a portless bridge is still assigning its address.
        deadline = time.monotonic() + 30
        while True:
            result = self.read(req["id"])
            if result and result["active"]:
                return result
            if time.monotonic() >= deadline:
                raise RuntimeError("bridge activation did not produce the expected address within 30 seconds")
            time.sleep(0.2)

    def delete(self, ident):
        found = self.read(ident)
        if found is None:
            return
        link = next((x for x in self.links() if x["ifname"] == found["name"]), None)
        if link:
            if not self.owned_link(link, ident):
                raise RuntimeError("interface is not owned by this provider")
            if any(x.get("master") in (link.get("ifindex"), link["ifname"]) for x in self.links()):
                raise RuntimeError("bridge has attached ports; destroy or detach the VMs first")
        if ident in self.profiles():
            self.run("nmcli", "connection", "delete", "uuid", ident)
        remaining = next((x for x in self.links() if x["ifname"] == found["name"]), None)
        if remaining:
            if remaining.get("ifalias") != ALIAS + ident:
                raise RuntimeError("remaining interface ownership cannot be proven; retained")
            try:
                self.run("ip", "link", "delete", "dev", found["name"])
            except RuntimeError:
                # NetworkManager can remove the device after the previous query.
                if any(x["ifname"] == found["name"] for x in self.links()):
                    raise

    def check(self):
        checks = [{"name": "python", "status": "PASS", "detail": sys.version.split()[0]}]
        for name, args in (("networkmanager", ("nmcli", "--version")), ("iproute2", ("ip", "-Version"))):
            try:
                detail = self.run(*args)
                if name == "networkmanager" and self.run("nmcli", "-g", "RUNNING", "general") != "running":
                    raise RuntimeError("NetworkManager is not running")
                checks.append({"name": name, "status": "PASS", "detail": detail})
            except Exception as err:
                checks.append({"name": name, "status": "FAIL", "detail": str(err)})
        try:
            filtering = Path("/proc/sys/net/bridge/bridge-nf-call-iptables").read_text().strip() == "1"
        except FileNotFoundError:
            filtering = False
        if filtering:
            try:
                rules = self.run("iptables", "-S", "FORWARD")
                policy = next((line for line in rules.splitlines() if line.startswith("-P ")), "policy unavailable")
            except Exception as err:
                policy = str(err)
            checks.append({"name": "firewall", "status": "WARN", "detail": "Bridged IPv4 filtering is enabled; allow traffic within each private bridge. " + policy})
        else:
            checks.append({"name": "firewall", "status": "PASS", "detail": "Bridge IPv4 iptables filtering is disabled; other firewall backends are not inspected."})
        return {"checks": checks}

    def execute(self, action, req):
        try:
            if action not in ("create", "read", "delete", "check"):
                raise ValueError("invalid bridge action")
            if not isinstance(req, dict):
                raise ValueError("request must be a JSON object")
            if action == "check":
                if req:
                    raise ValueError("check accepts an empty object")
                result = self.check()
            else:
                if set(req) - {"id", "name", "address", "active"}:
                    raise ValueError("unexpected request fields")
                if not isinstance(req.get("id"), str) or str(uuid.UUID(req["id"])) != req["id"]:
                    raise ValueError("id must be a canonical UUID")
                result = self._execute(action, req)
        except Exception as err:
            result = {"error": str(err)}
        return dict(result, protocol_version=1)

    def _execute(self, action, req):
        self.pending = None
        try:
            uuid.UUID(req["id"])
            if action == "read":
                found = self.read(req["id"])
                return {"bridge": found, "missing": found is None}
            if action not in ("create", "delete"):
                raise ValueError("invalid bridge action")
            if action == "create":
                validate(req["name"], req["address"])
            fd = os.open(LOCK_PATH, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
            with os.fdopen(fd, "w") as lock:
                metadata = os.fstat(lock.fileno())
                if metadata.st_uid != os.geteuid() or not stat.S_ISREG(metadata.st_mode) or metadata.st_nlink != 1:
                    raise RuntimeError("unsafe network lock ownership")
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    if action == "create":
                        return {"bridge": self.create(req)}
                    self.delete(req["id"])
                    return {}
                except Exception as err:
                    # Preserve the attempted UUID; Read/Delete still verify ownership.
                    partial = self.pending
                    if action == "create" and partial is not None:
                        try:
                            if req["id"] in self.profiles():
                                partial = self.profile(req["id"])
                        except Exception:
                            pass
                    return {"error": str(err), "bridge": partial}
        except Exception as err:
            return {"error": str(err)}


def main():
    try:
        if len(sys.argv) != 2:
            raise ValueError("expected exactly one action: create, read, delete or check")
        if sys.argv[1] == "--version":
            result = {"protocol_version": 1}
        else:
            raw = sys.stdin.buffer.read(65537)
            if len(raw) > 65536:
                raise ValueError("request exceeds 64 KiB")
            result = Host().execute(sys.argv[1], json.loads(raw))
    except Exception as error:
        result = {"protocol_version": 1, "error": str(error)}
    print(json.dumps(result))


if __name__ == "__main__":
    main()
