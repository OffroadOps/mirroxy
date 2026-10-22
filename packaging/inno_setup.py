"""Locate the Inno Setup 7 compiler without assuming a machine specific path."""
import os
from pathlib import Path


def find_iscc():
    """Return the path to ISCC.exe, or None when Inno Setup 7 is not installed.

    Common installation directories are probed, plus the ISCC environment
    variable and everything on PATH.
    """
    candidates = []
    override = os.environ.get('MIRROXY_ISCC') or os.environ.get('ISCC')
    if override:
        candidates.append(Path(override))
    for base in (os.environ.get('ProgramFiles'), os.environ.get('ProgramW6432'),
                 os.environ.get('ProgramFiles(x86)'), os.environ.get('LOCALAPPDATA')):
        if base:
            candidates.append(Path(base) / 'Inno Setup 7' / 'ISCC.exe')
    for entry in os.environ.get('PATH', '').split(os.pathsep):
        if entry:
            candidates.append(Path(entry) / 'ISCC.exe')
    return next((path for path in candidates if path.is_file()), None)


def require_iscc(explicit=None):
    """Return an ISCC path or explain how to provide one."""
    if explicit:
        path = Path(explicit)
        if not path.is_file():
            raise SystemExit(f'Inno Setup 7 compiler not found: {path}')
        return path
    path = find_iscc()
    if path is None:
        raise SystemExit(
            'Inno Setup 7 compiler not found. Install Inno Setup 7, set MIRROXY_ISCC, '
            'or pass --iscc with the path to ISCC.exe.')
    return path
