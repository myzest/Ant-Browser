#!/usr/bin/env python3
"""
Camoufox 指纹池构建期下载脚本。

用途：在开发机/CI 上一次性预生成若干 Firefox(Camoufox) 指纹，固化成 JSON 指纹池，
随应用发布包分发；运行时不再依赖 Python 也不再联网下载指纹数据。

数据来源：
  browserforge 1.2.4 (Apache-2.0, daijro)
  apify_fingerprint_datapoints 0.13.0 (Apache-2.0, Apify)
  camoufox 0.4.11 (libre)

依赖：
  pip install "browserforge>=1.2.4" "apify_fingerprint_datapoints>=0.13.0" "camoufox>=0.4.11"

用法：
  python3 scripts/gen_camoufox_fingerprints.py --count 64 --out backend/internal/browser/assets/camoufox_fingerprints.json

指纹生成采样的两个随机源均被固定 seed 控制：
  random.seed(seed)        -> browserforge 贝叶斯网络 + camoufox utils randint/randrange
  numpy.random.seed(seed)  -> camoufox webgl sample_webgl

因此同一组 (browserforge/apify/camoufox) 版本 + 同一 seed 能确定性复现同一条指纹；
只把生成结果写入 JSON 再 commit，无需用户运行时安装任何 Python 依赖。
"""
from __future__ import annotations

import argparse
import json
import os
import random
import sys
import hashlib
from pathlib import Path

try:
    import numpy as np  # noqa: F401
except ImportError:
    np = None

try:
    from camoufox.utils import launch_options
except ImportError:
    sys.stderr.write(
        "ERROR: 缺少 camoufox，请先安装：\n"
        '  pip install "browserforge>=1.2.4" "apify_fingerprint_datapoints>=0.13.0" "camoufox>=0.4.11"\n'
    )
    raise

# Camoufox 内部把 os 归一成三类目标系统；指纹池按这三类分别预生成。
CAMOUFOX_OS_LIST = ("windows", "macos", "linux")

# properties.json 里被 launch_options 预填的运行期随机量；每条指纹独立保存,
# 运行时直接从池里取用，不再重新随机。
RUNTIME_FIELDS_TO_CAPTURE = (
    "fonts",
    "fonts:spacing_seed",
    "canvas:aaOffset",
    "canvas:aaCapOffset",
    "window.history.length",
    "webGl:vendor",
    "webGl:renderer",
    "webGl:parameters",
    "webGl:contextAttributes",
    "webGl:shaderPrecisionFormats",
    "webGl:supportedExtensions",
    "webGl2:contextAttributes",
    "webGl2:parameters",
    "webGl2:shaderPrecisionFormats",
    "webGl2:supportedExtensions",
)


def pipeline_version_meta() -> dict:
    meta = {"generator": {}, "fixed": True}
    for dist in ("browserforge", "apify_fingerprint_datapoints", "camoufox"):
        try:
            import importlib.metadata as md
            meta["generator"][dist] = md.version(dist)
        except Exception:  # pragma: no cover
            meta["generator"][dist] = "unknown"
    return meta


def _set_seed(seed: int) -> None:
    random.seed(seed)
    if np is not None:
        np.random.seed(seed)


def generate_one(os_name: str, seed: int, ff_version: str | None = None) -> dict:
    _set_seed(seed)
    # i_know_what_im_doing=True 仅用于关掉 camoufox 的 leak warning（禁止手动覆盖
    # UA / locale 等），下载脚本并未手动覆盖任何 config，只是取默认生成的指纹。
    opts = launch_options(
        headless=False,
        os=os_name,
        i_know_what_im_doing=True,
    )
    cfg_raw = opts["env"].get("CAMOU_CONFIG_1", "")
    if not cfg_raw:
        raise RuntimeError(f"CAMOU_CONFIG_1 为空 (os={os_name}, seed={seed})")
    cfg = json.loads(cfg_raw)

    fingerprint = {}
    for key in sorted(cfg.keys()):
        if (
            key.startswith("navigator.")
            or key.startswith("screen.")
            or key.startswith("window.")
            or key.startswith("headers.")
        ):
            fingerprint[key] = cfg[key]
    runtime = {}
    for key in RUNTIME_FIELDS_TO_CAPTURE:
        if key in cfg:
            runtime[key] = cfg[key]

    record = {
        "os": os_name,
        "seed": seed,
        "fingerprint": fingerprint,
        "runtime": runtime,
    }
    return record


def fingerprint_hash(record: dict) -> str:
    payload = json.dumps(
        {"os": record["os"], "fingerprint": record["fingerprint"], "runtime": record["runtime"]},
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def generate_pool(count_per_os: int, ff_version: str | None) -> dict:
    pools: dict[str, list[dict]] = {}
    for os_name in CAMOUFOX_OS_LIST:
        items: list[dict] = []
        seen: set[str] = set()
        # seed 从 0 开始递增，尽量用尽；重复指纹会被剔除以保证池的多样性。
        seed = 0
        while len(items) < count_per_os:
            try:
                record = generate_one(os_name, seed, ff_version)
            except Exception as exc:
                sys.stderr.write(f"WARN 生成失败 os={os_name} seed={seed}: {exc}\n")
                seed += 1
                continue
            fp_hash = fingerprint_hash(record)
            if fp_hash in seen:
                seed += 1
                continue
            seen.add(fp_hash)
            record["hash"] = fp_hash
            items.append(record)
            seed += 1
        pools[os_name] = items
    return pools


def firefox_user_prefs_defaults() -> dict:
    # 取一次默认 prefs 作为池元数据（固定值，非指纹一部分）。
    _set_seed(0)
    opts = launch_options(headless=False, os="windows", i_know_what_im_doing=True)
    return dict(opts.get("firefox_user_prefs") or {})


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="预生成 Camoufox 指纹池")
    parser.add_argument("--count", type=int, default=64, help="每个 OS 生成多少条指纹")
    parser.add_argument(
        "--out",
        default="backend/internal/browser/assets/camoufox_fingerprints.json",
        help="指纹池输出 JSON 路径",
    )
    parser.add_argument(
        "--ff-version",
        default=None,
        help="可选 Firefox 版本号（默认随 camoufox 内核版本）",
    )
    parser.add_argument(
        "--min-width", type=int, default=None, help="(未用)预留字段",
    )
    args = parser.parse_args(argv)

    pools = generate_pool(args.count, args.ff_version)
    summary = {os_name: len(items) for os_name, items in pools.items()}

    payload = {
        "version": 1,
        "meta": {
            **pipeline_version_meta(),
            "countPerOS": args.count,
            "osList": list(CAMOUFOX_OS_LIST),
            "note": "构建期一次性预生成；运行时不再依赖 Python，也不联网下载指纹数据。",
            "source": {
                "browserforge": "https://github.com/daijro/browserforge",
                "apify_fingerprint_datapoints": "https://docs.apify.com/academy/anti-scraping/techniques/fingerprinting",
                "camoufox": "https://github.com/daijro/camoufox",
            },
        },
        "firefoxUserPrefs": firefox_user_prefs_defaults(),
        "pools": pools,
    }

    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    with out_path.open("w", encoding="utf-8") as fh:
        json.dump(payload, fh, ensure_ascii=False, separators=(",", ":"))
    total = sum(summary.values())
    sys.stdout.write(
        f"生成完成: {out_path} (总 {total} 条)\n"
        + "\n".join(f"  {k}: {v} 条" for k, v in summary.items())
        + "\n"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
