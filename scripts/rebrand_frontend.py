#!/usr/bin/env python3
"""Create a KnowPulse overlay for an existing UI release without upgrading it.

Usage: python3 scripts/rebrand_frontend.py EXISTING_DIST OUTPUT_OVERLAY
Build a derived nginx image with the overlay copied to /usr/share/nginx/html.
Original hashed assets are retained so open tabs can finish their requests.
"""
import hashlib
import json
import re
import shutil
import sys
from pathlib import Path


def main():
    source, output = map(Path, sys.argv[1:])
    repo = Path(__file__).resolve().parents[1]
    replacements_file = repo / 'frontend/branding-replacements.json'
    replacements = json.loads(replacements_file.read_text())
    logo_dir = repo / 'frontend/src/assets/img'
    public = repo / 'frontend/public'
    login_assets = ['knowpulse-login.css', 'knowpulse-login-visual.svg']
    revision = hashlib.sha256(
        replacements_file.read_bytes() + (logo_dir / 'knowpulse-logo.svg').read_bytes()
        + b''.join((public / name).read_bytes() for name in login_assets)
    ).hexdigest()[:10]
    scripts = sorted((source / 'assets').glob('*.js'))
    assert scripts and (source / 'index.html').exists(), 'Expected a built Vite UI'
    names = {p.name: f'kp-{revision}-{p.name}' for p in scripts}
    asset_pattern = re.compile('|'.join(map(re.escape, names)))
    old_logos = list((source / 'assets').glob('weknora-*.png'))
    assert old_logos, 'Expected the existing WeKnora wordmark'
    (output / 'assets').mkdir(parents=True, exist_ok=True)
    updated_texts = set()

    for script in scripts:
        content = script.read_text()
        for old, new in sorted(replacements.items(), key=lambda item: len(item[0]), reverse=True):
            if old in content:
                updated_texts.add(old)
                content = content.replace(old, json.dumps(new, ensure_ascii=False)[1:-1])
        for old_logo in old_logos:
            content = content.replace('/assets/' + old_logo.name, '/knowpulse-logo.svg')
        content = content.replace('alt:"WeKnora"', 'alt:"KnowPulse"')
        content = asset_pattern.sub(lambda match: names[match.group()], content)
        (output / 'assets' / names[script.name]).write_text(content)

    assert any('Hi，我是 WeKnora' in text for text in updated_texts), 'Missing Chinese greeting'
    for name in ['index.html', 'embed.html']:
        path = source / name
        if not path.exists():
            continue
        content = path.read_text().replace('<title>WeKnora', '<title>KnowPulse')
        content = content.replace('content="WeKnora', 'content="KnowPulse')
        content = re.sub(r'<link\b[^>]*rel="(?:shortcut icon|icon|apple-touch-icon)"[^>]*>', '', content)
        content = content.replace('</head>', '<link rel="icon" type="image/svg+xml" href="/knowpulse-icon.svg">\n<link rel="stylesheet" href="/knowpulse-brand.css">\n</head>')
        if name == 'index.html':
            content = content.replace('</head>', '<link rel="stylesheet" href="/knowpulse-login.css">\n</head>')
        content = asset_pattern.sub(lambda match: names[match.group()], content)
        (output / name).write_text(content)

    for name in ['knowpulse-logo.svg', 'knowpulse-logo-dark.svg']:
        shutil.copyfile(logo_dir / name, output / name)
    shutil.copyfile(repo / 'frontend/public/knowpulse-icon.svg', output / 'knowpulse-icon.svg')
    for name in login_assets:
        shutil.copyfile(public / name, output / name)
    (output / 'knowpulse-brand.css').write_text('''
.aside_box .logo_box .logo { width: 144px; height: auto; }
html[theme-mode="dark"] .aside_box .logo_box .logo,
html[theme-mode="dark"] .header-logo .logo-image {
  content: url("/knowpulse-logo-dark.svg");
  filter: none !important;
}
''')
    print(f'Prepared {len(scripts)} cache-busted JS assets; updated {len(updated_texts)} brand strings; revision {revision}')


if __name__ == '__main__':
    main()
