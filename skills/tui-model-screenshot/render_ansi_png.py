#!/usr/bin/env python3
"""Render xterm 256-color ANSI text to a PNG terminal screenshot."""

from __future__ import annotations

import argparse
import re
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


ANSI_RE = re.compile(r"\x1b\[([0-9;]*)m")
DEFAULT_FG = 250
DEFAULT_BG = 234


PALETTE_16 = [
    (0, 0, 0),
    (128, 0, 0),
    (0, 128, 0),
    (128, 128, 0),
    (0, 0, 128),
    (128, 0, 128),
    (0, 128, 128),
    (192, 192, 192),
    (128, 128, 128),
    (255, 0, 0),
    (0, 255, 0),
    (255, 255, 0),
    (0, 0, 255),
    (255, 0, 255),
    (0, 255, 255),
    (255, 255, 255),
]


def color256(value: int) -> tuple[int, int, int]:
    if value < 16:
        return PALETTE_16[value]
    if value <= 231:
        value -= 16
        r, g, b = value // 36, (value % 36) // 6, value % 6

        def channel(component: int) -> int:
            return 0 if component == 0 else 55 + component * 40

        return channel(r), channel(g), channel(b)
    gray = 8 + (value - 232) * 10
    return gray, gray, gray


def parse_line(line: str) -> list[tuple[str, tuple[int, int, int], tuple[int, int, int]]]:
    cells = []
    fg = color256(DEFAULT_FG)
    bg = color256(DEFAULT_BG)
    index = 0
    for match in ANSI_RE.finditer(line):
        for char in line[index : match.start()]:
            cells.append((char, fg, bg))
        codes = [0] if match.group(1) == "" else [int(code or 0) for code in match.group(1).split(";")]
        code_index = 0
        while code_index < len(codes):
            code = codes[code_index]
            if code == 0:
                fg = color256(DEFAULT_FG)
                bg = color256(DEFAULT_BG)
            elif code == 39:
                fg = color256(DEFAULT_FG)
            elif code == 49:
                bg = color256(DEFAULT_BG)
            elif 30 <= code <= 37:
                fg = color256(code - 30)
            elif 40 <= code <= 47:
                bg = color256(code - 40)
            elif 90 <= code <= 97:
                fg = color256(code - 90 + 8)
            elif 100 <= code <= 107:
                bg = color256(code - 100 + 8)
            elif code in (38, 48) and code_index + 2 < len(codes) and codes[code_index + 1] == 5:
                if code == 38:
                    fg = color256(codes[code_index + 2])
                else:
                    bg = color256(codes[code_index + 2])
                code_index += 2
            code_index += 1
        index = match.end()
    for char in line[index:]:
        cells.append((char, fg, bg))
    return cells


def load_font(size: int) -> ImageFont.FreeTypeFont | ImageFont.ImageFont:
    for path in (
        "/System/Library/Fonts/Menlo.ttc",
        "/System/Library/Fonts/Monaco.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
    ):
        try:
            return ImageFont.truetype(path, size)
        except OSError:
            pass
    return ImageFont.load_default()


def render(input_path: Path, output_path: Path, font_size: int, padding: int) -> None:
    raw = input_path.read_text(errors="ignore")
    lines = [parse_line(line.rstrip("\n")) for line in raw.splitlines()]
    columns = max((len(line) for line in lines), default=1)
    font = load_font(font_size)

    probe = Image.new("RGB", (1, 1))
    draw = ImageDraw.Draw(probe)
    bbox = draw.textbbox((0, 0), "M", font=font)
    cell_width = bbox[2] - bbox[0]
    cell_height = bbox[3] - bbox[1] + 8

    image = Image.new(
        "RGB",
        (columns * cell_width + padding * 2, len(lines) * cell_height + padding * 2),
        color256(DEFAULT_BG),
    )
    draw = ImageDraw.Draw(image)
    for y, line in enumerate(lines):
        for x in range(columns):
            char, fg, bg = (" ", color256(DEFAULT_FG), color256(DEFAULT_BG))
            if x < len(line):
                char, fg, bg = line[x]
            left = padding + x * cell_width
            top = padding + y * cell_height
            draw.rectangle([left, top, left + cell_width, top + cell_height], fill=bg)
            if char != " ":
                draw.text((left, top + 2), char, font=font, fill=fg)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    image.save(output_path)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--font-size", type=int, default=22)
    parser.add_argument("--padding", type=int, default=18)
    args = parser.parse_args()
    render(args.input, args.output, args.font_size, args.padding)


if __name__ == "__main__":
    main()
