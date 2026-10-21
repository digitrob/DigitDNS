#!/usr/bin/env python3
"""Regenerate the DigitDNS launcher-icon assets from the vector artwork.

The icon is a textbook adaptive icon: a plain field in the background layer and
the emblem in the foreground, with the emblem fitted to the 66dp safe circle.
The previous build put the whole artwork in the background layer, full-bleed,
so the launcher mask cropped the emblem: a launcher shows only the inner 72dp
of the 108dp layer, the emblem spanned 89dp of it, and Samsung's One UI
squircle cut it further still.

Usage:
    pip install -r android/tools/requirements.txt
    python3 android/tools/generate_icons.py [path/to/file.svg]

The artwork is not in the repository (it is the customer's own file), so the
path to it is a parameter. Everything else is derived.
"""

import io
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
# Optional vendored copy of the dependencies, for a machine without pip access.
sys.path.insert(0, os.path.join(HERE, "pylibs"))

try:
    import pymupdf
    from PIL import Image, ImageDraw
    import numpy as np
except ImportError as exc:  # pragma: no cover - developer tooling only
    sys.exit(
        "Missing generator dependencies: %s\n"
        "Install them with: pip install -r %s" % (exc, os.path.join(HERE, "requirements.txt"))
    )

DEFAULT_SRC = "/Users/sergei/Downloads/file.svg"
RES = os.path.abspath(os.path.join(HERE, "..", "app", "src", "main", "res"))
PREVIEW_DIR = os.path.abspath(os.path.join(HERE, "..", "..", "scratch", "icon-preview"))

# Adaptive icon geometry, in dp. The layer is 108dp; the launcher shows the
# inner 72dp; the safe zone is the 66dp circle inside it.
LAYER_DP = 108
VISIBLE_DP = 72
SAFE_DP = 66

# The field the emblem sits on. It matches the app's own dark surface rather
# than the artwork's patches, so the launcher tile and the app's cards read as
# the same product.
FIELD_TOP = (0x12, 0x1A, 0x2A)
FIELD_BOTTOM = (0x07, 0x0D, 0x1A)

DENSITIES = [("mdpi", 1), ("hdpi", 1.5), ("xhdpi", 2), ("xxhdpi", 3), ("xxxhdpi", 4)]

# The emblem is this group of paths: path 6 is the shield's top rim (it sits
# among the background patches), paths 12+ are the shield body.
EMBLEM_RIM_PATH = 6
EMBLEM_BODY_FROM = 12


def parse_paths(svg):
    return re.findall(r"<path\b.*?</path>", svg, re.S)


def render_svg(body, size):
    """Render a fragment of the artwork to an RGBA image of the given size."""
    path = os.path.join(HERE, "_render.svg")
    with open(path, "w") as fh:
        fh.write('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">%s</svg>' % body)
    doc = pymupdf.open(path)
    pix = doc[0].get_pixmap(matrix=pymupdf.Matrix(size / 1024.0, size / 1024.0), alpha=True)
    img = Image.frombytes("RGBA", (pix.width, pix.height), pix.samples)
    doc.close()
    os.remove(path)
    return img


def png_bytes(img, **kw):
    buf = io.BytesIO()
    img.save(buf, "PNG", optimize=True, **kw)
    return buf.getvalue()


def write_png(img, path):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as fh:
        fh.write(png_bytes(img))
    print("  %-58s %7.1f KB" % (os.path.relpath(path, RES), os.path.getsize(path) / 1024))


def circle_mask(size):
    """An antialiased filled circle as an L-mode mask."""
    m = Image.new("L", (size * 4, size * 4), 0)
    ImageDraw.Draw(m).ellipse((0, 0, size * 4 - 1, size * 4 - 1), fill=255)
    return m.resize((size, size), Image.LANCZOS)


def field_image(size):
    """The background field: the app's own dark surface, top to bottom."""
    strip = Image.new("RGB", (1, size))
    px = strip.load()
    for y in range(size):
        t = y / float(max(size - 1, 1))
        px[0, y] = tuple(int(FIELD_TOP[i] + (FIELD_BOTTOM[i] - FIELD_TOP[i]) * t) for i in range(3))
    return strip.resize((size, size), Image.BILINEAR).convert("RGBA")


