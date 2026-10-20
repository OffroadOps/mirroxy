"""Upload only built release assets. Uses the existing Git credential manager."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess

root=Path(__file__).resolve().parent.parent
repo='OffroadOps/scrcpyproxy'
parser=argparse.ArgumentParser()
parser.add_argument('--version',required=True)
parser.add_argument('--gh',default='gh')
parser.add_argument('--publish',action='store_true',help='Publish the already uploaded draft')
parser.add_argument('--proxy',default='')
args=parser.parse_args()
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
    report=json.loads((root/'work'/('s3-release-'+tag+'.json')).read_text(encoding='utf-8'))
    if report.get('version')!=version or not report.get('anonymousDownloadsVerified') or not report.get('pageURL'):
        raise SystemExit('Publish and verify S3 packages and release page first')
    repo_meta=json.loads(subprocess.check_output([args.gh,'api',f'repos/{repo}'],env=env))
    if not repo_meta.get('private'):raise SystemExit('Release repository must remain private')
    releases=json.loads(subprocess.check_output([args.gh,'api',f'repos/{repo}/releases?per_page=100'],env=env))
    release=next((r for r in releases if r['tag_name']==tag),None)
    if release is None:raise SystemExit('Release draft not found')
    assets={a['name']:a for a in release['assets']}
    if set(assets)!=set(allowed):raise SystemExit('Release asset list mismatch')
    for name in allowed:
        if assets[name].get('digest')!='sha256:'+hashlib.sha256((dist/name).read_bytes()).hexdigest():raise SystemExit('Uploaded checksum mismatch: '+name)
    gh('release','edit',tag,'--draft=false')
    for name,data in [('README.md',(root/'packaging/PUBLIC-README.md').read_bytes()),('install.sh',(dist/'install.sh').read_bytes())]:
        endpoint=f'repos/{repo}/contents/{name}'
        previous=json.loads(subprocess.check_output([args.gh,'api',endpoint],env=env))
        if base64.b64decode(previous['content'])==data:
            continue
        body={'message':f'Update binary distribution documentation for {tag}', 'content':base64.b64encode(data).decode(), 'sha':previous['sha']}
        subprocess.run([args.gh,'api','-X','PUT',endpoint,'--input','-'],input=json.dumps(body).encode(),env=env,check=True,stdout=subprocess.DEVNULL)
    print('Published verified release: https://github.com/'+repo+'/releases/tag/'+tag)
else:
    for line in (dist/'SHA256SUMS').read_text(encoding='utf-8').splitlines():
        digest,name=line.split('  ',1)
        if name not in allowed:raise SystemExit('Unexpected release file: '+name)
        if hashlib.sha256((dist/name).read_bytes()).hexdigest()!=digest:raise SystemExit('Checksum mismatch: '+name)
    notes=root/f'work/release-notes-{tag}.md'
    site='https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/index.html'
    notes.write_text((dist/'README.txt').read_text(encoding='utf-8')+'\n\n公开下载、历史版本与安装命令：['+site+']('+site+')。GitHub 仓库保持私有。\n',encoding='utf-8')
    existing=subprocess.run([args.gh,'release','view',tag,'--repo',repo,'--json','isDraft'],env=env,capture_output=True)
    if existing.returncode:
        gh('release','create',tag,'--draft','--title',f'Mirroxy {version} - Windows / Linux / macOS','--notes-file',str(notes))
    elif not json.loads(existing.stdout)['isDraft']:
        raise SystemExit('Release is already published; refusing to replace assets.')
    for name in allowed:
        print('Uploading '+name,flush=True)
        gh('release','upload',tag,str(dist/name),'--clobber')
