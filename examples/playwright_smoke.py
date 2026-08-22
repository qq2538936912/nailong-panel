#!/usr/bin/env python3
"""Playwright Chromium 冒烟：能启动、能打开页面、能截图即视为安装成功。"""

from __future__ import annotations

import sys
from pathlib import Path

OUT_DIR = Path(__file__).resolve().parent
SCREENSHOT = OUT_DIR / "playwright_smoke.png"
TEST_URL = "https://www.baidu.com"


def main() -> int:
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        print("未找到 Python 包 playwright。", file=sys.stderr)
        print("你现在装的是 Node 版，请改跑 playwright_smoke.js，不要用这条 Python 任务。", file=sys.stderr)
        print("若一定要用 Python，先在依赖管理里把类型选成 Python 再装 playwright。", file=sys.stderr)
        return 2

    print("启动 Chromium（无头）…")
    try:
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(
                headless=True,
                args=["--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu"],
            )
            page = browser.new_page(viewport={"width": 1280, "height": 720})
            page.goto(TEST_URL, wait_until="domcontentloaded", timeout=30_000)
            title = page.title()
            user_agent = page.evaluate("() => navigator.userAgent")
            page.screenshot(path=str(SCREENSHOT), full_page=False)
            browser.close()
    except Exception as exc:
        print("Chromium 启动或打开页面失败。", file=sys.stderr)
        print(f"原因：{exc}", file=sys.stderr)
        print(
            "缺 libglib 这类库：Debian 精简镜像里跑 playwright install-deps chromium；若还是 Alpine，先换 Debian。",
            file=sys.stderr,
        )
        return 1

    print(f"页面标题：{title}")
    print(f"User-Agent：{user_agent}")
    print(f"截图：{SCREENSHOT}")
    print("Playwright Chromium 可用。")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
