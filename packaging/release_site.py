"""Render the public, standalone S3 release page from published artifacts."""
import hashlib
import html
import json
import base64
from pathlib import Path
import re
from concurrent.futures import ThreadPoolExecutor
from urllib.parse import quote

ROOT = Path(__file__).resolve().parent.parent
FILES = {
    'Mirroxy-windows-amd64.exe': ('Windows', 'x86_64 / AMD64', '运行安装程序；升级自动保留 data 用户数据', '安装程序'),
    'Mirroxy-linux-arm64.run': ('Linux ARM64', 'Armbian · Debian · Ubuntu', 'sudo bash ./Mirroxy-linux-arm64.run', '离线安装包'),
    'Mirroxy-linux-amd64.run': ('Linux AMD64', 'Debian · Ubuntu · x86_64', 'sudo bash ./Mirroxy-linux-amd64.run', '离线安装包'),
    'Mirroxy-macos-arm64.zip': ('macOS', 'Apple Silicon · macOS 26+', '解压后运行 bash ./install-macos.sh', 'ZIP'),
    'Mirroxy-windows-amd64.zip': ('Windows 完整程序包', '在线安装器校验后安装', '', 'ZIP'),
    'Mirroxy-linux-arm64.tar.gz': ('Linux ARM64', '完整 TAR.GZ', '', 'TAR.GZ'),
    'Mirroxy-linux-amd64.tar.gz': ('Linux AMD64', '完整 TAR.GZ', '', 'TAR.GZ'),
    'install.sh': ('安装脚本', '', '', 'SH'),
    'README.txt': ('使用说明', '', '', 'TXT'),
    'SHA256SUMS': ('SHA-256 校验清单', '', '', 'TXT'),
    'Mirroxy-update-windows-amd64.zip': ('Windows 在线更新包', '', '', 'ZIP'),
    'Mirroxy-update-linux-arm64.zip': ('Linux ARM64 在线更新包', '', '', 'ZIP'),
    'Mirroxy-update-linux-amd64.zip': ('Linux AMD64 在线更新包', '', '', 'ZIP'),
    'Mirroxy-update-macos-arm64.zip': ('macOS ARM64 在线更新包', '', '', 'ZIP'),
	'scrcpyproxy-windows-amd64.zip': ('Windows（旧版）', 'x86_64 / AMD64', '', 'ZIP'),
	'scrcpyproxy-linux-arm64.run': ('Linux ARM64（旧版）', '', '', '离线安装包'),
	'scrcpyproxy-linux-amd64.run': ('Linux AMD64（旧版）', '', '', '离线安装包'),
	'scrcpyproxy-macos-arm64.zip': ('macOS（旧版）', '', '', 'ZIP'),
	'scrcpyproxy-linux-arm64.tar.gz': ('Linux ARM64（旧版）', '', '', 'TAR.GZ'),
	'scrcpyproxy-linux-amd64.tar.gz': ('Linux AMD64（旧版）', '', '', 'TAR.GZ'),
	'scrcpyproxy-update-windows-amd64.zip': ('Windows 在线更新包（旧版）', '', '', 'ZIP'),
	'scrcpyproxy-update-linux-arm64.zip': ('Linux ARM64 在线更新包（旧版）', '', '', 'ZIP'),
	'scrcpyproxy-update-linux-amd64.zip': ('Linux AMD64 在线更新包（旧版）', '', '', 'ZIP'),
	'scrcpyproxy-update-macos-arm64.zip': ('macOS ARM64 在线更新包（旧版）', '', '', 'ZIP'),
}
REQUIRED = {'Mirroxy-linux-arm64.run', 'Mirroxy-linux-amd64.run',
            'Mirroxy-linux-arm64.tar.gz', 'Mirroxy-linux-amd64.tar.gz',
            'install.sh', 'README.txt', 'SHA256SUMS'}
LEGACY_REQUIRED={'scrcpyproxy-linux-arm64.run','scrcpyproxy-linux-amd64.run','scrcpyproxy-linux-arm64.tar.gz','scrcpyproxy-linux-amd64.tar.gz','install.sh','README.txt','SHA256SUMS'}

def version_key(version):
    if not re.fullmatch(r'\d+\.\d+\.\d+', version):
        raise ValueError('Invalid release version')
    return tuple(int(part) for part in version.split('.'))

def size_label(size):
    return f'{size/1024/1024:.1f} MiB' if size >= 1024*1024 else f'{size/1024:.1f} KiB'

