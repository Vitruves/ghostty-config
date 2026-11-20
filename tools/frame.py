"""Frame a screen the picture interface drew as a Ghostty window: a title bar
in the screen's own ground, rounded corners and a soft shadow, scaled for a
README. Usage: frame.py in.png out.png [width]"""
import sys

from PIL import Image, ImageDraw, ImageFilter

src, dst = sys.argv[1], sys.argv[2]
width = int(sys.argv[3]) if len(sys.argv) > 3 else 1600

screen = Image.open(src).convert("RGB")
ground = screen.getpixel((4, 4))
bar, pad, radius, margin = 64, 28, 26, 56

w, h = screen.width + 2 * pad, screen.height + bar + pad
win = Image.new("RGB", (w, h), ground)
win.paste(screen, (pad, bar))
d = ImageDraw.Draw(win)
for i, c in enumerate([(255, 95, 87), (254, 188, 46), (40, 200, 64)]):
    x, y = 32 + i * 40, bar // 2
    d.ellipse((x - 12, y - 12, x + 12, y + 12), fill=c)

mask = Image.new("L", (w, h), 0)
ImageDraw.Draw(mask).rounded_rectangle((0, 0, w - 1, h - 1), radius, fill=255)

out = Image.new("RGBA", (w + 2 * margin, h + 2 * margin), (0, 0, 0, 0))
shadow = Image.new("RGBA", out.size, (0, 0, 0, 0))
ImageDraw.Draw(shadow).rounded_rectangle((margin, margin + 14, margin + w, margin + h + 14), radius, fill=(0, 0, 0, 110))
out = Image.alpha_composite(out, shadow.filter(ImageFilter.GaussianBlur(22)))
out.paste(win, (margin, margin), mask)

# A hairline round the window, as macOS draws one: light on a dark window, so
# it stands out from a dark page where the shadow cannot, faint dark on a
# light one. Thick enough to survive the scaling below.
dark = sum(ground) / 3 < 128
edge = Image.new("RGBA", out.size, (0, 0, 0, 0))
ImageDraw.Draw(edge).rounded_rectangle(
    (margin, margin, margin + w - 1, margin + h - 1), radius,
    outline=(255, 255, 255, 64) if dark else (0, 0, 0, 34), width=max(2, round(width / 400)) + 1)
out = Image.alpha_composite(out, edge)

scale = width / out.width
out = out.resize((width, round(out.height * scale)), Image.LANCZOS)
out.save(dst, optimize=True)
