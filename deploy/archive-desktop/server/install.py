#!/usr/bin/python3
"""Install only the restricted archive account and key refresh timer."""
import argparse
import json
import os
from pathlib import Path
import pwd
import re
import shutil
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument('--container', default='sub2api')
args = parser.parse_args()
if os.geteuid() != 0 or not re.fullmatch('[a-zA-Z0-9_.-]{1,100}', args.container):
    raise SystemExit('Run as root with a valid container name')
source = Path(__file__).resolve().parent
account = 'sub2api-archive'
try:
    existing = pwd.getpwnam(account)
    if existing.pw_dir != '/var/lib/sub2api-archive':
        raise SystemExit('Existing account has a different home; inspect before changing it')
except KeyError:
    subprocess.run(['useradd', '--system', '--home-dir', '/var/lib/sub2api-archive', '--no-create-home', '--shell', '/bin/sh', '--password', '*', account], check=True)
home = Path('/var/lib/sub2api-archive')
home.mkdir(exist_ok=True, mode=0o755)
os.chown(home, 0, 0)
os.chmod(home, 0o755)
config = Path('/etc/sub2api-archive.json')
config.write_text(json.dumps({'container': args.container}) + '\n')
os.chmod(config, 0o600)
libexec = Path('/usr/local/libexec')
libexec.mkdir(exist_ok=True)
for src, dest in [('archive-sync', 'sub2api-archive-sync'), ('archive-refresh', 'sub2api-archive-refresh')]:
    target = libexec / dest
    shutil.copy2(source / src, target)
    os.chown(target, 0, 0)
    os.chmod(target, 0o755)
sudoers = Path('/etc/sudoers.d/sub2api-archive')
sudoers.write_text(account + ' ALL=(root) NOPASSWD: /usr/local/libexec/sub2api-archive-sync *\n')
os.chmod(sudoers, 0o440)
subprocess.run(['/usr/sbin/visudo', '-cf', str(sudoers)], check=True)
ssh_config = Path('/etc/ssh/sshd_config.d/90-sub2api-archive.conf')
ssh_config.write_text('''Match User sub2api-archive
    AuthorizedKeysFile /var/lib/sub2api-archive/authorized_keys
    PasswordAuthentication no
    KbdInteractiveAuthentication no
    PermitTTY no
    DisableForwarding yes
    X11Forwarding no
    PermitUserRC no
Match all
''')
subprocess.run(['/usr/sbin/sshd', '-t'], check=True)
Path('/etc/systemd/system/sub2api-archive-keys.service').write_text('''[Unit]
Description=Refresh restricted Sub2API archive public keys
After=docker.service
[Service]
Type=oneshot
ExecStart=/usr/local/libexec/sub2api-archive-refresh
''')
Path('/etc/systemd/system/sub2api-archive-keys.timer').write_text('''[Unit]
Description=Refresh Sub2API archive access every 15 seconds
[Timer]
OnBootSec=20s
OnUnitActiveSec=15s
AccuracySec=1s
[Install]
WantedBy=timers.target
''')
subprocess.run(['/usr/local/libexec/sub2api-archive-refresh'], check=True)
subprocess.run(['systemctl', 'daemon-reload'], check=True)
subprocess.run(['systemctl', 'enable', '--now', 'sub2api-archive-keys.timer'], check=True)
subprocess.run(['systemctl', 'reload', 'ssh'], check=True)
print('Restricted archive transport installed; no shell, forwarding, or Docker group access was granted.')
