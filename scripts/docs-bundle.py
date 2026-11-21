#!/usr/bin/env python3
"""Build the offline documentation bundles (ZIP, PDF, EPUB) from docs/en.

Reads the page order from zensical.toml's nav, so the bundles follow the
same order as the site sidebar. Writes into docs/en/assets/downloads,
which is gitignored and copied into the built site by Zensical.

Usage: scripts/docs-bundle.py [--require-pdf] [output-dir]

The PDF is rendered with headless Chromium (or Chrome). With
--require-pdf a missing browser is an error instead of a warning.
"""

import argparse
import html
import os
import re
import shutil
import subprocess
import sys
import tempfile
import tomllib
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs" / "en"
NAME = "reticulum-go-docs"
TITLE = "Reticulum-Go Documentation"

try:
    import markdown
except ImportError:
    markdown = None

PAGE_RE = re.compile(r'"([^"]+\.md)"')
MD_LINK_RE = re.compile(r"\(([^()]*?)\.md(#[^()]*)?\)")


def nav_pages() -> list[str]:
    """Ordered list of Markdown pages, flattened from the zensical nav."""
    with open(ROOT / "zensical.toml", "rb") as fh:
        nav = tomllib.load(fh)["project"]["nav"]

    pages: list[str] = []

    def walk(entry):
        if isinstance(entry, str):
            pages.append(entry)
        elif isinstance(entry, list):
            for item in entry:
                walk(item)
        elif isinstance(entry, dict):
            for value in entry.values():
                walk(value)

    for item in nav:
        walk(item)
    return pages


def md_to_html(text: str) -> str:
    if markdown is None:
        sys.exit("error: python package 'markdown' is required (pip install markdown)")
    return markdown.markdown(
        text,
        extensions=["tables", "fenced_code", "admonition", "attr_list", "md_in_html"],
        output_format="xhtml",
    )


def page_title(slug: str, text: str) -> str:
    for line in text.splitlines():
        if line.startswith("# "):
            return line[2:].strip()
    return slug.replace("-", " ").title()


def rewrite_links(fragment: str, epub: bool) -> str:
    def repl(m: re.Match) -> str:
        slug = Path(m.group(1)).stem
        anchor = m.group(2) or ""
        if epub:
            return f"({slug}.xhtml)"
        return f"(#{slug}{anchor})"

    return MD_LINK_RE.sub(repl, fragment)


PRINT_CSS = """
body { font-family: system-ui, sans-serif; max-width: 44rem; margin: 2rem auto;
       padding: 0 1rem; color: #18181b; font-size: 11pt; line-height: 1.5; }
h1, h2, h3 { line-height: 1.25; }
h1 { border-bottom: 1px solid #d4d4d8; padding-bottom: .3rem; }
section { break-before: page; }
section:first-of-type { break-before: auto; }
pre, code { font-family: ui-monospace, monospace; font-size: 9.5pt; }
pre { background: #f4f4f5; border: 1px solid #e4e4e7; border-radius: 6px;
      padding: .8rem; overflow-wrap: break-word; white-space: pre-wrap; }
code { background: #f4f4f5; padding: .1rem .3rem; border-radius: 4px; }
pre code { background: none; padding: 0; }
table { border-collapse: collapse; width: 100%; }
th, td { border: 1px solid #d4d4d8; padding: .3rem .6rem; text-align: left; }
th { background: #f4f4f5; }
a { color: #18181b; }
blockquote { border-left: 3px solid #d4d4d8; margin-left: 0; padding-left: 1rem;
             color: #52525b; }
"""


def render_pages(pages: list[str]) -> list[tuple[str, str, str]]:
    """Return [(slug, title, xhtml_fragment)] in nav order."""
    out = []
    for rel in pages:
        text = (DOCS / rel).read_text(encoding="utf-8")
        slug = Path(rel).stem
        out.append((slug, page_title(slug, text), md_to_html(text)))
    return out


def build_zip(out_dir: Path) -> Path:
    target = out_dir / f"{NAME}.zip"
    with zipfile.ZipFile(target, "w", zipfile.ZIP_DEFLATED) as zf:
        for md in sorted(DOCS.glob("*.md")):
            zf.write(md, md.name)
    return target


def find_browser() -> str | None:
    for name in ("chromium", "chromium-browser", "google-chrome", "chrome"):
        found = shutil.which(name)
        if found:
            return found
    return None


