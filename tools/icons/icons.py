#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = ["pyyaml", "pillow", "resvg-py", "rich"]
# ///
"""Cad-lite's icon set — the mechanical checks, the Go codegen and the review sheets.

Run it through `just`: `just icons-check`, `icons-gen`, `icons-sheet`, `icons-variants`,
`icons-audit`, `icons-dupes` (and `uv run tools/icons/icons.py adopt` for Lucide rows).

The registry is `assets/icons/icons.yaml`; the files are `assets/icons/<name>.svg`. Sheets are
rendered with resvg (resvg-py) and only the alpha channel is kept, because the app draws an icon
as a monochrome alpha mask tinted with a theme colour (ui/icon.go, rasterised by oksvg). The two
rasterisers agree on the plain stroked paths the spec allows; an icon that carries a colour in
the file is discarded the same way by both.
"""

from __future__ import annotations

import argparse
import os
import re
import sys
import xml.etree.ElementTree as ET
from dataclasses import dataclass
from io import BytesIO
from pathlib import Path

import resvg_py
import yaml
from PIL import Image, ImageDraw, ImageFont
from rich.console import Console

ROOT = Path(__file__).resolve().parent.parent.parent
ICONS = ROOT / "assets" / "icons"
REGISTRY = ICONS / "icons.yaml"
# Competing takes on an icon that is not settled yet. They live beside the set rather than in it,
# so `check` never sees them; one of them is promoted to `assets/icons/<name>.svg` and the rest
# are deleted the moment a choice is made.
VARIANTS = ICONS / "variants"
# The sheets are written beside the icons so they are one click away from the file being drawn.
# Git ignores the directory.
OUT = ICONS / "preview"

SVG_NS = "{http://www.w3.org/2000/svg}"
# The spec, in one place. Lucide's grid, because most of the modify/view/edit rows are adopted Lucide.
BOX = "0 0 24 24"
STROKE = "2"
# The envelope every icon's ink has to stay inside: 1.5px of optical padding on a 24px grid. Ink
# includes the stroke, so a circle's radius is at most 9.5 and a round cap ends 1px past its point.
LIVE = (1.5, 22.5)
TOL = 0.05
SHAPES_MAX = 4
BANNED_TAGS = {
    "text", "image", "use", "defs", "style", "mask", "clipPath", "filter",
    "linearGradient", "radialGradient", "pattern", "script", "animate",
    "animateTransform", "animateMotion", "set", "switch", "foreignObject",
}
BANNED_ATTRS = {"style", "class", "opacity", "fill-opacity", "stroke-opacity"}

# The two palettes. "dark" is Cad-lite's shell (ui/theme.go: uiBg, uiText, uiMuted, uiOn); "light" is
# a sheet-only variant. `fg` is what toolbar icons are tinted with, `accent` the active state.
THEMES = {
    "dark": {"bg": "#181c21", "fg": "#d6dbe1", "muted": "#8c96a2", "accent": "#60a5fa"},
    "light": {"bg": "#f4f6f8", "fg": "#1f2933", "muted": "#627080", "accent": "#2563eb"},
}
SIZES = (16, 24, 64)

# Lucide is the source of `adopted:` rows. It is fetched once from the npm registry (pinned) into
# _tmp/ (gitignored) unless LUCIDE_DIR points at an existing `icons/` directory.
LUCIDE_VERSION = "1.57.0"
LUCIDE_TARBALL = f"https://registry.npmjs.org/lucide-static/-/lucide-static-{LUCIDE_VERSION}.tgz"

console = Console()


@dataclass
class Icon:
    name: str
    goal: str
    category: str
    source: str = "custom"
    keywords: tuple[str, ...] = ()

    @property
    def path(self) -> Path:
        return ICONS / f"{self.name}.svg"

    @property
    def adopted(self) -> bool:
        return self.source.startswith("adopted:")

    @property
    def variant(self) -> str:
        """The Lucide icon this row was adopted from."""
        return self.source.split(":", 1)[-1] if ":" in self.source else ""


