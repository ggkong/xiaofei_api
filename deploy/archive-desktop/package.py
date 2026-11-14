#!/usr/bin/env python3
"""Package standalone binaries with a public, pinned server profile."""
import argparse
import hashlib
import json
from pathlib import Path
import zipfile

parser = argparse.ArgumentParser()
parser.add_argument('--binaries', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
parser.add_argument('--host', required=True)
parser.add_argument('--host-key', required=True)
parser.add_argument('--web-url', required=True)
args = parser.parse_args()
args.output.mkdir(parents=True, exist_ok=True)
root = Path(__file__).resolve().parent
checksums = []
for arch, label in [('amd64', 'x64'), ('arm64', 'ARM64')]:
    name = 'Sub2API-Archive-Windows-' + label + '.zip'
    path = args.output / name
    with zipfile.ZipFile(path, 'w', zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
        archive.write(args.binaries / ('archive-desktop-windows-' + arch + '.exe'), 'archive-desktop.exe')
        for file in ['Install.cmd', 'Install.ps1', 'Uninstall.ps1', 'README.md']:
            archive.write(root / file, file)
        archive.writestr('server.json', json.dumps({'host': args.host, 'host_key': args.host_key, 'web_url': args.web_url}, indent=2))
    checksums.append(hashlib.sha256(path.read_bytes()).hexdigest() + '  ' + name)
(args.output / 'SHA256SUMS.txt').write_text('\n'.join(checksums) + '\n')
print('\n'.join(checksums))