def image_data(name):
    payload = (ROOT/'packaging/site-assets'/name).read_bytes()
    return 'data:image/png;base64,'+base64.b64encode(payload).decode()

def collect_releases(client, bucket, base_url):
    """Only list committed versions whose complete checksum manifest exists.

    A version's SHA256SUMS is the commit marker: the publisher writes it after
    all payloads and the versioned installer have been verified anonymously.
    Older versions without the new metadata file remain discoverable.
    """
    changes = {entry['version']: entry for entry in json.loads((ROOT/'web/changelog.json').read_text(encoding='utf-8'))}
    prefixes = []
    for page in client.get_paginator('list_objects_v2').paginate(Bucket=bucket, Prefix='scrcpyproxy/', Delimiter='/'):
        prefixes.extend(item['Prefix'] for item in page.get('CommonPrefixes', []))
    releases = []
    for prefix in prefixes:
        match = re.fullmatch(r'scrcpyproxy/v(\d+\.\d+\.\d+)/', prefix)
        if not match:
            continue
        version = match[1]
        objects = {}
        for page in client.get_paginator('list_objects_v2').paginate(Bucket=bucket, Prefix=prefix):
            objects.update({item['Key'][len(prefix):]: item for item in page.get('Contents', [])})
        if 'SHA256SUMS' not in objects:
            continue
        checksum = client.get_object(Bucket=bucket, Key=prefix+'SHA256SUMS')['Body'].read()
        hashes = {}
        for line in checksum.decode('utf-8').splitlines():
            digest, name = line.split('  ', 1)
            if name not in FILES or not re.fullmatch('[0-9a-f]{64}', digest) or name in hashes:
                raise ValueError('Invalid published checksum manifest: '+version)
            hashes[name] = digest
        required=REQUIRED if version_key(version)>=(0,5,0) else LEGACY_REQUIRED
        if not required-{'SHA256SUMS'} <= hashes.keys() or not hashes.keys() <= objects.keys():
            raise ValueError('Incomplete published release: '+version)
        if version_key(version)>=(0,5,0) and not {'Mirroxy-windows-amd64.exe','Mirroxy-windows-amd64.zip'} & hashes.keys():
            raise ValueError('Windows full release missing: '+version)
        hashes['SHA256SUMS'] = hashlib.sha256(checksum).hexdigest()
        # HEAD verifies that storage metadata still matches the committed hashes.
        def verify_object(item):
            name, digest = item
            head = client.head_object(Bucket=bucket, Key=prefix+name)
            if head['ContentLength'] != objects[name]['Size'] or head.get('Metadata', {}).get('sha256') != digest:
                raise ValueError('Published object differs from manifest: '+prefix+name)
        with ThreadPoolExecutor(max_workers=4) as pool:
            list(pool.map(verify_object, hashes.items()))
        note = changes.get(version, {})
        release = {'version': version, 'date': note.get('date') or objects['SHA256SUMS']['LastModified'].date().isoformat(),
            'changes': note.get('changes', []),
            'files': [{'name': name, 'size': objects[name]['Size'], 'sha256': digest,
                       'url': base_url.rstrip('/')+'/'+prefix+quote(name)} for name, digest in hashes.items()]}
        releases.append(release)
    return sorted(releases, key=lambda entry: version_key(entry['version']), reverse=True)

