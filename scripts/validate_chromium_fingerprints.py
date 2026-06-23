#!/usr/bin/env python3
"""校验 Chromium 指纹池 JSON。"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from gen_chromium_fingerprints import OS_LIST, validate_payload


def main() -> int:
    parser = argparse.ArgumentParser(description="校验 Chromium 指纹池")
    parser.add_argument(
        "path",
        nargs="?",
        default="backend/internal/browser/assets/chromium_fingerprints.json",
        help="待校验 JSON 路径",
    )
    args = parser.parse_args()
    path = Path(args.path)
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        print(f"ERROR 读取或解析失败: {exc}")
        return 1

    errors = validate_payload(payload)
    if errors:
        for err in errors:
            print(f"ERROR {err}")
        return 1
    total = sum(len(payload["pools"][os_name]) for os_name in OS_LIST)
    print(f"校验通过: {path} ({total} 条)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
