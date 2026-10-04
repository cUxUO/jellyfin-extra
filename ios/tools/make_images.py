#!/usr/bin/env python3
"""產生 App 圖示與啟動圖（純 Python，不需要影像函式庫）。

圖示：琥珀色底加深色播放三角形，與 Android 版導覽列頂端的標誌一致。iOS 會自行裁成圓角。
啟動圖：與 app 底色相同的深色畫面，讓 iOS 以原生解析度執行。
"""
import os
import struct
import zlib

ACCENT = (0xF2, 0xA9, 0x3B)
ON_ACCENT = (0x1A, 0x12, 0x06)
BG = (0x0E, 0x10, 0x14)
OUT = os.path.join(os.path.dirname(__file__), "..", "Resources")


def write_png(path, width, height, row_fn):
    raw = b"".join(b"\x00" + row_fn(y) for y in range(height))
    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b"")
    with open(path, "wb") as f:
        f.write(png)


def in_triangle(x, y, size):
    # 播放三角形：視覺置中略往右，對應 24 格座標的 (7,4)-(20,12)-(7,20)，整體縮到 60%
    s = size / 24.0
    ox = size * 0.2 + s * 0.6
    oy = size * 0.2
    k = s * 0.6
    ax, ay = ox + 7 * k, oy + 4 * k
    bx, by = ox + 20 * k, oy + 12 * k
    cx, cy = ox + 7 * k, oy + 20 * k
    def side(px, py, qx, qy, rx, ry):
        return (px - rx) * (qy - ry) - (qx - rx) * (py - ry)
    d1 = side(x, y, ax, ay, bx, by)
    d2 = side(x, y, bx, by, cx, cy)
    d3 = side(x, y, cx, cy, ax, ay)
    return not ((d1 < 0 or d2 < 0 or d3 < 0) and (d1 > 0 or d2 > 0 or d3 > 0))


def icon(size):
    n = 4  # 每像素 4×4 取樣做反鋸齒
    def row(y):
        out = bytearray()
        for x in range(size):
            hits = sum(in_triangle(x + (i + 0.5) / n, y + (j + 0.5) / n, size) for i in range(n) for j in range(n))
            a = hits / (n * n)
            out += bytes(round(ACCENT[c] * (1 - a) + ON_ACCENT[c] * a) for c in range(3))
        return bytes(out)
    return row


def solid(width):
    line = bytes(BG) * width
    return lambda y: line


if __name__ == "__main__":
    for name, size in [("AppIcon29x29", 29), ("AppIcon29x29@2x", 58), ("AppIcon40x40", 40),
                       ("AppIcon40x40@2x", 80), ("AppIcon76x76", 76), ("AppIcon76x76@2x", 152)]:
        write_png(os.path.join(OUT, name + ".png"), size, size, icon(size))
    write_png(os.path.join(OUT, "LaunchImage-Landscape~ipad.png"), 1024, 768, solid(1024))
    write_png(os.path.join(OUT, "LaunchImage-Landscape@2x~ipad.png"), 2048, 1536, solid(2048))
    print("ok")