def render_page(releases, installer_url, windows_online_url=''):
    if not releases:
        raise ValueError('No verified releases to publish')
    esc = html.escape
    latest = releases[0]
    latest_files = {item['name']: item for item in latest['files']}
    cards = []
    for number, name in enumerate(list(FILES)[:4]):
        if number == 0 and windows_online_url:
            cards.append(f'''<article class="platform-card"><span class="platform-number">01</span><h3>Windows</h3><p class="architecture">x86_64 / AMD64</p><a class="download" href="{esc(windows_online_url)}">下载在线安装器 <span>↗</span></a><p class="package-size">固定引导安装器 · 每次拉取最新版</p><p class="platform-instruction">校验后安装应用与核心；升级保留数据、端口及现有核心。下载失败可填写 HTTP 代理。</p></article>''')
            continue
        if name not in latest_files:
            continue
        item = latest_files[name]
        title, subtitle, instruction, kind = FILES[name]
        cards.append(f'''<article class="platform-card"><span class="platform-number">0{number+1}</span><h3>{esc(title)}</h3><p class="architecture">{esc(subtitle)}</p><a class="download" href="{esc(item['url'])}">下载 {kind} <span>↗</span></a><p class="package-size">{size_label(item['size'])} · 已预置内核</p><p class="platform-instruction">{esc(instruction)}</p></article>''')
    history = []
    for release in releases:
        files = {item['name']: item for item in release['files']}
        current = release['version'] == latest['version']
        rows = ''.join(f'''<tr><td><a href="{esc(files[name]['url'])}">{esc(name)}</a></td><td>{size_label(files[name]['size'])}</td><td><a class="file-download" href="{esc(files[name]['url'])}">下载 ↗</a></td></tr>''' for name in FILES if name in files)
        notes = ''.join('<li>'+esc(change)+'</li>' for change in release['changes'])
        label = '<span class="tag">最新版本</span>' if current else ''
        history.append(f'''<details class="release" id="v{esc(release['version'])}" {'open' if current else ''}><summary><span class="release-chevron">›</span><strong>v{esc(release['version'])}</strong>{label}<span class="release-date">{esc(release['date'])}</span><span class="file-count">{len(files)} 个文件</span></summary><div class="release-body"><ul class="changes">{notes}</ul><div class="files-wrap"><table><thead><tr><th>文件</th><th>大小</th><th></th></tr></thead><tbody>{rows}</tbody></table></div><p class="checksum-hint">使用 SHA256SUMS 核对下载文件。旧版本可下载离线安装包；上方一键安装始终使用最新版本。</p></div></details>''')
    replacements = {'TITLE_VERSION':esc(latest['version']), 'RELEASE_DATE':esc(latest['date']),
        'PLATFORM_CARDS':''.join(cards), 'VERSION_HISTORY':''.join(history),
        'RELEASE_COUNT':str(len(releases)),
        'INSTALL_COMMAND':esc('curl -fsSL --retry 3 '+installer_url+' | sudo bash'),
        'CHECKSUM_URL':esc(latest_files['SHA256SUMS']['url']),
        'README_URL':esc(latest_files['README.txt']['url']),
        'DASHBOARD_IMAGE':image_data('dashboard-redacted.png'),
        'SETTINGS_IMAGE':image_data('settings.png'),
        'SUBSCRIPTIONS_IMAGE':image_data('subscriptions.png'),
        'API_IMAGE':image_data('api-remote.png')}
    page = (ROOT/'packaging/release-page.html').read_text(encoding='utf-8')
    replacements['ICON_DATA'] = 'data:image/png;base64,'+base64.b64encode((ROOT/'web/icon.png').read_bytes()).decode()
    for key, value in replacements.items():
        page = page.replace('{{'+key+'}}', value)
    return page.encode('utf-8')

def publish_page(client, bucket, endpoint, output_dir, verify_download):
    base_url = endpoint.rstrip('/')+'/'+bucket
    releases = collect_releases(client, bucket, base_url)
    installer_url = base_url+'/scrcpyproxy/install.sh'
    # Never show a latest version whose stable installer still points elsewhere.
    stable = client.head_object(Bucket=bucket, Key='scrcpyproxy/install.sh')
    if stable.get('Metadata', {}).get('version') != releases[0]['version']:
        raise ValueError('Latest version and stable installer differ; publish the newest release first')
    windows_online_url=''
    if version_key(releases[0]['version']) >= (0,7,3):
        client.head_object(Bucket=bucket, Key='scrcpyproxy/Mirroxy-online-setup.exe')
        windows_online_url=base_url+'/scrcpyproxy/Mirroxy-online-setup.exe'
    content = render_page(releases, installer_url, windows_online_url)
    catalog = json.dumps({'latest':releases[0]['version'], 'installerURL':installer_url, 'windowsInstallerURL':windows_online_url, 'releases':releases},ensure_ascii=False,indent=2).encode('utf-8')
    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    for name, payload, content_type in [('releases.json', catalog, 'application/json; charset=utf-8'), ('index.html', content, 'text/html; charset=utf-8')]:
        (output_dir/name).write_bytes(payload)
        key = 'scrcpyproxy/'+name
        client.put_object(Bucket=bucket, Key=key, Body=payload, ContentType=content_type,
            ContentDisposition='inline', CacheControl='no-cache', Metadata={'sha256':hashlib.sha256(payload).hexdigest()})
        verify_download(base_url+'/'+key, payload, content_type)
    return base_url+'/scrcpyproxy/index.html'