def fit_emblem(emblem, layer_px):
    """Scale the emblem so that every part of it stays inside the safe circle.

    The emblem is not a compact shape — its shield body reaches further out
    than its width suggests — so the scale is set by the farthest point from
    the tile's centre, not by the width. That keeps the whole silhouette, and
    not just the bright part of it, inside the 66dp circle every launcher
    mask respects.
    """
    target_radius = (SAFE_DP / 2.0) * layer_px / float(LAYER_DP)
    a = np.asarray(emblem)[:, :, 3]
    ys, xs = np.where(a > 8)
    centre_x = (emblem.width - 1) / 2.0
    centre_y = (emblem.height - 1) / 2.0
    radius = np.sqrt((xs - centre_x) ** 2 + (ys - centre_y) ** 2).max()
    scale = target_radius / float(radius)
    w = max(int(round(emblem.width * scale)), 1)
    h = max(int(round(emblem.height * scale)), 1)
    art = emblem.resize((w, h), Image.LANCZOS)
    a = np.asarray(art)[:, :, 3]
    ys, xs = np.where(a > 8)
    if len(xs):
        art = art.crop((int(xs.min()), int(ys.min()), int(xs.max()) + 1, int(ys.max()) + 1))
    return art


def emblem_layer(emblem, layer_px):
    """The emblem, in the artwork's own colours, centred in a 108dp layer."""
    art = fit_emblem(emblem, layer_px)
    layer = Image.new("RGBA", (layer_px, layer_px), (0, 0, 0, 0))
    layer.paste(art, ((layer_px - art.width) // 2, (layer_px - art.height) // 2), art)
    return layer


def monochrome_layer(emblem, layer_px):
    """The emblem as one colour, for the themed (monochrome) icon."""
    art = fit_emblem(emblem, layer_px)
    solid = Image.new("RGBA", art.size, (0, 0, 0, 255))
    solid.putalpha(art.getchannel("A"))
    layer = Image.new("RGBA", (layer_px, layer_px), (0, 0, 0, 0))
    layer.paste(solid, ((layer_px - art.width) // 2, (layer_px - art.height) // 2), solid)
    return layer


def write_background_vector():
    """The background layer: a vertical gradient field, as a vector."""
    path = os.path.join(RES, "drawable", "ic_launcher_background.xml")
    top = "#%02X%02X%02X" % FIELD_TOP
    bottom = "#%02X%02X%02X" % FIELD_BOTTOM
    xml = """<?xml version="1.0" encoding="utf-8"?>
<!-- The background layer of the launcher icon: a plain field for the emblem to
     sit on. The artwork used to fill this layer edge to edge, but a launcher
     shows only the inner 72dp of the 108dp layer, so a full-bleed artwork was
     cropped to a shape the platform never promised. -->
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    xmlns:aapt="http://schemas.android.com/aapt"
    android:width="108dp"
    android:height="108dp"
    android:viewportWidth="108"
    android:viewportHeight="108">
    <path android:pathData="M0,0h108v108h-108z">
        <aapt:attr name="android:fillColor">
            <gradient
                android:type="linear"
                android:startX="54"
                android:startY="0"
                android:endX="54"
                android:endY="108"
                android:startColor="%s"
                android:endColor="%s" />
        </aapt:attr>
    </path>
</vector>
""" % (top, bottom)
    with open(path, "w") as fh:
        fh.write(xml)
    print("  %-58s %7.1f KB" % (os.path.relpath(path, RES), os.path.getsize(path) / 1024))


def measure(emblem, layer_px=1024):
    """Report the geometry the launcher will actually see.

    The emblem is measured in the layer it is placed in, so the distances are
    the distances from the tile's centre — the only thing that decides whether
    a launcher's mask clips the subject.
    """
    art = emblem_layer(emblem, layer_px)
    a = np.asarray(art)
    alpha = a[:, :, 3]
    lum = 0.2126 * a[:, :, 0] + 0.7152 * a[:, :, 1] + 0.0722 * a[:, :, 2]
    mask = alpha > 8
    ys, xs = np.where(mask)
    dp = LAYER_DP / float(layer_px)
    centre = (layer_px - 1) / 2.0
    radius = np.sqrt((xs - centre) ** 2 + (ys - centre) ** 2).max() * dp
    bright = mask & (lum >= 60)
    by, bx = np.where(bright)
    brad = np.sqrt((bx - centre) ** 2 + (by - centre) ** 2).max() * dp
    return {
        "emblem_w": (xs.max() - xs.min() + 1) * dp,
        "emblem_h": (ys.max() - ys.min() + 1) * dp,
        "emblem_radius": radius,
        "bright_radius": brad,
    }


def preview(emblem, layer_px=1024):
    """Render what the launcher shows, under a circle and under a squircle."""
    os.makedirs(PREVIEW_DIR, exist_ok=True)
    field = field_image(layer_px)
    field.alpha_composite(emblem_layer(emblem, layer_px))

    box = int(layer_px * VISIBLE_DP / LAYER_DP)
    lo, hi = (layer_px - box) // 2, (layer_px + box) // 2
    for name, draw_mask in (
        ("circle", lambda d: d.ellipse((lo, lo, hi - 1, hi - 1), fill=255)),
        ("squircle", lambda d: d.rounded_rectangle((lo, lo, hi - 1, hi - 1), radius=int(box * 0.28), fill=255)),
    ):
        m = Image.new("L", (layer_px, layer_px), 0)
        draw_mask(ImageDraw.Draw(m))
        masked = field.copy()
        masked.putalpha(m)
        out = Image.new("RGBA", (layer_px, layer_px), (0, 0, 0, 255))
        out.alpha_composite(masked)
        out.convert("RGB").save(os.path.join(PREVIEW_DIR, "new_mask_%s.png" % name))
    print("  previews written to %s" % os.path.relpath(PREVIEW_DIR, os.path.join(HERE, "..", "..")))


def main():
    src = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_SRC
    if not os.path.exists(src):
        sys.exit("Artwork not found: %s\nPass the path to the SVG as the first argument." % src)

    svg = open(src).read()
    paths = parse_paths(svg)
    print("source: %s (%d paths)" % (src, len(paths)))

    emblem = render_svg(paths[EMBLEM_RIM_PATH] + "".join(paths[EMBLEM_BODY_FROM:]), 2048)
    emblem = emblem.resize((1024, 1024), Image.LANCZOS)
    a = np.asarray(emblem)[:, :, 3]
    ys, xs = np.where(a > 8)
    emblem = emblem.crop((int(xs.min()), int(ys.min()), int(xs.max()) + 1, int(ys.max()) + 1))
    print("emblem: %dx%d in artwork coordinates" % (emblem.width, emblem.height))

    m = measure(emblem)
    print("  emblem %.1f x %.1f dp, farthest point %.1f dp from the centre" % (
        m["emblem_w"], m["emblem_h"], m["emblem_radius"]))
    print("  the bright artwork reaches %.1f dp from the centre; the safe circle is %.0f dp" % (
        m["bright_radius"], SAFE_DP / 2.0))

    print("\nbackground layer (the field)")
    write_background_vector()

    print("\nforeground layer (the emblem, in colour)")
    for name, scale in DENSITIES:
        px = int(LAYER_DP * scale)
        write_png(emblem_layer(emblem, px), os.path.join(RES, "drawable-%s" % name, "ic_launcher_foreground.png"))

    print("\nmonochrome layer (the same emblem, one colour)")
    for name, scale in DENSITIES:
        px = int(LAYER_DP * scale)
        write_png(monochrome_layer(emblem, px), os.path.join(RES, "drawable-%s" % name, "ic_launcher_monochrome.png"))

    print("\nlegacy ic_launcher.png (48dp)")
    for name, scale in DENSITIES:
        px = int(48 * scale)
        img = field_image(px)
        art = fit_emblem(emblem, px)
        img.paste(art, ((px - art.width) // 2, (px - art.height) // 2), art)
        write_png(img.convert("RGB"), os.path.join(RES, "mipmap-%s" % name, "ic_launcher.png"))

    print("\nlegacy ic_launcher_round.png (48dp, circle)")
    for name, scale in DENSITIES:
        px = int(48 * scale)
        img = field_image(px)
        art = fit_emblem(emblem, px)
        img.paste(art, ((px - art.width) // 2, (px - art.height) // 2), art)
        img.putalpha(circle_mask(px))
        write_png(img, os.path.join(RES, "mipmap-%s" % name, "ic_launcher_round.png"))

    print("\npreviews")
    preview(emblem)
    print("done")


if __name__ == "__main__":
    main()
