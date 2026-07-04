#!/usr/bin/env python3
"""Rewrite the landing page's download links for a release.

Usage: update_landing_downloads.py VERSION [INDEX_HTML]

VERSION is the semver with or without a leading v (e.g. v1.2.0 or 1.2.0).
Every <a> tag carrying a data-download="<goos>-<goarch>" attribute gets its
href pointed at the matching GoReleaser asset for that version, and the
#dl-version badge text becomes vVERSION. Asset names must stay in sync with
the archives.name_template in .goreleaser.yaml.
"""

import re
import sys
from pathlib import Path

REPO = "dsandor/familytime"
ARCHIVE_EXT = {"windows": "zip"}  # everything else is tar.gz


def asset_url(version: str, target: str) -> str:
    goos, goarch = target.split("-", 1)
    ext = ARCHIVE_EXT.get(goos, "tar.gz")
    name = f"familytime_{version}_{goos}_{goarch}.{ext}"
    return f"https://github.com/{REPO}/releases/download/v{version}/{name}"


def main() -> None:
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    version = sys.argv[1].lstrip("v")
    path = Path(sys.argv[2]) if len(sys.argv) > 2 else Path("landing/index.html")
    html = path.read_text()

    targets = re.findall(r'data-download="([^"]+)"', html)
    if not targets:
        sys.exit(f"error: no data-download anchors found in {path}")

    def rewrite_anchor(match: re.Match) -> str:
        tag = match.group(0)
        target = re.search(r'data-download="([^"]+)"', tag).group(1)
        return re.sub(r'href="[^"]*"', f'href="{asset_url(version, target)}"', tag)

    html = re.sub(r'<a\b[^>]*data-download="[^"]+"[^>]*>', rewrite_anchor, html)

    html, badge_count = re.subn(
        r'(<span id="dl-version">)[^<]*(</span>)',
        rf"\g<1>v{version}\g<2>",
        html,
    )
    if badge_count == 0:
        sys.exit(f"error: no #dl-version badge found in {path}")

    path.write_text(html)
    print(f"updated {len(targets)} download links and version badge to v{version}")


if __name__ == "__main__":
    main()
