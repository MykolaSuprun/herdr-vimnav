#!/usr/bin/env python3
"""Convert a `tmux capture-pane -e` dump (SGR ANSI) into a styled HTML page
for headless-Chromium screenshotting, with DETERMINISTIC geometry: the page
is sized from the real character grid (DejaVu Sans Mono advance = 0.6023em),
so the screenshot can be cropped to an exact pixel box instead of resized
(which is what previously destroyed the aspect ratio).

Usage: ansi2html.py in.txt out.html [--cols N] [--rows N] [--font PX]
                                 [--pad PX] [--scale S]
Prints "<crop_w> <crop_h>" — device pixels to crop from the top-left of the
screenshot taken at --scale.
"""
import argparse
import html
import re

CSI = re.compile(r"\x1b\[([0-9;?]*)([A-Za-z])")
DEFAULT_FG = "#4c4f69"  # catppuccin latte text
DEFAULT_BG = "#eff1f5"  # catppuccin latte base
ADVANCE_EM = 0.6023  # DejaVu Sans Mono advance width, in em


def style_of(st):
    parts = []
    if st["faint"]:
        h = st["fg"] or DEFAULT_FG
        r, g, b = (int(h[1 + 2 * k:3 + 2 * k], 16) for k in range(3))
        parts.append(f"color:rgba({r},{g},{b},0.55)")
    else:
        parts.append("color:" + (st["fg"] or DEFAULT_FG))
    if st["bg"]:
        parts.append("background:" + st["bg"])
    if st["bold"]:
        parts.append("font-weight:700")
    return ";".join(parts)


def apply_sgr(st, params):
    ps = [p for p in params.split(";") if p != ""] or ["0"]
    j = 0
    while j < len(ps):
        p = int(ps[j])
        if p == 0:
            st.update(fg=None, bg=None, bold=False, faint=False)
        elif p == 1:
            st["bold"] = True
        elif p == 2:
            st["faint"] = True
        elif p == 22:
            st["bold"] = st["faint"] = False
        elif p == 39:
            st["fg"] = None
        elif p == 49:
            st["bg"] = None
        elif p in (38, 48) and j + 4 < len(ps) and ps[j + 1] == "2":
            rgb = "#%02x%02x%02x" % (int(ps[j + 2]), int(ps[j + 3]), int(ps[j + 4]))
            st["fg" if p == 38 else "bg"] = rgb
            j += 4
        elif p in (38, 48) and j + 2 < len(ps) and ps[j + 1] == "5":
            st["fg" if p == 38 else "bg"] = None
            j += 2
        j += 1


def render_line(line):
    runs, buf = [], []
    st = {"fg": None, "bg": None, "bold": False, "faint": False}

    def flush():
        nonlocal buf
        if buf:
            runs.append((dict(st), "".join(buf)))
            buf = []

    i = 0
    while i < len(line):
        ch = line[i]
        if ch == "\x1b":
            m = CSI.match(line, i)
            if not m:
                buf.append(html.escape(ch))
                i += 1
                continue
            if m.group(2) == "m":
                flush()
                apply_sgr(st, m.group(1))
            i = m.end()
            continue
        buf.append(html.escape(ch))
        i += 1
    flush()
    return "".join(f'<span style="{style_of(s)}">{t}</span>' for s, t in runs if t)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("src")
    ap.add_argument("dst")
    ap.add_argument("--cols", type=int, default=0)
    ap.add_argument("--rows", type=int, default=0)
    ap.add_argument("--font", type=int, default=14)
    ap.add_argument("--pad", type=int, default=22)
    ap.add_argument("--scale", type=float, default=3.0)
    ap.add_argument("--line-height", type=float, default=1.06)
    args = ap.parse_args()

    text = open(args.src, encoding="utf-8", errors="replace").read()
    lines = text.split("\n")
    plain = [re.sub(CSI, "", l) for l in lines]
    cols = args.cols or max((len(l) for l in plain), default=80)
    rows = args.rows or len(lines)

    body = "\n".join(render_line(l) for l in lines)
    page = (
        "<!doctype html><meta charset='utf-8'>"
        "<body style='margin:0;background:%s'>"
        "<pre style=\"margin:0;padding:%dpx;background:%s;color:%s;"
        "font-family:'DejaVu Sans Mono','Liberation Mono',monospace;"
        "font-size:%dpx;line-height:%s;display:inline-block;"
        "white-space:pre\">%s</pre>"
        % (DEFAULT_BG, args.pad, DEFAULT_BG, DEFAULT_FG,
           args.font, args.line_height, body)
    )
    with open(args.dst, "w", encoding="utf-8") as f:
        f.write(page)

    advance = ADVANCE_EM * args.font
    pitch = args.line_height * args.font
    # Page box in CSS px, generously rounded up so chromium never clips.
    css_w = int(cols * advance + 2 * args.pad) + 24
    css_h = int(rows * pitch + 2 * args.pad) + 24
    # Exact content crop in device pixels.
    crop_w = round((cols * advance + 2 * args.pad) * args.scale)
    crop_h = round((rows * pitch + 2 * args.pad) * args.scale)
    print(css_w, css_h, crop_w, crop_h)


if __name__ == "__main__":
    main()