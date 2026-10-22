"""Render the repository CHANGELOG.md from the dashboard changelog files.

The dashboard renders web/changelog.json and web/changelog.en.json directly, so
those two files stay the single source of truth.
"""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def load(name):
    return json.loads((ROOT / 'web' / name).read_text(encoding='utf-8'))


def main():
    zh = load('changelog.json')
    en = load('changelog.en.json')
    english = {entry['version']: entry for entry in en}
    lines = ['# 更新日志 / Changelog', '',
             '本文件由 `packaging/render-changelog.py` 从 `web/changelog.json` 与',
             '`web/changelog.en.json` 生成；面板内的「更新日志」页面读的是同样这两个文件。', '']
    for entry in zh:
        version = entry['version']
        date = entry.get('date', '')
        lines.append(f'## {version}' + (f' ({date})' if date else ''))
        lines.append('')
        for change in entry.get('changes', []):
            lines.append(f'- {change}')
        translated = english.get(version)
        if translated and translated.get('changes'):
            lines.append('')
            lines.append('<details><summary>English</summary>')
            lines.append('')
            for change in translated['changes']:
                lines.append(f'- {change}')
            lines.append('')
            lines.append('</details>')
        lines.append('')
    (ROOT / 'CHANGELOG.md').write_text('\n'.join(lines), encoding='utf-8')
    print(f'CHANGELOG.md: {len(zh)} entries')


if __name__ == '__main__':
    main()
