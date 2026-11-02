import hashlib
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest
import zipfile


class ReleaseVerificationTests(unittest.TestCase):
    def test_complete_release_and_corrupt_checksum(self):
        root = pathlib.Path(__file__).resolve().parent.parent
        with tempfile.TemporaryDirectory() as tmp:
            folder = pathlib.Path(tmp)
            prefix = 'terraform-provider-multipass_0.1.0'
            for platform in ('linux_amd64', 'linux_arm64', 'darwin_amd64', 'darwin_arm64', 'windows_amd64', 'windows_arm64'):
                suffix = '.exe' if platform.startswith('windows') else ''
                with zipfile.ZipFile(folder / (prefix + '_' + platform + '.zip'), 'w') as archive:
                    archive.writestr('terraform-provider-multipass_v0.1.0' + suffix, b'provider')
                    archive.writestr('multipass-doctor' + suffix, b'doctor')
            (folder / (prefix + '_manifest.json')).write_text(json.dumps({'metadata': {'protocol_versions': ['6.0']}}))
            (folder / 'nanofaas-multipass-signing-key.asc').write_bytes(b'public key fixture')
            subprocess.run([sys.executable, str(root / 'scripts/package-host-tools.py'), str(folder / (prefix + '_host-tools.tar.gz'))], check=True)
            sums = '\n'.join(hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name for p in sorted(folder.iterdir()))
            (folder / (prefix + '_SHA256SUMS')).write_text(sums + '\n')
            command = [sys.executable, str(root / 'scripts/verify-release.py'), str(folder)]
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            (folder / 'nanofaas-multipass-signing-key.asc').write_bytes(b'corrupted')
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('checksum mismatch', result.stderr)
