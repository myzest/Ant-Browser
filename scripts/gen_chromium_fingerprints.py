#!/usr/bin/env python3
"""
Chromium 指纹池构建期生成脚本。

用途：按 OS 候选矩阵生成 fingerprint-chromium 可消费的 CLI 指纹画像，
固化为 backend/internal/browser/assets/chromium_fingerprints.json。

生成是确定性的：同一候选库、同一 seed、同一数量会得到同一输出。
运行时不依赖 Python，也不会联网生成。
"""
from __future__ import annotations

import argparse
import hashlib
import json
import random
from dataclasses import dataclass
from pathlib import Path
from typing import Any


OS_LIST = ("windows", "mac", "linux")
DEFAULT_COUNTS = {"windows": 128, "mac": 64, "linux": 64}
DEFAULT_SEED = 20260623
DEFAULT_GENERATED_AT = "2026-06-23"

REGIONS = {
    "CN": {"lang": "zh-CN", "timezone": "Asia/Shanghai"},
    "US-EAST": {"lang": "en-US", "timezone": "America/New_York"},
    "US-CENTRAL": {"lang": "en-US", "timezone": "America/Chicago"},
    "US-WEST": {"lang": "en-US", "timezone": "America/Los_Angeles"},
    "JP": {"lang": "ja-JP", "timezone": "Asia/Tokyo"},
    "UK": {"lang": "en-GB", "timezone": "Europe/London"},
    "DE": {"lang": "de-DE", "timezone": "Europe/Berlin"},
}


@dataclass(frozen=True)
class OSProfile:
    brands: tuple[tuple[str, int], ...]
    regions: tuple[tuple[str, int], ...]
    window_sizes: tuple[tuple[str, int], ...]
    hardware: tuple[tuple[int, int, int], ...]  # cpu, memory, weight
    gpus: tuple[tuple[str, str, int], ...]  # vendor, renderer, weight
    fonts_by_region: dict[str, tuple[tuple[str, ...], ...]]
    color_depth: int


