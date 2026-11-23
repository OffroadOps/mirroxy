"""Compile the application icon; the architecture suffix scopes it to Windows."""
import argparse
from pathlib import Path
import shutil
import subprocess

ROOT=Path(__file__).resolve().parent.parent
TARGETS=[
 ('packaging/windows.rc', ROOT/'resource_windows_amd64.syso'),
 ('packaging/desktop-windows.rc', ROOT/'desktop'/'resource_windows_amd64.syso'),
]
if __name__=='__main__':
 parser=argparse.ArgumentParser()
 parser.add_argument('--windres',default=shutil.which('windres'))
 args=parser.parse_args()
 if not args.windres:
  if all(out.is_file() for _,out in TARGETS):
   print('Using bundled Windows icon resources; use windres to regenerate after changing app.ico')
  else:raise SystemExit('Install MinGW windres to build the Windows application icons')
 else:
  for rc,out in TARGETS:
   out.parent.mkdir(parents=True,exist_ok=True)
   subprocess.run([args.windres,'--target=pe-x86-64','-i',rc,'-O','coff','-o',str(out)],cwd=ROOT,check=True)
