#!/usr/bin/env python3
"""Render a captured terminal frame (ANSI text) to a PNG, cell by cell.

Usage: ansi2png.py frame.ansi out.png [cols rows]
When frame.layers.json and an img/ directory sit beside the frame, the pictures
are composited too: placed ones under the text, placeholder cells in place.
Handles SGR truecolor/256, bold, dim, reverse and underline with its own
colour. Block elements are drawn as rectangles so they join exactly;
everything else is drawn with Menlo. When frame.term sits beside the frame and
holds two colours, "#bg #fg", they stand for the terminal's own: the cells
that paint no background take them, as they would in that terminal.
"""
import re, sys, os, json, glob
from PIL import Image, ImageDraw, ImageFont

CW, CH, FS = 10, 21, 16
FONT = "/System/Library/Fonts/Menlo.ttc"
reg = ImageFont.truetype(FONT, FS, index=0)
bold = ImageFont.truetype(FONT, FS, index=1)
try:
    sym = ImageFont.truetype("/System/Library/Fonts/Apple Symbols.ttf", FS)
except Exception:
    sym = reg

def load_diacritics():
    for path in glob.glob(os.path.expanduser("~/go/pkg/mod/github.com/charmbracelet/x/ansi@*/kitty/graphics.go")):
        src = open(path, encoding="utf-8").read()
        i = src.find("var diacritics = []rune{")
        if i < 0: continue
        body = src[i:src.find("}", i)]
        cps = [int(x, 16) for x in re.findall(r"\\u([0-9A-Fa-f]{4})|\\U([0-9A-Fa-f]{8})", body) for x in [x[0] or x[1]]]
        if cps: return {cp: n for n, cp in enumerate(cps)}
    return {}
DIAC = load_diacritics()
PH = 0x10EEEE