def foreign(icon: Icon | None) -> bool:
    """Geometry we did not choose: an icon adopted from Lucide."""
    return bool(icon) and icon.adopted


def lucide_dir() -> Path:
    """Lucide's `icons/` directory, downloaded on first use."""
    if env := os.environ.get("LUCIDE_DIR"):
        return Path(env)
    target = ROOT / "_tmp" / "lucide-static"
    icons = target / "package" / "icons"
    if not icons.exists():
        import tarfile
        import urllib.request

        target.mkdir(parents=True, exist_ok=True)
        console.print(f"fetching {LUCIDE_TARBALL}")
        with urllib.request.urlopen(LUCIDE_TARBALL, timeout=120) as resp:
            data = resp.read()
        with tarfile.open(fileobj=BytesIO(data), mode="r:gz") as tar:
            tar.extractall(target, filter="data")
    return icons


def registry() -> tuple[dict[str, Icon], list[str]]:
    """Parse `icons.yaml`. An entry's value is either the goal string or a mapping."""
    doc = yaml.safe_load(REGISTRY.read_text()) or {}
    icons: dict[str, Icon] = {}
    for category, entries in (doc.get("categories") or {}).items():
        for name, body in (entries or {}).items():
            if isinstance(body, str):
                body = {"goal": body}
            icons[name] = Icon(
                name=name,
                goal=body.get("goal", ""),
                category=category,
                source=body.get("source", "custom"),
                keywords=tuple(body.get("keywords", ())),
            )
    return icons, list(doc.get("canon") or [])


# --- the Go side ----------------------------------------------------------------------------

# The generated constants. Written rather than hand-kept, because a registry row and a constant
# that drift apart is a blank square nobody notices.
GENERATED = ICONS / "names_gen.go"


def camel(name: str) -> str:
    """`modify-move` -> `ModifyMove`: the registry key as a Go identifier."""
    return "".join(part.capitalize() for part in name.split("-"))


def generated(icons: dict[str, Icon]) -> str:
    """The whole of `assets/icons/names_gen.go`, from the rows that have a file."""
    ours = sorted((i for i in icons.values() if i.path.exists()), key=lambda i: i.name)
    consts = "\n\n".join(f'\t// {camel(i.name)}: {i.goal}\n\t{camel(i.name)} Name = "{i.name}"' for i in ours)
    names = "\n".join(f"\t{camel(i.name)}," for i in ours)
    return f'''// Code generated by tools/icons/icons.py codegen; DO NOT EDIT.

package icons

const (
{consts}
)

// All lists every icon that has a file, in name order.
var All = []Name{{
{names}
}}
'''


# --- rendering ------------------------------------------------------------------------------


def mask(svg: str, px: int) -> Image.Image:
    """The alpha mask resvg produces, which is all the app keeps of an SVG."""
    png = resvg_py.svg_to_bytes(svg_string=svg, width=px, height=px)
    return Image.open(BytesIO(bytes(png))).convert("RGBA").getchannel("A")


def tinted(svg: str, px: int, fg: str, bg: str) -> Image.Image:
    """The mask painted in `fg` over `bg` — what the app actually draws."""
    tile = Image.new("RGB", (px, px), bg)
    tile.paste(Image.new("RGB", (px, px), fg), (0, 0), mask(svg, px))
    return tile


