"""Publish built release assets as a public GitHub release.

Uses the GitHub CLI or the Git credential manager already present on the
machine. The repository is public, so the release notes and the assets are the
only things this script touches; it never rewrites the source README.
"""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess

root=Path(__file__).resolve().parent.parent
parser=argparse.ArgumentParser()
parser.add_argument('--version',required=True)
parser.add_argument('--gh',default='gh')
parser.add_argument('--repo',default='OffroadOps/mirroxy',help='GitHub repository in owner/name form')
parser.add_argument('--publish',action='store_true',help='Publish the already uploaded draft')
parser.add_argument('--proxy',default='')
args=parser.parse_args()
repo=args.repo
version=args.version
if not all(part.isdigit() for part in version.split('.')):raise SystemExit('Invalid version')
tag='v'+version
dist=root/'dist'/tag
env=dict(os.environ)
if not env.get('GH_TOKEN'):
    result=subprocess.run(['git','-c','credential.interactive=false','credential','fill'],
                          input='protocol=https\nhost=github.com\n\n',text=True,capture_output=True,check=True)
    credentials=dict(line.split('=',1) for line in result.stdout.splitlines() if '=' in line)
    if not credentials.get('password'): raise SystemExit('Sign into GitHub CLI or Git Credential Manager first.')
    env['GH_TOKEN']=credentials['password']
if args.proxy:env['HTTPS_PROXY']=args.proxy
def gh(*parts):
    subprocess.run([args.gh,*parts,'--repo',repo],env=env,check=True)
allowed=['Mirroxy-linux-arm64.run','Mirroxy-linux-amd64.run',
         'Mirroxy-linux-arm64.tar.gz','Mirroxy-linux-amd64.tar.gz',
         'Mirroxy-macos-arm64.zip',
         'install.sh','README.txt','SHA256SUMS','Mirroxy-update-windows-amd64.zip',
         'Mirroxy-update-linux-arm64.zip','Mirroxy-update-linux-amd64.zip','Mirroxy-update-macos-arm64.zip']
allowed += [name for name in ['Mirroxy-windows-amd64.zip','Mirroxy-windows-amd64.exe'] if (dist/name).is_file()]
if args.publish:
    releases=json.loads(subprocess.check_output([args.gh,'api',f'repos/{repo}/releases?per_page=100'],env=env))
    release=next((r for r in releases if r['tag_name']==tag),None)
    if release is None:raise SystemExit('Release draft not found')
    assets={a['name']:a for a in release['assets']}
    if set(assets)!=set(allowed):raise SystemExit('Release asset list mismatch')
    for name in allowed:
        if assets[name].get('digest')!='sha256:'+hashlib.sha256((dist/name).read_bytes()).hexdigest():raise SystemExit('Uploaded checksum mismatch: '+name)
    gh('release','edit',tag,'--draft=false')
    # Keep the stable one-line installer in the repository so the raw
    # GitHub URL keeps working. The source README is never touched.
    endpoint=f'repos/{repo}/contents/install.sh'
    data=(dist/'install.sh').read_bytes()
    try:
        previous=json.loads(subprocess.check_output([args.gh,'api',endpoint],env=env))
        if base64.b64decode(previous['content'])==data:
            previous=None
    except subprocess.CalledProcessError:
        previous=None
    if previous is None:
        body={'message':f'Update the stable Linux installer for {tag}','content':base64.b64encode(data).decode()}
        subprocess.run([args.gh,'api','-X','PUT',endpoint,'--input','-'],input=json.dumps(body).encode(),env=env,check=True,stdout=subprocess.DEVNULL)
    else:
        body={'message':f'Update the stable Linux installer for {tag}','content':base64.b64encode(data).decode(),'sha':previous['sha']}
        subprocess.run([args.gh,'api','-X','PUT',endpoint,'--input','-'],input=json.dumps(body).encode(),env=env,check=True,stdout=subprocess.DEVNULL)
    print('Published release: https://github.com/'+repo+'/releases/tag/'+tag)
else:
    for line in (dist/'SHA256SUMS').read_text(encoding='utf-8').splitlines():
        digest,name=line.split('  ',1)
        if name not in allowed:raise SystemExit('Unexpected release file: '+name)
        if hashlib.sha256((dist/name).read_bytes()).hexdigest()!=digest:raise SystemExit('Checksum mismatch: '+name)
    notes=root/f'work/release-notes-{tag}.md'
    site=f'https://github.com/{repo}/releases/tag/{tag}'
    notes.write_text((dist/'README.txt').read_text(encoding='utf-8')+'\n\n源码与构建说明：['+repo+'](https://github.com/'+repo+')。\n\n本版本下载与校验：['+site+']('+site+')，下载后可用 `SHA256SUMS` 校验。\n',encoding='utf-8')
    existing=subprocess.run([args.gh,'release','view',tag,'--repo',repo,'--json','isDraft'],env=env,capture_output=True)
    if existing.returncode:
        gh('release','create',tag,'--draft','--title',f'Mirroxy {version} - Windows / Linux / macOS','--notes-file',str(notes))
    elif not json.loads(existing.stdout)['isDraft']:
        raise SystemExit('Release is already published; refusing to replace assets.')
    for name in allowed:
        print('Uploading '+name,flush=True)
        gh('release','upload',tag,str(dist/name),'--clobber')