def xterm(n):
    if n < 16:
        base = [(0,0,0),(205,49,49),(13,188,121),(229,229,16),(36,114,200),(188,63,188),(17,168,205),(229,229,229),(102,102,102),(241,76,76),(35,209,139),(245,245,67),(59,142,234),(214,112,214),(41,184,219),(255,255,255)]
        return base[n]
    if n < 232:
        n -= 16
        v = [0,95,135,175,215,255]
        return (v[n//36], v[(n//6)%6], v[n%6])
    g = 8 + (n-232)*10
    return (g,g,g)

ULCOL = re.compile(r'(?:^|(?<=;))58[:;]2[:;]{1,2}(\d+)[:;](\d+)[:;](\d+);?')

def parse(text):
    rows = [[]]
    fg, bg, b, d, rv = None, None, False, False, False
    ul, ulc = False, None
    i = 0
    tok = re.compile(r'\x1b\[([0-9;:]*)m|\x1b\[[0-9;?]*[A-Za-ln-zA-Z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\n|.', re.S)
    for m in tok.finditer(text):
        s = m.group(0)
        if m.group(1) is not None:
            params = m.group(1)
            # The underline's own colour is taken out first: its numbers are
            # not attributes.
            um = ULCOL.search(params)
            if um:
                ulc = tuple(int(x) for x in um.groups())
                params = ULCOL.sub('', params).strip(';')
                if not params: continue
            a = [int(x) if x else 0 for x in re.split('[;:]', params)] or [0]
            k = 0
            while k < len(a):
                c = a[k]
                if c == 0: fg=bg=None; b=d=rv=False; ul=False; ulc=None
                elif c == 4: ul = True
                elif c == 24: ul = False
                elif c == 59: ulc = None
                elif c == 1: b = True
                elif c == 2: d = True
                elif c == 22: b = d = False
                elif c == 7: rv = True
                elif c == 27: rv = False
                elif c == 39: fg = None
                elif c == 49: bg = None
                elif 30 <= c <= 37: fg = xterm(c-30)
                elif 90 <= c <= 97: fg = xterm(c-90+8)
                elif 40 <= c <= 47: bg = xterm(c-40)
                elif 100 <= c <= 107: bg = xterm(c-100+8)
                elif c in (38, 48):
                    if a[k+1] == 2: col = tuple(a[k+2:k+5]); k += 4
                    else: col = xterm(a[k+2]); k += 2
                    if c == 38: fg = col
                    else: bg = col
                k += 1
        elif s == "\n":
            rows.append([])
        elif s.startswith("\x1b"):
            pass
        else:
            if ord(s) in DIAC and rows[-1] and rows[-1][-1][0].startswith(chr(PH)):
                prev = rows[-1][-1]
                rows[-1][-1] = (prev[0] + s,) + prev[1:]
            else:
                rows[-1].append((s, fg, bg, b, d, rv, (ulc or True) if ul else None))
    return rows

def placeholder_extent(pic):
    # Pictures are drawn at 20x42 device pixels a cell in the tests.
    return pic.size[0] / 20, pic.size[1] / 42

def main():
    src, out = sys.argv[1], sys.argv[2]
    rows = parse(open(src, encoding="utf-8").read())
    cols = max(len(r) for r in rows)
    if len(sys.argv) > 4: cols, nrows = int(sys.argv[3]), int(sys.argv[4])
    else: nrows = len(rows)
    DEFBG, DEFFG = (20,20,24), (220,220,225)
    base = os.path.splitext(src)[0]
    if os.path.exists(base + ".term"):
        own = re.findall(r'#([0-9a-fA-F]{6})', open(base + ".term").read())
        if len(own) >= 2:
            DEFBG, DEFFG = (tuple(int(h[i:i+2], 16) for i in (0, 2, 4)) for h in own[:2])
    img = Image.new("RGB", (cols*CW, nrows*CH), DEFBG)
    dr = ImageDraw.Draw(img)
    imgdir = os.path.join(os.path.dirname(src), "img")
    cache = {}
    def picture(i):
        if i not in cache:
            p = os.path.join(imgdir, "%d.png" % i)
            cache[i] = Image.open(p).convert("RGBA") if os.path.exists(p) else None
        return cache[i]
    # pass 1: cell backgrounds
    for y, row in enumerate(rows[:nrows]):
        for x, (ch, fg, bg, b, d, rv, ul) in enumerate(row[:cols]):
            f, g = fg or DEFFG, bg or DEFBG
            if rv: g = f
            dr.rectangle([x*CW, y*CH, x*CW+CW-1, y*CH+CH-1], fill=g)
    # pass 2: pictures placed under the text
    lay = base + ".layers.json"
    if os.path.exists(lay):
        for w in (json.load(open(lay)) or []):
            pic = picture(w["id"])
            if pic is None: continue
            tile = pic.resize((w["cols"]*CW, w["rows"]*CH), Image.LANCZOS)
            img.paste(tile, (w["col"]*CW, w["row"]*CH), tile)
    dr = ImageDraw.Draw(img)
    # pass 3: text, and placeholder cells showing their slice of a picture
    for y, row in enumerate(rows[:nrows]):
        for x, (ch, fg, bg, b, d, rv, ul) in enumerate(row[:cols]):
            f, g = fg or DEFFG, bg or DEFBG
            if rv: f, g = g, f
            if d: f = tuple((a+c)//2 for a, c in zip(f, g))
            X, Y = x*CW, y*CH
            if ul: dr.rectangle([X, Y+CH-3, X+CW-1, Y+CH-2], fill=f if ul is True else ul)
            if ch.startswith(chr(PH)):
                marks = [DIAC.get(ord(c)) for c in ch[1:]]
                if len(marks) >= 2 and fg:
                    pic = picture(fg[0]*65536 + fg[1]*256 + fg[2])
                    if pic is not None:
                        # the picture was drawn for the cells it covers: find them
                        cells = pic.getbbox() and None
                        r, c = marks[0], marks[1]
                        pw, ph = placeholder_extent(pic)
                        cwp, chp = pic.size[0] / pw, pic.size[1] / ph
                        tile = pic.crop((int(c*cwp), int(r*chp), int((c+1)*cwp), int((r+1)*chp))).resize((CW, CH), Image.LANCZOS)
                        img.paste(tile, (X, Y), tile)
                continue
            if ch == " ": continue
            if ch == "█": dr.rectangle([X,Y,X+CW-1,Y+CH-1], fill=f)
            elif ch == "▀": dr.rectangle([X,Y,X+CW-1,Y+CH//2-1], fill=f)
            elif ch == "▄": dr.rectangle([X,Y+CH//2,X+CW-1,Y+CH-1], fill=f)
            elif ch == "▌": dr.rectangle([X,Y,X+CW//2-1,Y+CH-1], fill=f)
            elif ch == "▐": dr.rectangle([X+CW//2,Y,X+CW-1,Y+CH-1], fill=f)
            elif ch in "░▒▓":
                a = {"░":.25,"▒":.5,"▓":.75}[ch]
                col = tuple(int(c*a+gg*(1-a)) for c, gg in zip(f, g))
                dr.rectangle([X,Y,X+CW-1,Y+CH-1], fill=col)
            elif ch in "\ue0b6\ue0b4":
                if ch == "\ue0b6": dr.pieslice([X, Y, X+CW*2-1, Y+CH-1], 90, 270, fill=f)
                else: dr.pieslice([X-CW, Y, X+CW-1, Y+CH-1], 270, 90, fill=f)
            else:
                font = bold if b else reg
                if ord(ch[0]) > 0x2000 and font.getmask(ch[0]).getbbox() is None: font = sym
                dr.text((X, Y+1), ch[0], font=font, fill=f)
    img.save(out)
    print(out, img.size)
main()
