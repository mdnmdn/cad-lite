#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = ["pillow", "resvg-py", "rich"]
# ///
"""Cad-lite's application icon: the macOS .icns, the Windows .ico and the runtime window icon.

    just appicon          # build every output from assets/appicon/*.svg
    just appicon-sheet    # review sheet into assets/appicon/preview/ (gitignored)
    uv run tools/icons/appicon.py check

The sources are `assets/appicon/appicon.svg` and `appicon-small.svg` (used up to SMALL_MAX px).
Everything is rasterised with resvg and written in pure Python, so it runs on any OS; the .icns
is written by Pillow, not iconutil. The spec is in assets/appicon/README.md.
"""

from __future__ import annotations

import argparse
import hashlib
import struct
import sys
from io import BytesIO
from pathlib import Path

import resvg_py
from PIL import Image, IcnsImagePlugin, ImageDraw

ROOT = Path(__file__).resolve().parent.parent.parent
DIR = ROOT / "assets" / "appicon"
MAIN, SMALL = DIR / "appicon.svg", DIR / "appicon-small.svg"
STAMP = DIR / "source.sha256"
PREVIEW = DIR / "preview"
ICONS = ROOT / "assets" / "icons"

SMALL_MAX = 32
ICO_SIZES = (16, 24, 32, 48, 64, 128, 256)
WINDOW_SIZES = (16, 32, 48, 64, 128, 256)
# iconset pixel sizes: 16, 16@2x, 32, 32@2x, 128, 128@2x, 256, 256@2x, 512, 512@2x.
ICNS_SIZES = (16, 32, 64, 128, 256, 512, 1024)
ICNS = DIR / "AppIcon.icns"
ICO = DIR / "AppIcon.ico"

BACKGROUNDS = {"light": "#f4f6f8", "dark": "#181c21"}


def source_hash() -> str:
    h = hashlib.sha256()
    for p in (MAIN, SMALL):
        h.update(p.read_bytes())
    return h.hexdigest()


def render(px: int) -> Image.Image:
    """The icon at px pixels: the simplified drawing for small sizes."""
    svg = (SMALL if px <= SMALL_MAX else MAIN).read_text()
    png = resvg_py.svg_to_bytes(svg_string=svg, width=px, height=px)
    return Image.open(BytesIO(bytes(png))).convert("RGBA")


def png_bytes(img: Image.Image) -> bytes:
    buf = BytesIO()
    img.save(buf, "PNG", optimize=True)
    return buf.getvalue()


def write_ico(path: Path, sizes: tuple[int, ...]) -> None:
    """An .ico holding one PNG-compressed image per size (Vista+; Pillow only resizes one)."""
    blobs = [png_bytes(render(s)) for s in sizes]
    out = bytearray(struct.pack("<HHH", 0, 1, len(sizes)))
    offset = 6 + 16 * len(sizes)
    for s, b in zip(sizes, blobs):
        out += struct.pack("<BBBBHHII", s % 256, s % 256, 0, 0, 1, 32, len(b), offset)
        offset += len(b)
    for b in blobs:
        out += b
    path.write_bytes(bytes(out))


def icns_chunks(data: bytes) -> list[tuple[str, int]]:
    """The (type, length) of each chunk of an .icns."""
    out, i = [], 8
    while i < len(data):
        kind, size = struct.unpack(">4sI", data[i : i + 8])
        out.append((kind.decode("latin1"), size))
        i += size
    return out


def add_1x_small(path: Path) -> None:
    """Pillow writes no 16 px and 32 px @1x entries (icp4, icp5); append them as PNG chunks."""
    data = bytearray(path.read_bytes())
    for kind, px in (("icp4", 16), ("icp5", 32)):
        blob = png_bytes(render(px))
        data += kind.encode() + struct.pack(">I", 8 + len(blob)) + blob
    data[4:8] = struct.pack(">I", len(data))
    path.write_bytes(bytes(data))


def build() -> None:
    DIR.mkdir(parents=True, exist_ok=True)
    for s in WINDOW_SIZES:
        (DIR / f"window-{s}.png").write_bytes(png_bytes(render(s)))
    write_ico(ICO, ICO_SIZES)
    imgs = {s: render(s) for s in ICNS_SIZES}
    big = imgs[1024]
    big.save(ICNS, append_images=[im for s, im in imgs.items() if s != 1024])
    add_1x_small(ICNS)
    STAMP.write_text(source_hash() + "\n")
    print(f"built {ICNS.name}, {ICO.name}, window-*.png in {DIR.relative_to(ROOT)}")


