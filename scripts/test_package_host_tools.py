import pathlib,subprocess,sys,tarfile,tempfile,unittest

class HostToolsPackageTests(unittest.TestCase):
    def test_archive_is_complete_and_has_safe_paths(self):
        root=pathlib.Path(__file__).resolve().parent.parent
        with tempfile.TemporaryDirectory() as tmp:
            archive=pathlib.Path(tmp)/'host-tools.tar.gz'
            subprocess.run([sys.executable,str(root/'scripts/package-host-tools.py'),str(archive)],check=True)
            with tarfile.open(archive) as tar:
                self.assertEqual(set(tar.getnames()),{'nanofaas-multipass-network','install-host-helper.sh','README.md','LICENSE'})
                for member in tar:
                    self.assertTrue(member.isfile())
                    self.assertEqual(member.uid,0)
                    self.assertFalse(member.mode & 0o022)
                self.assertTrue(tar.extractfile('nanofaas-multipass-network').read().startswith(b'#!/usr/bin/python3 -I\n'))
                self.assertEqual(tar.getmember('nanofaas-multipass-network').mode,0o755)
