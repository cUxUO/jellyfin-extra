#!/usr/bin/env python3
"""產生轉碼伺服器的圖示 server/cmd/xcode/icon.ico（純 Python，不需要影像函式庫）。

琥珀色圓角方塊加深色播放三角形，與播放程式的圖示一致。系統匣與 exe 圖示共用（exe 的圖示資源
由 go-winres 從這個檔案產生，見 CLAUDE.md）。ICO 內每個尺寸各存一張 RGBA PNG。
"""
import os
import struct
import zlib

ACCENT = (0xF2, 0xA9, 0x3B)
ON_ACCENT = (0x1A, 0x12, 0x06)
SIZES = [16, 20, 24, 32, 40, 48, 64, 256]
SS = 4  # 每像素 4×4 取樣做反鋸齒
OUT = os.path.join(os.path.dirname(__file__), "..", "server", "cmd", "xcode", "icon.ico")


def png(width, height, rows):
    raw = b"".join(b"\x00" + r for r in rows)

    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)

    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b""))


def in_rounded_square(x, y, size):
    # 小尺寸留 0.5px 邊、圓角半徑約 22%
    m = size * 0.03
    r = size * 0.22
    lo, hi = m, size - m
    if not (lo <= x <= hi and lo <= y <= hi):
        return False
    cx = min(max(x, lo + r), hi - r)
    cy = min(max(y, lo + r), hi - r)
    return (x - cx) ** 2 + (y - cy) ** 2 <= r * r


def in_triangle(x, y, size):
    # 播放三角形，視覺置中略往右
    ax, ay = size * 0.37, size * 0.27
    bx, by = size * 0.37, size * 0.73
    cx, cy = size * 0.75, size * 0.50

    def side(px, py, qx, qy):
        return (x - qx) * (py - qy) - (px - qx) * (y - qy)

    d1, d2, d3 = side(ax, ay, bx, by), side(bx, by, cx, cy), side(cx, cy, ax, ay)
    neg = d1 < 0 or d2 < 0 or d3 < 0
    pos = d1 > 0 or d2 > 0 or d3 > 0
    return not (neg and pos)


def render(size):
    rows = []
    n = SS * SS
    for py in range(size):
        row = bytearray()
        for px in range(size):
            bg = tri = 0
            for sy in range(SS):
                for sx in range(SS):
                    x, y = px + (sx + 0.5) / SS, py + (sy + 0.5) / SS
                    if in_rounded_square(x, y, size):
                        bg += 1
                        if in_triangle(x, y, size):
                            tri += 1
            if bg == 0:
                row += b"\x00\x00\x00\x00"
                continue
            t = tri / bg  # 底色內三角形所佔比例
            rgb = bytes(round(a * (1 - t) + b * t) for a, b in zip(ACCENT, ON_ACCENT))
            row += rgb + bytes([round(255 * bg / n)])
        rows.append(bytes(row))
    return png(size, size, rows)


def main():
    images = [render(s) for s in SIZES]
    header = struct.pack("<HHH", 0, 1, len(images))
    offset = 6 + 16 * len(images)
    entries = b""
    for s, data in zip(SIZES, images):
        dim = 0 if s >= 256 else s  # 256 在 ICO 目錄裡寫成 0
        entries += struct.pack("<BBBBHHII", dim, dim, 0, 0, 1, 32, len(data), offset)
        offset += len(data)
    with open(OUT, "wb") as f:
        f.write(header + entries + b"".join(images))
    print("wrote", os.path.normpath(OUT))


if __name__ == "__main__":
    main()
