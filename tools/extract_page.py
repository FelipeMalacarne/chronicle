#!/usr/bin/env python3
"""
extract_page.py - Convert a rendered ironman.guide HTML page into clean Markdown.

Usage:
    python3 -I tools/extract_page.py INPUT.html -o OUTPUT.md [--url https://ironman.guide/quests]
    python3 -I tools/extract_page.py --batch "DIR/*.html" --out-dir data/raw/ironman-guide/pages

Why this exists
---------------
ironman.guide is a Next.js (App Router / RSC) site. The main guide (/guide) ships a
structured props object in the RSC payload, but the companion pages (/quests, /gear,
/bosses, /hcim, /articles/*, ...) only ship rendered React elements. So we take the
server-rendered HTML and walk the DOM:

  * keep <main> only (site header, top nav, footer and in-page <nav> blocks are dropped)
  * drop scripts, styles, svg, images, ad/affiliate blocks
  * h1-h6 -> #..######, p/leaf-div -> paragraphs, ul/ol -> lists (nested), table -> GFM table,
    details/summary -> bold summary + body, strong/em/code/a -> inline markdown
  * links are kept and made absolute (relative "/x" -> https://ironman.guide/x); wiki links
    (oldschool.runescape.wiki) are preserved verbatim
  * React's "<!-- -->" text-splitting comments are ignored so sentences re-join naturally
  * a small YAML front matter (title, source url, description, author, fetched_at) is prepended

Dependency: beautifulsoup4 (pip install beautifulsoup4). No markdownify needed: a hand-written
walker gives better control over the Tailwind "card" div soup than a generic converter.

Downloaded HTML is untrusted input: run with `python3 -I`, never exec anything from it.
"""
from __future__ import annotations

import argparse
import datetime as _dt
import glob
import os
import re
import sys

try:
    from bs4 import BeautifulSoup, Comment, NavigableString, Tag
except ImportError:  # pragma: no cover
    sys.exit("beautifulsoup4 is required: pip install beautifulsoup4")

SITE = "https://ironman.guide"

DROP_TAGS = {"script", "style", "svg", "noscript", "iframe", "canvas", "template", "button", "input", "select", "form"}
BLOCK_TAGS = {
    "div", "section", "article", "main", "header", "aside", "figure", "figcaption", "details",
    "blockquote", "pre", "hr", "p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "table",
    "dl", "dt", "dd", "summary", "address", "footer", "nav",
}
HEADING = {"h1": 1, "h2": 2, "h3": 3, "h4": 4, "h5": 5, "h6": 6}
AD_HOST_MARKERS = ("creatorcrafted.com", "utm_source=ironman", "ref=pbssnyzn")


# --------------------------------------------------------------------------- helpers

def _abs(href: str) -> str:
    if not href:
        return href
    if href.startswith("/"):
        return SITE + href
    return href


def _clean_inline(s: str) -> str:
    s = s.replace("\xa0", " ")
    s = re.sub(r"[ \t\r\f\v]+", " ", s)
    return s


def _esc_cell(s: str) -> str:
    return s.replace("|", "\\|").replace("\n", " ").strip()


def _strip_bullets(s: str) -> str:
    # the site often hard-codes "- " or "• " inside <li>
    return re.sub(r"^\s*[-•·]\s+", "", s)


# --------------------------------------------------------------------------- pre-clean

def preclean(soup: BeautifulSoup) -> Tag | None:
    for t in soup.find_all(DROP_TAGS):
        t.decompose()
    for c in soup.find_all(string=lambda x: isinstance(x, Comment)):
        c.extract()
    main = soup.find("main")
    if main is None:
        main = soup.body or soup
    # nested <main><main> on some pages -> use innermost that still has headings
    inner = main.find("main")
    if inner is not None:
        main = inner
    # in-page navs (breadcrumbs, phase jump bars, "On this page", prev/next) and footers
    for t in main.find_all(["nav", "footer"]):
        t.decompose()
    # affiliate / ad blocks: climb from the affiliate anchor to the smallest ancestor that
    # also holds the "Affiliate link" / "Ad" disclaimer, then remove it.
    for a in list(main.find_all("a", href=True)):
        if a.attrs is None or a.parent is None:  # already removed with an earlier ad block
            continue
        href = a.get("href") or ""
        if not any(m in href for m in AD_HOST_MARKERS):
            continue
        node = a
        for _ in range(8):
            if node.parent is None or node.parent is main:
                break
            node = node.parent
            txt = node.get_text(" ", strip=True)
            if "Affiliate link" in txt or re.search(r"\bAd\b", txt):
                break
        if node is not main and node.parent is not None:
            node.decompose()
    # breadcrumb / prev-next anchors that live outside <nav> ("← Home", "Next: ... →")
    for a in list(main.find_all("a")):
        if a.parent is None:
            continue
        t = a.get_text(" ", strip=True)
        if re.match(r"^(←|→|↑|Back to )", t) or re.search(r"(→|←)$", t):
            a.decompose()
    # standalone affiliate disclaimers that survived
    for p in list(main.find_all("p")):
        if "Affiliate link" in p.get_text() or "earns a commission" in p.get_text():
            p.decompose()
    return main


