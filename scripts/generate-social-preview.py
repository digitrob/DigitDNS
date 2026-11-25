#!/usr/bin/env python3
"""Render the 1280x640 DigitDNS social card from the shipped app tile.

Run from the repository root:

    python3 scripts/generate-social-preview.py

The output is used by the GitHub repository social preview and by the dashboard's
Open Graph metadata. It deliberately has no dependency on a headless browser.
"""

from __future__ import annotations

import argparse
import math
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

ROOT = Path(__file__).resolve().parent.parent
TILE = ROOT / "web" / "img" / "icon-512.png"
DEFAULT_OUT = ROOT / "docs" / "digitdns-social-preview.png"
OG_OUT = ROOT / "web" / "img" / "og-image.png"

# GitHub renders the repository card at 2:1 (1280x640). Keeping the canvas at
# that size avoids the centre crop that clipped the footer on a 1200x630 card.
W, H = 1280, 640
BG_TOP = (5, 9, 20)
BG_BOTTOM = (12, 26, 54)
CYAN = (103, 232, 249)
MINT = (153, 246, 228)
TEXT = (241, 245, 249)
BODY = (203, 213, 225)
MUTED = (148, 163, 184)


def font(size: int, weight: str = "regular") -> ImageFont.FreeTypeFont:
    candidates = [
        ("/System/Library/Fonts/Avenir Next.ttc", {"Bold": 0, "Heavy": 8, "Medium": 5, "Regular": 7}),
        ("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", {}),
        ("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", {}),
    ]
    for path, indexes in candidates:
        try:
            if indexes:
                return ImageFont.truetype(path, size, index=indexes.get(weight, 7))
            return ImageFont.truetype(path, size)
        except OSError:
            continue
    return ImageFont.load_default()


def vertical_gradient() -> Image.Image:
    image = Image.new("RGB", (W, H), BG_TOP)
    draw = ImageDraw.Draw(image)
    for y in range(H):
        t = y / (H - 1)
        color = tuple(round(BG_TOP[i] + (BG_BOTTOM[i] - BG_TOP[i]) * t) for i in range(3))
        draw.line([(0, y), (W, y)], fill=color)
    return image.convert("RGBA")


def radial(image: Image.Image, center: tuple[int, int], radius: int, color: tuple[int, int, int], alpha: int) -> Image.Image:
    glow = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    draw = ImageDraw.Draw(glow)
    x, y = center
    draw.ellipse((x - radius, y - radius, x + radius, y + radius), fill=(*color, alpha))
    glow = glow.filter(ImageFilter.GaussianBlur(radius // 2))
    return Image.alpha_composite(image, glow)


def add_atmosphere(image: Image.Image) -> Image.Image:
    image = radial(image, (240, 120), 360, (34, 211, 238), 64)
    image = radial(image, (1080, 640), 420, (37, 99, 235), 72)
    image = radial(image, (900, -60), 340, (124, 58, 237), 34)
    return image


def add_grid(image: Image.Image) -> Image.Image:
    layer = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)
    for x in range(0, W, 64):
        draw.line([(x, 0), (x, H)], fill=(148, 233, 255, 7), width=1)
    for y in range(0, H, 64):
        draw.line([(0, y), (W, y)], fill=(148, 233, 255, 7), width=1)
    # A few brighter grid intersections keep the surface from looking flat.
    for x in range(0, W, 128):
        for y in range(0, H, 128):
            draw.ellipse((x - 1, y - 1, x + 1, y + 1), fill=(148, 233, 255, 30))
    return Image.alpha_composite(image, layer)


def add_rings(image: Image.Image) -> None:
    """Concentric signal rings and a few nodes behind the app tile."""
    layer = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)
    cx, cy = 246, 320
    for radius, alpha in ((190, 48), (232, 34), (280, 22), (336, 14)):
        draw.ellipse((cx - radius, cy - radius, cx + radius, cy + radius), outline=(103, 232, 249, alpha), width=2)
    for angle, radius in ((-34, 190), (18, 232), (52, 280), (-72, 280)):
        x = cx + math.cos(math.radians(angle)) * radius
        y = cy + math.sin(math.radians(angle)) * radius
        draw.ellipse((x - 5, y - 5, x + 5, y + 5), fill=(103, 232, 249, 170))
        draw.ellipse((x - 11, y - 11, x + 11, y + 11), outline=(103, 232, 249, 60), width=1)
    image.alpha_composite(layer.filter(ImageFilter.GaussianBlur(0.4)))


