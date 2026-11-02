import importlib.util
import json
import tempfile
import threading
import time
import unittest
from pathlib import Path
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("bridge", Path(__file__).with_name("bridge.py"))
bridge = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge)

class Store:
    def __init__(self):
        self.profiles = {}
        self.links = []
        self.commands = []
        self.fail_up = False
        self.routes = [{"dst": "default", "gateway": "192.0.2.1"}]

class FakeHost(bridge.Host):
    def __init__(self, store):
        self.store = store

    def run(self, *args):
        s = self.store
        s.commands.append(args)
        if args[:4] == ("ip", "-j", "-d", "address"):
            return json.dumps(s.links)
        if args[:4] == ("ip", "-j", "-4", "route"):
            return json.dumps(s.routes)
        if args[:3] == ("nmcli", "-g", "UUID"):
            return "\n".join(s.profiles)
        if args[:3] == ("nmcli", "-g", "GENERAL.CON-UUID"):
            return next(ident for ident,p in s.profiles.items() if p['name']==args[-1])
        if args[:2] == ("nmcli", "-g"):
            p = s.profiles[args[-1]]
            fields = args[2]
            if fields == "ipv4.addresses": return p["address"]
            if fields == "connection.interface-name": return p["name"]
            return "\n".join(["nanofaas-multipass-" + p["name"], p["name"], p["address"], "bridge", p["owner"]])
        if args[:3] == ("nmcli", "connection", "add"):
            if "user.data" in args:
                raise RuntimeError("NetworkManager 1.46 bridge does not support user.data")
            values = dict(zip(args[3::2], args[4::2]))
            ident = values["connection.uuid"]
            s.profiles[ident] = {"name": values["ifname"], "address": values["ipv4.addresses"], "owner": values["connection.stable-id"]}
            time.sleep(0.02)
            return ""
        if args[:3] == ("nmcli", "connection", "up"):
            if s.fail_up: raise RuntimeError("activation failed")
            ident = args[-1]; p = s.profiles[ident]
            ip, prefix = p["address"].split("/")
            s.links.append({"ifname": p["name"], "ifindex": 9, "flags": ["UP"], "linkinfo": {"info_kind": "bridge"}, "addr_info": [{"family": "inet", "local": ip, "prefixlen": int(prefix)}]})
            return ""
        if args[:4] == ("ip", "link", "set", "dev"):
            next(x for x in s.links if x["ifname"] == args[4])["ifalias"] = args[6]
            return ""
        if args[:3] == ("nmcli", "connection", "delete"):
            p=s.profiles.pop(args[-1]); s.links[:]=[x for x in s.links if x["ifname"]!=p["name"]]; return ""
        if args[:4] == ("ip", "link", "delete", "dev"):
            s.links[:]=[x for x in s.links if x["ifname"]!=args[4]]; return ""
        raise AssertionError(args)

class BridgeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        patcher = patch.object(bridge, "LOCK_PATH", str(Path(self.tmp.name) / "lock"))
        patcher.start(); self.addCleanup(patcher.stop)
        self.store = Store(); self.host = FakeHost(self.store)
        self.req = {"id": "afc40530-1111-4111-8111-111111111111", "name": "nf-test", "address": "10.60.0.1/24"}

    def test_lifecycle_and_activation_failure(self):
        self.store.fail_up = True
        result = self.host.execute("create", self.req)
        self.assertIn("activation failed", result["error"])
        self.assertEqual(result["bridge"]["id"], self.req["id"])
        self.assertEqual(len(self.store.profiles), 1)
        self.store.fail_up = False
        self.assertFalse(self.host.execute("read", self.req)["bridge"]["active"])
        self.assertNotIn("error", self.host.execute("delete", self.req))
        self.assertNotIn("error", self.host.execute("delete", self.req))
        self.assertEqual(self.store.profiles, {})

    def test_ownership_and_ports(self):
        self.assertNotIn("error", self.host.execute("create", self.req))
        self.assertTrue(self.host.execute("read", self.req)["bridge"]["active"])
        self.store.links.append({"ifname": "tap1", "master": 9})
        self.assertIn("attached", self.host.execute("delete", self.req)["error"])
        self.store.links.pop()
        self.store.profiles[self.req["id"]]["owner"] = "other.owner=someone"
        self.assertIn("owned", self.host.execute("delete", self.req)["error"])
        self.assertEqual(len(self.store.profiles), 1)

    def test_overlap_and_foreign_interface(self):
        self.store.routes.append({"dst": "10.60.0.0/16"})
        self.assertIn("overlap", self.host.execute("create", self.req)["error"])
        self.assertEqual(self.store.profiles, {})
        self.store.routes.pop()
        self.store.links.append({"ifname": "nf-test", "ifindex": 8, "addr_info": []})
        self.assertIn("exists", self.host.execute("create", self.req)["error"])

    def test_concurrent_creation(self):
        results = []
        def create(n):
            req = dict(self.req, id=f"afc40530-1111-4111-8111-{n:012d}", name=f"nf-{n}")
            results.append(FakeHost(self.store).execute("create", req))
        workers = [threading.Thread(target=create, args=(i,)) for i in (1, 2)]
        for worker in workers: worker.start()
        for worker in workers: worker.join()
        self.assertEqual(sum("error" not in r for r in results), 1)
        self.assertEqual(len(self.store.profiles), 1)

    def test_orphan_recovery_and_missing(self):
        self.assertNotIn("error", self.host.execute("create", self.req))
        self.store.profiles.clear()
        self.assertFalse(self.host.execute("read", self.req)["bridge"]["active"])
        self.assertNotIn("error", self.host.execute("delete", self.req))
        self.assertEqual(self.store.links, [])
        self.assertTrue(self.host.execute("read", self.req)["missing"])

    def test_invalid_input_does_not_execute_commands(self):
        for address in ["8.8.8.8/24", "10.60.0.0/24", "10.60.0.255/24", "10.0.0.1/7"]:
            self.assertIn("error", self.host.execute("create", dict(self.req, address=address)))
        self.assertEqual(self.store.commands, [])

    def test_creation_identity_survives_failed_readback(self):
        original = self.host.run
        def failed_readback(*args):
            if self.store.profiles and args[:2] == ('nmcli', '-g'):
                raise RuntimeError('NetworkManager unavailable after add')
            return original(*args)
        self.host.run = failed_readback
        result = self.host.execute('create', self.req)
        self.assertIn('error', result)
        self.assertEqual(len(self.store.profiles), 1)
        self.assertIsNotNone(result['bridge'])
        self.assertEqual(result['bridge']['id'], self.req['id'])
        self.assertFalse(result['bridge']['active'])

    def test_wait_for_address_after_nmcli_up(self):
        original = self.host.run
        delayed = [True]
        def lagging_address(*args):
            result = original(*args)
            if args[:4] == ('ip', '-j', '-d', 'address') and self.store.links and delayed[0]:
                delayed[0] = False
                links = json.loads(result)
                links[0]['addr_info'] = []
                return json.dumps(links)
            return result
        self.host.run = lagging_address
        result = self.host.execute('create', self.req)
        self.assertNotIn('error', result)
        self.assertTrue(result['bridge']['active'])

    def test_delete_when_networkmanager_removes_link_asynchronously(self):
        self.assertNotIn('error', self.host.execute('create', self.req))
        original = self.host.run
        def asynchronous_removal(*args):
            if args[:3] == ('nmcli', 'connection', 'delete'):
                self.store.profiles.pop(args[-1])
                return ''
            if args[:4] == ('ip', 'link', 'delete', 'dev'):
                self.store.links.clear()
                raise RuntimeError('Cannot find device')
            return original(*args)
        self.host.run = asynchronous_removal
        self.assertNotIn('error', self.host.execute('delete', self.req))
        self.assertEqual(self.store.links, [])

    def test_request_boundaries_before_commands(self):
        for req in [dict(self.req, command='ignored'), dict(self.req, id=self.req['id'].replace('-', '')), dict(self.req, id=3)]:
            result = self.host.execute('create', req)
            self.assertIn('error', result)
            self.assertEqual(self.store.commands, [])
        self.assertEqual(self.host.execute('read', self.req).get('protocol_version'), 1)

    def test_delete_after_reboot_with_delayed_interface_removal(self):
        self.assertNotIn('error', self.host.execute('create', self.req))
        self.store.links[0].pop('ifalias')
        original = self.host.run
        def delayed_removal(*args):
            if args[:3] == ('nmcli', 'connection', 'delete'):
                self.store.profiles.pop(args[-1])
                return ''
            return original(*args)
        self.host.run = delayed_removal
        self.assertTrue(self.host.execute('read', self.req)['bridge']['active'])
        self.assertNotIn('ifalias', self.store.links[0])
        self.assertNotIn('error', self.host.execute('delete', self.req))
        self.assertEqual(self.store.links, [])
        self.assertEqual(self.store.profiles, {})

    def test_subprocess_environment_is_not_user_controlled(self):
        import os, subprocess
        with patch.dict(os.environ, {'PATH':'/tmp/untrusted', 'PYTHONPATH':'/tmp/untrusted'}):
            with patch.object(bridge.subprocess, 'run', return_value=subprocess.CompletedProcess([],0,'ok','')) as run:
                self.assertEqual(bridge.Host().run('nmcli','--version'), 'ok')
                env=run.call_args.kwargs['env']
                self.assertTrue('PYTHONPATH' not in env, 'untrusted Python environment inherited')
                self.assertEqual(env['PATH'],'/usr/sbin:/usr/bin:/sbin:/bin')

    def test_dispatch_rejects_oversized_input_and_extra_arguments(self):
        import subprocess,sys
        helper=Path(__file__).with_name('bridge.py')
        for args,payload in [(['create'],' '*65537),(['create','unexpected'],'{}')]:
            result=subprocess.run([sys.executable,'-I',str(helper),*args],input=payload,text=True,capture_output=True,check=True)
            self.assertIn('error',json.loads(result.stdout))

    def test_check_is_read_only_and_warns_about_bridge_filtering(self):
        commands=[]
        def read_command(*args):
            commands.append(args)
            responses={('nmcli','-g','RUNNING','general'):'running',('nmcli','--version'):'nmcli tool, version 1.46.0',('ip','-Version'):'ip utility',('iptables','-S','FORWARD'):'-P FORWARD DROP'}
            return responses[args]
        host=bridge.Host();host.run=read_command
        with patch('pathlib.Path.read_text',return_value='1\n'):
            result=host.execute('check',{})
        self.assertNotIn('error',result)
        self.assertTrue(any(c['name']=='firewall' and c['status']=='WARN' for c in result['checks']))
        self.assertFalse(any(c['status']=='FAIL' for c in result['checks']))
        self.assertTrue(all('add' not in c and 'delete' not in c for c in commands))