# --------------------------------------------------------------------------- inline

def inline(node) -> str:
    """Render a node (and children) as inline markdown text."""
    if isinstance(node, Comment):
        return ""
    if isinstance(node, NavigableString):
        return _clean_inline(str(node))
    if not isinstance(node, Tag):
        return ""
    name = node.name
    if name in DROP_TAGS:
        return ""
    if name == "br":
        return "\n"
    if name == "img":
        alt = (node.get("alt") or "").strip()
        src = node.get("src") or ""
        if src.startswith("data:") or not alt:
            return ""
        return f"![{alt}]({_abs(src)})"
    inner = _join_inline([(c, inline(c)) for c in node.children])
    if name == "a":
        href = _abs(node.get("href") or "")
        text = inner.strip()
        if not text:
            return ""
        if not href or href.startswith("#") or href.startswith("javascript:"):
            return text
        return f"[{text}]({href})"
    if name in ("strong", "b"):
        t = inner.strip()
        return f"**{t}**" if t else ""
    if name in ("em", "i"):
        t = inner.strip()
        return f"*{t}*" if t else ""
    if name in ("code", "kbd"):
        t = inner.strip()
        return f"`{t}`" if t else ""
    if name in ("del", "s"):
        t = inner.strip()
        return f"~~{t}~~" if t else ""
    if name in ("sup",):
        return inner
    # span and other inline containers: keep text, add spacing guard
    return inner


def _join_inline(parts) -> str:
    """Join rendered inline parts, inserting a space where two *elements* butt against each
    other with letters on both sides (e.g. <span>Robetop</span><span>Important</span>) or where a
    block child (p/div inside a table cell) would otherwise glue sentences together."""
    out = ""
    for node, text in parts:
        if not text:
            continue
        is_block = isinstance(node, Tag) and node.name in BLOCK_TAGS
        if out and (is_block or isinstance(node, Tag)) and re.search(r"[A-Za-z0-9)\]*]$", out) \
                and re.match(r"^[A-Za-z0-9(\[*]", text):
            out += " "
        elif out and is_block and not out.endswith((" ", "\n")) and not text.startswith((" ", "\n")):
            out += " "
        elif out and isinstance(node, Tag) and re.search(r"[.!?;:,]$", out) and re.match(r"^[A-Z(]", text):
            out += " "  # <span>Sentence.</span><span>Next sentence.</span>
        out += text
    return out


def is_inline_only(tag: Tag) -> bool:
    for c in tag.children:
        if isinstance(c, Tag) and c.name in BLOCK_TAGS:
            return False
    return True


# --------------------------------------------------------------------------- blocks

