"""Compile the stable bootstrap once, separately from application releases."""
import argparse
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile

ROOT=Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser()
parser.add_argument('--go',default='go')
parser.add_argument('--iscc',type=Path,default=Path(r'D:\DevTools\Inno Setup 7\ISCC.exe'))
args=parser.parse_args()
output=ROOT/'dist/bootstrap'
output.mkdir(parents=True,exist_ok=True)
destination=output/'Mirroxy-online-setup.exe'
if destination.exists():
    raise SystemExit('Stable installer already exists. Reuse it; do not recompile for application releases.')
env={**os.environ,'GOOS':'windows','GOARCH':'amd64','CGO_ENABLED':'0','GOTOOLCHAIN':'local'}
with tempfile.TemporaryDirectory(prefix='online-installer-',dir=ROOT/'work') as temporary:
    stage=Path(temporary)
    subprocess.run([args.go,'build','-trimpath','-ldflags=-s -w','-o',str(stage/'Mirroxy-bootstrap.exe'),'./cmd/bootstrap'],cwd=ROOT,env=env,check=True)
    subprocess.run([str(args.iscc),'--quiet-progress','--define=OnlineBootstrap=1','--define=MyAppVersion=1.0.0',f'--define=SourceDir={stage}',f'--define=OutputDir={output}','--define=OutputName=Mirroxy-online-setup',str(ROOT/'packaging/Mirroxy.iss')],cwd=ROOT,check=True)
digest=hashlib.sha256(destination.read_bytes()).hexdigest()
(output/'SHA256SUMS').write_text(digest+'  '+destination.name+'\n',encoding='ascii')
print(f'{destination}: {destination.stat().st_size/1024/1024:.1f} MiB; SHA256 {digest}')
