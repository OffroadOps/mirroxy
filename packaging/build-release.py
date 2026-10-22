"""Build binary-only releases; optionally build a Windows offline installer."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parent.parent
ASSETS = ROOT / 'work/release-assets'
VERSION = '0.8.0'
DIST = ROOT / 'dist' / f'v{VERSION}'
RELEASE_URL = f'https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/v{VERSION}'

def sha(data):
    return hashlib.sha256(data).hexdigest()

def text(path):
    return Path(path).read_text(encoding='utf-8').replace('\r\n', '\n').encode()

def official_asset(release, name):
    meta = next(a for a in release['assets'] if a['name'] == name)
    payload = (ASSETS / name).read_bytes()
    if meta['digest'] != 'sha256:' + sha(payload):
        raise ValueError('Official checksum mismatch: ' + name)
    return payload, {'version': release['tag_name'], 'asset': name,
                     'url': meta['browser_download_url'], 'sha256': sha(payload)}

def member(payload, name):
    if payload[:2] == b'PK':
        with zipfile.ZipFile(io.BytesIO(payload)) as archive:
            return archive.read(next(n for n in archive.namelist() if n.split('/')[-1] == name))
    with tarfile.open(fileobj=io.BytesIO(payload), mode='r:gz') as archive:
        return archive.extractfile(next(m for m in archive if m.isfile() and m.name.split('/')[-1] == name)).read()

def cached_url(name, url):
    path = ASSETS / name
    if not path.exists():
        with urllib.request.urlopen(url, timeout=90) as response:
            path.write_bytes(response.read())
    return path.read_bytes()

ARIA2_RELEASE = 'https://github.com/aria2/aria2/releases/download/release-1.37.0/aria2-1.37.0-win-64bit-build1.zip'
ARIA2_SHA256 = '67d015301eef0b612191212d564c5bb0a14b5b9c4796b76454276a4d28d9b288'

def aria2_archive(local=None):
    """Return the pinned official aria2 archive, downloading it when needed."""
    if local is not None:
        payload = Path(local).read_bytes()
    else:
        payload = cached_url('aria2-1.37.0-win-64bit-build1.zip', ARIA2_RELEASE)
    if sha(payload) != ARIA2_SHA256:
        raise ValueError('aria2 1.37.0 official archive differs from the pinned release')
    return payload

def build(arch, go, rules, goos='linux', iscc=None):
    env = {**os.environ, 'GOOS': goos, 'GOARCH': arch, 'CGO_ENABLED': '0', 'GOTOOLCHAIN': 'local'}
    binary = ROOT / 'work' / f'release-Mirroxy-{goos}-{arch}'
    flags = '-s -w' + (' -H=windowsgui' if goos == 'windows' else '')
    subprocess.run([go, 'build', '-trimpath', '-ldflags='+flags, '-o', str(binary), '.'], cwd=ROOT, env=env, check=True)
    upgrade = None
    if goos == 'windows':
        upgrade = ROOT/'work'/'release-upgrade-windows-amd64.exe'
        subprocess.run([go,'build','-trimpath','-ldflags='+flags,'-o',str(upgrade),'./cmd/upgrade'],cwd=ROOT,env=env,check=True)
    xmeta = json.loads((ASSETS/'xray-release.json').read_text(encoding='utf-8'))
    smeta = json.loads((ASSETS/'sing-box-release.json').read_text(encoding='utf-8'))
    xos = 'macos' if goos == 'darwin' else goos
    xname = f"Xray-{xos}-{'arm64-v8a' if arch == 'arm64' else '64'}.zip"
    sext = '.zip' if goos == 'windows' else '-musl.tar.gz' if goos == 'linux' else '.tar.gz'
    sname = f"sing-box-{smeta['tag_name'].removeprefix('v')}-{goos}-{arch}{sext}"
    xray, xm = official_asset(xmeta, xname)
    singbox, sm = official_asset(smeta, sname)
    files = {}
    def add(name, data, mode=0o644):
        files[name] = (data, mode)
    exe = '.exe' if goos == 'windows' else ''
    add('Mirroxy'+exe, binary.read_bytes(), 0o755)
    if upgrade: add('upgrade.exe',upgrade.read_bytes(),0o755)
    add('bin/runtime/Mirroxy_core_A'+exe, member(xray, 'xray'+exe), 0o755)
    add('bin/runtime/Mirroxy_core_B'+exe, member(singbox, 'sing-box'+exe), 0o755)
    if goos == 'windows':
        # Pin the exact unmodified official Windows release downloaded over HTTPS.
        # Upstream's older GitHub release has no asset digest field.
        aria = aria2_archive(args.aria2_zip)
        add('bin/runtime/Mirroxy_downloader.exe', member(aria, 'aria2c.exe'), 0o755)
        add('licenses/ARIA2-COPYING', member(aria, 'COPYING'))
        add('licenses/ARIA2-OpenSSL-LICENSE', member(aria, 'LICENSE.OpenSSL'))
        add('licenses/ARIA2-NOTICE.txt', b'aria2 1.37.0 is an unmodified, separate executable distributed under GPL-2.0-or-later.\nOfficial release: https://github.com/aria2/aria2/releases/tag/release-1.37.0\nCorresponding upstream source: https://github.com/aria2/aria2/releases/download/release-1.37.0/aria2-1.37.0.tar.xz\nBuild information: https://github.com/aria2/aria2/tree/release-1.37.0\nMirroxy communicates with aria2 via its private JSON-RPC interface.\n')
    rule_names = ['geoip.dat', 'geosite.dat'] + ['srss/'+name+'.srs' for name in ['geoip-cn','geosite-cn','geosite-private','geosite-gfw','geosite-greatfire','geosite-google','geoip-facebook','geoip-fastly','geoip-google','geoip-netflix','geoip-telegram','geoip-twitter']]
    generic={'geoip.dat':'Mirroxy_route_ip.bin','geosite.dat':'Mirroxy_route_domain.bin'}
    for index,name in enumerate(rule_names[2:],1): generic[name]=f'Mirroxy_set_{index:02d}.bin'
    for name in rule_names:
        source = rules/name
        if not source.is_file():
            source = rules/generic[name]
        if not source.is_file():
            raise FileNotFoundError(f'Missing routing asset {name} (or {generic[name]}) under {rules}')
        add('bin/resources/' + generic[name], source.read_bytes())
    add('bin/resources/Mirroxy_trust.pem', (ASSETS/'cacert.pem').read_bytes())
    scripts = ['install-local.sh', 'mirroxyctl', 'mirroxy.service'] if goos == 'linux' else ['install-macos.sh','install.command'] if goos == 'darwin' else []
    for name in scripts:
        add(name, text(ROOT/'packaging'/name), 0o644 if name.endswith('.service') else 0o755)
    add('PLATFORM', f'{goos}-{arch}\n'.encode())
    add('README.txt', text(ROOT/f'packaging/{xos.upper()}-README.txt'))
    add('licenses/COMPONENT-A-LICENSE', cached_url('Xray-LICENSE', f"https://raw.githubusercontent.com/XTLS/Xray-core/{xmeta['tag_name']}/LICENSE"))
    add('licenses/COMPONENT-B-LICENSE', cached_url('sing-box-LICENSE', f"https://raw.githubusercontent.com/SagerNet/sing-box/{smeta['tag_name']}/LICENSE"))
    add('licenses/NOTICE.txt', (
        'Mirroxy is distributed as a binary-only application. Its source code is not part of this release.\n'
        'Xray and sing-box are unmodified, independent upstream executables.\n'
        f"Xray source and release: https://github.com/XTLS/Xray-core/tree/{xmeta['tag_name']}\n"
        f"sing-box source and release: https://github.com/SagerNet/sing-box/tree/{smeta['tag_name']}\n"
        'Core licenses are included in this directory; their terms remain applicable.\n'
        'Routing files are a snapshot of the existing v2rayN-compatible local data files.\n'
        'GeoIP/Geosite sources: https://github.com/Loyalsoldier/v2ray-rules-dat ; https://github.com/SagerNet/sing-geoip ; https://github.com/SagerNet/sing-geosite\n'
        'CA certificates: https://curl.se/docs/caextract.html (Mozilla CA bundle).\n'
    ).encode())
    manifest = {'application': 'Mirroxy', 'version': VERSION, 'platform': f'{goos}-{arch}',
                'components': {'A': xm, 'B': sm}, 'routing': {'source': 'local immutable rule snapshot',
                'files': list(generic.values())}, 'files': {n: sha(v[0]) for n, v in sorted(files.items())}}
    if goos == 'darwin': manifest['minimumOS'] = 'macOS 26 (required by bundled upstream sing-box)'
    add('manifest.json', (json.dumps(manifest, ensure_ascii=False, indent=2)+'\n').encode())
    add('SHA256SUMS', ''.join(f'{sha(data)}  {name}\n' for name, (data, _) in sorted(files.items())).encode())
    # Older app-update validators forbid every bin/runtime entry. Keep online
    # updates compatible; the new UI can explicitly install a missing aria2.
    update_files = {name: value for name, value in files.items() if name == 'Mirroxy'+exe or name == 'upgrade.exe' or name.startswith('bin/resources/Mirroxy_set_') or name.startswith('licenses/')}
    update_manifest = {'application':'Mirroxy','kind':'app-update','version':VERSION,'platform':f'{goos}-{arch}',
                       'files':{name:sha(value[0]) for name,value in update_files.items()}}
    update_files['manifest.json'] = ((json.dumps(update_manifest,indent=2)+'\n').encode(),0o644)
    update_output = DIST/f'Mirroxy-update-{xos}-{arch}.zip'
    with zipfile.ZipFile(update_output,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=6) as archive:
        for name,(data,mode) in sorted(update_files.items()):
            info=zipfile.ZipInfo(name);info.create_system=3;info.external_attr=(0o100000|mode)<<16;info.compress_type=zipfile.ZIP_DEFLATED
            archive.writestr(info,data)
    print(f'{update_output.name}: {update_output.stat().st_size/1024/1024:.1f} MiB',flush=True)
    if goos == 'windows':
        output = DIST/'Mirroxy-windows-amd64.zip'
        with zipfile.ZipFile(output,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=6) as archive:
            for name,(data,mode) in sorted(files.items()):
                info=zipfile.ZipInfo(name);info.create_system=3;info.external_attr=(0o100000|mode)<<16;info.compress_type=zipfile.ZIP_DEFLATED
                archive.writestr(info,data)
        checksum=sha(output.read_bytes())
        print(f'{output.name}: {output.stat().st_size/1024/1024:.1f} MiB, SHA256 {checksum}',flush=True)
        if not args.offline_windows:
            return checksum
        if not iscc:
            raise ValueError('Inno Setup 7 ISCC.exe is required for the Windows release')
        for name in files:
            if Path(name).parts and Path(name).parts[0].lower() == 'data':
                raise ValueError('Windows installer staging must never contain user data')
        with tempfile.TemporaryDirectory(prefix='mirroxy-inno-', dir=ROOT/'work') as temporary:
            stage = Path(temporary)
            for name, (data, mode) in files.items():
                destination = stage / Path(name)
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(data)
                destination.chmod(mode)
            output = DIST/'Mirroxy-windows-amd64.exe'
            subprocess.run([str(iscc), '--quiet-progress', f'--define=MyAppVersion={VERSION}',
                f'--define=SourceDir={stage}', f'--define=OutputDir={DIST}',
                '--define=OutputName=Mirroxy-windows-amd64', str(ROOT/'packaging/Mirroxy.iss')],
                cwd=ROOT, check=True)
        checksum = sha(output.read_bytes())
        print(f'{output.name}: {output.stat().st_size/1024/1024:.1f} MiB, SHA256 {checksum}', flush=True)
        return checksum
    if goos != 'linux':
        output = DIST / f'Mirroxy-{xos}-{arch}.zip'
        with zipfile.ZipFile(output,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=6) as archive:
            for name,(data,mode) in sorted(files.items()):
                info=zipfile.ZipInfo(f'Mirroxy-{xos}-{arch}/'+name)
                info.create_system=3
                info.external_attr=(0o100000|mode)<<16
                info.compress_type=zipfile.ZIP_DEFLATED
                archive.writestr(info,data)
        checksum=sha(output.read_bytes())
        print(f'{output.name}: {output.stat().st_size/1024/1024:.1f} MiB, SHA256 {checksum}',flush=True)
        return checksum
    output = DIST / f'Mirroxy-linux-{arch}.tar.gz'
    with output.open('wb') as raw, gzip.GzipFile(fileobj=raw, mode='wb', mtime=0, compresslevel=6) as gz:
        with tarfile.open(fileobj=gz, mode='w', format=tarfile.PAX_FORMAT) as archive:
            for name, (data, mode) in sorted(files.items()):
                info = tarfile.TarInfo(name)
                info.size, info.mode = len(data), mode
                info.uid = info.gid = info.mtime = 0
                info.uname = info.gname = 'root'
                archive.addfile(info, io.BytesIO(data))
    payload = output.read_bytes()
    checksum = sha(payload)
    header = f'''#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $(id -u) == 0 ]] || {{ echo '请使用 sudo bash 运行安装包' >&2; exit 1; }}
temp_dir=$(mktemp -d /tmp/Mirroxy-bundle.XXXXXX)
cleanup() {{ case "$temp_dir" in /tmp/Mirroxy-bundle.*) rm -rf -- "$temp_dir" ;; esac; }}
trap cleanup EXIT
line=$(awk '/^__MIRROXY_ARCHIVE_BELOW__$/ {{print NR + 1; exit}}' "$0")
tail -n +"$line" "$0" > "$temp_dir/payload.tar.gz"
echo '{checksum}  '"$temp_dir/payload.tar.gz" | sha256sum -c - >/dev/null
mkdir "$temp_dir/package"
tar -xzf "$temp_dir/payload.tar.gz" -C "$temp_dir/package" --no-same-owner
bash "$temp_dir/package/install-local.sh" "$@"
exit 0
__MIRROXY_ARCHIVE_BELOW__
'''.encode()
    (DIST/f'Mirroxy-linux-{arch}.run').write_bytes(header+payload)
    print(f'{output.name}: {len(payload)/1024/1024:.1f} MiB, SHA256 {checksum}', flush=True)
    return checksum

def bootstrap(checksums):
    script = '''#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $(id -u) == 0 ]] || { echo '请使用 sudo bash 运行安装脚本' >&2; exit 1; }
[[ $(uname -s) == Linux ]] || { echo '仅支持 Linux' >&2; exit 1; }
case "$(uname -m)" in
  aarch64|arm64) arch=arm64; expected=ARM64_CHECKSUM ;;
  x86_64|amd64) arch=amd64; expected=AMD64_CHECKSUM ;;
  *) echo '仅支持 ARM64 / AMD64' >&2; exit 1 ;;
esac
base=${MIRROXY_DOWNLOAD_BASE:-RELEASE_URL}
temp_dir=$(mktemp -d /tmp/Mirroxy-download.XXXXXX)
cleanup() { case "$temp_dir" in /tmp/Mirroxy-download.*) rm -rf -- "$temp_dir" ;; esac; }
trap cleanup EXIT
echo "正在下载 Mirroxy linux-$arch（包含两个内核和路由数据）…"
if ! curl --fail --location --retry 5 --retry-delay 3 --connect-timeout 15 --max-time 900 "$base/Mirroxy-linux-$arch.tar.gz" -o "$temp_dir/package.tar.gz"; then
  echo '下载失败。可下载对应架构的 .run 文件，复制到设备离线安装；也可设置 MIRROXY_DOWNLOAD_BASE 使用镜像。' >&2
  exit 1
fi
echo "$expected  $temp_dir/package.tar.gz" | sha256sum -c -
mkdir "$temp_dir/package"
tar -xzf "$temp_dir/package.tar.gz" -C "$temp_dir/package" --no-same-owner
bash "$temp_dir/package/install-local.sh" "$@"
'''
    script = script.replace('ARM64_CHECKSUM', checksums['arm64']).replace('AMD64_CHECKSUM', checksums['amd64']).replace('RELEASE_URL', RELEASE_URL)
    (DIST/'install.sh').write_text(script, encoding='utf-8', newline='\n')

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--go', default='go')
    parser.add_argument('--offline-windows', action='store_true', help='Also compile the optional versioned offline Inno installer (not needed by the fixed online installer)')
    parser.add_argument('--aria2-zip', type=Path, default=None,
                        help='Official aria2 1.37.0 Windows x64 zip; downloaded from the pinned upstream release when omitted')
    parser.add_argument('--rules', type=Path, default=ROOT/'bin/resources',
                        help='Routing assets; defaults to Mirroxy/bin/resources and also accepts legacy geoip.dat/geosite.dat/srss layout')
    parser.add_argument('--iscc', type=Path, default=None,
                        help='Path to ISCC.exe; auto-detected from common Inno Setup 7 locations when omitted')
    args = parser.parse_args()
    if args.iscc is None:
        candidates = [Path(r'D:\Program Files\Inno Setup 7\ISCC.exe'),
                      Path(r'C:\Program Files\Inno Setup 7\ISCC.exe'),
                      Path(r'C:\Program Files (x86)\Inno Setup 7\ISCC.exe')]
        if os.environ.get('LOCALAPPDATA'):
            candidates.append(Path(os.environ['LOCALAPPDATA']) / r'Programs\Inno Setup 7\ISCC.exe')
        args.iscc = next((path for path in candidates if path.is_file()), None)
    if args.offline_windows and (not args.iscc or not args.iscc.is_file()):
        raise SystemExit('Inno Setup 7 compiler not found; pass --iscc with the ISCC.exe path')
    print(f"ISCC: {args.iscc}", flush=True)
    subprocess.run([sys.executable,str(ROOT/'packaging/build-windows-resource.py')],check=True)
    DIST.mkdir(parents=True, exist_ok=True)
    hashes = {arch: build(arch, args.go, args.rules) for arch in ['arm64', 'amd64']}
    build('amd64',args.go,args.rules,'windows',args.iscc)
    build('arm64',args.go,args.rules,'darwin')
    bootstrap(hashes)
    (DIST/'README.txt').write_bytes(text(ROOT/'packaging/RELEASE-README.txt'))
    deliverables = sorted(p for p in DIST.iterdir() if p.is_file() and p.name != 'SHA256SUMS')
    (DIST/'SHA256SUMS').write_text(''.join(f'{sha(p.read_bytes())}  {p.name}\n' for p in deliverables), encoding='utf-8', newline='\n')
