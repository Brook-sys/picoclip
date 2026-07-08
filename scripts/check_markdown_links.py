#!/usr/bin/env python3
"""Validate local Markdown links and heading anchors.

This checker intentionally uses only the Python standard library so it stays
portable in local-first development environments.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path
from urllib.parse import unquote

LINK_RE = re.compile(r"(?<!!)\[[^\]]+\]\(([^)]+)\)")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.+?)\s*$")
SKIP_SCHEMES = ("http://", "https://", "mailto:", "tel:")


def github_slug(heading: str) -> str:
    """Return a GitHub-style slug for a Markdown heading."""
    heading = re.sub(r"<[^>]+>", "", heading)
    heading = heading.strip().lower()
    heading = re.sub(r"[`*_~]", "", heading)
    heading = re.sub(r"[^\w\s-]", "", heading, flags=re.UNICODE)
    heading = re.sub(r"\s+", "-", heading)
    heading = re.sub(r"-+", "-", heading).strip("-")
    return heading


def heading_anchors(text: str) -> set[str]:
    anchors: set[str] = set()
    seen: dict[str, int] = {}
    for line in text.splitlines():
        match = HEADING_RE.match(line)
        if not match:
            continue
        slug = github_slug(match.group(2))
        if not slug:
            continue
        count = seen.get(slug, 0)
        seen[slug] = count + 1
        anchors.add(slug if count == 0 else f"{slug}-{count}")
    return anchors


def markdown_files(root: Path) -> list[Path]:
    candidates = [root / "README.md", root / "README.pt-BR.md", root / "AGENTS.md"]
    docs_dir = root / "docs"
    if docs_dir.exists():
        candidates.extend(sorted(docs_dir.glob("*.md")))
    return [path for path in candidates if path.exists()]


def validate(root: Path) -> list[str]:
    files = markdown_files(root)
    anchors_by_file = {path.resolve(): heading_anchors(path.read_text(errors="ignore")) for path in files}
    errors: list[str] = []

    for source in files:
        text = source.read_text(errors="ignore")
        for match in LINK_RE.finditer(text):
            raw_link = match.group(1).strip()
            if not raw_link or raw_link.startswith(SKIP_SCHEMES):
                continue
            link = raw_link.split(None, 1)[0]
            path_part, _, fragment = link.partition("#")
            if not path_part and not fragment:
                continue
            target = (source.parent / unquote(path_part)).resolve() if path_part else source.resolve()
            if path_part and not target.exists():
                errors.append(f"{source}: missing linked file {raw_link!r} -> {target}")
                continue
            if fragment:
                expected = unquote(fragment).lower()
                anchors = anchors_by_file.get(target)
                if anchors is None and target.exists() and target.suffix.lower() == ".md":
                    anchors = heading_anchors(target.read_text(errors="ignore"))
                    anchors_by_file[target] = anchors
                if anchors is not None and expected not in anchors:
                    errors.append(f"{source}: missing anchor #{fragment} in {target}")
    return errors


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", nargs="?", default=".", help="repository root")
    args = parser.parse_args(argv)

    errors = validate(Path(args.root).resolve())
    if errors:
        print("Markdown link validation failed:")
        for error in errors:
            print(f"- {error}")
        return 1
    print("Markdown links and anchors OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