class Renderer:
    def __init__(self):
        self.out: list[str] = []

    # emit a block separated by blank lines
    def emit(self, text: str):
        text = text.strip("\n")
        if text.strip():
            self.out.append(text)

    def render(self, node, depth=0):
        if isinstance(node, Comment):
            return
        if isinstance(node, NavigableString):
            t = _clean_inline(str(node)).strip()
            if t:
                self.emit(t)
            return
        if not isinstance(node, Tag):
            return
        name = node.name
        if name in DROP_TAGS:
            return
        if name in HEADING:
            t = inline(node).strip()
            t = re.sub(r"\s+", " ", t)
            if t:
                self.emit("#" * HEADING[name] + " " + t)
            return
        if name == "hr":
            self.emit("---")
            return
        if name == "pre":
            self.emit("```\n" + node.get_text().rstrip() + "\n```")
            return
        if name == "table":
            self.emit(self.table(node))
            return
        if name in ("ul", "ol"):
            self.emit(self.list(node, ordered=(name == "ol")))
            return
        if name == "blockquote":
            sub = Renderer()
            for c in node.children:
                sub.render(c)
            body = "\n\n".join(sub.out)
            self.emit("\n".join("> " + ln for ln in body.splitlines()))
            return
        if name == "details":
            summ = node.find("summary")
            if summ is not None:
                self.emit("**" + inline(summ).strip() + "**")
                summ.extract()
            for c in node.children:
                self.render(c)
            return
        if name == "dl":
            lines = []
            for c in node.children:
                if isinstance(c, Tag) and c.name == "dt":
                    lines.append("**" + inline(c).strip() + "**")
                elif isinstance(c, Tag) and c.name == "dd":
                    lines.append(": " + inline(c).strip())
            self.emit("\n".join(lines))
            return
        if name == "p" or (name in BLOCK_TAGS and is_inline_only(node)):
            t = inline(node)
            t = "\n".join(ln.strip() for ln in t.split("\n"))
            t = re.sub(r" {2,}", " ", t).strip()
            if t:
                self.emit(t)
            return
        # generic block container: recurse. Consecutive inline children (text, span, a,
        # strong...) are grouped into one paragraph; block children are rendered on their own.
        # If every child renders to a single short line (card label/value pairs), keep them
        # tight (one block, newline-separated).
        sub = Renderer()
        run: list = []

        def flush():
            if run:
                t = _join_inline([(x, inline(x)) for x in run])
                t = re.sub(r"[ \t]+", " ", t).strip()
                if t:
                    sub.emit(t)
                run.clear()

        for c in node.children:
            if isinstance(c, Tag) and (c.name in BLOCK_TAGS or c.name in DROP_TAGS):
                flush()
                sub.render(c, depth + 1)
            else:
                run.append(c)
        flush()
        if not sub.out:
            return
        if (
            len(sub.out) >= 2
            and all("\n" not in b and not b.startswith(("#", "- ", "|", "1. ")) and len(b) <= 60 for b in sub.out)
        ):
            self.emit("  \n".join(sub.out))
        else:
            self.out.extend(sub.out)

    def list(self, node: Tag, ordered: bool, indent: int = 0) -> str:
        lines = []
        items = [c for c in node.children if isinstance(c, Tag) and c.name == "li"]
        n = 0
        for li in items:
            n += 1
            # split li into inline head + nested lists / block children
            head_parts, tail_blocks = [], []
            for c in li.children:
                if isinstance(c, Tag) and c.name in ("ul", "ol"):
                    tail_blocks.append(self.list(c, c.name == "ol", indent + 1))
                elif isinstance(c, Tag) and c.name in BLOCK_TAGS and not is_inline_only(c):
                    sub = Renderer()
                    sub.render(c)
                    head_parts.append(" ".join(b.replace("\n", " ") for b in sub.out))
                else:
                    head_parts.append(inline(c))
            head = re.sub(r"\s+", " ", " ".join(head_parts)).strip()
            head = _strip_bullets(head)
            # numbered span hard-coded inside <li>: "1. Foo" -> keep numbering, make ordered
            m = re.match(r"^(\d+)\.\s+(.*)$", head)
            if m and not ordered:
                ordered = True
            if m:
                head = m.group(2)
                n = int(m.group(1))
            if not head and not tail_blocks:
                continue
            marker = f"{n}." if ordered else "-"
            pad = "  " * indent
            lines.append(f"{pad}{marker} {head}".rstrip())
            for tb in tail_blocks:
                lines.append(tb)
        return "\n".join(lines)

    def table(self, node: Tag) -> str:
        rows = []
        for tr in node.find_all("tr"):
            cells = [c for c in tr.children if isinstance(c, Tag) and c.name in ("td", "th")]
            if not cells:
                continue
            rows.append(([_esc_cell(inline(c)) for c in cells], all(c.name == "th" for c in cells)))
        if not rows:
            return ""
        width = max(len(r[0]) for r in rows)
        out = []
        header, body = None, rows
        if rows[0][1]:
            header, body = rows[0][0], rows[1:]
        if header is None:
            header = [""] * width
        header = header + [""] * (width - len(header))
        out.append("| " + " | ".join(header) + " |")
        out.append("|" + "|".join([" --- "] * width) + "|")
        for cells, _ in body:
            cells = cells + [""] * (width - len(cells))
            out.append("| " + " | ".join(cells) + " |")
        cap = node.find("caption")
        if cap is not None:
            out.insert(0, "**" + inline(cap).strip() + "**\n")
        return "\n".join(out)


