"""Package the same reviewed helper and installer used by the source checkout."""
import pathlib
import sys
import tarfile

root = pathlib.Path(__file__).resolve().parent.parent
output = pathlib.Path(sys.argv[1])
output.parent.mkdir(parents=True, exist_ok=True)
files = {
    'nanofaas-multipass-network': ('internal/hostnetwork/scripts/bridge.py', 0o755),
    'install-host-helper.sh': ('scripts/install-host-helper.sh', 0o755),
    'README.md': ('docs/guides/installation.md', 0o644),
    'LICENSE': ('LICENSE', 0o644),
}
with tarfile.open(output, 'w:gz') as archive:
    for name, (source, mode) in files.items():
        with (root / source).open('rb') as data:
            info = archive.gettarinfo(data.name, arcname=name)
            info.uid = info.gid = 0
            info.uname = info.gname = 'root'
            info.mode = mode
            archive.addfile(info, data)
