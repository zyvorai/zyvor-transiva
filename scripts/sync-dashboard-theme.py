#!/usr/bin/env python3
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
"""Keep standalone dashboard HTML themes in sync with the daemon stylesheet."""
from pathlib import Path
import re

root = Path(__file__).resolve().parents[1]
css = (root / 'daemon/dashboard/static/css/transiva-workspace.css').read_text()
block = '<style id="transiva-workspace-theme">\n' + css + '\n</style>'
for page in (root / 'web/dashboard').glob('*.html'):
    text = page.read_text()
    pattern = r'<style id="transiva-workspace-theme">.*?</style>'
    if not re.search(pattern, text, re.S):
        raise SystemExit(f'Missing theme marker in {page}')
    page.write_text(re.sub(pattern, lambda _: block, text, flags=re.S))
    print(page.relative_to(root))