def check() -> int:
    bad: list[str] = []
    for p in (MAIN, SMALL):
        if not p.exists():
            bad.append(f"missing {p.name}")
        elif 'viewBox="0 0 1024 1024"' not in p.read_text():
            bad.append(f"{p.name}: viewBox must be 0 0 1024 1024")
    if bad:
        return report(bad)
    want = [ICNS, ICO] + [DIR / f"window-{s}.png" for s in WINDOW_SIZES]
    for p in want:
        if not p.exists():
            bad.append(f"missing {p.relative_to(ROOT)}")
    if bad:
        return report(bad)
    if not STAMP.exists() or STAMP.read_text().strip() != source_hash():
        bad.append("outputs are stale vs the SVGs: run `just appicon`")
    for s in WINDOW_SIZES:
        if Image.open(DIR / f"window-{s}.png").size != (s, s):
            bad.append(f"window-{s}.png has the wrong size")
    with Image.open(ICO) as ico:
        got = tuple(sorted(ico.info.get("sizes", ())))
        if got != tuple((s, s) for s in ICO_SIZES):
            bad.append(f"AppIcon.ico sizes {got}")
    kinds = {k for k, _ in icns_chunks(ICNS.read_bytes())}
    need = {"icp4", "icp5", "ic07", "ic08", "ic09", "ic10", "ic11", "ic12", "ic13", "ic14"}
    if need - kinds:
        bad.append(f"AppIcon.icns lacks {sorted(need - kinds)}")
    with Image.open(ICNS) as icns:
        if icns.size[0] < 512:
            bad.append("AppIcon.icns does not read back at 512 px or more")
    return report(bad)


def report(bad: list[str]) -> int:
    for b in bad:
        print("FAIL", b)
    if not bad:
        print("appicon: ok")
    return 1 if bad else 0


def ui_icon(name: str, px: int, fg: str, bg: str) -> Image.Image:
    svg = (ICONS / f"{name}.svg").read_text().replace("currentColor", fg)
    png = resvg_py.svg_to_bytes(svg_string=svg, width=px, height=px)
    tile = Image.new("RGB", (px, px), bg)
    tile.paste(Image.open(BytesIO(bytes(png))).convert("RGBA"), (0, 0), Image.open(BytesIO(bytes(png))).convert("RGBA"))
    return tile


def sheet() -> None:
    PREVIEW.mkdir(parents=True, exist_ok=True)
    sizes = (16, 24, 32, 48, 64, 128, 256)
    pad = 16
    ui_names = ["draw-line", "draw-circle", "modify-move", "snap-ortho"]
    fgs = {"light": "#1f2933", "dark": "#d6dbe1"}
    row_h = 256 + 2 * pad
    width = sum(s + pad for s in sizes) + pad + 4 * (40 + pad) + 300
    sh = Image.new("RGB", (width, row_h * 2 + 2 * pad), "white")
    for r, (theme, bg) in enumerate(BACKGROUNDS.items()):
        y0 = r * row_h
        ImageDraw.Draw(sh).rectangle([0, y0, width, y0 + row_h], fill=bg)
        x = pad
        for s in sizes:
            im = render(s)
            sh.paste(im, (x, y0 + pad + (256 - s) // 2), im)
            x += s + pad
        x += pad
        # beside UI icons at the toolbar size, and the app icon at the same 24 px.
        im = render(24)
        sh.paste(im, (x, y0 + pad), im)
        x += 24 + pad
        for n in ui_names:
            t = ui_icon(n, 24, fgs[theme], bg)
            sh.paste(t, (x, y0 + pad))
            x += 24 + pad
        # 1:1 and the Dock-style 128 on a shared line, plus 16 enlarged 8x to see the pixels.
        im16 = render(16).resize((128, 128), Image.NEAREST)
        sh.paste(im16, (x + 40, y0 + pad), im16)
    out = PREVIEW / "sheet.png"
    sh.save(out)
    big = render(1024)
    bg = Image.new("RGBA", big.size, BACKGROUNDS["light"])
    bg.alpha_composite(big)
    bg.convert("RGB").resize((512, 512), Image.LANCZOS).save(PREVIEW / "main.png")
    print(f"wrote {out.relative_to(ROOT)} and main.png")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("cmd", choices=["build", "sheet", "check"])
    cmd = ap.parse_args().cmd
    if cmd == "build":
        build()
    elif cmd == "sheet":
        sheet()
    else:
        return check()
    return 0


if __name__ == "__main__":
    sys.exit(main())