OS_PROFILES: dict[str, OSProfile] = {
    "windows": OSProfile(
        brands=(("Chrome", 80), ("Edge", 20)),
        regions=(("CN", 34), ("US-EAST", 18), ("US-CENTRAL", 12), ("US-WEST", 18), ("JP", 12), ("UK", 4), ("DE", 2)),
        window_sizes=(("1366,768", 18), ("1440,900", 16), ("1600,900", 18), ("1920,1080", 38), ("2560,1440", 10)),
        hardware=((4, 4, 14), (6, 8, 18), (8, 8, 34), (12, 16, 18), (16, 16, 16)),
        gpus=(
            ("Intel", "Intel(R) HD Graphics 520", 12),
            ("Intel", "Intel(R) UHD Graphics 620", 16),
            ("Intel", "Intel(R) UHD Graphics 630", 22),
            ("Intel", "Intel(R) Iris(R) Xe Graphics", 18),
            ("NVIDIA", "NVIDIA GeForce GTX 1660", 8),
            ("NVIDIA", "NVIDIA GeForce RTX 3060", 8),
            ("NVIDIA", "NVIDIA GeForce RTX 3080", 4),
            ("AMD", "AMD Radeon RX 580", 4),
            ("AMD", "AMD Radeon RX 6600", 8),
        ),
        fonts_by_region={
            "CN": (
                ("Arial", "Microsoft YaHei", "SimSun", "SimHei", "Segoe UI", "Times New Roman"),
                ("Arial", "Microsoft YaHei UI", "Microsoft YaHei", "SimSun", "Tahoma", "Times New Roman"),
            ),
            "JP": (
                ("Arial", "Meiryo", "Yu Gothic", "Segoe UI", "Times New Roman"),
                ("Arial", "Meiryo UI", "Yu Gothic UI", "Segoe UI", "Verdana", "Times New Roman"),
            ),
            "*": (
                ("Arial", "Calibri", "Segoe UI", "Tahoma", "Times New Roman"),
                ("Arial", "Calibri", "Consolas", "Segoe UI", "Verdana", "Times New Roman"),
                ("Arial", "Segoe UI", "Tahoma", "Trebuchet MS", "Verdana", "Times New Roman"),
            ),
        },
        color_depth=24,
    ),
    "mac": OSProfile(
        brands=(("Chrome", 100),),
        regions=(("US-WEST", 28), ("US-EAST", 18), ("CN", 18), ("JP", 12), ("UK", 14), ("DE", 10)),
        window_sizes=(("1280,800", 14), ("1440,900", 24), ("1512,982", 16), ("1680,1050", 16), ("1920,1080", 14), ("2560,1440", 16)),
        hardware=((8, 8, 28), (8, 16, 24), (10, 16, 24), (12, 16, 24)),
        gpus=(
            ("Apple", "Apple M1", 26),
            ("Apple", "Apple M2", 26),
            ("Apple", "Apple M1 Pro", 18),
            ("Apple", "Apple M2 Pro", 18),
            ("Intel", "Intel Iris OpenGL Engine", 12),
        ),
        fonts_by_region={
            "CN": (
                ("Arial", "Helvetica", "Hiragino Sans GB", "Menlo", "PingFang SC", "Times New Roman"),
                ("Arial", "Helvetica", "PingFang SC", "SF Pro Text", "Times New Roman"),
            ),
            "JP": (
                ("Arial", "Helvetica", "Hiragino Sans", "Menlo", "Times New Roman"),
                ("Arial", "Helvetica", "Hiragino Kaku Gothic ProN", "SF Pro Text", "Times New Roman"),
            ),
            "*": (
                ("Arial", "Helvetica", "Menlo", "SF Pro Text", "Times New Roman"),
                ("Arial", "Helvetica Neue", "Menlo", "SF Pro Display", "Times New Roman"),
            ),
        },
        color_depth=30,
    ),
    "linux": OSProfile(
        brands=(("Chrome", 100),),
        regions=(("US-EAST", 22), ("US-WEST", 18), ("UK", 18), ("DE", 16), ("CN", 14), ("JP", 12)),
        window_sizes=(("1366,768", 16), ("1440,900", 18), ("1600,900", 18), ("1920,1080", 38), ("2560,1440", 10)),
        hardware=((4, 4, 16), (6, 8, 18), (8, 8, 34), (8, 16, 16), (12, 16, 16)),
        gpus=(
            ("Intel", "Mesa Intel(R) HD Graphics 520", 14),
            ("Intel", "Mesa Intel(R) UHD Graphics 620", 20),
            ("Intel", "Mesa Intel(R) UHD Graphics 630", 18),
            ("Intel", "Mesa Intel(R) Iris(R) Xe Graphics", 18),
            ("AMD", "AMD Radeon Graphics (RADV RENOIR)", 12),
            ("AMD", "AMD Radeon RX 6600 (RADV NAVI23)", 8),
            ("NVIDIA", "NVIDIA GeForce GTX 1660/PCIe/SSE2", 6),
            ("NVIDIA", "NVIDIA GeForce RTX 3060/PCIe/SSE2", 4),
        ),
        fonts_by_region={
            "CN": (
                ("DejaVu Sans", "Noto Sans", "Noto Sans CJK SC", "WenQuanYi Micro Hei", "Times New Roman"),
                ("DejaVu Sans", "Liberation Sans", "Noto Sans CJK SC", "Ubuntu", "Times New Roman"),
            ),
            "JP": (
                ("DejaVu Sans", "Noto Sans", "Noto Sans CJK JP", "Ubuntu", "Times New Roman"),
            ),
            "*": (
                ("DejaVu Sans", "Liberation Sans", "Noto Sans", "Ubuntu", "Times New Roman"),
                ("DejaVu Sans", "Liberation Mono", "Liberation Sans", "Noto Sans", "Times New Roman"),
            ),
        },
        color_depth=24,
    ),
}


def weighted_choice(rng: random.Random, items: tuple[tuple[Any, int], ...]) -> Any:
    total = sum(weight for _, weight in items)
    pick = rng.randint(1, total)
    running = 0
    for value, weight in items:
        running += weight
        if pick <= running:
            return value
    return items[-1][0]


def weighted_tuple_choice(rng: random.Random, items: tuple[tuple[Any, ...], ...]) -> tuple[Any, ...]:
    weighted = tuple((item[:-1], int(item[-1])) for item in items)
    return tuple(weighted_choice(rng, weighted))


def fonts_for_region(profile: OSProfile, region: str, rng: random.Random) -> list[str]:
    choices = profile.fonts_by_region.get(region) or profile.fonts_by_region["*"]
    fonts = list(rng.choice(choices))
    return list(dict.fromkeys(fonts))


