"""Validate archive contents and every checksum; optionally stage GoReleaser extras."""
import hashlib
import json
import pathlib
import shutil
import sys
import tarfile
import zipfile

folder = pathlib.Path(sys.argv[1])
stage = sys.argv[2:] == ['--stage']
assert sys.argv[2:] in ([], ['--stage']), 'unsupported arguments'
sums = list(folder.glob('*_SHA256SUMS'))
assert len(sums) == 1, 'expected exactly one checksum manifest'
entries = {}
for line in sums[0].read_text().splitlines():
    digest, name = line.split(maxsplit=1)
    name = name.lstrip('*')
    assert pathlib.Path(name).name == name, 'unsafe release path'
    assert name not in entries, 'duplicate release asset'
    target = folder / name
    if stage and not target.exists():
        root = pathlib.Path(__file__).resolve().parent.parent
        if name.endswith('_manifest.json'):
            source = root / 'terraform-registry-manifest.json'
        elif name.endswith('_host-tools.tar.gz'):
            source = root / 'build/release/host-tools.tar.gz'
        elif name == 'nanofaas-multipass-signing-key.asc':
            source = root / 'packaging/public-key.asc'
        else:
            raise AssertionError('missing release artifact: ' + name)
        shutil.copyfile(source, target)
    assert hashlib.sha256(target.read_bytes()).hexdigest() == digest, 'checksum mismatch: ' + name
    entries[name] = target
archives = {name: p for name,p in entries.items() if name.endswith('.zip')}
assert len(archives) == 6, 'expected six platform archives'
for platform in ('linux_amd64','linux_arm64','darwin_amd64','darwin_arm64','windows_amd64','windows_arm64'):
    matches = [p for name,p in archives.items() if name.endswith('_'+platform+'.zip')]
    assert len(matches) == 1, platform
    with zipfile.ZipFile(matches[0]) as archive:
        names = archive.namelist()
        suffix = '.exe' if platform.startswith('windows') else ''
        assert 'multipass-doctor'+suffix in names, 'missing doctor'
        assert sum(n.startswith('terraform-provider-multipass_v') and n.endswith(suffix) for n in names) == 1, 'missing provider'
        assert all(pathlib.PurePosixPath(n).name == n for n in names), 'unexpected archive paths'
manifest = next(p for n,p in entries.items() if n.endswith('_manifest.json'))
assert json.loads(manifest.read_text())['metadata']['protocol_versions'] == ['6.0']
host = next(p for n,p in entries.items() if n.endswith('_host-tools.tar.gz'))
with tarfile.open(host) as archive:
    assert set(archive.getnames()) == {'nanofaas-multipass-network','install-host-helper.sh','README.md','LICENSE'}
    assert all(m.isfile() for m in archive), 'host tools must contain regular files only'
assert 'nanofaas-multipass-signing-key.asc' in entries
print('Verified checksums, six provider/doctor archives, host tools and registry manifest')
