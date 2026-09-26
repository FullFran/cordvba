#!/usr/bin/env node
/**
 * Checks that the page never inherits the visitor's browser/OS colour
 * scheme (issue #119): a maintainer report on Firefox with a transparent
 * dark theme found the page showing the desktop wallpaper through an
 * unstyled `<body>`, with markers rendering as white-on-white.
 *
 * Renders the page twice, once with `colorScheme: 'dark'` and once with
 * `colorScheme: 'light'` emulated, and asserts the computed
 * `background-color` of `html`, `body` and `#root` is opaque and
 * IDENTICAL in both — i.e. this app is always dark, regardless of the
 * browser's own theme.
 *
 * This intentionally does not add Playwright as a project dependency
 * (apps/web's unit suite is vitest+jsdom, and a browser + its binary
 * would be a large, one-purpose addition to this package). Run it with
 * an external Playwright install instead — `require()` (unlike ESM
 * `import`) honours NODE_PATH, which is what lets this resolve an
 * install that lives outside apps/web/node_modules:
 *
 *   NODE_PATH=<path-to-a-playwright-install>/node_modules \
 *     node scripts/check-color-scheme-isolation.mjs http://localhost:4173/
 *
 * Exit 0 if both schemes render an identical, opaque background; 1
 * otherwise (including when Playwright itself is unavailable).
 */
import { createRequire } from "node:module";

const { chromium } = createRequire(import.meta.url)("playwright");

const url = process.argv[2];
if (!url) {
  console.log("usage: node check-color-scheme-isolation.mjs <url>");
  process.exit(1);
}

function isOpaque(rgbString) {
  // getComputedStyle returns "rgb(r, g, b)" (opaque) or "rgba(r, g, b, a)".
  // Only the rgba() form can be non-opaque; treat missing alpha as opaque.
  const match = /rgba\(\s*[\d.]+\s*,\s*[\d.]+\s*,\s*[\d.]+\s*,\s*([\d.]+)\s*\)/.exec(rgbString);
  return match ? Number(match[1]) === 1 : rgbString.startsWith("rgb(");
}

const browser = await chromium.launch();
const backgrounds = {};

for (const colorScheme of ["dark", "light"]) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme });
  await page.goto(url, { waitUntil: "networkidle", timeout: 60000 });
  await page.waitForTimeout(1000);
  backgrounds[colorScheme] = await page.evaluate(() => ({
    html: getComputedStyle(document.documentElement).backgroundColor,
    body: getComputedStyle(document.body).backgroundColor,
    root: document.getElementById("root")
      ? getComputedStyle(document.getElementById("root")).backgroundColor
      : null,
  }));
  await page.screenshot({ path: `color-scheme-${colorScheme}.png` });
  await page.close();
}
await browser.close();

const { dark, light } = backgrounds;
const allOpaque = [dark.html, dark.body, dark.root, light.html, light.body, light.root].every(isOpaque);
const identical = dark.html === light.html && dark.body === light.body && dark.root === light.root;

console.log(JSON.stringify({ dark, light, allOpaque, identical }, null, 2));

if (!allOpaque || !identical) {
  console.error(
    "FAIL: the page's background must be opaque and identical under both colorScheme: 'dark' and 'light'.",
  );
  process.exit(1);
}
console.log("OK: background is opaque and identical regardless of the browser's colour scheme.");