def build_pdf(rendered: list[tuple[str, str, str]], out_dir: Path,
              require: bool) -> Path | None:
    browser = find_browser()
    if browser is None:
        msg = "warning: no Chromium/Chrome found, skipping PDF"
        if require:
            sys.exit("error: " + msg)
        print(msg, file=sys.stderr)
        return None

    body = [f"<h1>{html.escape(TITLE)}</h1>"]
    for slug, _title, fragment in rendered:
        body.append(f'<section id="{slug}">{rewrite_links(fragment, epub=False)}</section>')
    doc = (
        "<!doctype html><html><head><meta charset='utf-8'>"
        f"<style>{PRINT_CSS}</style></head><body>{''.join(body)}</body></html>"
    )

    target = out_dir / f"{NAME}.pdf"
    with tempfile.TemporaryDirectory() as tmp:
        src = Path(tmp) / "docs.html"
        src.write_text(doc, encoding="utf-8")
        subprocess.run(
            [
                browser, "--headless", "--disable-gpu", "--no-sandbox",
                f"--print-to-pdf={target}", "--no-pdf-header-footer",
                src.as_uri(),
            ],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    return target


EPUB_CSS = """
body { font-family: serif; line-height: 1.5; }
pre, code { font-family: monospace; font-size: 0.9em; }
pre { white-space: pre-wrap; }
table { border-collapse: collapse; }
th, td { border: 1px solid #999; padding: 0.3em 0.6em; }
"""

XHTML_WRAP = (
    '<?xml version="1.0" encoding="utf-8"?>\n'
    '<!DOCTYPE html>\n'
    '<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en">\n'
    "<head><title>{title}</title>"
    '<link rel="stylesheet" type="text/css" href="style.css"/></head>\n'
    "<body>\n{body}\n</body>\n</html>\n"
)


def build_epub(rendered: list[tuple[str, str, str]], out_dir: Path) -> Path:
    target = out_dir / f"{NAME}.epub"
    chapters = []
    for slug, title, fragment in rendered:
        chapters.append((slug, title, rewrite_links(fragment, epub=True)))

    manifest_items = []
    spine_items = []
    nav_lis = []
    for slug, title, _ in chapters:
        manifest_items.append(
            f'<item id="{slug}" href="{slug}.xhtml" media-type="application/xhtml+xml"/>'
        )
        spine_items.append(f'<itemref idref="{slug}"/>')
        nav_lis.append(f'<li><a href="{slug}.xhtml">{html.escape(title)}</a></li>')

    nav_xhtml = XHTML_WRAP.format(
        title="Contents",
        body="<nav epub:type='toc' xmlns:epub='http://www.idpf.org/2007/ops'>"
        f"<h1>Contents</h1><ol>{''.join(nav_lis)}</ol></nav>",
    )
    opf = f"""<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0"
         unique-identifier="bookid" xml:lang="en">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">reticulum-go-docs</dc:identifier>
    <dc:title>{html.escape(TITLE)}</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2026-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml"
          properties="nav"/>
    <item id="css" href="style.css" media-type="text/css"/>
    {''.join(manifest_items)}
  </manifest>
  <spine>
    <itemref idref="nav"/>
    {''.join(spine_items)}
  </spine>
</package>
"""
    container = """<?xml version="1.0" encoding="utf-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf"
              media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
"""

    with zipfile.ZipFile(target, "w") as zf:
        zf.writestr("mimetype", "application/epub+zip",
                    compress_type=zipfile.ZIP_STORED)
        zf.writestr("META-INF/container.xml", container, zipfile.ZIP_DEFLATED)
        zf.writestr("OEBPS/content.opf", opf, zipfile.ZIP_DEFLATED)
        zf.writestr("OEBPS/style.css", EPUB_CSS, zipfile.ZIP_DEFLATED)
        zf.writestr("OEBPS/nav.xhtml", nav_xhtml, zipfile.ZIP_DEFLATED)
        for slug, title, fragment in chapters:
            zf.writestr(
                f"OEBPS/{slug}.xhtml",
                XHTML_WRAP.format(title=html.escape(title), body=fragment),
                zipfile.ZIP_DEFLATED,
            )
    return target


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("out_dir", nargs="?", default=str(DOCS / "assets" / "downloads"))
    parser.add_argument("--require-pdf", action="store_true")
    args = parser.parse_args()

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    pages = nav_pages()
    rendered = render_pages(pages)

    for path in (build_zip(out_dir),
                 build_pdf(rendered, out_dir, args.require_pdf),
                 build_epub(rendered, out_dir)):
        if path is not None:
            size = os.path.getsize(path)
            print(f"wrote {path} ({size / 1024:.0f} KiB)")


if __name__ == "__main__":
    main()
