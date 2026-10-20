"""Publish an explicit binary release allowlist to the configured S3 bucket.

Credentials come from an external file and are never included in artifacts.
Every uploaded object is downloaded anonymously and verified before publishing
the installation entry point. The bucket's existing public policy is preserved.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
import hashlib
import json
import getpass
from pathlib import Path
import time
import urllib.request
from botocore.exceptions import ClientError
from release_site import publish_page, version_key

import boto3
from botocore.config import Config

ROOT = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser()
parser.add_argument('--credentials', required=True, help="JSON file, or '-' for a hidden interactive prompt")
parser.add_argument('--version', required=True)
parser.add_argument('--archive-only', action='store_true', help='Backfill an older release without moving the latest installer')
parser.add_argument('--endpoint', default='https://ny-1s.enzonix.com')
parser.add_argument('--bucket', default='bucket-1776-2431')
parser.add_argument('--region', default='us-east-1')
parser.add_argument('--verification-proxy', default='', help='Optional HTTP proxy used only for anonymous download verification')
parser.add_argument('--bootstrap', type=Path, default=ROOT/'dist/bootstrap/Mirroxy-online-setup.exe', help='Fixed Windows online installer; upload only once, never replace different bytes')
args = parser.parse_args()
if not all(part.isdigit() for part in args.version.split('.')):
    raise SystemExit('Invalid version')
dist = ROOT / 'dist' / ('v' + args.version)
prefix = 'scrcpyproxy/v' + args.version + '/'
base = args.endpoint.rstrip('/') + '/' + args.bucket + '/' + prefix
progress_path = ROOT/'work'/('s3-progress-v'+args.version+'.json')
verified = {}
if progress_path.exists():
    progress = json.loads(progress_path.read_text(encoding='utf-8'))
    if progress.get('baseURL') == base:
        verified = {item['name']: item for item in progress.get('files', [])}
if args.credentials == '-':
    creds = json.loads(getpass.getpass('S3 credential JSON: '))
else:
    creds = json.loads(Path(args.credentials).read_text(encoding='utf-8-sig'))
client = boto3.client('s3', endpoint_url=args.endpoint, region_name=args.region,
    aws_access_key_id=creds['access_key'], aws_secret_access_key=creds['secret_key'],
    config=Config(signature_version='s3v4', s3={'addressing_style': 'path'},
        connect_timeout=20, read_timeout=180, retries={'max_attempts': 3, 'mode':'standard'},
        request_checksum_calculation='when_required', response_checksum_validation='when_required'))
packages = ['Mirroxy-linux-arm64.run',
    'Mirroxy-linux-amd64.run', 'Mirroxy-macos-arm64.zip',
    'Mirroxy-linux-arm64.tar.gz', 'Mirroxy-linux-amd64.tar.gz',
    'Mirroxy-update-windows-amd64.zip','Mirroxy-update-linux-arm64.zip',
    'Mirroxy-update-linux-amd64.zip','Mirroxy-update-macos-arm64.zip']
packages += [name for name in ['Mirroxy-windows-amd64.zip','Mirroxy-windows-amd64.exe'] if (dist/name).is_file()]
if not any(name.startswith('Mirroxy-windows-amd64.') for name in packages):
    raise SystemExit('Windows full program package missing')
if version_key(args.version)>=(0,7,3) and 'Mirroxy-windows-amd64.zip' not in packages:
    raise SystemExit('Online installer requires the Windows full ZIP')
if args.archive_only:
    stable = client.head_object(Bucket=args.bucket, Key='scrcpyproxy/install.sh')
    if version_key(args.version) >= version_key(stable['Metadata']['version']):
        raise SystemExit('--archive-only requires an older version than the stable installer')
    packages = [name for name in packages if (dist/name).is_file()]
allowed = packages + ['install.sh', 'README.txt', 'SHA256SUMS']
hashes = {}
for name in allowed:
    data = (dist/name).read_bytes()
    if any(secret.encode() in data for secret in creds.values()):
        raise SystemExit('Credential detected in release artifact')
    hashes[name] = hashlib.sha256(data).hexdigest()
checksums = {}
for line in (dist/'SHA256SUMS').read_text(encoding='utf-8').splitlines():
    digest, name = line.split('  ', 1)
    if name not in allowed or hashes[name] != digest:
        raise SystemExit('Invalid release checksum: ' + name)
    checksums[name] = digest
if set(checksums) != set(allowed)-{'SHA256SUMS'}:
    raise SystemExit('Incomplete checksum list')
try:
    committed = client.get_object(Bucket=args.bucket, Key=prefix+'SHA256SUMS')['Body'].read()
    if committed != (dist/'SHA256SUMS').read_bytes():
        raise SystemExit('This version was already published with different bytes; use a new version')
except ClientError as exc:
    if exc.response['Error']['Code'] not in ('NoSuchKey', '404'):
        raise

# Refuse different bytes even when an earlier upload failed before the commit
# marker. Do the complete read-only audit before uploading any versioned object.
for name in allowed:
    try:
        existing=client.head_object(Bucket=args.bucket,Key=prefix+name)
    except ClientError as exc:
        if exc.response['Error']['Code'] not in ('NoSuchKey','404'): raise
        continue
    if existing.get('ContentLength')!=(dist/name).stat().st_size or existing.get('Metadata',{}).get('sha256')!=hashes[name]:
        raise SystemExit('Refusing to overwrite historical bytes: '+prefix+name+'; use a new version')

def anonymous_verify(name):
    # Explicitly avoid proxy credentials, browser cookies or signed URLs.
    proxies = {'https':args.verification_proxy} if args.verification_proxy else {}
    opener = urllib.request.build_opener(urllib.request.ProxyHandler(proxies))
    for attempt in range(3):
        try:
            with opener.open(base+name, timeout=90) as response:
                digest = hashlib.sha256()
                size = 0
                started = time.monotonic()
                while chunk := response.read(1024*1024):
                    digest.update(chunk)
                    size += len(chunk)
                    if time.monotonic()-started > 180:
                        raise TimeoutError('Slow verification; retry with --verification-proxy if needed')
            if digest.hexdigest() != hashes[name] or size != (dist/name).stat().st_size:
                raise ValueError(f'Download mismatch: {name}, bytes={size}, SHA256={digest.hexdigest()}')
            return size
        except Exception as exc:
            print(f'Anonymous verification attempt {attempt+1}/3 failed for {name}: {exc}', flush=True)
            if attempt == 2:
                raise
            time.sleep(2)

def upload(name):
    content_type = ('application/vnd.microsoft.portable-executable' if name.endswith('.exe') else
        'application/zip' if name.endswith('.zip') else
        'application/gzip' if name.endswith('.tar.gz') else
        'application/octet-stream' if name.endswith('.run') else 'text/plain; charset=utf-8')
    path = dist/name
    try:
        head = client.head_object(Bucket=args.bucket, Key=prefix+name)
    except ClientError as exc:
        if exc.response['Error']['Code'] not in ('NoSuchKey', '404'):
            raise
        head = {}
    matches = head.get('ContentLength') == path.stat().st_size and head.get('Metadata', {}).get('sha256') == hashes[name]
    if head and not matches:
        raise ValueError('Historical object changed; refusing replacement: '+prefix+name)
    previous = verified.get(name, {})
    if matches and previous.get('sha256') == hashes[name] and previous.get('size') == path.stat().st_size and previous.get('url') == base+name:
        print('Resuming already verified '+name, flush=True)
        return previous
    if not matches:
        print('Uploading '+name, flush=True)
        with path.open('rb') as source:
            client.put_object(Bucket=args.bucket, Key=prefix+name, Body=source,
                ContentLength=path.stat().st_size, ContentType=content_type,
                ContentDisposition='attachment; filename="'+name+'"',
                Metadata={'sha256':hashes[name]}, CacheControl='public, max-age=3600')
    head = client.head_object(Bucket=args.bucket, Key=prefix+name)
    if head['ContentLength'] != path.stat().st_size or head['Metadata'].get('sha256') != hashes[name]:
        raise ValueError('S3 object metadata mismatch: ' + name)
    size = anonymous_verify(name)
    print('Anonymous SHA256 verified '+name, flush=True)
    return {'name':name, 'url':base+name, 'sha256':hashes[name], 'size':size}

report = []
def record_verified(item):
    report.append(item)
    temp = progress_path.with_suffix('.tmp')
    temp.write_text(json.dumps({'baseURL':base, 'files':report},indent=2),encoding='utf-8')
    temp.replace(progress_path)
failures = []
with ThreadPoolExecutor(max_workers=2) as pool:
    for future in as_completed([pool.submit(upload, name) for name in packages]):
        try:
            record_verified(future.result())
        except Exception as exc:
            failures.append(str(exc))
if failures:
    raise SystemExit('Publication incomplete; completed checks saved for retry.\n'+'\n'.join(failures))
# Publish the script only after both architecture payloads are fully available.
for name in ['README.txt','install.sh','SHA256SUMS']:
    record_verified(upload(name))
alias_key = 'scrcpyproxy/install.sh'
if not args.archive_only:
    # Re-publishing an older archive must never move the stable installer back.
    try:
        stable = client.head_object(Bucket=args.bucket, Key=alias_key)
        if version_key(stable['Metadata']['version']) > version_key(args.version):
            raise SystemExit('Refusing to downgrade stable installer; use --archive-only')
    except ClientError as exc:
        if exc.response['Error']['Code'] not in ('NoSuchKey', '404'):
            raise
    client.copy_object(Bucket=args.bucket, Key=alias_key,
        CopySource={'Bucket':args.bucket, 'Key':prefix+'install.sh'},
        MetadataDirective='REPLACE', Metadata={'sha256':hashes['install.sh'], 'version':args.version},
        ContentType='text/plain; charset=utf-8', CacheControl='no-cache')
installer_url = args.endpoint.rstrip('/')+'/'+args.bucket+'/'+alias_key
opener = urllib.request.build_opener(urllib.request.ProxyHandler({'https':args.verification_proxy} if args.verification_proxy else {}))
if not args.archive_only:
    with opener.open(installer_url,timeout=45) as response:
        if hashlib.sha256(response.read()).hexdigest() != hashes['install.sh']:
            raise ValueError('Installation entry point checksum mismatch')
def verify_page(url, payload, content_type):
    with opener.open(url, timeout=45) as response:
        if response.read() != payload or response.headers.get_content_type() != content_type.split(';')[0]:
            raise ValueError('Published page content/type mismatch')
        if 'attachment' in response.headers.get('Content-Disposition', '').lower():
            raise ValueError('S3 forces a download instead of displaying the page')
if not args.archive_only and version_key(args.version)>=(0,7,3):
    online_key='scrcpyproxy/Mirroxy-online-setup.exe'
    try:
        online_head=client.head_object(Bucket=args.bucket,Key=online_key)
    except ClientError as exc:
        if exc.response['Error']['Code'] not in ('NoSuchKey','404'): raise
        online_head=None
    if args.bootstrap.is_file():
        online_payload=args.bootstrap.read_bytes()
        if len(online_payload)>16<<20 or not online_payload.startswith(b'MZ'):
            raise SystemExit('Invalid fixed Windows online installer')
        if any(secret.encode() in online_payload for secret in creds.values()):
            raise SystemExit('Credential detected in bootstrap artifact')
        online_hash=hashlib.sha256(online_payload).hexdigest()
        if online_head and (online_head.get('ContentLength')!=len(online_payload) or online_head.get('Metadata',{}).get('sha256')!=online_hash):
            raise SystemExit('Fixed installer already published with different bytes; do not replace it')
        if not online_head:
            client.put_object(Bucket=args.bucket,Key=online_key,Body=online_payload,ContentType='application/vnd.microsoft.portable-executable',ContentDisposition='attachment; filename="Mirroxy-online-setup.exe"',CacheControl='public, max-age=3600',Metadata={'sha256':online_hash,'bootstrap-version':'1.0.0'})
        with opener.open(args.endpoint.rstrip('/')+'/'+args.bucket+'/'+online_key,timeout=90) as response:
            if hashlib.sha256(response.read()).hexdigest()!=online_hash:
                raise ValueError('Fixed Windows installer anonymous checksum mismatch')
    elif not online_head:
        raise SystemExit('Build the fixed online installer once before first publication')
page_url = publish_page(client, args.bucket, args.endpoint, ROOT/'work/release-site', verify_page)
result = {'version':args.version, 'baseURL':base, 'installerURL':installer_url, 'pageURL':page_url, 'anonymousDownloadsVerified':True,
    'files': sorted(report, key=lambda item:item['name'])}
report_path = ROOT/'work'/('s3-release-v'+args.version+'.json')
report_path.write_text(json.dumps(result,indent=2), encoding='utf-8')
print('Release page ready: '+page_url, flush=True)
