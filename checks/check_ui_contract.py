#!/usr/bin/env python3
"""Validate the approved Android exclusion controls against the existing Settings.vue.

This is a structural guard, not a pixel-by-pixel browser screenshot test.
It never edits any source file.
"""
import argparse
from pathlib import Path
import re
import sys

SECTION_START = '<section class="json-compact-block json-app-exclusions">'
SECTION_END = '<div class="builder-foot json-compact-foot">'


def check(path: Path) -> None:
    src = path.read_text(encoding='utf-8')
    if src.count(SECTION_START) != 1:
        raise ValueError('exactly one Android exclusions block is required')
    begin = src.index(SECTION_START)
    end = src.find(SECTION_END, begin)
    if end < 0:
        raise ValueError('the standard editor/options footer must follow the exclusions block')
    section = src[begin:end]

    # Keep the order visible in the approved iPad screenshot.
    ordered = [
        'setting.packageExcludeTitle',
        'setting.packageExcludeManualIDs',
        'v-model="tunExcludePkg"',
        'class="json-app-source-divider"',
        'setting.packageExcludeSources',
        'v-for="(source, index) in packageExcludeSources"',
        'class="json-source-status"',
        'setting.packageExcludeApplications',
        'setting.packageExcludeLastCheck',
        'setting.packageExcludeFailed',
        '@click="removePackageSource(index)"',
        '@click="addPackageSource"',
        'setting.packageExcludeScheduleDays',
        '@click="refreshPackageExclude()"',
        'setting.packageExcludePrepared',
        'setting.packageExcludeConflicts',
    ]
    pos = -1
    for marker in ordered:
        if section.count(marker) != 1:
            raise ValueError(f'expected one instance of visible UI marker {marker!r} (got {section.count(marker)})')
        nxt = section.find(marker, pos+1)
        if nxt < pos:
            raise ValueError(f'visible UI elements changed order near {marker!r}')
        pos = nxt

    for marker in (
        'class="json-package-row"',
        'class="json-package-value"',
        'class="json-package-footer"',
        'class="json-package-interval"',
        'class="json-package-count"',
        'class="json-source-error"',
        'name="trash"',
        'name="plus"',
        'name="refresh"',
        'class="input mono" dir="ltr"',
        ':disabled="stateChange || !packageExcludeSources.some(s => s.trim()) || packageExcludeRefreshing"',
    ):
        if marker not in section:
            raise ValueError(f'missing control, error state or style hook: {marker!r}')

    if 'class="json-compact-fold"' not in src[:begin]:
        raise ValueError('standard compact DNS/TUN/log fold group is missing before exclusions')
    if 'jsonEditor = true' not in src[end:end+700] or 'setting.jsonSubOptions' not in src[end:end+900]:
        raise ValueError('standard JSON editor/options controls missing after exclusions')

    # Require the responsive styling that keeps the screenshot's layout usable
    # on narrow devices. Changes to shared CSS still require a visual check.
    for selector in (
        '.json-app-exclusions', '.json-app-source-divider', '.json-package-row',
        '.json-package-value', '.json-source-status', '.json-source-error',
        '.json-package-footer', '.json-package-interval', '.json-package-count',
        '@media (max-width: 520px)',
    ):
        if selector not in src:
            raise ValueError(f'missing styling/responsive rule {selector}')
    if src.count('const packageExcludeSources = computed') != 1:
        raise ValueError('duplicated or absent source-list state')
    if src.count('const tunExcludePkg = computed') != 1:
        raise ValueError('duplicated or absent manual package-ID state')
    if src.count('const refreshPackageExclude = async') != 1:
        raise ValueError('duplicated or absent refresh action')

    print('UI CONTRACT OK: approved grouping, sequence, controls, counters and responsive styles')
    print('NOTE: compare the rendered iPad/desktop/mobile interface visually before installation')


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('settings_vue', nargs='?', type=Path, default=Path('frontend/src/views/Settings.vue'))
    args = p.parse_args()
    try:
        check(args.settings_vue)
    except (ValueError, OSError) as ex:
        print('UI CONTRACT FAILED:', ex, file=sys.stderr)
        sys.exit(1)
