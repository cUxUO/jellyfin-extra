#!/usr/bin/env python3
"""產生 Android 的啟動圖示 PNG（純 Python，不需要影像函式庫）。

給 Android 7（API 24、25，例如 ZenPad）用：琥珀色圓角方塊加深色播放三角形，與 iOS 圖示
（ios/tools/make_images.py）一致。Android 8 以上用 res/mipmap-anydpi-v26/ic_launcher.xml 的
adaptive icon（向量），不讀這些 PNG。改圖示時兩邊一起改。
"""
import os
import struct
import zlib

ACCENT = (0xF2, 0xA9, 0x3B)
ON_ACCENT = (0x1A, 0x12, 0x06)
RES = os.path.join(os.path.dirname(__file__), "..", "android", "app", "src", "main", "res")
DENSITIES = {"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192}


def write_png(path, size, pixels):
    raw = b"".join(b"\x00" + bytes(pixels[y * size * 4:(y + 1) * size * 4]) for y in range(size))
    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b"")
    with open(path, "wb") as f:
        f.write(png)


def in_rounded_square(x, y, size):
    # 留 4% 邊，圓角半徑是方塊的 18%（接近 Android 7 系統 app 的圓角方形）
    m = size * 0.04
    r = (size - 2 * m) * 0.18
    lo, hi = m + r, size - m - r
    if x < m or x > size - m or y < m or y > size - m:
        return False
    dx = lo - x if x < lo else x - hi if x > hi else 0
    dy = lo - y if y < lo else y - hi if y > hi else 0
    return dx * dx + dy * dy <= r * r


def in_triangle(x, y, size):
    # 與 iOS 相同：24 格座標的 (7,4)-(20,12)-(7,20)，整體縮到 60%、略往右
    s = size / 24.0
    ox, oy, k = size * 0.2 + s * 0.6, size * 0.2, s * 0.6
    ax, ay, bx, by, cx, cy = ox + 7 * k, oy + 4 * k, ox + 20 * k, oy + 12 * k, ox + 7 * k, oy + 20 * k
    def side(px, py, qx, qy, rx, ry):
        return (px - rx) * (qy - ry) - (qx - rx) * (py - ry)
    d1, d2, d3 = side(x, y, ax, ay, bx, by), side(x, y, bx, by, cx, cy), side(x, y, cx, cy, ax, ay)
    return not ((d1 < 0 or d2 < 0 or d3 < 0) and (d1 > 0 or d2 > 0 or d3 > 0))


def icon(size):
    n = 4  # 每像素 4×4 取樣做反鋸齒
    px = bytearray()
    for y in range(size):
        for x in range(size):
            bg = tri = 0
            for i in range(n):
                for j in range(n):
                    sx, sy = x + (i + 0.5) / n, y + (j + 0.5) / n
                    if in_rounded_square(sx, sy, size):
                        bg += 1
                        tri += in_triangle(sx, sy, size)
            if bg == 0:
                px += bytes(4)
                continue
            a = tri / bg
            px += bytes(round(ACCENT[c] * (1 - a) + ON_ACCENT[c] * a) for c in range(3)) + bytes([round(255 * bg / (n * n))])
    return px


if __name__ == "__main__":
    for density, size in DENSITIES.items():
        d = os.path.join(RES, "mipmap-" + density)
        os.makedirs(d, exist_ok=True)
        write_png(os.path.join(d, "ic_launcher.png"), size, icon(size))
    print("ok")
