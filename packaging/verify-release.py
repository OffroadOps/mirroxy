"""Verify release checksums, architectures, allowlisted layout, and .run payloads."""
import argparse
import hashlib
import io
from pathlib import Path, PurePosixPath
import struct
import tarfile
import zipfile

PACKAGE_NAMES = {
    'Mirroxy-linux-arm64.run', 'Mirroxy-linux-amd64.run',
    'Mirroxy-macos-arm64.zip', 'Mirroxy-linux-arm64.tar.gz', 'Mirroxy-linux-amd64.tar.gz',
    'Mirroxy-update-windows-amd64.zip', 'Mirroxy-update-linux-arm64.zip',
    'Mirroxy-update-linux-amd64.zip', 'Mirroxy-update-macos-arm64.zip',
    'install.sh', 'README.txt',
}
FORBIDDEN_COMPONENTS = {'xray', 'sing_box', 'sing-box', 'srss', 'mihomo', 'data'}

def digest(data):
    return hashlib.sha256(data).hexdigest()

def architecture(data):
    magic = data[:4]
    if magic == b'\x7fELF':
        order = '<' if data[5] == 1 else '>'
        return {62: 'linux-amd64', 183: 'linux-arm64'}.get(struct.unpack(order+'H', data[18:20])[0], 'unknown')
    if data[:2] == b'MZ':
        offset = struct.unpack('<I', data[60:64])[0]
        return {0x8664: 'windows-amd64'}.get(struct.unpack('<H', data[offset+4:offset+6])[0], 'unknown')
    if magic in (bytes.fromhex('cffaedfe'), bytes.fromhex('feedfacf')):
        order = '<' if magic == bytes.fromhex('cffaedfe') else '>'
        return 'macos-arm64' if struct.unpack(order+'I', data[4:8])[0] == 0x0100000C else 'unknown'
    return 'unknown'

def archive_files(path):
    if path.suffix == '.zip':
        with zipfile.ZipFile(path) as archive:
            return {item.filename: archive.read(item) for item in archive.infolist() if not item.is_dir()}
    with tarfile.open(path, 'r:gz') as archive:
        return {item.name: archive.extractfile(item).read() for item in archive if item.isfile()}

def logical_name(name):
    parts = PurePosixPath(name).parts
    if parts and parts[0].startswith('Mirroxy-'):
        parts = parts[1:]
    return '/'.join(parts)

def verify_archive(path, target, update=False):
    files = archive_files(path)
    logical = {logical_name(name): payload for name, payload in files.items()}
    for name in logical:
        parts = {part.lower() for part in PurePosixPath(name).parts}
        if parts & FORBIDDEN_COMPONENTS or name.endswith(('.go', '.c', '.h', '.py', '.ps1')):
            raise ValueError(f'forbidden release path: {path.name}: {name}')
    app = 'Mirroxy.exe' if target.startswith('windows') else 'Mirroxy'
    required = {app, 'manifest.json'}
    if update:
        required |= {f'bin/resources/Mirroxy_set_{number:02d}.bin' for number in range(1, 13)}
        if target.startswith('windows'):
            required.add('upgrade.exe')
        if any(name.startswith('bin/runtime/') or name.startswith('data/') for name in logical):
            raise ValueError(f'update package overwrites core or data: {path.name}')
    else:
        required |= {
            'bin/runtime/Mirroxy_core_A' + ('.exe' if target.startswith('windows') else ''),
            'bin/runtime/Mirroxy_core_B' + ('.exe' if target.startswith('windows') else ''),
            'bin/resources/Mirroxy_route_ip.bin', 'bin/resources/Mirroxy_route_domain.bin',
            'bin/resources/Mirroxy_trust.pem', 'licenses/NOTICE.txt', 'SHA256SUMS', 'README.txt',
        }
        required |= {f'bin/resources/Mirroxy_set_{number:02d}.bin' for number in range(1, 13)}
        if target.startswith('windows'):
            required.add('upgrade.exe')
    if missing := required - logical.keys():
        raise ValueError(f'missing files in {path.name}: {sorted(missing)}')
    if architecture(logical[app]) != target:
        raise ValueError(f'wrong application architecture in {path.name}: {architecture(logical[app])}')
    if 'bin/runtime/Mirroxy_downloader.exe' in logical and architecture(logical['bin/runtime/Mirroxy_downloader.exe']) != 'windows-amd64':
        raise ValueError('wrong aria2 architecture')
    if not update:
        suffix = '.exe' if target.startswith('windows') else ''
        for core in ('A', 'B'):
            name = f'bin/runtime/Mirroxy_core_{core}{suffix}'
            if architecture(logical[name]) != target:
                raise ValueError(f'wrong {name} architecture in {path.name}: {architecture(logical[name])}')
    return len(logical)