def font(size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.load_default(size=size)


def normalise(svg: str) -> str:
    """A shipped icon rewritten as one of ours: our root, its shapes, nothing else.

    Lucide's files carry `width`, `height` and a `class`, and the stroke settings live on the
    root. Adopting one keeps the geometry and drops everything the spec forbids, so the copy
    passes `check` exactly as a drawn icon does.
    """
    keep = {"d", "cx", "cy", "r", "rx", "ry", "x", "y", "x1", "x2", "y1", "y2",
            "width", "height", "points", "fill", "fill-rule"}
    shapes = []
    for el in ET.fromstring(svg):
        tag = el.tag.removeprefix(SVG_NS)
        attrs = "".join(f' {k}="{v}"' for k, v in el.items() if k in keep)
        shapes.append(f"<{tag}{attrs}/>")
    return (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"'
        ' stroke="currentColor" stroke-width="2" stroke-linecap="round"'
        f' stroke-linejoin="round">{"".join(shapes)}</svg>\n'
    )


# --- check ----------------------------------------------------------------------------------


def check(icons: dict[str, Icon]) -> tuple[list[str], list[str]]:
    """The mechanical rules. Everything catchable without looking at a picture.

    Returns violations and, separately, the registry rows still waiting to be drawn — those are
    the worklist, not a failure, or the set could never be added to one category at a time.
    """
    problems: list[str] = []
    on_disk = {p.stem for p in ICONS.glob("*.svg")}
    listed = set(icons)

    pending = sorted(listed - on_disk)
    for name in sorted(on_disk - listed):
        problems.append(f"I02 {name}.svg: a file nobody registered — add it or delete it")
    for name, icon in sorted(icons.items()):
        if not icon.goal:
            problems.append(f"I03 {name}: no goal — say what the icon has to communicate")
        if icon.source != "custom" and not icon.adopted:
            problems.append(f"I11 {name}: source {icon.source!r} — use custom or adopted:<lucide-name>")
        if icon.adopted and not (lucide_dir() / f"{icon.variant}.svg").exists():
            problems.append(f"I11 {name}: Lucide {LUCIDE_VERSION} has no {icon.variant}.svg")

    # Every file on disk, registered or not — an unregistered file is still going to be drawn.
    for path in sorted(ICONS.glob("*.svg")):
        problems += check_one(path.stem, path.read_text(), foreign(icons.get(path.stem)))
    return problems, pending


def check_one(name: str, svg: str, foreign: bool = False) -> list[str]:
    """`foreign` is geometry that is not ours to judge: an icon adopted from Lucide. The rules about
    how many shapes it took, where the ink sits and how many decimals it carries are dropped for
    those, because an adopted icon is coherent by construction. Everything the app actually needs
    still applies, because it is drawn as one alpha mask like anything else."""
    out: list[str] = []
    try:
        root = ET.fromstring(svg)
    except ET.ParseError as err:
        return [f"I04 {name}: not parseable — {err}"]

    if root.get("viewBox") != BOX:
        out.append(f"I05 {name}: viewBox is {root.get('viewBox')!r}, must be {BOX!r}")
    if root.get("width") or root.get("height"):
        out.append(f"I05 {name}: drop width/height from the root — the element sizes the icon")

    shapes = 0
    for el in root.iter():
        tag = el.tag.removeprefix(SVG_NS)
        if tag in BANNED_TAGS:
            out.append(f"I06 {name}: <{tag}> — the app keeps only an alpha mask, this cannot survive")
        if tag != "svg":
            shapes += 1
        for attr in BANNED_ATTRS.intersection(el.keys()):
            out.append(f"I06 {name}: {attr}= on <{tag}> — put it on the element in Go, not here")
    if not foreign and shapes > SHAPES_MAX:
        out.append(f"I07 {name}: {shapes} shapes, at most {SHAPES_MAX} — simplify")

    for el in root.iter():
        if (w := el.get("stroke-width")) and w != STROKE:
            out.append(f"I08 {name}: stroke-width {w}, must be {STROKE}")
        if (f := el.get("fill")) and f not in ("none", "currentColor"):
            out.append(f"I08 {name}: fill={f!r} — the mask discards it; use none or currentColor")
        if (s := el.get("stroke")) and s != "currentColor":
            out.append(f"I08 {name}: stroke={s!r} — must be currentColor")
        for number in [] if foreign else re.findall(r"-?\d+\.\d{3,}", el.get("d", "")):
            out.append(f"I09 {name}: {number} — two decimals is plenty")
            break

    # Where the ink actually lands, measured off the mask rather than off the path data, so a
    # stroke's own width counts.
    px = 96
    scale = 24 / px
    if box := mask(svg, px).point(lambda v: 255 if v > 24 else 0).getbbox():
        x0, y0, x1, y1 = (v * scale for v in box)
        if not foreign and (x0 < LIVE[0] - TOL or y0 < LIVE[0] - TOL
                            or x1 > LIVE[1] + TOL or y1 > LIVE[1] + TOL):
            out.append(
                f"I10 {name}: ink reaches ({x0:.1f},{y0:.1f})-({x1:.1f},{y1:.1f}), "
                f"must stay inside {LIVE[0]}-{LIVE[1]}"
            )
        if (x1 - x0) < 10 and (y1 - y0) < 10:
            out.append(f"I10 {name}: {x1 - x0:.1f}x{y1 - y0:.1f} — too small, it will read as a dot")
    else:
        out.append(f"I10 {name}: renders empty")
    return out


# --- sheets ---------------------------------------------------------------------------------


def rows(icons: list[Icon], accent: bool) -> Image.Image:
    """One row per icon: every size, in both themes. The 16px column is where icons fail."""
    label_w, pad, gap = 190, 12, 18
    cell = max(SIZES)
    row_h = cell + gap
    block = sum(s + gap for s in SIZES) + gap
    width = label_w + block * 2 + pad * 2
    height = pad * 2 + 26 + row_h * len(icons)

    sheet = Image.new("RGB", (width, height), "#2a2f37")
    draw = ImageDraw.Draw(sheet)
    tint_key = "accent" if accent else "fg"
    for n, (theme, cols) in enumerate(THEMES.items()):
        x = label_w + pad + block * n
        draw.rectangle([x - gap // 2, pad + 22, x + block - gap, height - pad], fill=cols["bg"])
        draw.text((x, pad), f"{theme} / {tint_key}", font=font(12), fill="#8c96a2")

    for r, icon in enumerate(icons):
        y = pad + 26 + r * row_h
        draw.text((pad, y + cell // 2 - 12), icon.name, font=font(13), fill="#d6dbe1")
        # The default font has no em dash, and a goal line is full of them.
        draw.text((pad, y + cell // 2 + 4), icon.goal.replace("—", "-")[:34],
                  font=font(10), fill="#8c96a2")
        svg = icon.path.read_text()
        for n, cols in enumerate(THEMES.values()):
            x = label_w + pad + block * n
            for size in SIZES:
                sheet.paste(tinted(svg, size, cols[tint_key], cols["bg"]), (x, y + (cell - size) // 2))
                x += size + gap
    return sheet


def takes(concepts: dict[str, list[Path]], siblings: list[Icon]) -> Image.Image:
    """One row per concept, the takes across it, each at 48 and 16px.

    The row above is the concept's already-settled siblings, so a take is chosen against the
    family rather than against the other takes.
    """
    colours = THEMES["dark"]
    label_w, pad, cw, ch = 150, 14, 96, 82
    columns = max(len(v) for v in concepts.values())
    width = label_w + max(columns, len(siblings)) * cw + pad * 2
    height = pad * 2 + ch * (len(concepts) + bool(siblings))

    sheet = Image.new("RGB", (width, height), colours["bg"])
    draw = ImageDraw.Draw(sheet)
    y = pad
    if siblings:
        draw.text((pad, y + 26), "already settled", font=font(12), fill=colours["accent"])
        for n, icon in enumerate(siblings):
            x = label_w + n * cw
            sheet.paste(tinted(icon.path.read_text(), 48, colours["muted"], colours["bg"]), (x, y))
            draw.text((x, y + 54), icon.name, font=font(10), fill=colours["muted"])
        y += ch
        draw.line([pad, y - 8, width - pad, y - 8], fill="#2a2f37")

    for name, files in concepts.items():
        draw.text((pad, y + 26), name, font=font(13), fill=colours["fg"])
        for n, file in enumerate(sorted(files)):
            x = label_w + n * cw
            svg = file.read_text()
            sheet.paste(tinted(svg, 48, colours["fg"], colours["bg"]), (x, y))
            sheet.paste(tinted(svg, 16, colours["fg"], colours["bg"]), (x + 54, y + 16))
            draw.text((x, y + 54), file.stem.rsplit("-", 1)[-1], font=font(11),
                      fill=colours["accent"])
        y += ch
    return sheet


def zoom(files: list[Path]) -> Image.Image:
    """One concept's takes, large enough to see the joins, with the true sizes under each.

    The row-per-concept sheet is for choosing between takes; this is for looking at one.
    """
    colours = THEMES["dark"]
    big, pad, cw = 128, 16, 156
    sheet = Image.new("RGB", (pad * 2 + cw * len(files), big + 96), colours["bg"])
    draw = ImageDraw.Draw(sheet)
    for n, file in enumerate(sorted(files)):
        x = pad + n * cw
        svg = file.read_text()
        sheet.paste(tinted(svg, big, colours["fg"], colours["bg"]), (x, pad))
        for m, size in enumerate(SIZES):
            sheet.paste(tinted(svg, size, colours["fg"], colours["bg"]),
                        (x + m * 30, pad + big + 14))
        draw.text((x, pad + big + 66), file.stem, font=font(12), fill=colours["accent"])
    return sheet


def grid(icons: list[Icon], theme: str, px: int = 48, cols: int = 6) -> Image.Image:
    """A contact sheet. One question only: which of these breaks the family?"""
    colours = THEMES[theme]
    cw, ch = px + 46, px + 34
    sheet = Image.new("RGB", (cols * cw, -(-len(icons) // cols) * ch), colours["bg"])
    draw = ImageDraw.Draw(sheet)
    for n, icon in enumerate(icons):
        x, y = (n % cols) * cw, (n // cols) * ch
        sheet.paste(tinted(icon.path.read_text(), px, colours["fg"], colours["bg"]),
                    (x + 23, y + 8))
        draw.text((x + cw // 2, y + px + 16), icon.name, font=font(10),
                  fill=colours["muted"], anchor="ma")
    return sheet


def dhash(icon: Icon) -> int:
    """A perceptual hash of the mask, for spotting an icon you have already drawn."""
    small = mask(icon.path.read_text(), 32).resize((9, 8))
    px = small.load()
    bits = 0
    for y in range(8):
        for x in range(8):
            bits = bits << 1 | (px[x, y] > px[x + 1, y])
    return bits


# --- commands -------------------------------------------------------------------------------


def save(image: Image.Image, name: str) -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    out = OUT / name
    image.save(out)
    console.print(f"[green]{out.relative_to(ROOT)}[/] {image.width}x{image.height}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="cmd", required=True)
    sub.add_parser("check", help="the mechanical rules — registry, spec, ink")
    sub.add_parser("codegen", help="write assets/icons/names_gen.go from the registry")
    p = sub.add_parser("sheet", help="a review sheet for a few icons, every size, both themes")
    p.add_argument("names", nargs="*", help="icon names; default is the canon strip")
    p.add_argument("--category", help="the whole category, siblings and all")
    p.add_argument("--accent", action="store_true", help="tint with the accent token instead")
    p = sub.add_parser("adopt", help="copy a Lucide icon in-tree as one of ours")
    p.add_argument("names", nargs="*", help="registry names; default is every unfilled adopted: row")
    p = sub.add_parser("variants", help="the competing takes in assets/icons/variants/")
    p.add_argument("--category", help="whose settled siblings to show above them")
    p = sub.add_parser("audit", help="the whole set, one contact sheet per category")
    p.add_argument("--theme", default="dark", choices=list(THEMES))
    sub.add_parser("dupes", help="pairs that look alike")
    args = parser.parse_args()

    icons, canon = registry()

    # Take sheets are scaffolding: once the winner is promoted and the takes deleted, the pictures
    # of them go too. Only the sheets that describe the set itself are kept.
    if not any(VARIANTS.glob("*.svg")):
        for stale in OUT.glob("variants*.png"):
            stale.unlink()

    if args.cmd == "codegen":
        GENERATED.write_text(generated(icons))
        console.print(f"[green]{GENERATED.relative_to(ROOT)}[/]")
        return 0

    if args.cmd == "check":
        problems, pending = check(icons)
        # A row added without regenerating is a variant that does not exist yet, so the drift is
        # a mechanical problem like any other rather than something to notice at the next build.
        current = GENERATED.read_text() if GENERATED.exists() else ""
        if current != generated(icons):
            problems.append("[red]names_gen.go is out of date — run `just icons-gen`[/]")
        for line in problems:
            console.print(line)
        if pending:
            console.print(f"[yellow]to draw ({len(pending)})[/]: {', '.join(pending)}")
        console.print(f"{len(icons)} registered, {len(pending)} to draw, "
                      f"[{'red' if problems else 'green'}]{len(problems)} problem(s)[/]")
        return 1 if problems else 0

    if args.cmd == "adopt":
        names = args.names or [n for n, i in icons.items() if i.adopted and not i.path.exists()]
        for name in names:
            icon = icons[name]
            source = lucide_dir() / f"{icon.variant}.svg"
            if not source.exists():
                console.print(f"[red]{name}: Lucide {LUCIDE_VERSION} has no {source.name}[/]")
                return 1
            icon.path.write_text(normalise(source.read_text()))
            console.print(f"[green]{icon.path.name}[/] from {source.name}")
        return 0

    drawn = {n: i for n, i in icons.items() if i.path.exists()}

    if args.cmd == "sheet":
        names = (args.names
                 or [n for n, i in drawn.items() if i.category == args.category]
                 or canon or list(drawn))
        chosen = [drawn[n] for n in names if n in drawn]
        if not chosen:
            console.print("[red]nothing to draw[/]")
            return 1
        # The canon strip rides along on every sheet — coherence is judged against a fixed point,
        # never against the previous batch.
        reference = [drawn[n] for n in canon if n in drawn and n not in names]
        save(rows(reference + chosen, args.accent), "sheet.png")
        return 0

    if args.cmd == "variants":
        concepts: dict[str, list[Path]] = {}
        for file in sorted(VARIANTS.glob("*.svg")):
            concepts.setdefault(file.stem.rsplit("-", 1)[0], []).append(file)
        if not concepts:
            console.print(f"[red]no takes in {VARIANTS.relative_to(ROOT)}[/]")
            return 1
        # The spec applies to a take too — no point choosing one that cannot ship.
        problems = [
            p
            for name, files in concepts.items()
            for f in files
            for p in check_one(f.stem, f.read_text(), foreign(icons.get(name)))
        ]
        for line in problems:
            console.print(line)
        siblings = [i for i in drawn.values()
                    if i.category == args.category and i.name not in concepts]
        save(takes(concepts, sorted(siblings, key=lambda i: i.name)), "variants.png")
        for name, files in concepts.items():
            save(zoom(files), f"variants-{name}.png")
        return 1 if problems else 0

    if args.cmd == "audit":
        for category in sorted({i.category for i in drawn.values()}):
            members = sorted((i for i in drawn.values() if i.category == category),
                             key=lambda i: i.name)
            save(grid(members, args.theme), f"audit-{category}.png")
        return 0

    hashes = {n: dhash(i) for n, i in drawn.items()}
    names = sorted(hashes)
    found = False
    for a in range(len(names)):
        for b in range(a + 1, len(names)):
            # Several rows may be adopted from the same Lucide icon on purpose: one icon serving
            # several concepts, not two that look alike.
            first, second = drawn[names[a]], drawn[names[b]]
            if first.variant and first.variant == second.variant:
                continue
            distance = bin(hashes[names[a]] ^ hashes[names[b]]).count("1")
            if distance <= 6:
                found = True
                console.print(f"[yellow]{names[a]} ~ {names[b]}[/] (distance {distance})")
    if not found:
        console.print("[green]no near-duplicates[/]")
    return 0


if __name__ == "__main__":
    sys.exit(main())