def add_text_backdrop(image: Image.Image) -> Image.Image:
    """Darken the right side so thin type stays readable over the grid."""
    shade = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    draw = ImageDraw.Draw(shade)
    ramp_start, ramp_end, peak = 360, 620, 150
    for x in range(ramp_start, W):
        t = min(1.0, (x - ramp_start) / (ramp_end - ramp_start))
        draw.line([(x, 0), (x, H)], fill=(3, 8, 19, round(peak * t)))
    return Image.alpha_composite(image, shade)


def pill(layer: ImageDraw.ImageDraw, x: int, y: int, width: int) -> None:
    height = 42
    layer.rounded_rectangle((x, y, x + width, y + height), radius=21, fill=(255, 255, 255, 12), outline=(103, 232, 249, 80), width=1)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    parser.add_argument("--og", type=Path, default=OG_OUT)
    args = parser.parse_args()

    image = add_atmosphere(vertical_gradient())
    image = add_grid(image)
    add_rings(image)
    image = add_text_backdrop(image)

    # Soft cyan halo, then the shipped tile.
    halo = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    hd = ImageDraw.Draw(halo)
    hd.rounded_rectangle((80, 154, 416, 490), radius=72, fill=(34, 211, 238, 45))
    halo = halo.filter(ImageFilter.GaussianBlur(52))
    image = Image.alpha_composite(image, halo)

    tile = Image.open(TILE).convert("RGBA").resize((296, 296), Image.LANCZOS)
    shadow = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    sd = ImageDraw.Draw(shadow)
    sd.rounded_rectangle((84, 168, 408, 492), radius=64, fill=(0, 0, 0, 150))
    shadow = shadow.filter(ImageFilter.GaussianBlur(28))
    image = Image.alpha_composite(image, shadow)
    image.alpha_composite(tile, (98, 182))

    # Translucent chrome is drawn on its own layer: ImageDraw replaces alpha on
    # RGBA pixels instead of blending, so compositing keeps the fills subtle.
    chrome = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    cd = ImageDraw.Draw(chrome)
    cd.rounded_rectangle((456, 126, 682, 170), radius=22, fill=(103, 232, 249, 20), outline=(103, 232, 249, 95), width=1)
    for x, width in ((452, 150), (614, 148), (774, 172)):
        pill(cd, x, 372, width)
    image = Image.alpha_composite(image, chrome)

    draw = ImageDraw.Draw(image)

    # Eyebrow.
    draw.ellipse((472, 143, 480, 151), fill=CYAN)
    draw.text((490, 134), "SELF-HOSTED DNS", font=font(19, "Heavy"), fill=CYAN)

    # Wordmark with a soft shadow for lift.
    shadow_text = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    st = ImageDraw.Draw(shadow_text)
    st.text((450, 186), "DigitDNS", font=font(100, "Heavy"), fill=(0, 0, 0, 190))
    shadow_text = shadow_text.filter(ImageFilter.GaussianBlur(12))
    image = Image.alpha_composite(image, shadow_text)
    draw = ImageDraw.Draw(image, "RGBA")
    draw.text((448, 180), "DigitDNS", font=font(100, "Heavy"), fill=(255, 255, 255))

    draw.text((452, 306), "Your own DNS. Your rules. No VPN.", font=font(36, "Medium"), fill=TEXT)

    for x, label in ((452, "Smart DNS"), (614, "SNI Proxy"), (774, "Ad Blocking")):
        draw.ellipse((x + 15, 390, x + 23, 398), fill=CYAN)
        draw.text((x + 33, 381), label, font=font(19, "Medium"), fill=TEXT)

    # Footer.
    draw.line((452, 468, 1214, 468), fill=(103, 232, 249, 70), width=1)
    draw.text((452, 486), "github.com/digitrob/DigitDNS", font=font(23, "Medium"), fill=MINT)
    draw.text((452, 528), "AGPL-3.0  ·  Linux  ·  Android", font=font(20, "Regular"), fill=BODY)

    out = image.convert("RGB")
    args.out.parent.mkdir(parents=True, exist_ok=True)
    out.save(args.out, "PNG", optimize=True)
    if args.og:
        args.og.parent.mkdir(parents=True, exist_ok=True)
        out.save(args.og, "PNG", optimize=True)
    print(f"wrote {args.out} and {args.og}")


if __name__ == "__main__":
    main()