# --------------------------------------------------------------------------- page

def meta(soup: BeautifulSoup, name: str) -> str:
    t = soup.find("meta", attrs={"name": name}) or soup.find("meta", attrs={"property": name})
    return (t.get("content") or "").strip() if t else ""


def html_to_markdown(html: str, url: str = "", fetched_at: str = "") -> str:
    soup = BeautifulSoup(html, "html.parser")
    title = (soup.title.string if soup.title and soup.title.string else "").strip()
    title = re.sub(r"\s*\|\s*ironman\.guide\s*$", "", title)
    desc = meta(soup, "description")
    authors = [m.get("content", "") for m in soup.find_all("meta", attrs={"name": "author"})]
    canonical = soup.find("link", rel="canonical")
    if not url and canonical is not None:
        url = canonical.get("href", "")
    main = preclean(soup)
    r = Renderer()
    r.render(main)
    blocks = r.out
    # "12." ordinal rendered as its own block right before a heading -> fold into the heading
    merged: list[str] = []
    # article breadcrumb row ("[Home](...)/[Guides](...)/Skill Guide") is not inside <nav>
    if blocks and re.match(r"^\[Home\]\(https://ironman\.guide/?\)\s*/", blocks[0]):
        blocks = blocks[1:]
    i = 0
    while i < len(blocks):
        b = blocks[i]
        if re.fullmatch(r"\d+\s*\.", b) and i + 1 < len(blocks) and blocks[i + 1].startswith("#"):
            nxt = blocks[i + 1]
            hashes, rest = nxt.split(" ", 1)
            merged.append(f"{hashes} {b.replace(' ', '')} {rest}")
            i += 2
            continue
        merged.append(b)
        i += 1
    body = "\n\n".join(merged)
    body = re.sub(r"\n{3,}", "\n\n", body)
    fm = ["---", f'title: "{title.replace(chr(34), chr(39))}"', f"source: {url}"]
    if desc:
        fm.append(f'description: "{desc.replace(chr(34), chr(39))}"')
    if authors:
        fm.append("author: " + ", ".join(a for a in authors if a))
    fm.append(f"fetched_at: {fetched_at or _dt.date.today().isoformat()}")
    fm.append("license_note: personal use; content (c) OzirisLoL / ironman.guide, keep attribution")
    fm.append("---")
    return "\n".join(fm) + "\n\n" + body.strip() + "\n"


def slug_from_filename(path: str) -> str:
    base = os.path.basename(path)
    base = re.sub(r"\.html?$", "", base)
    base = re.sub(r"^(page|article|ch)-", lambda m: "articles-" if m.group(1) == "article" else "", base)
    return base


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("input", nargs="?", help="HTML file")
    ap.add_argument("-o", "--output", help="Markdown output file (default: stdout)")
    ap.add_argument("--url", default="", help="source URL to record in front matter")
    ap.add_argument("--batch", help="glob of HTML files; writes <slug>.md into --out-dir")
    ap.add_argument("--out-dir", default=".")
    ap.add_argument("--fetched-at", default="")
    args = ap.parse_args(argv)

    if args.batch:
        os.makedirs(args.out_dir, exist_ok=True)
        files = sorted(glob.glob(args.batch))
        for f in files:
            slug = slug_from_filename(f)
            url = SITE + "/" + slug.replace("articles-", "articles/")
            with open(f, encoding="utf-8", errors="replace") as fh:
                md = html_to_markdown(fh.read(), url=url, fetched_at=args.fetched_at)
            out = os.path.join(args.out_dir, slug + ".md")
            with open(out, "w", encoding="utf-8") as fh:
                fh.write(md)
            print(f"{f} -> {out} ({len(md)} chars)")
        return 0

    if not args.input:
        ap.error("input file or --batch required")
    with open(args.input, encoding="utf-8", errors="replace") as fh:
        md = html_to_markdown(fh.read(), url=args.url, fetched_at=args.fetched_at)
    if args.output:
        os.makedirs(os.path.dirname(os.path.abspath(args.output)), exist_ok=True)
        with open(args.output, "w", encoding="utf-8") as fh:
            fh.write(md)
    else:
        sys.stdout.write(md)
    return 0


if __name__ == "__main__":
    sys.exit(main())