def fingerprint_hash(os_name: str, profile: dict[str, Any]) -> str:
    payload = json.dumps({"os": os_name, "profile": profile}, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def generate_one(os_name: str, seed: int, rng: random.Random) -> dict[str, Any]:
    os_profile = OS_PROFILES[os_name]
    brand = weighted_choice(rng, os_profile.brands)
    region = weighted_choice(rng, os_profile.regions)
    region_data = REGIONS[region]
    window_size = weighted_choice(rng, os_profile.window_sizes)
    cpu, memory = weighted_tuple_choice(rng, os_profile.hardware)
    webgl_vendor, webgl_renderer = weighted_tuple_choice(rng, os_profile.gpus)
    profile = {
        "brand": brand,
        "platform": os_name,
        "lang": region_data["lang"],
        "timezone": region_data["timezone"],
        "windowSize": window_size,
        "colorDepth": os_profile.color_depth,
        "hardwareConcurrency": cpu,
        "deviceMemory": memory,
        "webglVendor": webgl_vendor,
        "webglRenderer": webgl_renderer,
        "fonts": fonts_for_region(os_profile, region, rng),
        "doNotTrack": False,
        "touchPoints": 0,
        "webrtcPolicy": "disable_non_proxied_udp",
        "canvasNoise": True,
        "audioNoise": True,
    }
    return {
        "os": os_name,
        "seed": seed,
        "hash": fingerprint_hash(os_name, profile),
        "profile": profile,
    }


def generate_pool(counts: dict[str, int], seed: int) -> dict[str, list[dict[str, Any]]]:
    pools: dict[str, list[dict[str, Any]]] = {}
    for os_name in OS_LIST:
        rng = random.Random(f"{seed}:{os_name}")
        target = counts[os_name]
        items: list[dict[str, Any]] = []
        seen: set[str] = set()
        candidate_seed = 0
        attempts = 0
        while len(items) < target:
            attempts += 1
            if attempts > target * 100:
                raise RuntimeError(f"{os_name} 指纹候选空间不足，无法生成 {target} 条唯一记录")
            record = generate_one(os_name, candidate_seed, rng)
            candidate_seed += 1
            if record["hash"] in seen:
                continue
            record["seed"] = len(items)
            seen.add(record["hash"])
            items.append(record)
        pools[os_name] = items
    return pools


def validate_payload(payload: dict[str, Any], expected_counts: dict[str, int] | None = None) -> list[str]:
    errors: list[str] = []
    if payload.get("version") != 2:
        errors.append("version 必须为 2")
    pools = payload.get("pools")
    if not isinstance(pools, dict):
        return ["pools 必须是对象"]
    global_hashes: set[str] = set()
    for os_name in OS_LIST:
        items = pools.get(os_name)
        if not isinstance(items, list):
            errors.append(f"缺少 {os_name} 池")
            continue
        if expected_counts and len(items) != expected_counts[os_name]:
            errors.append(f"{os_name} 数量不符: got={len(items)} want={expected_counts[os_name]}")
        for index, item in enumerate(items):
            errors.extend(validate_record(os_name, index, item, global_hashes))
    return errors


def validate_record(os_name: str, index: int, item: dict[str, Any], global_hashes: set[str]) -> list[str]:
    errors: list[str] = []
    prefix = f"{os_name}[{index}]"
    profile = item.get("profile")
    if item.get("os") != os_name:
        errors.append(f"{prefix}: os 不符")
    if item.get("seed") != index:
        errors.append(f"{prefix}: seed 必须连续")
    item_hash = item.get("hash")
    if not isinstance(item_hash, str) or not item_hash:
        errors.append(f"{prefix}: hash 为空")
    elif item_hash in global_hashes:
        errors.append(f"{prefix}: hash 重复 {item_hash}")
    else:
        global_hashes.add(item_hash)
    if not isinstance(profile, dict):
        errors.append(f"{prefix}: profile 必须是对象")
        return errors
    expected_hash = fingerprint_hash(os_name, profile)
    if item_hash != expected_hash:
        errors.append(f"{prefix}: hash 与内容不匹配")

    required_strings = ("brand", "platform", "lang", "timezone", "windowSize", "webglVendor", "webglRenderer", "webrtcPolicy")
    for key in required_strings:
        if not isinstance(profile.get(key), str) or not profile[key].strip():
            errors.append(f"{prefix}: {key} 为空")
    if profile.get("platform") != os_name:
        errors.append(f"{prefix}: platform 必须等于 {os_name}")
    if profile.get("touchPoints") != 0:
        errors.append(f"{prefix}: 桌面画像 touchPoints 必须为 0")
    if profile.get("colorDepth") != OS_PROFILES[os_name].color_depth:
        errors.append(f"{prefix}: colorDepth 不符合 OS 规则")
    if profile.get("canvasNoise") is not True or profile.get("audioNoise") is not True:
        errors.append(f"{prefix}: canvas/audio noise 必须开启")
    if profile.get("doNotTrack") is not False:
        errors.append(f"{prefix}: doNotTrack 第一版固定为 false")

    fonts = profile.get("fonts")
    if not isinstance(fonts, list) or not fonts or any(not isinstance(font, str) or not font.strip() for font in fonts):
        errors.append(f"{prefix}: fonts 必须是非空字符串数组")
    elif len(fonts) != len(set(fonts)):
        errors.append(f"{prefix}: fonts 存在重复")

    if profile.get("lang") not in {region["lang"] for region in REGIONS.values()}:
        errors.append(f"{prefix}: lang 不在地区矩阵")
    if profile.get("timezone") not in {region["timezone"] for region in REGIONS.values()}:
        errors.append(f"{prefix}: timezone 不在地区矩阵")

    serialized = json.dumps(item, ensure_ascii=False)
    forbidden = ("navigator.", "headers.", "webGl:parameters", "webGl2:parameters", "Gecko", "Firefox")
    for marker in forbidden:
        if marker in serialized:
            errors.append(f"{prefix}: 包含禁止字段/标记 {marker}")
    return errors


def build_payload(counts: dict[str, int], seed: int, generated_at: str) -> dict[str, Any]:
    pools = generate_pool(counts, seed)
    return {
        "version": 2,
        "meta": {
            "schema": "chromium-fingerprint-pool/v2",
            "countPerOS": counts,
            "osList": list(OS_LIST),
            "generator": {
                "name": "ant-browser-chromium-fingerprint-generator",
                "revision": "v1",
                "seed": seed,
            },
            "generatedAt": generated_at,
            "note": "构建期确定性生成；运行时不依赖 Python，也不联网生成指纹。",
        },
        "pools": pools,
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="生成 Chromium 指纹池")
    parser.add_argument("--windows", type=int, default=DEFAULT_COUNTS["windows"], help="Windows 指纹数量")
    parser.add_argument("--mac", type=int, default=DEFAULT_COUNTS["mac"], help="macOS 指纹数量")
    parser.add_argument("--linux", type=int, default=DEFAULT_COUNTS["linux"], help="Linux 指纹数量")
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED, help="生成器固定 seed")
    parser.add_argument("--generated-at", default=DEFAULT_GENERATED_AT, help="写入 meta.generatedAt 的固定日期")
    parser.add_argument(
        "--out",
        default="backend/internal/browser/assets/chromium_fingerprints.json",
        help="输出 JSON 路径",
    )
    parser.add_argument("--pretty", action="store_true", help="输出格式化 JSON")
    parser.add_argument("--validate-only", action="store_true", help="只校验 --out 指向的现有文件")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    counts = {"windows": args.windows, "mac": args.mac, "linux": args.linux}
    out_path = Path(args.out)

    if args.validate_only:
        payload = json.loads(out_path.read_text(encoding="utf-8"))
    else:
        payload = build_payload(counts, args.seed, args.generated_at)

    errors = validate_payload(payload, counts if not args.validate_only else None)
    if errors:
        for err in errors:
            print(f"ERROR {err}")
        return 1

    if not args.validate_only:
        out_path.parent.mkdir(parents=True, exist_ok=True)
        with out_path.open("w", encoding="utf-8") as fh:
            if args.pretty:
                json.dump(payload, fh, ensure_ascii=False, indent=2)
                fh.write("\n")
            else:
                json.dump(payload, fh, ensure_ascii=False, separators=(",", ":"))
        print(f"生成完成: {out_path} ({sum(counts.values())} 条)")
    else:
        total = sum(len(payload["pools"][os_name]) for os_name in OS_LIST)
        print(f"校验通过: {out_path} ({total} 条)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
