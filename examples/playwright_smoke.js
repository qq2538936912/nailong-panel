const path = require("path");
const { chromium } = require("playwright");

const screenshot = path.resolve(__dirname, "playwright_smoke.png");
const testUrl = "https://www.baidu.com";

async function main() {
  console.log("启动 Chromium（无头）…");
  const browser = await chromium.launch({
    headless: true,
    args: ["--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu"],
  });
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  await page.goto(testUrl, { waitUntil: "domcontentloaded", timeout: 30000 });
  const title = await page.title();
  const userAgent = await page.evaluate(() => navigator.userAgent);
  await page.screenshot({ path: screenshot });
  await browser.close();
  console.log("页面标题：", title);
  console.log("User-Agent：", userAgent);
  console.log("截图：", screenshot);
  console.log("Playwright Chromium 可用。");
}

main().catch((error) => {
  console.error("Chromium 启动或打开页面失败。");
  console.error("原因：", error && error.message ? error.message : error);
  console.error(
    "缺 libglib 这类库：Debian 精简镜像里跑 playwright install-deps chromium；若还是 Alpine，先换 Debian。",
  );
  process.exit(1);
});