def verify_windows_installer(path):
    payload = path.read_bytes()
    if architecture(payload) != 'windows-amd64':
        raise ValueError(f'wrong installer architecture: {architecture(payload)}')
    if len(payload) < 8 << 20:
        raise ValueError('Windows installer is unexpectedly small')
    return len(payload)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--version', required=True)
    parser.add_argument('--dist', type=Path)
    args = parser.parse_args()
    dist = args.dist or Path(__file__).resolve().parent.parent/'dist'/('v'+args.version)
    sums = {}
    for line in (dist/'SHA256SUMS').read_text(encoding='utf-8').splitlines():
        value, name = line.split('  ', 1)
        if name in sums or len(value) != 64:
            raise ValueError('invalid SHA256SUMS')
        sums[name] = value
    expected = PACKAGE_NAMES | ({'Mirroxy-windows-amd64.zip'} if (dist/'Mirroxy-windows-amd64.zip').is_file() else set()) | ({'Mirroxy-windows-amd64.exe'} if (dist/'Mirroxy-windows-amd64.exe').is_file() else set())
    if not (dist/'Mirroxy-windows-amd64.zip').is_file() and not (dist/'Mirroxy-windows-amd64.exe').is_file():
        raise ValueError('Windows full package missing')
    if sums.keys() != expected:
        raise ValueError(f'unexpected deliverables: missing={PACKAGE_NAMES-sums.keys()}, extra={sums.keys()-PACKAGE_NAMES}')
    for name, value in sums.items():
        if digest((dist/name).read_bytes()) != value:
            raise ValueError('checksum mismatch: '+name)
    results = []
    installer = dist/'Mirroxy-windows-amd64.exe'
    if installer.is_file():
        print(f'{installer.name}: {verify_windows_installer(installer)} bytes verified')
    windows_full = dist/'Mirroxy-windows-amd64.zip'
    if windows_full.is_file():
        results.append((windows_full.name, verify_archive(windows_full, 'windows-amd64')))
    targets = [('linux-arm64', False), ('linux-amd64', False), ('macos-arm64', False)]
    for target, update in targets:
        ext = '.tar.gz' if target.startswith('linux') else '.zip'
        path = dist/f'Mirroxy-{target}{ext}'
        results.append((path.name, verify_archive(path, target, update)))
        update_path = dist/f'Mirroxy-update-{target}.zip'
        results.append((update_path.name, verify_archive(update_path, target, True)))
    windows_update = dist/'Mirroxy-update-windows-amd64.zip'
    results.append((windows_update.name, verify_archive(windows_update, 'windows-amd64', True)))
    marker = b'__MIRROXY_ARCHIVE_BELOW__\n'
    for arch in ('arm64', 'amd64'):
        run = (dist/f'Mirroxy-linux-{arch}.run').read_bytes()
        if marker not in run or run.split(marker, 1)[1] != (dist/f'Mirroxy-linux-{arch}.tar.gz').read_bytes():
            raise ValueError('self-extracting payload mismatch: '+arch)
    for name, count in results:
        print(f'{name}: {count} files verified')
    print(f'{len(expected)} deliverable checksums and both .run payloads verified')

if __name__ == '__main__':
    main()
