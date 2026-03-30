// WebGPU WASM browser test using Playwright.
//
// Usage:
//   node scripts/webgpu-wasm-test.mjs
//
// Prerequisites:
//   npx playwright install chromium
//
// This script:
//   1. Builds the WASM test binary
//   2. Starts a local HTTP server
//   3. Launches headless Chromium with WebGPU enabled
//   4. Loads the test page and waits for results
//   5. Reports pass/fail

import { execSync } from "child_process";
import { createServer } from "http";
import { readFileSync, existsSync, copyFileSync } from "fs";
import { resolve, join, extname } from "path";

const WASM_DIR = resolve("internal/backend/webgpu/testdata/wasm");
const TIMEOUT_MS = 30000;

// MIME types for the static server.
const MIME = {
  ".html": "text/html",
  ".js":   "application/javascript",
  ".wasm": "application/wasm",
};

function log(msg) {
  process.stderr.write(`[webgpu-wasm-test] ${msg}\n`);
}

// Step 1: Build WASM binary.
log("Building WASM binary...");
try {
  execSync(
    `GOOS=js GOARCH=wasm go build -o ${join(WASM_DIR, "main.wasm")} ./internal/backend/webgpu/testdata/wasm/`,
    { stdio: "inherit" }
  );
} catch (e) {
  log("FAIL: WASM build failed");
  process.exit(1);
}

// Step 2: Copy wasm_exec.js from Go toolchain.
const goRoot = execSync("go env GOROOT", { encoding: "utf-8" }).trim();
// Go 1.24+ moved wasm_exec.js from misc/wasm/ to lib/wasm/.
let wasmExecSrc = join(goRoot, "lib/wasm/wasm_exec.js");
if (!existsSync(wasmExecSrc)) {
  wasmExecSrc = join(goRoot, "misc/wasm/wasm_exec.js");
}
const wasmExecDst = join(WASM_DIR, "wasm_exec.js");
if (!existsSync(wasmExecSrc)) {
  log(`FAIL: wasm_exec.js not found in ${goRoot}/{lib,misc}/wasm/`);
  process.exit(1);
}
copyFileSync(wasmExecSrc, wasmExecDst);

// Step 3: Start local HTTP server.
const server = createServer((req, res) => {
  const filePath = join(WASM_DIR, req.url === "/" ? "index.html" : req.url);
  if (!existsSync(filePath)) {
    res.writeHead(404);
    res.end("Not found");
    return;
  }
  const ext = extname(filePath);
  res.writeHead(200, { "Content-Type": MIME[ext] || "application/octet-stream" });
  res.end(readFileSync(filePath));
});

await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const port = server.address().port;
const url = `http://127.0.0.1:${port}/`;
log(`Server listening on ${url}`);

// Step 4: Launch headless Chromium with WebGPU.
let browser, exitCode = 1;
try {
  const { chromium } = await import("playwright");

  // Use system Chromium if PLAYWRIGHT_CHROMIUM_PATH is set or Playwright's isn't available.
  const launchOpts = {
    headless: true,
    args: [
      "--enable-unsafe-webgpu",
      "--enable-features=Vulkan,WebGPU",
      "--use-angle=swiftshader",         // Software GPU for headless
      "--use-webgpu-adapter=swiftshader", // Force SwiftShader WebGPU adapter
      "--disable-gpu-sandbox",
    ],
  };
  if (process.env.PLAYWRIGHT_CHROMIUM_PATH) {
    launchOpts.executablePath = process.env.PLAYWRIGHT_CHROMIUM_PATH;
  }

  browser = await chromium.launch(launchOpts);

  const context = await browser.newContext();
  const page = await context.newPage();

  // Collect console logs.
  page.on("console", (msg) => log(`  [browser] ${msg.text()}`));
  page.on("pageerror", (err) => log(`  [browser error] ${err}`));

  log("Loading test page...");
  await page.goto(url, { waitUntil: "domcontentloaded" });

  // Wait for the title to change from default (indicates completion).
  await page.waitForFunction(
    () => document.title.startsWith("PASS") || document.title.startsWith("FAIL"),
    { timeout: TIMEOUT_MS }
  );

  const title = await page.title();
  const resultText = await page.textContent("#output");

  log(`Result: ${resultText}`);

  let result;
  try {
    result = JSON.parse(resultText);
  } catch {
    result = { ok: false, error: "failed to parse result JSON" };
  }

  if (result.ok) {
    log(`PASS — WebGPU rendered successfully in browser`);
    if (result.caps) {
      log(`  Capabilities: ${result.caps}`);
    }
    exitCode = 0;
  } else {
    log(`FAIL at stage "${result.stage}": ${result.error}`);
    exitCode = 1;
  }

  // Wait for browser to composite the WebGPU canvas content.
  await page.waitForTimeout(500);

  // Save a screenshot of the rendered output.
  const screenshotPath = resolve("testdata/visual/webgpu_wasm_browser.png");
  await page.screenshot({ path: screenshotPath, fullPage: true });
  log(`Screenshot saved to ${screenshotPath}`);
} catch (e) {
  if (e.message && e.message.includes("playwright")) {
    log(`SKIP: Playwright not installed. Run: npx playwright install chromium`);
    exitCode = 0; // Don't fail CI if Playwright isn't available
  } else {
    log(`FAIL: ${e.message || e}`);
    exitCode = 1;
  }
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
