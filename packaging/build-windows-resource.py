"""Compile the application icon; the architecture suffix scopes it to Windows."""
import argparse
from pathlib import Path
import shutil
import subprocess

ROOT=Path(__file__).resolve().parent.parent
if __name__=='__main__':
 parser=argparse.ArgumentParser()
 parser.add_argument('--windres',default=shutil.which('windres'))
 args=parser.parse_args()
 output=ROOT/'resource_windows_amd64.syso'
 if not args.windres:
  if output.is_file():
   print('Using bundled Windows icon resource; use windres to regenerate after changing app.ico')
  else:raise SystemExit('Install MinGW windres to build the Windows application icon')
 else:
  subprocess.run([args.windres,'--target=pe-x86-64','-i','packaging/windows.rc','-O','coff','-o',str(output)],cwd=ROOT,check=True)
